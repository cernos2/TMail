package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"
)

var (
	loginRe  = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{1,62}$`)
	localRe  = regexp.MustCompile(`^[a-z0-9]([a-z0-9._+-]{0,62}[a-z0-9])?$`)
	domainRe = regexp.MustCompile(`^([a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z]{2,63}$`)
)

// rspamd's uid in its container (see services/rspamd/Dockerfile).
const rspamdUID = 11333

// admin wraps auth and requires the administrator flag.
func (s *Server) admin(h func(http.ResponseWriter, *http.Request, *Session)) http.HandlerFunc {
	return s.auth(func(w http.ResponseWriter, r *http.Request, sess *Session) {
		if !sess.IsAdmin {
			writeErr(w, http.StatusForbidden, "This section is available to administrators only")
			return
		}
		h(w, r, sess)
	})
}

func (s *Server) adminRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/admin/antivirus", s.admin(s.adminAntivirus))
	mux.HandleFunc("GET /api/admin/mcp-tokens", s.admin(s.adminTokens))
	mux.HandleFunc("DELETE /api/admin/mcp-tokens/{id}", s.admin(s.adminTokenRevoke))
	mux.HandleFunc("POST /api/admin/antivirus/test", s.admin(s.adminAntivirusTest))
	mux.HandleFunc("POST /api/admin/antivirus/quarantine/{id}", s.admin(s.adminQuarantineAction))
	mux.HandleFunc("GET /api/admin/overview", s.admin(s.adminOverview))
	mux.HandleFunc("GET /api/admin/ai", s.admin(s.adminAI))
	mux.HandleFunc("/api/admin/ai/provider", s.admin(s.adminAIProvider))
	mux.HandleFunc("GET /api/admin/crm", s.admin(s.adminCrmStatus))
	mux.HandleFunc("GET /api/admin/collector", s.admin(s.adminExtList))
	mux.HandleFunc("GET /api/admin/sender-rules", s.admin(s.handleSenderRules))
	mux.HandleFunc("DELETE /api/admin/sender-rules/{id}", s.admin(s.handleSenderRuleDelete))
	mux.HandleFunc("POST /api/admin/collector", s.admin(s.adminExtSave))
	mux.HandleFunc("PUT /api/admin/collector/{id}", s.admin(s.adminExtSave))
	mux.HandleFunc("DELETE /api/admin/collector/{id}", s.admin(s.adminExtDelete))
	mux.HandleFunc("POST /api/admin/collector/test", s.admin(s.adminExtTest))
	mux.HandleFunc("POST /api/admin/collector/{id}/sync", s.admin(s.adminExtSync))
	mux.HandleFunc("POST /api/admin/crm/backfill", s.admin(s.adminCrmBackfill))
	mux.HandleFunc("POST /api/admin/crm/reprocess", s.admin(s.adminCrmReprocess))
	mux.HandleFunc("GET /api/admin/import-mbox", s.admin(s.adminImportMbox))
	mux.HandleFunc("POST /api/admin/import-mbox", s.admin(s.adminImportMbox))
	mux.HandleFunc("PUT /api/admin/ai/prompts/{key}", s.admin(s.adminAIUpdate))
	mux.HandleFunc("POST /api/admin/ai/test", s.admin(s.adminAITest))
	mux.HandleFunc("GET /api/admin/users", s.admin(s.adminUsers))
	mux.HandleFunc("POST /api/admin/users", s.admin(s.adminCreateUser))
	mux.HandleFunc("PATCH /api/admin/users/{id}", s.admin(s.adminUpdateUser))
	mux.HandleFunc("DELETE /api/admin/users/{id}", s.admin(s.adminDeleteUser))
	mux.HandleFunc("POST /api/admin/users/{id}/password", s.admin(s.adminUserPassword))
	mux.HandleFunc("POST /api/admin/users/{id}/logout", s.admin(s.adminUserLogout))
	mux.HandleFunc("PUT /api/admin/users/{id}/mailboxes", s.admin(s.adminUserMailboxes))
	mux.HandleFunc("GET /api/admin/mailboxes", s.admin(s.adminMailboxes))
	mux.HandleFunc("POST /api/admin/mailboxes", s.admin(s.adminCreateMailbox))
	mux.HandleFunc("PATCH /api/admin/mailboxes/{id}", s.admin(s.adminUpdateMailbox))
	mux.HandleFunc("POST /api/admin/mailboxes/{id}/delete", s.admin(s.adminDeleteMailbox))
	mux.HandleFunc("POST /api/admin/mailboxes/{id}/restore", s.admin(s.adminRestoreMailbox))
	mux.HandleFunc("POST /api/admin/mailboxes/{id}/password", s.admin(s.adminMailboxPassword))
	mux.HandleFunc("PUT /api/admin/mailboxes/{id}/users", s.admin(s.adminMailboxUsers))
	mux.HandleFunc("GET /api/admin/aliases", s.admin(s.adminAliases))
	mux.HandleFunc("POST /api/admin/aliases", s.admin(s.adminSaveAlias))
	mux.HandleFunc("PATCH /api/admin/aliases/{id}", s.admin(s.adminSaveAlias))
	mux.HandleFunc("DELETE /api/admin/aliases/{id}", s.admin(s.adminDeleteAlias))
	mux.HandleFunc("GET /api/admin/domains", s.admin(s.adminDomains))
	mux.HandleFunc("POST /api/admin/domains", s.admin(s.adminCreateDomain))
	mux.HandleFunc("DELETE /api/admin/domains/{id}", s.admin(s.adminDeleteDomain))
	mux.HandleFunc("GET /api/admin/domains/{id}/dns", s.admin(s.adminDomainDNS))
	mux.HandleFunc("GET /api/admin/server", s.admin(s.adminServer))
	mux.HandleFunc("GET /api/admin/audit", s.admin(s.adminAudit))
}

// ---------- audit ----------

func (s *Server) audit(ctx context.Context, actorID int64, actor, ip, action, target string, details map[string]any) {
	if details == nil {
		details = map[string]any{}
	}
	var id any
	if actorID > 0 {
		id = actorID
	}
	if _, err := s.db.Exec(ctx, `INSERT INTO audit_log(actor_id, actor_login, action, target, details, ip) VALUES ($1,$2,$3,$4,$5,$6)`,
		id, actor, action, target, details, ip); err != nil {
		slog.Error("audit", "err", err)
	}
}

func (s *Server) auditAdmin(r *http.Request, sess *Session, action, target string, details map[string]any) {
	s.audit(r.Context(), sess.UserID, sess.Login, clientIP(r), action, target, details)
}

// ---------- helpers ----------

func pathID(r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	return id, err == nil && id > 0
}

func genPassword(n int) string {
	const alphabet = "abcdefghijkmnpqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	b := make([]byte, n)
	for i := range b {
		v, _ := rand.Int(rand.Reader, big.NewInt(int64(len(alphabet))))
		b[i] = alphabet[v.Int64()]
	}
	return string(b)
}

// pickPassword returns the given password or a generated one; generated=true means show it once.
func pickPassword(p string) (pass string, generated bool, err error) {
	if p == "" {
		return genPassword(16), true, nil
	}
	if len([]rune(p)) < 10 {
		return "", false, errors.New("Password is too short: at least 10 characters are required")
	}
	if len(p) > 72 { // bcrypt limit; longer ones would be refused and leave an empty hash
		return "", false, errors.New("Password is too long: at most 72 bytes (72 Latin characters, fewer in other alphabets)")
	}
	return p, false, nil
}

func isUnique(err error) bool { return err != nil && strings.Contains(err.Error(), "SQLSTATE 23505") }

// ---------- users ----------

type adminUser struct {
	ID        int64              `json:"id"`
	Login     string             `json:"login"`
	Name      string             `json:"name"`
	IsAdmin   bool               `json:"isAdmin"`
	Active    bool               `json:"active"`
	CreatedAt time.Time          `json:"createdAt"`
	LastSeen  *time.Time         `json:"lastSeen"`
	Sessions  int                `json:"sessions"`
	Mailboxes []userMailboxGrant `json:"mailboxes"`
	Lang      string             `json:"lang"`
	Timezone  string             `json:"timezone"`
}

type userMailboxGrant struct {
	MailboxID int64  `json:"mailboxId"`
	Address   string `json:"address,omitempty"`
	CanSend   bool   `json:"canSend"`
	Personal  bool   `json:"personal"`
}

func (s *Server) adminUsers(w http.ResponseWriter, r *http.Request, sess *Session) {
	rows, err := s.db.Query(r.Context(), `
		SELECT u.id, u.login, u.display_name, u.is_admin, u.active, u.created_at,
		       (SELECT max(last_seen_at) FROM sessions se WHERE se.user_id = u.id),
		       (SELECT count(*) FROM sessions se WHERE se.user_id = u.id AND se.expires_at > now()),
		       coalesce((SELECT json_agg(json_build_object('mailboxId', m.id, 'address', m.address, 'canSend', um.can_send, 'personal', um.personal)
		                 ORDER BY um.personal DESC, um.position, m.address)
		                 FROM user_mailboxes um JOIN mailboxes m ON m.id = um.mailbox_id WHERE um.user_id = u.id), '[]'),
		       u.lang, u.timezone
		FROM users u ORDER BY u.login`)
	if err != nil {
		serverErr(w, "admin users", err)
		return
	}
	users, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (adminUser, error) {
		var u adminUser
		var mbs []byte
		err := row.Scan(&u.ID, &u.Login, &u.Name, &u.IsAdmin, &u.Active, &u.CreatedAt, &u.LastSeen, &u.Sessions, &mbs, &u.Lang, &u.Timezone)
		if err == nil {
			err = json.Unmarshal(mbs, &u.Mailboxes)
		}
		return u, err
	})
	if err != nil {
		serverErr(w, "admin users scan", err)
		return
	}
	writeJSON(w, http.StatusOK, users)
}

func (s *Server) adminCreateUser(w http.ResponseWriter, r *http.Request, sess *Session) {
	var in struct {
		Login     string             `json:"login"`
		Name      string             `json:"name"`
		Password  string             `json:"password"`
		IsAdmin   bool               `json:"isAdmin"`
		Mailboxes []userMailboxGrant `json:"mailboxes"`
	}
	if err := readJSON(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "Invalid request")
		return
	}
	login := strings.ToLower(strings.TrimSpace(in.Login))
	if !loginRe.MatchString(login) {
		writeErr(w, http.StatusBadRequest, "Username: Latin letters, digits, dot, hyphen or underscore, at least 2 characters")
		return
	}
	pass, generated, err := pickPassword(in.Password)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	hash, _ := bcrypt.GenerateFromPassword([]byte(pass), 12)
	var id int64
	err = pgx.BeginFunc(r.Context(), s.db, func(tx pgx.Tx) error {
		if err := tx.QueryRow(r.Context(), `INSERT INTO users(login, display_name, password_hash, is_admin) VALUES ($1,$2,$3,$4) RETURNING id`,
			login, strings.TrimSpace(in.Name), string(hash), in.IsAdmin).Scan(&id); err != nil {
			return err
		}
		return setUserMailboxes(r.Context(), tx, id, in.Mailboxes)
	})
	if isUnique(err) {
		writeErr(w, http.StatusConflict, fmt.Sprintf("Username %q is already taken", login))
		return
	}
	if err != nil {
		serverErr(w, "create user", err)
		return
	}
	s.auditAdmin(r, sess, "user.create", login, map[string]any{"admin": in.IsAdmin, "mailboxes": len(in.Mailboxes)})
	out := map[string]any{"id": id}
	if generated {
		out["password"] = pass
	}
	writeJSON(w, http.StatusCreated, out)
}

func setUserMailboxes(ctx context.Context, tx pgx.Tx, userID int64, grants []userMailboxGrant) error {
	if _, err := tx.Exec(ctx, `DELETE FROM user_mailboxes WHERE user_id=$1`, userID); err != nil {
		return err
	}
	for i, g := range grants {
		if _, err := tx.Exec(ctx, `INSERT INTO user_mailboxes(user_id, mailbox_id, can_send, position, personal) VALUES ($1,$2,$3,$4,$5)`,
			userID, g.MailboxID, g.CanSend || g.Personal, i, g.Personal); err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) adminUpdateUser(w http.ResponseWriter, r *http.Request, sess *Session) {
	id, ok := pathID(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, "No user specified")
		return
	}
	var in struct {
		Name     *string `json:"name"`
		IsAdmin  *bool   `json:"isAdmin"`
		Active   *bool   `json:"active"`
		Lang     *string `json:"lang"`
		Timezone *string `json:"timezone"`
	}
	if err := readJSON(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "Invalid request")
		return
	}
	deref := func(p *string) string {
		if p == nil {
			return ""
		}
		return *p
	}
	if msg := checkLocale(deref(in.Lang), deref(in.Timezone)); msg != "" {
		writeErr(w, http.StatusBadRequest, msg)
		return
	}
	if id == sess.UserID && ((in.IsAdmin != nil && !*in.IsAdmin) || (in.Active != nil && !*in.Active)) {
		writeErr(w, http.StatusBadRequest, "You cannot disable yourself or remove your own administrator rights")
		return
	}
	var login string
	err := s.db.QueryRow(r.Context(), `
		UPDATE users SET display_name = coalesce($2, display_name), is_admin = coalesce($3, is_admin),
		       active = coalesce($4, active), lang = coalesce($5, lang), timezone = coalesce($6, timezone), updated_at = now()
		WHERE id = $1 RETURNING login`, id, in.Name, in.IsAdmin, in.Active, in.Lang, in.Timezone).Scan(&login)
	if err != nil {
		writeErr(w, http.StatusNotFound, "User not found")
		return
	}
	if in.Active != nil && !*in.Active {
		_, _ = s.db.Exec(r.Context(), `DELETE FROM sessions WHERE user_id=$1`, id)
	}
	s.auditAdmin(r, sess, "user.update", login, map[string]any{"name": in.Name, "admin": in.IsAdmin, "active": in.Active})
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) adminDeleteUser(w http.ResponseWriter, r *http.Request, sess *Session) {
	id, ok := pathID(r)
	if !ok || id == sess.UserID {
		writeErr(w, http.StatusBadRequest, "You cannot delete yourself")
		return
	}
	var login string
	if err := s.db.QueryRow(r.Context(), `DELETE FROM users WHERE id=$1 RETURNING login`, id).Scan(&login); err != nil {
		writeErr(w, http.StatusNotFound, "User not found")
		return
	}
	s.auditAdmin(r, sess, "user.delete", login, nil)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) adminUserPassword(w http.ResponseWriter, r *http.Request, sess *Session) {
	id, ok := pathID(r)
	var in struct {
		Password string `json:"password"`
	}
	if !ok || readJSON(r, &in) != nil {
		writeErr(w, http.StatusBadRequest, "Invalid request")
		return
	}
	pass, generated, err := pickPassword(in.Password)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	hash, _ := bcrypt.GenerateFromPassword([]byte(pass), 12)
	var login string
	if err := s.db.QueryRow(r.Context(), `UPDATE users SET password_hash=$2, updated_at=now() WHERE id=$1 RETURNING login`, id, string(hash)).Scan(&login); err != nil {
		writeErr(w, http.StatusNotFound, "User not found")
		return
	}
	// Other devices of this user are signed out; the admin's own current session stays.
	_, _ = s.db.Exec(r.Context(), `DELETE FROM sessions WHERE user_id=$1 AND id <> $2`, id, sess.ID)
	s.auditAdmin(r, sess, "user.password", login, nil)
	out := map[string]any{"ok": true}
	if generated {
		out["password"] = pass
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) adminUserLogout(w http.ResponseWriter, r *http.Request, sess *Session) {
	id, ok := pathID(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, "No user specified")
		return
	}
	tag, err := s.db.Exec(r.Context(), `DELETE FROM sessions WHERE user_id=$1 AND id <> $2`, id, sess.ID)
	if err != nil {
		serverErr(w, "logout user", err)
		return
	}
	var login string
	_ = s.db.QueryRow(r.Context(), `SELECT login FROM users WHERE id=$1`, id).Scan(&login)
	s.auditAdmin(r, sess, "user.logout", login, map[string]any{"sessions": tag.RowsAffected()})
	writeJSON(w, http.StatusOK, map[string]int64{"closed": tag.RowsAffected()})
}

func (s *Server) adminUserMailboxes(w http.ResponseWriter, r *http.Request, sess *Session) {
	id, ok := pathID(r)
	var in []userMailboxGrant
	if !ok || readJSON(r, &in) != nil {
		writeErr(w, http.StatusBadRequest, "Invalid request")
		return
	}
	err := pgx.BeginFunc(r.Context(), s.db, func(tx pgx.Tx) error { return setUserMailboxes(r.Context(), tx, id, in) })
	if err != nil {
		writeErr(w, http.StatusBadRequest, "Could not save access: check that the mailboxes exist")
		return
	}
	var login string
	_ = s.db.QueryRow(r.Context(), `SELECT login FROM users WHERE id=$1`, id).Scan(&login)
	s.auditAdmin(r, sess, "user.access", login, map[string]any{"mailboxes": in})
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// ---------- mailboxes ----------

type adminMailbox struct {
	ID         int64      `json:"id"`
	Address    string     `json:"address"`
	Domain     string     `json:"domain"`
	Name       string     `json:"name"`
	QuotaBytes int64      `json:"quotaBytes"`
	UsedBytes  *int64     `json:"usedBytes"`
	Messages   *int64     `json:"messages"`
	Active     bool       `json:"active"`
	CreatedAt  time.Time  `json:"createdAt"`
	Signature  string     `json:"signature"`
	External   bool       `json:"external"`  // mirrored external mailbox (Gmail…), see the mail collector
	DeletedAt  *time.Time `json:"deletedAt"` // deleted, letters erased at PurgeAfter unless restored
	PurgeAfter *time.Time `json:"purgeAfter"`
	DeletedBy  string     `json:"deletedBy"`
	Template   string     `json:"template"`
	Users      []struct {
		ID       int64  `json:"id"`
		Login    string `json:"login"`
		CanSend  bool   `json:"canSend"`
		Personal bool   `json:"personal"`
	} `json:"users"`
}

func (s *Server) adminMailboxes(w http.ResponseWriter, r *http.Request, sess *Session) {
	rows, err := s.db.Query(r.Context(), `
		SELECT m.id, m.address, d.name, m.display_name, m.quota_bytes, m.active, m.created_at,
		       coalesce(ms.signature, ''), coalesce(ms.template, ''), d.external, m.deleted_at, m.purge_after, m.deleted_by,
		       coalesce((SELECT json_agg(json_build_object('id', u.id, 'login', u.login, 'canSend', um.can_send, 'personal', um.personal) ORDER BY u.login)
		                 FROM user_mailboxes um JOIN users u ON u.id = um.user_id WHERE um.mailbox_id = m.id), '[]')
		FROM mailboxes m JOIN domains d ON d.id = m.domain_id
		LEFT JOIN mailbox_settings ms ON ms.mailbox_id = m.id
		ORDER BY d.name, m.address`)
	if err != nil {
		serverErr(w, "admin mailboxes", err)
		return
	}
	list, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (adminMailbox, error) {
		var m adminMailbox
		var users []byte
		err := row.Scan(&m.ID, &m.Address, &m.Domain, &m.Name, &m.QuotaBytes, &m.Active, &m.CreatedAt, &m.Signature, &m.Template, &m.External, &m.DeletedAt, &m.PurgeAfter, &m.DeletedBy, &users)
		if err == nil {
			err = json.Unmarshal(users, &m.Users)
		}
		return m, err
	})
	if err != nil {
		serverErr(w, "admin mailboxes scan", err)
		return
	}
	addrs := make([]string, 0, len(list))
	for _, m := range list {
		if m.Active {
			addrs = append(addrs, m.Address)
		}
	}
	usage := s.quotaUsage(r.Context(), addrs)
	for i := range list {
		if u, ok := usage[list[i].Address]; ok {
			b, n := u.bytes, u.messages
			list[i].UsedBytes, list[i].Messages = &b, &n
		}
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) adminCreateMailbox(w http.ResponseWriter, r *http.Request, sess *Session) {
	var in struct {
		LocalPart  string  `json:"localPart"`
		Domain     string  `json:"domain"`
		Name       string  `json:"name"`
		QuotaBytes int64   `json:"quotaBytes"`
		Password   string  `json:"password"`
		GrantUsers []int64 `json:"grantUsers"`
	}
	if err := readJSON(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "Invalid request")
		return
	}
	local, domain := strings.ToLower(strings.TrimSpace(in.LocalPart)), strings.ToLower(strings.TrimSpace(in.Domain))
	if !localRe.MatchString(local) {
		writeErr(w, http.StatusBadRequest, "Mailbox name: Latin letters, digits, dot, hyphen, plus or underscore")
		return
	}
	if in.QuotaBytes < 0 {
		writeErr(w, http.StatusBadRequest, "Quota cannot be negative")
		return
	}
	pass, generated, err := pickPassword(in.Password)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	hash, _ := bcrypt.GenerateFromPassword([]byte(pass), 12)
	addr := local + "@" + domain
	var aliasClash bool
	_ = s.db.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM aliases WHERE source=$1)`, addr).Scan(&aliasClash)
	if aliasClash {
		writeErr(w, http.StatusConflict, fmt.Sprintf("Address %s is already used by an alias. Delete the alias or choose another name.", addr))
		return
	}
	var id int64
	err = pgx.BeginFunc(r.Context(), s.db, func(tx pgx.Tx) error {
		err := tx.QueryRow(r.Context(), `
			INSERT INTO mailboxes(domain_id, local_part, address, display_name, password_hash, quota_bytes)
			SELECT id, $2, $3, $4, $5, $6 FROM domains WHERE name = $1 RETURNING id`,
			domain, local, addr, strings.TrimSpace(in.Name), "{BLF-CRYPT}"+string(hash), in.QuotaBytes).Scan(&id)
		if err != nil {
			return err
		}
		for _, uid := range in.GrantUsers {
			if _, err := tx.Exec(r.Context(), `INSERT INTO user_mailboxes(user_id, mailbox_id, can_send, position)
				VALUES ($1, $2, true, (SELECT count(*) FROM user_mailboxes WHERE user_id=$1)) ON CONFLICT DO NOTHING`, uid, id); err != nil {
				return err
			}
		}
		return nil
	})
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		writeErr(w, http.StatusBadRequest, fmt.Sprintf("Domain %s not found", domain))
		return
	case isUnique(err):
		writeErr(w, http.StatusConflict, fmt.Sprintf("Mailbox %s already exists", addr))
		return
	case err != nil:
		serverErr(w, "create mailbox", err)
		return
	}
	s.auditAdmin(r, sess, "mailbox.create", addr, map[string]any{"quota": in.QuotaBytes})
	out := map[string]any{"id": id, "address": addr}
	if generated {
		out["password"] = pass
	}
	writeJSON(w, http.StatusCreated, out)
}

