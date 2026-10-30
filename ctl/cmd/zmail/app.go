package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"io/fs"
	"math/big"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"

	"zmail/ctl/migrations"
)

type App struct{ db *pgxpool.Pool }

var (
	domainRe = regexp.MustCompile(`^([a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z]{2,63}$`)
	localRe  = regexp.MustCompile(`^[a-z0-9]([a-z0-9._+-]{0,62}[a-z0-9])?$`)
)

// rspamd runs as this uid/gid (see services/rspamd/Dockerfile) and must be able to read DKIM keys.
const rspamdUID = 11333

// ---------- migrations ----------

func (a *App) Migrate(ctx context.Context) error {
	if _, err := a.db.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		name text PRIMARY KEY, applied_at timestamptz NOT NULL DEFAULT now())`); err != nil {
		return err
	}
	files, err := fs.Glob(migrations.FS, "*.sql")
	if err != nil {
		return err
	}
	sort.Strings(files)
	for _, f := range files {
		var done bool
		if err := a.db.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE name=$1)`, f).Scan(&done); err != nil {
			return err
		}
		if done {
			continue
		}
		sql, _ := migrations.FS.ReadFile(f)
		err := pgx.BeginFunc(ctx, a.db, func(tx pgx.Tx) error {
			if _, err := tx.Exec(ctx, string(sql)); err != nil {
				return err
			}
			_, err := tx.Exec(ctx, `INSERT INTO schema_migrations(name) VALUES ($1)`, f)
			return err
		})
		if err != nil {
			return fmt.Errorf("migration %s: %w", f, err)
		}
		fmt.Println("applied migration", f)
	}
	fmt.Println("database schema is up to date")
	return nil
}

// ---------- domains ----------

func (a *App) DomainCmd(ctx context.Context, args []string) error {
	pos, _ := splitArgs(args)
	if err := need(pos, 1, "domain add|list"); err != nil {
		return err
	}
	switch pos[0] {
	case "add":
		if err := need(pos, 2, "domain name"); err != nil {
			return err
		}
		name := strings.ToLower(strings.TrimSpace(pos[1]))
		if !domainRe.MatchString(name) {
			return fmt.Errorf("invalid domain name %q", name)
		}
		if _, err := a.db.Exec(ctx, `INSERT INTO domains(name) VALUES ($1)`, name); err != nil {
			return friendly(err, "domain "+name+" already exists")
		}
		fmt.Printf("domain %s added. Next: dkim gen %s, then dns %s\n", name, name, name)
	case "list":
		rows, err := a.db.Query(ctx, `
			SELECT d.name, d.active,
			       (SELECT count(*) FROM mailboxes m WHERE m.domain_id = d.id),
			       (SELECT count(*) FROM aliases al WHERE al.domain_id = d.id)
			FROM domains d ORDER BY d.name`)
		if err != nil {
			return err
		}
		defer rows.Close()
		w := tab()
		fmt.Fprintln(w, "DOMAIN\tACTIVE\tMAILBOXES\tALIASES")
		for rows.Next() {
			var n string
			var act bool
			var mb, al int
			if err := rows.Scan(&n, &act, &mb, &al); err != nil {
				return err
			}
			fmt.Fprintf(w, "%s\t%s\t%d\t%d\n", n, yes(act), mb, al)
		}
		return w.Flush()
	default:
		return fmt.Errorf("domain: unknown operation %q", pos[0])
	}
	return nil
}

// ---------- mailboxes ----------

