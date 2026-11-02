package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"math/big"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// App passwords: one per phone or mail program, so a lost phone is cut off without changing the
// mailbox password everywhere. Dovecot (and Postfix through it) checks them with mta.app_login;
// calendars and contacts (dav.go) check them here.

type appPassword struct {
	ID        int64      `json:"id"`
	Name      string     `json:"name"`
	Mailboxes []string   `json:"mailboxes"`
	Created   time.Time  `json:"created"`
	LastUsed  *time.Time `json:"lastUsed"`
	LastIP    string     `json:"lastIp"`
}

// newAppSecret: 16 lowercase letters in groups of four ("abcd-efgh-ijkl-mnop"), easy to type on a
// phone, about 75 bits. The first 8 characters find the row (Dovecot passes them as %8w).
func newAppSecret() string {
	const letters = "abcdefghijkmnopqrstuvwxyz" // no "l": it looks like "1" and "I"
	var b strings.Builder
	for i := 0; i < 16; i++ {
		if i > 0 && i%4 == 0 {
			b.WriteByte('-')
		}
		n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(letters))))
		b.WriteByte(letters[n.Int64()])
	}
	return b.String()
}

func (s *Server) handleAppPasswords(w http.ResponseWriter, r *http.Request, sess *Session) {
	rows, err := s.db.Query(r.Context(), `SELECT id, name, mailboxes, created_at, last_used_at, last_ip FROM app_passwords
		WHERE user_id=$1 ORDER BY created_at DESC`, sess.UserID)
	if err != nil {
		serverErr(w, "app passwords", err)
		return
	}
	defer rows.Close()
	out := []appPassword{}
	for rows.Next() {
		var p appPassword
		if rows.Scan(&p.ID, &p.Name, &p.Mailboxes, &p.Created, &p.LastUsed, &p.LastIP) == nil {
			out = append(out, p)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out, "host": s.cfg.Hostname, "login": sess.Login,
		"dav": "https://" + s.cfg.Hostname + "/dav/"})
}

func (s *Server) handleAppPasswordCreate(w http.ResponseWriter, r *http.Request, sess *Session) {
	var in struct {
		Name      string   `json:"name"`
		Mailboxes []string `json:"mailboxes"`
	}
	if err := readJSON(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "Invalid request")
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		writeErr(w, http.StatusBadRequest, "Name the device, for example \"iPhone\" or \"Outlook at work\"")
		return
	}
	boxes := []string{}
	for _, m := range in.Mailboxes {
		if a, ok := sess.account(m); ok && !slices.Contains(boxes, a.Address) {
			boxes = append(boxes, a.Address)
		}
	}
	var n int
	_ = s.db.QueryRow(r.Context(), `SELECT count(*) FROM app_passwords WHERE user_id=$1`, sess.UserID).Scan(&n)
	if n >= 50 {
		writeErr(w, http.StatusBadRequest, "Too many app passwords: delete the ones you no longer use")
		return
	}
	secret := newAppSecret()
	hash, err := bcrypt.GenerateFromPassword([]byte(secret), 10)
	if err != nil {
		serverErr(w, "app password hash", err)
		return
	}
	var p appPassword
	err = s.db.QueryRow(r.Context(), `INSERT INTO app_passwords(user_id, name, prefix, password_hash, mailboxes) VALUES ($1,$2,$3,$4,$5)
		RETURNING id, name, mailboxes, created_at`, sess.UserID, truncate(in.Name, 80), secret[:8], string(hash), boxes).
		Scan(&p.ID, &p.Name, &p.Mailboxes, &p.Created)
	if err != nil {
		serverErr(w, "app password create", err)
		return
	}
	s.audit(r.Context(), sess.UserID, sess.Login, clientIP(r), "apppass.create", p.Name, map[string]any{"mailboxes": boxes})
	writeJSON(w, http.StatusOK, map[string]any{"item": p, "password": secret})
}

func (s *Server) handleAppPasswordDelete(w http.ResponseWriter, r *http.Request, sess *Session) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	var name string
	err := s.db.QueryRow(r.Context(), `DELETE FROM app_passwords WHERE id=$1 AND user_id=$2 RETURNING name`, id, sess.UserID).Scan(&name)
	if err != nil {
		writeErr(w, http.StatusNotFound, "Not found")
		return
	}
	davAuth.forget(sess.UserID)
	s.audit(r.Context(), sess.UserID, sess.Login, clientIP(r), "apppass.revoke", name, nil)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// ---------- sign-in of calendar and contact programs ----------

// davAuthCache keeps successful sign-ins for a few minutes: phones send dozens of requests per
// sync, and bcrypt on each would be slow.
type davAuthCacheT struct {
	mu sync.Mutex
	m  map[string]davAuthHit
}

type davAuthHit struct {
	user int64
	at   time.Time
}

var davAuth = &davAuthCacheT{m: map[string]davAuthHit{}}

func (c *davAuthCacheT) get(key string) (int64, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	h, ok := c.m[key]
	if !ok || time.Since(h.at) > 5*time.Minute {
		delete(c.m, key)
		return 0, false
	}
	return h.user, true
}

func (c *davAuthCacheT) put(key string, user int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.m) > 5000 {
		clear(c.m)
	}
	c.m[key] = davAuthHit{user, time.Now()}
}

// forget drops a user's cached sign-ins (a password was revoked or changed).
func (c *davAuthCacheT) forget(user int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for k, h := range c.m {
		if h.user == user {
			delete(c.m, k)
		}
	}
}

var errDavAuth = errors.New("wrong username or password")

// davLogin checks a calendar/contacts program's credentials: the TMAIL login (or one of the user's
// mailbox addresses) with an app password, or the login with the account password.
func (s *Server) davLogin(ctx context.Context, username, password, ip string) (int64, error) {
	username = strings.ToLower(strings.TrimSpace(username))
	sum := sha256.Sum256([]byte(username + "\x00" + password))
	key := hex.EncodeToString(sum[:])
	if uid, ok := davAuth.get(key); ok {
		return uid, nil
	}
	if s.limiter.blocked(limitKey(ip)) {
		return 0, errDavAuth
	}
	fail := func() (int64, error) {
		s.limiter.fail(limitKey(ip))
		return 0, errDavAuth
	}
	if len(password) >= 8 {
		var id, uid int64
		var hash string
		err := s.db.QueryRow(ctx, `SELECT a.id, a.user_id, a.password_hash FROM app_passwords a JOIN users u ON u.id = a.user_id
			WHERE a.prefix=$1 AND u.active AND (u.login=$2 OR EXISTS (SELECT 1 FROM user_mailboxes um JOIN mailboxes m ON m.id = um.mailbox_id
				WHERE um.user_id = u.id AND m.address = $2))`, password[:8], username).Scan(&id, &uid, &hash)
		if err == nil && bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil {
			_, _ = s.db.Exec(ctx, `UPDATE app_passwords SET last_used_at=now(), last_ip=$2 WHERE id=$1`, id, truncate(ip, 64))
			davAuth.put(key, uid)
			return uid, nil
		}
	}
	var uid int64
	var hash string
	if err := s.db.QueryRow(ctx, `SELECT id, password_hash FROM users WHERE login=$1 AND active`, username).Scan(&uid, &hash); err != nil {
		_ = bcrypt.CompareHashAndPassword(dummyHash, []byte(password))
		return fail()
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) != nil {
		return fail()
	}
	davAuth.put(key, uid)
	return uid, nil
}