func (s *Server) adminUpdateMailbox(w http.ResponseWriter, r *http.Request, sess *Session) {
	id, ok := pathID(r)
	var in struct {
		Name       *string `json:"name"`
		QuotaBytes *int64  `json:"quotaBytes"`
		Active     *bool   `json:"active"`
		Signature  *string `json:"signature"`
		Template   *string `json:"template"`
	}
	if !ok || readJSON(r, &in) != nil || (in.QuotaBytes != nil && *in.QuotaBytes < 0) {
		writeErr(w, http.StatusBadRequest, "Invalid request")
		return
	}
	var addr string
	err := s.db.QueryRow(r.Context(), `
		UPDATE mailboxes SET display_name = coalesce($2, display_name), quota_bytes = coalesce($3, quota_bytes),
		       active = coalesce($4, active), updated_at = now()
		WHERE id = $1 AND deleted_at IS NULL RETURNING address`, id, in.Name, in.QuotaBytes, in.Active).Scan(&addr)
	if err != nil {
		writeErr(w, http.StatusNotFound, "Mailbox not found")
		return
	}
	if in.Signature != nil || in.Template != nil {
		var sig, tpl string
		_ = s.db.QueryRow(r.Context(), `SELECT coalesce(ms.signature,''), coalesce(ms.template,'') FROM mailboxes m
			LEFT JOIN mailbox_settings ms ON ms.mailbox_id = m.id WHERE m.id=$1`, id).Scan(&sig, &tpl)
		if in.Signature != nil {
			sig = *in.Signature
		}
		if in.Template != nil {
			tpl = *in.Template
		}
		if err := s.saveMailboxSettings(r.Context(), addr, sig, tpl, sess.Login); err != nil {
			serverErr(w, "mailbox settings", err)
			return
		}
	}
	s.auditAdmin(r, sess, "mailbox.update", addr, map[string]any{"name": in.Name, "quota": in.QuotaBytes, "active": in.Active})
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) adminMailboxPassword(w http.ResponseWriter, r *http.Request, sess *Session) {
	id, ok := pathID(r)
	var in struct {
		Password string `json:"password"`
	}
	if !ok || readJSON(r, &in) != nil {
		writeErr(w, http.StatusBadRequest, "Invalid request")
		return
	}
	pass, generated, err := pickPassword(in.Password)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	hash, _ := bcrypt.GenerateFromPassword([]byte(pass), 12)
	var addr string
	if err := s.db.QueryRow(r.Context(), `UPDATE mailboxes SET password_hash=$2, updated_at=now() WHERE id=$1 RETURNING address`,
		id, "{BLF-CRYPT}"+string(hash)).Scan(&addr); err != nil {
		writeErr(w, http.StatusNotFound, "Mailbox not found")
		return
	}
	s.auditAdmin(r, sess, "mailbox.password", addr, nil)
	out := map[string]any{"ok": true}
	if generated {
		out["password"] = pass
	}
	writeJSON(w, http.StatusOK, out)
}