func (a *App) MailboxCmd(ctx context.Context, args []string) error {
	pos, flags := splitArgs(args)
	if err := need(pos, 1, "mailbox add|list|passwd|quota|disable|enable"); err != nil {
		return err
	}
	switch pos[0] {
	case "add":
		if err := need(pos, 2, "address"); err != nil {
			return err
		}
		local, domain, err := splitAddress(pos[1])
		if err != nil {
			return err
		}
		quota := int64(5 << 30)
		if q, ok := flags["quota"]; ok {
			if quota, err = parseSize(q); err != nil {
				return err
			}
		}
		pass, generated := flags["password"], false
		if pass == "" {
			pass, generated = genPassword(16), true
		} else if len(pass) < 10 {
			return errors.New("password too short: at least 10 characters required")
		}
		hash, err := hashPassword(pass)
		if err != nil {
			return err
		}
		tag, err := a.db.Exec(ctx, `
			INSERT INTO mailboxes(domain_id, local_part, address, display_name, password_hash, quota_bytes)
			SELECT id, $2, $2 || '@' || name, $3, $4, $5 FROM domains WHERE name = $1`,
			domain, local, flags["name"], hash, quota)
		if err != nil {
			return friendly(err, "address "+local+"@"+domain+" is already taken by a mailbox")
		}
		if tag.RowsAffected() == 0 {
			return fmt.Errorf("domain %s not found; first run: domain add %s", domain, domain)
		}
		var clash bool
		_ = a.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM aliases WHERE source=$1)`, local+"@"+domain).Scan(&clash)
		fmt.Printf("mailbox %s@%s created, quota %s\n", local, domain, humanSize(quota))
		if generated {
			fmt.Printf("password: %s\n(save it: it will not be shown again)\n", pass)
		}
		if clash {
			fmt.Println("warning: an alias exists for this address; mail will follow the alias")
		}
	case "list":
		q := `SELECT m.address, m.display_name, m.quota_bytes, m.active FROM mailboxes m JOIN domains d ON d.id=m.domain_id`
		var rows pgx.Rows
		var err error
		if len(pos) > 1 {
			rows, err = a.db.Query(ctx, q+` WHERE d.name=$1 ORDER BY m.address`, strings.ToLower(pos[1]))
		} else {
			rows, err = a.db.Query(ctx, q+` ORDER BY m.address`)
		}
		if err != nil {
			return err
		}
		defer rows.Close()
		w := tab()
		fmt.Fprintln(w, "ADDRESS\tNAME\tQUOTA\tACTIVE")
		for rows.Next() {
			var addr, name string
			var quota int64
			var act bool
			if err := rows.Scan(&addr, &name, &quota, &act); err != nil {
				return err
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", addr, name, humanSize(quota), yes(act))
		}
		return w.Flush()
	case "passwd":
		if err := need(pos, 2, "address"); err != nil {
			return err
		}
		pass, generated := flags["password"], false
		if pass == "" {
			pass, generated = genPassword(16), true
		} else if len(pass) < 10 {
			return errors.New("password too short: at least 10 characters required")
		}
		hash, err := hashPassword(pass)
		if err != nil {
			return err
		}
		if err := a.updateMailbox(ctx, pos[1], `password_hash=$2`, hash); err != nil {
			return err
		}
		fmt.Println("password changed")
		if generated {
			fmt.Printf("new password: %s\n", pass)
		}
	case "quota":
		if err := need(pos, 3, "address and size"); err != nil {
			return err
		}
		q, err := parseSize(pos[2])
		if err != nil {
			return err
		}
		if err := a.updateMailbox(ctx, pos[1], `quota_bytes=$2`, q); err != nil {
			return err
		}
		fmt.Println("quota:", humanSize(q))
	case "disable", "enable":
		if err := need(pos, 2, "address"); err != nil {
			return err
		}
		if err := a.updateMailbox(ctx, pos[1], `active=$2`, pos[0] == "enable"); err != nil {
			return err
		}
		fmt.Println("done")
	default:
		return fmt.Errorf("mailbox: unknown operation %q", pos[0])
	}
	return nil
}

func (a *App) updateMailbox(ctx context.Context, addr, set string, val any) error {
	tag, err := a.db.Exec(ctx, `UPDATE mailboxes SET `+set+`, updated_at=now() WHERE address=$1`, strings.ToLower(addr), val)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("mailbox %s not found", addr)
	}
	return nil
}

// ---------- aliases ----------

func (a *App) AliasCmd(ctx context.Context, args []string) error {
	pos, _ := splitArgs(args)
	if err := need(pos, 1, "alias add|list|del"); err != nil {
		return err
	}
	switch pos[0] {
	case "add":
		if err := need(pos, 3, "address and recipients"); err != nil {
			return err
		}
		local, domain, err := splitAddress(pos[1])
		if err != nil {
			return err
		}
		var dests []string
		for _, d := range strings.Split(pos[2], ",") {
			d = strings.ToLower(strings.TrimSpace(d))
			if d == "" {
				continue
			}
			if _, _, err := splitAddress(d); err != nil {
				return err
			}
			dests = append(dests, d)
		}
		if len(dests) == 0 {
			return errors.New("no recipients given")
		}
		src := local + "@" + domain
		tag, err := a.db.Exec(ctx, `
			INSERT INTO aliases(domain_id, source, destinations) SELECT id, $2, $3 FROM domains WHERE name=$1
			ON CONFLICT (source) DO UPDATE SET destinations = EXCLUDED.destinations, active = true`,
			domain, src, dests)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return fmt.Errorf("domain %s not found", domain)
		}
		fmt.Printf("%s → %s\n", src, strings.Join(dests, ", "))
	case "list":
		q := `SELECT a.source, array_to_string(a.destinations, ', '), a.active FROM aliases a JOIN domains d ON d.id=a.domain_id`
		var rows pgx.Rows
		var err error
		if len(pos) > 1 {
			rows, err = a.db.Query(ctx, q+` WHERE d.name=$1 ORDER BY a.source`, strings.ToLower(pos[1]))
		} else {
			rows, err = a.db.Query(ctx, q+` ORDER BY a.source`)
		}
		if err != nil {
			return err
		}
		defer rows.Close()
		w := tab()
		fmt.Fprintln(w, "ALIAS\tDESTINATIONS\tACTIVE")
		for rows.Next() {
			var s, d string
			var act bool
			if err := rows.Scan(&s, &d, &act); err != nil {
				return err
			}
			fmt.Fprintf(w, "%s\t%s\t%s\n", s, d, yes(act))
		}
		return w.Flush()
	case "del":
		if err := need(pos, 2, "address"); err != nil {
			return err
		}
		tag, err := a.db.Exec(ctx, `DELETE FROM aliases WHERE source=$1`, strings.ToLower(pos[1]))
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return fmt.Errorf("alias %s not found", pos[1])
		}
		fmt.Println("deleted")
	default:
		return fmt.Errorf("alias: unknown operation %q", pos[0])
	}
	return nil
}

// ---------- webmail users and their mailboxes ----------

var loginRe = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{1,62}$`)

