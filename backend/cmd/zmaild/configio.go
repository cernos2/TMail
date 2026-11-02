package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/scrypt"
)

// Export and import of the whole configuration, to move TMAIL to another server or restore it:
// domains and DKIM keys, mailboxes (with their passwords), aliases, users and their rights,
// signatures, templates, folders and rules, labels, auto-replies, the mail collector, calendars,
// counterparties, AI and mail server settings. Letters themselves are not in the file: they move
// with the mail volume (backup) or are fetched again by the collector.
//
// The file is encrypted with a passphrase (scrypt + AES-256-GCM): it holds password hashes,
// DKIM private keys and the secret that protects external mailbox passwords.

const configMagic = "TMAILCFG1\n"

// configTables in the order they are restored (a table after the ones it refers to).
var configTables = []string{
	"domains", "dkim_keys", "mailboxes", "aliases", "users", "user_mailboxes", "user_settings", "mailbox_settings",
	"user_avatars", "user_backgrounds", "image_senders", "sent_messages",
	"ai_provider", "ai_prompts", "reply_templates", "folder_rules", "labels", "autoreplies", "sender_rules", "spell_dict",
	"external_accounts", "external_folders",
	"mcp_tokens", "app_passwords",
	"calendars", "calendar_shares", "cal_events", "cal_months",
	"crm_parties", "crm_addresses", "crm_person_companies", "crm_company_links", "crm_aliases", "crm_self",
	"crm_files", "crm_letters", "crm_suggestions", "crm_processed",
}

// Large and only useful when the letters move too: what the collector already fetched.
var collectorHistoryTables = []string{"external_seen", "external_messages"}

type configBundle struct {
	Version    int                        `json:"version"`
	Created    time.Time                  `json:"created"`
	Hostname   string                     `json:"hostname"`
	Migrations []string                   `json:"migrations"`
	SecretKey  []byte                     `json:"secretKey"`
	Tables     map[string]json.RawMessage `json:"tables"`
	DKIM       map[string]string          `json:"dkim"`
	State      map[string]json.RawMessage `json:"state"` // app_state keys worth keeping
	Counts     map[string]int             `json:"counts"`
}

func deriveKey(pass string, salt []byte) ([]byte, error) {
	return scrypt.Key([]byte(pass), salt, 1<<15, 8, 1, 32)
}

func sealBundle(plain []byte, pass string) ([]byte, error) {
	salt := make([]byte, 16)
	_, _ = rand.Read(salt)
	key, err := deriveKey(pass, salt)
	if err != nil {
		return nil, err
	}
	block, _ := aes.NewCipher(key)
	gcm, _ := cipher.NewGCM(block)
	nonce := make([]byte, gcm.NonceSize())
	_, _ = rand.Read(nonce)
	var out bytes.Buffer
	out.WriteString(configMagic)
	out.Write(salt)
	out.Write(nonce)
	out.Write(gcm.Seal(nil, nonce, plain, []byte(configMagic)))
	return out.Bytes(), nil
}

func openBundle(data []byte, pass string) ([]byte, error) {
	if !bytes.HasPrefix(data, []byte(configMagic)) || len(data) < len(configMagic)+16+12+16 {
		return nil, errors.New("This is not a TMAIL configuration file")
	}
	data = data[len(configMagic):]
	key, err := deriveKey(pass, data[:16])
	if err != nil {
		return nil, err
	}
	block, _ := aes.NewCipher(key)
	gcm, _ := cipher.NewGCM(block)
	plain, err := gcm.Open(nil, data[16:28], data[28:], []byte(configMagic))
	if err != nil {
		return nil, errors.New("Wrong passphrase, or the file is damaged")
	}
	return plain, nil
}