// ---------- aliases ----------

func (s *Server) adminAliases(w http.ResponseWriter, r *http.Request, sess *Session) {
	rows, err := s.db.Query(r.Context(), `SELECT a.id, a.source, a.destinations, a.active, d.name, a.created_at
		FROM aliases a JOIN domains d ON d.id = a.domain_id ORDER BY d.name, a.source`)
	if err != nil {
		serverErr(w, "admin aliases", err)
		return
	}
	type alias struct {
		ID           int64     `json:"id"`
		Source       string    `json:"source"`
		Destinations []string  `json:"destinations"`
		Active       bool      `json:"active"`
		Domain       string    `json:"domain"`
		CreatedAt    time.Time `json:"createdAt"`
	}
	list, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (alias, error) {
		var a alias
		err := row.Scan(&a.ID, &a.Source, &a.Destinations, &a.Active, &a.Domain, &a.CreatedAt)
		return a, err
	})
	if err != nil {
		serverErr(w, "admin aliases scan", err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

var emailRe = regexp.MustCompile(`^[a-z0-9._%+-]+@([a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z]{2,63}$`)

func (s *Server) adminSaveAlias(w http.ResponseWriter, r *http.Request, sess *Session) {
	var in struct {
		Source       string   `json:"source"`
		Destinations []string `json:"destinations"`
		Active       *bool    `json:"active"`
	}
	if err := readJSON(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "Invalid request")
		return
	}
	var dests []string
	for _, d := range in.Destinations {
		d = strings.ToLower(strings.TrimSpace(d))
		if d == "" {
			continue
		}
		if !emailRe.MatchString(d) {
			writeErr(w, http.StatusBadRequest, fmt.Sprintf("Invalid recipient address: %s", d))
			return
		}
		dests = append(dests, d)
	}
	if r.Method == http.MethodPatch {
		id, ok := pathID(r)
		if !ok || (in.Destinations != nil && len(dests) == 0) {
			writeErr(w, http.StatusBadRequest, "Specify at least one recipient")
			return
		}
		var dArg any
		if in.Destinations != nil {
			dArg = dests
		}
		var src string
		if err := s.db.QueryRow(r.Context(), `UPDATE aliases SET destinations = coalesce($2, destinations), active = coalesce($3, active)
			WHERE id=$1 RETURNING source`, id, dArg, in.Active).Scan(&src); err != nil {
			writeErr(w, http.StatusNotFound, "Alias not found")
			return
		}
		s.auditAdmin(r, sess, "alias.update", src, map[string]any{"destinations": dests, "active": in.Active})
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
		return
	}
	src := strings.ToLower(strings.TrimSpace(in.Source))
	local, domain, ok := strings.Cut(src, "@")
	if !ok || !localRe.MatchString(local) || !domainRe.MatchString(domain) {
		writeErr(w, http.StatusBadRequest, "Invalid alias address")
		return
	}
	if len(dests) == 0 {
		writeErr(w, http.StatusBadRequest, "Specify at least one recipient")
		return
	}
	var isMailbox bool
	_ = s.db.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM mailboxes WHERE address=$1)`, src).Scan(&isMailbox)
	if isMailbox {
		writeErr(w, http.StatusConflict, fmt.Sprintf("%s is already a mailbox. An alias with this address would intercept its mail.", src))
		return
	}
	var id int64
	err := s.db.QueryRow(r.Context(), `INSERT INTO aliases(domain_id, source, destinations) SELECT id, $2, $3 FROM domains WHERE name=$1 RETURNING id`,
		domain, src, dests).Scan(&id)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		writeErr(w, http.StatusBadRequest, fmt.Sprintf("Domain %s is not served by this server", domain))
		return
	case isUnique(err):
		writeErr(w, http.StatusConflict, fmt.Sprintf("Alias %s already exists", src))
		return
	case err != nil:
		serverErr(w, "create alias", err)
		return
	}
	s.auditAdmin(r, sess, "alias.create", src, map[string]any{"destinations": dests})
	writeJSON(w, http.StatusCreated, map[string]int64{"id": id})
}

func (s *Server) adminDeleteAlias(w http.ResponseWriter, r *http.Request, sess *Session) {
	id, ok := pathID(r)
	var src string
	if !ok || s.db.QueryRow(r.Context(), `DELETE FROM aliases WHERE id=$1 RETURNING source`, id).Scan(&src) != nil {
		writeErr(w, http.StatusNotFound, "Alias not found")
		return
	}
	s.auditAdmin(r, sess, "alias.delete", src, nil)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// ---------- domains ----------

func (s *Server) adminDomains(w http.ResponseWriter, r *http.Request, sess *Session) {
	rows, err := s.db.Query(r.Context(), `
		SELECT d.id, d.name, d.active, d.created_at,
		       (SELECT count(*) FROM mailboxes m WHERE m.domain_id = d.id),
		       (SELECT count(*) FROM aliases a WHERE a.domain_id = d.id),
		       coalesce((SELECT k.selector FROM dkim_keys k WHERE k.domain_id = d.id AND k.active ORDER BY k.created_at DESC LIMIT 1), '')
		FROM domains d WHERE NOT d.external ORDER BY d.name`)
	if err != nil {
		serverErr(w, "admin domains", err)
		return
	}
	type domain struct {
		ID        int64     `json:"id"`
		Name      string    `json:"name"`
		Active    bool      `json:"active"`
		CreatedAt time.Time `json:"createdAt"`
		Mailboxes int       `json:"mailboxes"`
		Aliases   int       `json:"aliases"`
		Selector  string    `json:"dkimSelector"`
	}
	list, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (domain, error) {
		var d domain
		err := row.Scan(&d.ID, &d.Name, &d.Active, &d.CreatedAt, &d.Mailboxes, &d.Aliases, &d.Selector)
		return d, err
	})
	if err != nil {
		serverErr(w, "admin domains scan", err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) adminCreateDomain(w http.ResponseWriter, r *http.Request, sess *Session) {
	var in struct {
		Name string `json:"name"`
	}
	if err := readJSON(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "Invalid request")
		return
	}
	name := strings.ToLower(strings.TrimSpace(in.Name))
	if !domainRe.MatchString(name) {
		writeErr(w, http.StatusBadRequest, "Invalid domain name, for example: example.com")
		return
	}
	var id int64
	if err := s.db.QueryRow(r.Context(), `INSERT INTO domains(name) VALUES ($1) RETURNING id`, name).Scan(&id); err != nil {
		if isUnique(err) {
			writeErr(w, http.StatusConflict, fmt.Sprintf("Domain %s has already been added", name))
			return
		}
		serverErr(w, "create domain", err)
		return
	}
	if err := s.genDKIM(r.Context(), id, name); err != nil {
		slog.Error("dkim gen", "domain", name, "err", err)
		writeErr(w, http.StatusInternalServerError, fmt.Sprintf("The domain was added, but its DKIM key could not be created. Create it with the command zmail dkim gen %s", name))
		return
	}
	s.auditAdmin(r, sess, "domain.create", name, nil)
	writeJSON(w, http.StatusCreated, map[string]int64{"id": id})
}

func (s *Server) adminDeleteDomain(w http.ResponseWriter, r *http.Request, sess *Session) {
	id, ok := pathID(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, "No domain specified")
		return
	}
	var name string
	var used int
	if err := s.db.QueryRow(r.Context(), `SELECT name, (SELECT count(*) FROM mailboxes WHERE domain_id=$1) + (SELECT count(*) FROM aliases WHERE domain_id=$1)
		FROM domains WHERE id=$1`, id).Scan(&name, &used); err != nil {
		writeErr(w, http.StatusNotFound, "Domain not found")
		return
	}
	if used > 0 {
		writeErr(w, http.StatusConflict, "The domain still has mailboxes or aliases. Move or delete them first.")
		return
	}
	if _, err := s.db.Exec(r.Context(), `DELETE FROM domains WHERE id=$1`, id); err != nil {
		serverErr(w, "delete domain", err)
		return
	}
	_ = s.writeSelectorMap(r.Context())
	s.auditAdmin(r, sess, "domain.delete", name, nil)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// genDKIM creates a 2048-bit key readable by rspamd and records the public part.
func (s *Server) genDKIM(ctx context.Context, domainID int64, domain string) error {
	sel := s.cfg.DKIMSelector
	path := filepath.Join(s.cfg.DKIMDir, domain+"."+sel+".key")
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return err
	}
	pub, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		return err
	}
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	if err := os.WriteFile(path, pemBytes, 0o600); err != nil {
		return err
	}
	_ = os.Chown(path, rspamdUID, rspamdUID)
	if _, err := s.db.Exec(ctx, `INSERT INTO dkim_keys(domain_id, selector, public_key) VALUES ($1,$2,$3)
		ON CONFLICT (domain_id, selector) DO UPDATE SET public_key = EXCLUDED.public_key, active = true`,
		domainID, sel, base64.StdEncoding.EncodeToString(pub)); err != nil {
		os.Remove(path)
		return err
	}
	return s.writeSelectorMap(ctx)
}

func (s *Server) writeSelectorMap(ctx context.Context) error {
	rows, err := s.db.Query(ctx, `SELECT d.name, k.selector FROM dkim_keys k JOIN domains d ON d.id=k.domain_id WHERE k.active ORDER BY d.name`)
	if err != nil {
		return err
	}
	defer rows.Close()
	var b strings.Builder
	for rows.Next() {
		var d, sel string
		if err := rows.Scan(&d, &sel); err != nil {
			return err
		}
		fmt.Fprintf(&b, "%s %s\n", d, sel)
	}
	p := filepath.Join(s.cfg.DKIMDir, "selectors.map")
	if err := os.WriteFile(p+".tmp", []byte(b.String()), 0o644); err != nil {
		return err
	}
	return os.Rename(p+".tmp", p)
}

// adminMailboxUsers replaces the list of users who work with a mailbox (the mailbox-side view of
// user_mailboxes; the user editor offers the same from the other side).
func (s *Server) adminMailboxUsers(w http.ResponseWriter, r *http.Request, sess *Session) {
	id, ok := pathID(r)
	var in []struct {
		UserID   int64 `json:"userId"`
		CanSend  bool  `json:"canSend"`
		Personal bool  `json:"personal"`
	}
	if !ok || readJSON(r, &in) != nil {
		writeErr(w, http.StatusBadRequest, "Invalid request")
		return
	}
	var addr string
	err := pgx.BeginFunc(r.Context(), s.db, func(tx pgx.Tx) error {
		if err := tx.QueryRow(r.Context(), `SELECT address FROM mailboxes WHERE id=$1`, id).Scan(&addr); err != nil {
			return err
		}
		if _, err := tx.Exec(r.Context(), `DELETE FROM user_mailboxes WHERE mailbox_id=$1`, id); err != nil {
			return err
		}
		for _, g := range in {
			if _, err := tx.Exec(r.Context(), `
				INSERT INTO user_mailboxes(user_id, mailbox_id, can_send, personal, position)
				VALUES ($1, $2, $3, $4, (SELECT count(*) FROM user_mailboxes WHERE user_id=$1))`,
				g.UserID, id, g.CanSend || g.Personal, g.Personal); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		writeErr(w, http.StatusBadRequest, "Could not save access: check that the users exist")
		return
	}
	s.auditAdmin(r, sess, "mailbox.access", addr, map[string]any{"users": len(in)})
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