func (a *App) UserCmd(ctx context.Context, args []string) error {
	pos, flags := splitArgs(args)
	if err := need(pos, 1, "user add|list|passwd|disable|enable"); err != nil {
		return err
	}
	switch pos[0] {
	case "add":
		if err := need(pos, 2, "login"); err != nil {
			return err
		}
		login := strings.ToLower(strings.TrimSpace(pos[1]))
		if !loginRe.MatchString(login) {
			return fmt.Errorf("login %q: use latin letters, digits, dot, hyphen or underscore, at least 2 characters", login)
		}
		pass, generated, err := passwordFlag(flags)
		if err != nil {
			return err
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(pass), 12)
		if err != nil {
			return err
		}
		admin := flags["admin"] == "yes"
		if _, err := a.db.Exec(ctx, `INSERT INTO users(login, display_name, password_hash, is_admin) VALUES ($1,$2,$3,$4)`,
			login, flags["name"], string(hash), admin); err != nil {
			return friendly(err, "login "+login+" is already taken")
		}
		fmt.Printf("user %s created%s\n", login, map[bool]string{true: " (administrator)", false: ""}[admin])
		if generated {
			fmt.Printf("password: %s\n(save it: it will not be shown again)\n", pass)
		}
		if mbs := flags["mailboxes"]; mbs != "" {
			for i, mb := range strings.Split(mbs, ",") {
				// The first mailbox in the list is the user's own (personal) one.
				if err := a.grant(ctx, login, strings.TrimSpace(mb), true, i, i == 0); err != nil {
					return err
				}
			}
		}
	case "list":
		rows, err := a.db.Query(ctx, `
			SELECT u.login, u.display_name, u.is_admin, u.active,
			       coalesce(string_agg(m.address || CASE WHEN um.can_send THEN '' ELSE ' (read-only)' END, ', ' ORDER BY um.position, m.address), '')
			FROM users u
			LEFT JOIN user_mailboxes um ON um.user_id = u.id
			LEFT JOIN mailboxes m ON m.id = um.mailbox_id
			GROUP BY u.id ORDER BY u.login`)
		if err != nil {
			return err
		}
		defer rows.Close()
		w := tab()
		fmt.Fprintln(w, "LOGIN\tNAME\tADMIN\tACTIVE\tMAILBOXES")
		for rows.Next() {
			var l, n, mbs string
			var adm, act bool
			if err := rows.Scan(&l, &n, &adm, &act, &mbs); err != nil {
				return err
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", l, n, yes(adm), yes(act), mbs)
		}
		return w.Flush()
	case "passwd":
		if err := need(pos, 2, "login"); err != nil {
			return err
		}
		pass, generated, err := passwordFlag(flags)
		if err != nil {
			return err
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(pass), 12)
		if err != nil {
			return err
		}
		if err := a.updateUser(ctx, pos[1], `password_hash=$2`, string(hash)); err != nil {
			return err
		}
		// Sign the user out everywhere.
		_, _ = a.db.Exec(ctx, `DELETE FROM sessions WHERE user_id=(SELECT id FROM users WHERE login=$1)`, strings.ToLower(pos[1]))
		fmt.Println("password changed, all user sessions signed out")
		if generated {
			fmt.Printf("new password: %s\n", pass)
		}
	case "disable", "enable":
		if err := need(pos, 2, "login"); err != nil {
			return err
		}
		if err := a.updateUser(ctx, pos[1], `active=$2`, pos[0] == "enable"); err != nil {
			return err
		}
		if pos[0] == "disable" {
			_, _ = a.db.Exec(ctx, `DELETE FROM sessions WHERE user_id=(SELECT id FROM users WHERE login=$1)`, strings.ToLower(pos[1]))
		}
		fmt.Println("done")
	default:
		return fmt.Errorf("user: unknown operation %q", pos[0])
	}
	return nil
}

func passwordFlag(flags map[string]string) (pass string, generated bool, err error) {
	pass = flags["password"]
	if pass == "" {
		return genPassword(16), true, nil
	}
	if len(pass) < 10 {
		return "", false, errors.New("password too short: at least 10 characters required")
	}
	return pass, false, nil
}

func (a *App) updateUser(ctx context.Context, login, set string, val any) error {
	tag, err := a.db.Exec(ctx, `UPDATE users SET `+set+`, updated_at=now() WHERE login=$1`, strings.ToLower(login), val)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("user %s not found", login)
	}
	return nil
}

