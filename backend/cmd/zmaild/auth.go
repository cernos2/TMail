package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"
)

const (
	sessionCookie = "zm_sess"
	sessionTTL    = 30 * 24 * time.Hour
)

// Account is a mailbox the signed-in user works with.
type Account struct {
	Address   string `json:"address"`
	Name      string `json:"name"`
	CanSend   bool   `json:"canSend"`
	Personal  bool   `json:"personal"`
	Signature string `json:"signature"`
	Template  string `json:"template"`
	CanEdit   bool   `json:"canEditSignature"` // personal mailbox or administrator
}

type Session struct {
	ID       string
	UserID   int64
	Login    string
	Name     string
	IsAdmin  bool
	Accounts []Account
}

func (s *Session) account(addr string) (Account, bool) {
	addr = strings.ToLower(addr)
	for _, a := range s.Accounts {
		if a.Address == addr {
			return a, true
		}
	}
	return Account{}, false
}

func hashToken(t string) string {
	h := sha256.Sum256([]byte(t))
	return hex.EncodeToString(h[:])
}

// auth loads the session and rejects cross-site writes: browsers only send the custom
// X-ZM header from our own pages.
func (s *Server) auth(h func(http.ResponseWriter, *http.Request, *Session)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if (r.Method != http.MethodGet || needsAppHeader(r)) && r.Header.Get("X-ZM") != "1" {
			writeErr(w, http.StatusForbidden, "Request rejected")
			return
		}
		c, err := r.Cookie(sessionCookie)
		if err != nil || c.Value == "" {
			writeErr(w, http.StatusUnauthorized, "Please sign in")
			return
		}
		sess, err := s.loadSession(r.Context(), hashToken(c.Value))
		if err != nil {
			writeErr(w, http.StatusUnauthorized, "Your session has expired, please sign in again")
			return
		}
		h(w, r, sess)
	}
}

// loadSession: the session, its user and the user's mailboxes in one query (it runs on every
// request). "Last seen" is written at most every 5 minutes, not on every click.
func (s *Server) loadSession(ctx context.Context, id string) (*Session, error) {
	sess := &Session{ID: id}
	var accounts []struct {
		Address   string `json:"address"`
		Name      string `json:"name"`
		CanSend   bool   `json:"canSend"`
		Personal  bool   `json:"personal"`
		Signature string `json:"signature"`
		Template  string `json:"template"`
	}
	err := s.db.QueryRow(ctx, `
		WITH s AS (
			SELECT se.id, se.last_seen_at, u.id AS uid, u.login, u.display_name, u.is_admin
			FROM sessions se JOIN users u ON u.id = se.user_id
			WHERE se.id = $1 AND se.expires_at > now() AND se.last_seen_at > now() - interval '14 days' AND u.active
		), touch AS (
			UPDATE sessions SET last_seen_at = now()
			WHERE id IN (SELECT id FROM s WHERE last_seen_at < now() - interval '5 minutes')
		)
		SELECT s.uid, s.login, s.display_name, s.is_admin, COALESCE((
			SELECT json_agg(json_build_object('address', m.address, 'name', m.display_name, 'canSend', um.can_send,
				'personal', um.personal, 'signature', coalesce(ms.signature, ''), 'template', coalesce(ms.template, ''))
				ORDER BY um.personal DESC, um.position, m.address)
			FROM user_mailboxes um
			JOIN mailboxes m ON m.id = um.mailbox_id
			JOIN domains d ON d.id = m.domain_id
			LEFT JOIN mailbox_settings ms ON ms.mailbox_id = m.id
			WHERE um.user_id = s.uid AND m.active AND d.active), '[]')
		FROM s`, id).Scan(&sess.UserID, &sess.Login, &sess.Name, &sess.IsAdmin, &accounts)
	if err != nil {
		return nil, err
	}
	sess.Accounts = make([]Account, len(accounts))
	for i, a := range accounts {
		sess.Accounts[i] = Account{Address: a.Address, Name: a.Name, CanSend: a.CanSend, Personal: a.Personal, Signature: a.Signature, Template: a.Template}
		sess.Accounts[i].CanEdit = a.Personal || sess.IsAdmin
	}
	return sess, nil
}