// POST /api/admin/config/export {passphrase, collectorHistory}
func (s *Server) adminConfigExport(w http.ResponseWriter, r *http.Request, sess *Session) {
	var in struct {
		Passphrase       string `json:"passphrase"`
		CollectorHistory bool   `json:"collectorHistory"`
	}
	if err := readJSON(r, &in); err != nil || len([]rune(in.Passphrase)) < 10 {
		writeErr(w, http.StatusBadRequest, "The passphrase must be at least 10 characters long")
		return
	}
	ctx := r.Context()
	b := configBundle{Version: 1, Created: time.Now().UTC(), Hostname: s.cfg.Hostname, SecretKey: s.secretKey(),
		Tables: map[string]json.RawMessage{}, DKIM: map[string]string{}, State: map[string]json.RawMessage{}, Counts: map[string]int{}}
	if rows, err := s.db.Query(ctx, `SELECT name FROM schema_migrations ORDER BY name`); err == nil {
		for rows.Next() {
			var n string
			if rows.Scan(&n) == nil {
				b.Migrations = append(b.Migrations, n)
			}
		}
		rows.Close()
	}
	tables := slices.Clone(configTables)
	if in.CollectorHistory {
		tables = append(tables, collectorHistoryTables...)
	}
	for _, t := range tables {
		var raw []byte
		var n int
		if err := s.db.QueryRow(ctx, `SELECT COALESCE(json_agg(x), '[]'), count(*) FROM `+pgx.Identifier{t}.Sanitize()+` x`).Scan(&raw, &n); err != nil {
			serverErr(w, "config export "+t, err)
			return
		}
		b.Tables[t], b.Counts[t] = raw, n
	}
	if rows, err := s.db.Query(ctx, `SELECT key, value FROM app_state WHERE key = ANY($1)`, []string{"mail_settings"}); err == nil {
		for rows.Next() {
			var k string
			var v []byte
			if rows.Scan(&k, &v) == nil {
				b.State[k] = v
			}
		}
		rows.Close()
	}
	files, _ := filepath.Glob(filepath.Join(s.cfg.DKIMDir, "*.key"))
	for _, f := range files {
		if data, err := os.ReadFile(f); err == nil {
			b.DKIM[filepath.Base(f)] = string(data)
		}
	}
	plain, err := json.Marshal(b)
	if err != nil {
		serverErr(w, "config export marshal", err)
		return
	}
	var gz bytes.Buffer
	zw := gzip.NewWriter(&gz)
	_, _ = zw.Write(plain)
	_ = zw.Close()
	out, err := sealBundle(gz.Bytes(), in.Passphrase)
	if err != nil {
		serverErr(w, "config export seal", err)
		return
	}
	s.auditAdmin(r, sess, "config.export", s.cfg.Hostname, map[string]any{"bytes": len(out), "collectorHistory": in.CollectorHistory})
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="tmail-config-%s-%s.tmailcfg"`, s.cfg.Hostname, time.Now().Format("2006-01-02")))
	_, _ = w.Write(out)
}

// readBundle decrypts and unpacks an uploaded configuration file.
func readBundle(data []byte, pass string) (*configBundle, error) {
	plain, err := openBundle(data, pass)
	if err != nil {
		return nil, err
	}
	zr, err := gzip.NewReader(bytes.NewReader(plain))
	if err != nil {
		return nil, errors.New("The file is damaged")
	}
	raw, err := io.ReadAll(io.LimitReader(zr, 2<<30))
	if err != nil {
		return nil, errors.New("The file is damaged")
	}
	var b configBundle
	if err := json.Unmarshal(raw, &b); err != nil || b.Version != 1 {
		return nil, errors.New("Unknown configuration format")
	}
	return &b, nil
}

// POST /api/admin/config/import (multipart: file, passphrase, mode=check|apply)
// "check" only reads the file and tells what is inside; "apply" replaces the configuration.
func (s *Server) adminConfigImport(w http.ResponseWriter, r *http.Request, sess *Session) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<30)
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		writeErr(w, http.StatusBadRequest, "Choose the configuration file")
		return
	}
	f, _, err := r.FormFile("file")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "Choose the configuration file")
		return
	}
	data, err := io.ReadAll(f)
	f.Close()
	if err != nil {
		writeErr(w, http.StatusBadRequest, "Could not read the file")
		return
	}
	b, err := readBundle(data, r.FormValue("passphrase"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	ctx := r.Context()
	var have []string
	if rows, err := s.db.Query(ctx, `SELECT name FROM schema_migrations`); err == nil {
		for rows.Next() {
			var n string
			if rows.Scan(&n) == nil {
				have = append(have, n)
			}
		}
		rows.Close()
	}
	var newer []string
	for _, m := range b.Migrations {
		if !slices.Contains(have, m) {
			newer = append(newer, m)
		}
	}
	summary := map[string]any{"hostname": b.Hostname, "created": b.Created, "counts": b.Counts, "dkim": len(b.DKIM)}
	if len(newer) > 0 {
		writeErr(w, http.StatusConflict, "The file comes from a newer TMAIL. Update this server first (git pull && ./install.sh)")
		return
	}
	if r.FormValue("mode") != "apply" {
		writeJSON(w, http.StatusOK, summary)
		return
	}
	if err := s.applyBundle(ctx, b); err != nil {
		slog.Error("config import", "err", err)
		writeErr(w, http.StatusBadRequest, "The configuration was not changed: "+err.Error())
		return
	}
	s.auditAdmin(r, sess, "config.import", b.Hostname, map[string]any{"counts": b.Counts})
	summary["ok"] = true
	writeJSON(w, http.StatusOK, summary)
}

func (s *Server) applyBundle(ctx context.Context, b *configBundle) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var tables []string
	for _, t := range append(slices.Clone(configTables), collectorHistoryTables...) {
		if _, ok := b.Tables[t]; ok {
			tables = append(tables, t)
		}
	}
	ids := make([]string, len(tables))
	for i, t := range tables {
		ids[i] = pgx.Identifier{t}.Sanitize()
	}
	// Everything that refers to the replaced rows (sessions, caches, logs) goes too.
	if _, err := tx.Exec(ctx, `TRUNCATE `+strings.Join(ids, ", ")+` CASCADE`); err != nil {
		return err
	}
	for _, t := range tables {
		var rows []map[string]json.RawMessage
		if err := json.Unmarshal(b.Tables[t], &rows); err != nil {
			return fmt.Errorf("%s: %w", t, err)
		}
		if len(rows) == 0 {
			continue
		}
		// Only columns both sides know: a column added since the export takes its default.
		var cols []string
		crows, err := tx.Query(ctx, `SELECT column_name FROM information_schema.columns WHERE table_schema='public' AND table_name=$1
			AND is_generated = 'NEVER'`, t)
		if err != nil {
			return err
		}
		hasID := false
		for crows.Next() {
			var c string
			if crows.Scan(&c) == nil {
				hasID = hasID || c == "id"
				if _, ok := rows[0][c]; ok {
					cols = append(cols, pgx.Identifier{c}.Sanitize())
				}
			}
		}
		crows.Close()
		if len(cols) == 0 {
			continue
		}
		list := strings.Join(cols, ", ")
		if _, err := tx.Exec(ctx, `INSERT INTO `+pgx.Identifier{t}.Sanitize()+` (`+list+`) SELECT `+list+
			` FROM json_populate_recordset(NULL::`+pgx.Identifier{t}.Sanitize()+`, $1::json)`, string(b.Tables[t])); err != nil {
			return fmt.Errorf("%s: %w", t, err)
		}
		// Sequences continue after the restored ids. (A failed statement would abort the whole
		// transaction, so only tables that have an id column are asked.)
		if hasID {
			if _, err := tx.Exec(ctx, `SELECT setval(seq, GREATEST((SELECT COALESCE(max(id), 0) FROM `+pgx.Identifier{t}.Sanitize()+`), 1))
				FROM (SELECT pg_get_serial_sequence($1, 'id') AS seq) x WHERE seq IS NOT NULL`, t); err != nil {
				return fmt.Errorf("%s: %w", t, err)
			}
		}
	}
	for k, v := range b.State {
		if _, err := tx.Exec(ctx, `INSERT INTO app_state(key, value) VALUES ($1, $2::text::jsonb) ON CONFLICT (key) DO UPDATE SET value=EXCLUDED.value, updated_at=now()`, k, string(v)); err != nil {
			return err
		}
	}
	// Secrets sealed with the old server's key are sealed again with this one's.
	if len(b.SecretKey) == 32 && !bytes.Equal(b.SecretKey, s.secretKey()) {
		if err := s.resealSecrets(ctx, tx, b.SecretKey); err != nil {
			return err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	for name, data := range b.DKIM {
		if name != filepath.Base(name) || !strings.HasSuffix(name, ".key") {
			continue
		}
		p := filepath.Join(s.cfg.DKIMDir, name)
		if err := os.WriteFile(p, []byte(data), 0o600); err == nil {
			_ = os.Chown(p, rspamdUID, rspamdUID)
		}
	}
	_ = s.writeSelectorMap(ctx)
	mailSettingsCache.Lock()
	mailSettingsCache.v = nil
	mailSettingsCache.Unlock()
	s.syncRelayConfig(ctx)
	return nil
}

func unsealWith(key, data []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(data) < gcm.NonceSize() {
		return nil, errors.New("bad secret")
	}
	return gcm.Open(nil, data[:gcm.NonceSize()], data[gcm.NonceSize():], nil)
}

func (s *Server) resealSecrets(ctx context.Context, tx pgx.Tx, oldKey []byte) error {
	reseal := func(data []byte) ([]byte, error) {
		if len(data) == 0 {
			return data, nil
		}
		plain, err := unsealWith(oldKey, data)
		if err != nil {
			return nil, err
		}
		return s.seal(string(plain))
	}
	for _, c := range []struct{ table, col string }{
		{"external_accounts", "password_enc"}, {"external_accounts", "smtp_password_enc"},
		{"ai_provider", "key_enc"}, {"ai_provider", "asr_key_enc"},
	} {
		var exists bool
		_ = tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema='public' AND table_name=$1 AND column_name=$2)`,
			c.table, c.col).Scan(&exists)
		if !exists {
			continue // a table or column this version does not have
		}
		rows, err := tx.Query(ctx, `SELECT ctid::text, `+c.col+` FROM `+c.table+` WHERE `+c.col+` IS NOT NULL`)
		if err != nil {
			return err
		}
		type item struct {
			ctid string
			data []byte
		}
		var items []item
		for rows.Next() {
			var it item
			if rows.Scan(&it.ctid, &it.data) == nil {
				items = append(items, it)
			}
		}
		rows.Close()
		for _, it := range items {
			nd, err := reseal(it.data)
			if err != nil {
				return fmt.Errorf("%s: a stored password could not be decrypted", c.table)
			}
			if _, err := tx.Exec(ctx, `UPDATE `+c.table+` SET `+c.col+`=$2 WHERE ctid=$1::tid`, it.ctid, nd); err != nil {
				return err
			}
		}
	}
	var raw []byte
	if tx.QueryRow(ctx, `SELECT value FROM app_state WHERE key='mail_settings'`).Scan(&raw) == nil {
		var m mailSettings
		if json.Unmarshal(raw, &m) == nil && len(m.Relay.PassEnc) > 0 {
			nd, err := reseal(m.Relay.PassEnc)
			if err != nil {
				return errors.New("the relay password could not be decrypted")
			}
			m.Relay.PassEnc = nd
			out, _ := json.Marshal(m)
			if _, err := tx.Exec(ctx, `UPDATE app_state SET value=$1::text::jsonb WHERE key='mail_settings'`, string(out)); err != nil {
				return err
			}
		}
	}
	return nil
}