func (a *App) grant(ctx context.Context, login, mailbox string, canSend bool, position int, personal bool) error {
	login, mailbox = strings.ToLower(login), strings.ToLower(mailbox)
	tag, err := a.db.Exec(ctx, `
		INSERT INTO user_mailboxes(user_id, mailbox_id, can_send, position, personal)
		SELECT u.id, m.id, $3, $4, $5 FROM users u, mailboxes m WHERE u.login=$1 AND m.address=$2
		ON CONFLICT (user_id, mailbox_id) DO UPDATE SET can_send = EXCLUDED.can_send, personal = EXCLUDED.personal`,
		login, mailbox, canSend || personal, position, personal)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("user %s or mailbox %s not found", login, mailbox)
	}
	mode := "read and send"
	if !canSend {
		mode = "read-only"
	}
	if personal {
		mode += ", personal mailbox"
	}
	fmt.Printf("%s → %s (%s)\n", login, mailbox, mode)
	return nil
}

func (a *App) AccessCmd(ctx context.Context, args []string) error {
	pos, flags := splitArgs(args)
	if err := need(pos, 1, "access grant|revoke|list"); err != nil {
		return err
	}
	switch pos[0] {
	case "grant":
		if err := need(pos, 3, "login and mailbox"); err != nil {
			return err
		}
		var n int
		_ = a.db.QueryRow(ctx, `SELECT count(*) FROM user_mailboxes um JOIN users u ON u.id=um.user_id WHERE u.login=$1`, strings.ToLower(pos[1])).Scan(&n)
		return a.grant(ctx, pos[1], pos[2], flags["send"] != "no", n, flags["personal"] == "yes")
	case "revoke":
		if err := need(pos, 3, "login and mailbox"); err != nil {
			return err
		}
		tag, err := a.db.Exec(ctx, `DELETE FROM user_mailboxes
			WHERE user_id=(SELECT id FROM users WHERE login=$1) AND mailbox_id=(SELECT id FROM mailboxes WHERE address=$2)`,
			strings.ToLower(pos[1]), strings.ToLower(pos[2]))
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return fmt.Errorf("%s has no access to %s", pos[1], pos[2])
		}
		fmt.Println("access revoked")
	case "list":
		q := `SELECT u.login, m.address, um.can_send FROM user_mailboxes um
			JOIN users u ON u.id=um.user_id JOIN mailboxes m ON m.id=um.mailbox_id`
		var rows pgx.Rows
		var err error
		if len(pos) > 1 {
			rows, err = a.db.Query(ctx, q+` WHERE u.login=$1 ORDER BY um.position, m.address`, strings.ToLower(pos[1]))
		} else {
			rows, err = a.db.Query(ctx, q+` ORDER BY u.login, um.position, m.address`)
		}
		if err != nil {
			return err
		}
		defer rows.Close()
		w := tab()
		fmt.Fprintln(w, "LOGIN\tMAILBOX\tSEND")
		for rows.Next() {
			var u, m string
			var send bool
			if err := rows.Scan(&u, &m, &send); err != nil {
				return err
			}
			fmt.Fprintf(w, "%s\t%s\t%s\n", u, m, yes(send))
		}
		return w.Flush()
	default:
		return fmt.Errorf("access: unknown operation %q", pos[0])
	}
	return nil
}