func (s *Server) accountsFor(ctx context.Context, userID int64) ([]Account, error) {
	rows, err := s.db.Query(ctx, `
		SELECT m.address, m.display_name, um.can_send, um.personal,
		       coalesce(ms.signature, ''), coalesce(ms.template, '')
		FROM user_mailboxes um
		JOIN mailboxes m ON m.id = um.mailbox_id
		JOIN domains d ON d.id = m.domain_id
		LEFT JOIN mailbox_settings ms ON ms.mailbox_id = m.id
		WHERE um.user_id = $1 AND m.active AND d.active
		ORDER BY um.personal DESC, um.position, m.address`, userID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (Account, error) {
		var a Account
		err := r.Scan(&a.Address, &a.Name, &a.CanSend, &a.Personal, &a.Signature, &a.Template)
		return a, err
	})
}

// Compared against when the login does not exist, so response time does not reveal valid logins.
var dummyHash, _ = bcrypt.GenerateFromPassword([]byte("zmail-dummy-password"), 12)

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("X-ZM") != "1" {
		writeErr(w, http.StatusForbidden, "Request rejected")
		return
	}
	var in struct {
		Login    string `json:"login"`
		Password string `json:"password"`
	}
	if err := readJSON(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "Invalid request")
		return
	}
	login := strings.ToLower(strings.TrimSpace(in.Login))
	ip := clientIP(r)
	ipKey := limitKey(ip)
	if s.limiter.blocked(ipKey) {
		writeErr(w, http.StatusTooManyRequests, "Too many failed attempts. Wait 10 minutes.")
		return
	}
	if s.limLogin.blocked(login) {
		slog.Warn("web login locked", "login", login, "ip", ip)
		writeErr(w, http.StatusTooManyRequests, "Too many failed sign-in attempts for this account. Wait 15 minutes.")
		return
	}

	var userID int64
	var hash string
	var active bool
	err := s.db.QueryRow(r.Context(), `SELECT id, password_hash, active FROM users WHERE login=$1`, login).Scan(&userID, &hash, &active)
	if err != nil {
		hash = string(dummyHash)
	}
	ok := bcrypt.CompareHashAndPassword([]byte(hash), []byte(in.Password)) == nil && err == nil
	if !ok {
		s.limiter.fail(ipKey)
		s.limLogin.fail(login)
		slog.Warn("web login failed", "login", login, "ip", ip)
		s.audit(r.Context(), 0, login, ip, "login.failed", login, map[string]any{"ua": truncate(r.UserAgent(), 160)})
		writeErr(w, http.StatusUnauthorized, "Wrong username or password")
		return
	}
	if !active {
		writeErr(w, http.StatusForbidden, "This account is disabled. Contact your administrator.")
		return
	}

	buf := make([]byte, 32)
	_, _ = rand.Read(buf)
	token := hex.EncodeToString(buf)
	_, err = s.db.Exec(r.Context(), `INSERT INTO sessions(id, user_id, expires_at, user_agent, ip) VALUES ($1,$2,$3,$4,$5)`,
		hashToken(token), userID, time.Now().Add(sessionTTL), truncate(r.UserAgent(), 300), ip)
	if err != nil {
		serverErr(w, "create session", err)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: token, Path: "/", MaxAge: int(sessionTTL.Seconds()),
		HttpOnly: true, Secure: true, SameSite: http.SameSiteLaxMode,
	})
	slog.Info("web login", "login", login, "ip", ip)
	s.audit(r.Context(), userID, login, ip, "login.ok", login, map[string]any{"ua": truncate(r.UserAgent(), 160)})
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request, sess *Session) {
	_, _ = s.db.Exec(r.Context(), `DELETE FROM sessions WHERE id=$1`, sess.ID)
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", MaxAge: -1, HttpOnly: true, Secure: true, SameSite: http.SameSiteLaxMode})
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request, sess *Session) {
	s.syncContactsSoon(sess.Accounts) // keep the address book fresh for suggestions
	// The user's own settings in one query (pictures policy, allowed senders, background).
	policy, senders, bg := "ask", []string{}, ""
	var ver time.Time
	_ = s.db.QueryRow(r.Context(), `
		SELECT u.image_policy, COALESCE((SELECT array_agg(sender ORDER BY sender) FROM image_senders WHERE user_id = u.id), '{}'),
		       u.background, COALESCE(b.updated_at, 'epoch')
		FROM users u LEFT JOIN user_backgrounds b ON b.user_id = u.id WHERE u.id = $1`, sess.UserID).Scan(&policy, &senders, &bg, &ver)
	bgVer := ver.Unix()
	lang, tz := s.userLocale(r.Context(), sess.UserID)
	writeJSON(w, http.StatusOK, map[string]any{
		"login": sess.Login, "name": sess.Name, "isAdmin": sess.IsAdmin, "accounts": sess.Accounts,
		"imagePolicy": policy, "imageSenders": senders, "background": bg, "backgroundVersion": bgVer,
		"aiName": s.ai().Name, "lang": lang, "timezone": tz, "defaults": serverLocale(),
	})
}