// ---------- DKIM ----------

func (a *App) DkimCmd(ctx context.Context, args []string) error {
	pos, flags := splitArgs(args)
	if err := need(pos, 2, "dkim gen <domain>"); err != nil {
		return err
	}
	if pos[0] != "gen" {
		return fmt.Errorf("dkim: unknown operation %q", pos[0])
	}
	domain := strings.ToLower(pos[1])
	selector := flags["selector"]
	if selector == "" {
		selector = envOr("DKIM_SELECTOR", "z2026")
	}
	dir := envOr("DKIM_DIR", "/dkim")

	var domainID int64
	if err := a.db.QueryRow(ctx, `SELECT id FROM domains WHERE name=$1`, domain).Scan(&domainID); err != nil {
		return fmt.Errorf("domain %s not found", domain)
	}
	keyPath := filepath.Join(dir, domain+"."+selector+".key")
	if _, err := os.Stat(keyPath); err == nil {
		return fmt.Errorf("key %s already exists; to rotate the key, use a different --selector", keyPath)
	}

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return err
	}
	pub, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		return err
	}
	pubB64 := base64.StdEncoding.EncodeToString(pub)
	privPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})

	if err := os.WriteFile(keyPath, privPEM, 0o600); err != nil {
		return err
	}
	_ = os.Chown(keyPath, rspamdUID, rspamdUID)

	_, err = a.db.Exec(ctx, `
		WITH off AS (UPDATE dkim_keys SET active=false WHERE domain_id=$1)
		INSERT INTO dkim_keys(domain_id, selector, public_key) VALUES ($1, $2, $3)`, domainID, selector, pubB64)
	if err != nil {
		os.Remove(keyPath)
		return err
	}
	if err := a.writeSelectorMap(ctx, dir); err != nil {
		return err
	}
	fmt.Printf("DKIM key for %s created (selector %s). DNS record: zmail dns %s\n", domain, selector, domain)
	return nil
}

// selectors.map tells rspamd which selector to sign each domain with.
func (a *App) writeSelectorMap(ctx context.Context, dir string) error {
	rows, err := a.db.Query(ctx, `SELECT d.name, k.selector FROM dkim_keys k JOIN domains d ON d.id=k.domain_id WHERE k.active ORDER BY d.name`)
	if err != nil {
		return err
	}
	defer rows.Close()
	var b strings.Builder
	for rows.Next() {
		var d, s string
		if err := rows.Scan(&d, &s); err != nil {
			return err
		}
		fmt.Fprintf(&b, "%s %s\n", d, s)
	}
	p := filepath.Join(dir, "selectors.map")
	if err := os.WriteFile(p+".tmp", []byte(b.String()), 0o644); err != nil {
		return err
	}
	return os.Rename(p+".tmp", p)
}

// ---------- DNS ----------

func (a *App) DNSCmd(ctx context.Context, args []string) error {
	pos, _ := splitArgs(args)
	if err := need(pos, 1, "domain"); err != nil {
		return err
	}
	domain := strings.ToLower(pos[0])
	host := envOr("MAIL_HOSTNAME", "mail."+domain)
	var exists bool
	if err := a.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM domains WHERE name=$1)`, domain).Scan(&exists); err != nil || !exists {
		return fmt.Errorf("domain %s not found", domain)
	}
	var selector, pub string
	err := a.db.QueryRow(ctx, `SELECT k.selector, k.public_key FROM dkim_keys k JOIN domains d ON d.id=k.domain_id
		WHERE d.name=$1 AND k.active ORDER BY k.created_at DESC LIMIT 1`, domain).Scan(&selector, &pub)
	if errors.Is(err, pgx.ErrNoRows) {
		selector = ""
	} else if err != nil {
		return err
	}

	fmt.Printf("DNS records for %s (mail server %s)\n\n", domain, host)
	w := tab()
	fmt.Fprintln(w, "TYPE\tNAME\tVALUE")
	fmt.Fprintf(w, "MX\t@\t10 %s.\n", host)
	fmt.Fprintf(w, "TXT\t@\tv=spf1 mx ~all\n")
	fmt.Fprintf(w, "TXT\t_dmarc\tv=DMARC1; p=none; rua=mailto:postmaster@%s; adkim=s; aspf=r\n", domain)
	fmt.Fprintf(w, "SRV\t_submissions._tcp\t0 1 465 %s.\n", host)
	fmt.Fprintf(w, "SRV\t_submission._tcp\t0 1 587 %s.\n", host)
	fmt.Fprintf(w, "SRV\t_imaps._tcp\t0 1 993 %s.\n", host)
	fmt.Fprintf(w, "SRV\t_pop3s._tcp\t0 1 995 %s.\n", host)
	w.Flush()
	if selector == "" {
		fmt.Printf("\nNo DKIM key yet: zmail dkim gen %s\n", domain)
	} else {
		fmt.Printf("\nTXT  %s._domainkey\n", selector)
		fmt.Printf("v=DKIM1; k=rsa; p=%s\n", pub)
	}
	fmt.Printf("\nPTR (set at the server hosting provider, not the registrar): server IP → %s\n", host)
	return nil
}

// ---------- helpers ----------

func splitAddress(addr string) (local, domain string, err error) {
	addr = strings.ToLower(strings.TrimSpace(addr))
	local, domain, ok := strings.Cut(addr, "@")
	if !ok || !localRe.MatchString(local) || !domainRe.MatchString(domain) {
		return "", "", fmt.Errorf("invalid address %q", addr)
	}
	return local, domain, nil
}

// Dovecot verifies these with its BLF-CRYPT scheme.
func hashPassword(p string) (string, error) {
	h, err := bcrypt.GenerateFromPassword([]byte(p), 12)
	if err != nil {
		return "", err
	}
	return "{BLF-CRYPT}" + string(h), nil
}

// Avoids look-alike characters so passwords can be dictated or retyped.
func genPassword(n int) string {
	const alphabet = "abcdefghijkmnpqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	b := make([]byte, n)
	for i := range b {
		v, _ := rand.Int(rand.Reader, big.NewInt(int64(len(alphabet))))
		b[i] = alphabet[v.Int64()]
	}
	return string(b)
}

func parseSize(s string) (int64, error) {
	s = strings.ToUpper(strings.TrimSpace(s))
	mult := int64(1)
	switch {
	case strings.HasSuffix(s, "G"):
		mult, s = 1<<30, strings.TrimSuffix(s, "G")
	case strings.HasSuffix(s, "M"):
		mult, s = 1<<20, strings.TrimSuffix(s, "M")
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil || n < 0 {
		return 0, fmt.Errorf("invalid size %q (examples: 500M, 5G, 0)", s)
	}
	return n * mult, nil
}

func humanSize(b int64) string {
	switch {
	case b == 0:
		return "unlimited"
	case b%(1<<30) == 0:
		return fmt.Sprintf("%d GB", b>>30)
	default:
		return fmt.Sprintf("%d MB", b>>20)
	}
}

func friendly(err error, uniqueMsg string) error {
	if strings.Contains(err.Error(), "SQLSTATE 23505") {
		return errors.New(uniqueMsg)
	}
	return err
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func yes(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

func tab() *tabwriter.Writer { return tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0) }