// imagePrefs: whether pictures from the internet are shown in letters without asking.
func (s *Server) imagePrefs(ctx context.Context, userID int64) (string, []string) {
	policy, senders := "ask", []string{}
	_ = s.db.QueryRow(ctx, `SELECT image_policy FROM users WHERE id=$1`, userID).Scan(&policy)
	if rows, err := s.db.Query(ctx, `SELECT sender FROM image_senders WHERE user_id=$1 ORDER BY sender`, userID); err == nil {
		for rows.Next() {
			var x string
			if rows.Scan(&x) == nil {
				senders = append(senders, x)
			}
		}
		rows.Close()
	}
	return policy, senders
}

// POST /api/me/images {policy?: "ask"|"always", allow?: "a@b.ru", remove?: "a@b.ru"}
func (s *Server) handleImagePrefs(w http.ResponseWriter, r *http.Request, sess *Session) {
	var in struct{ Policy, Allow, Remove string }
	if err := readJSON(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "Invalid request")
		return
	}
	if in.Policy == "ask" || in.Policy == "always" {
		_, _ = s.db.Exec(r.Context(), `UPDATE users SET image_policy=$2 WHERE id=$1`, sess.UserID, in.Policy)
	}
	if a := strings.ToLower(strings.TrimSpace(in.Allow)); emailRe.MatchString(a) {
		_, _ = s.db.Exec(r.Context(), `INSERT INTO image_senders(user_id, sender) VALUES ($1,$2) ON CONFLICT DO NOTHING`, sess.UserID, a)
	}
	if rm := strings.ToLower(strings.TrimSpace(in.Remove)); rm != "" {
		_, _ = s.db.Exec(r.Context(), `DELETE FROM image_senders WHERE user_id=$1 AND sender=$2`, sess.UserID, rm)
	}
	policy, senders := s.imagePrefs(r.Context(), sess.UserID)
	writeJSON(w, http.StatusOK, map[string]any{"imagePolicy": policy, "imageSenders": senders})
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

// ---------- who sent what ----------

type SentBy struct {
	Login string `json:"login"`
	Name  string `json:"name"`
}

func normMsgID(id string) string { return strings.Trim(strings.TrimSpace(id), "<>") }

// sentByFor looks up which webmail user sent the given messages from this mailbox.
func (s *Server) sentByFor(ctx context.Context, mailbox string, msgIDs []string) map[string]SentBy {
	out := map[string]SentBy{}
	ids := make([]string, 0, len(msgIDs))
	for _, id := range msgIDs {
		if id = normMsgID(id); id != "" {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return out
	}
	rows, err := s.db.Query(ctx, `
		SELECT sm.message_id, sm.user_login, coalesce(u.display_name, '')
		FROM sent_messages sm JOIN mailboxes m ON m.id = sm.mailbox_id
		LEFT JOIN users u ON u.id = sm.user_id
		WHERE m.address = $1 AND sm.message_id = ANY($2)`, mailbox, ids)
	if err != nil {
		slog.Warn("sent_by lookup", "err", err)
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var sb SentBy
		if rows.Scan(&id, &sb.Login, &sb.Name) == nil {
			out[id] = sb
		}
	}
	return out
}

func (s *Server) recordSent(ctx context.Context, sess *Session, mailbox, msgID, subject string, rcpts []string, size int) {
	_, err := s.db.Exec(ctx, `
		INSERT INTO sent_messages(user_id, user_login, mailbox_id, message_id, subject, recipients, size_bytes)
		SELECT $1, $2, id, $4, $5, $6, $7 FROM mailboxes WHERE address = $3
		ON CONFLICT (mailbox_id, message_id) DO NOTHING`,
		sess.UserID, sess.Login, mailbox, normMsgID(msgID), subject, rcpts, size)
	if err != nil {
		slog.Error("record sent message", "err", err)
	}
}

// handleMailboxSettings saves the signature and start template of a mailbox.
// Users may edit their personal mailbox; administrators may edit any mailbox they work with.
func (s *Server) handleMailboxSettings(w http.ResponseWriter, r *http.Request, sess *Session) {
	var in struct {
		Address   string `json:"address"`
		Signature string `json:"signature"`
		Template  string `json:"template"`
	}
	if err := readJSON(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "Invalid request")
		return
	}
	acc, ok := sess.account(in.Address)
	if !ok || !acc.CanEdit {
		writeErr(w, http.StatusForbidden, "The signature of this mailbox is set by the administrator")
		return
	}
	if len(in.Signature) > 40000 || len(in.Template) > 20000 { // HTML signatures with links and pictures
		writeErr(w, http.StatusBadRequest, "The signature or template is too long")
		return
	}
	if err := s.saveMailboxSettings(r.Context(), acc.Address, in.Signature, in.Template, sess.Login); err != nil {
		serverErr(w, "mailbox settings", err)
		return
	}
	s.audit(r.Context(), sess.UserID, sess.Login, clientIP(r), "mailbox.signature", acc.Address, nil)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) saveMailboxSettings(ctx context.Context, address, signature, template, by string) error {
	_, err := s.db.Exec(ctx, `
		INSERT INTO mailbox_settings(mailbox_id, signature, template, updated_by)
		SELECT id, $2, $3, $4 FROM mailboxes WHERE address = $1
		ON CONFLICT (mailbox_id) DO UPDATE SET signature = EXCLUDED.signature, template = EXCLUDED.template,
		       updated_at = now(), updated_by = EXCLUDED.updated_by`,
		strings.ToLower(address), strings.TrimRight(signature, " \n\r\t"), template, by)
	return err
}

// needsAppHeader: GETs that mark mail as read or start heavy work (OCR, conversion) must come from
// our own scripts, not from an <img src="/api/…"> planted in a letter.
func needsAppHeader(r *http.Request) bool {
	switch r.URL.Path {
	case "/api/message":
		return true
	case "/api/attachment":
		switch r.URL.Query().Get("as") {
		case "ocr", "text", "html", "scan":
			return true
		}
	}
	return false
}

// limitKey groups IPv6 clients by /64: one host usually owns the whole prefix.
func limitKey(ip string) string {
	a := net.ParseIP(ip)
	if a == nil || a.To4() != nil {
		return ip
	}
	return a.Mask(net.CIDRMask(64, 128)).String() + "/64"
}
