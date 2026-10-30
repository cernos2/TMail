package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/mail"
	"net/smtp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap/client"
)

// Mail collector: brings mail from external mailboxes into ours. IMAP keeps folders (Sent, Drafts,
// Trash, Junk and custom ones by name) and the read/answered/flagged marks; POP3 only has the
// Inbox and no marks. The first run imports the history, later runs only what is new.

type extAccount struct {
	ID           int64      `json:"id"`
	Mailbox      string     `json:"mailbox"`
	Address      string     `json:"address"`
	DisplayName  string     `json:"displayName"`
	Protocol     string     `json:"protocol"`
	Host         string     `json:"host"`
	Port         int        `json:"port"`
	Security     string     `json:"security"`
	Username     string     `json:"username"`
	Password     string     `json:"password,omitempty"` // only on input
	SMTPHost     string     `json:"smtpHost"`
	SMTPPort     int        `json:"smtpPort"`
	SMTPSecurity string     `json:"smtpSecurity"`
	SMTPUsername string     `json:"smtpUsername"`
	SMTPPassword string     `json:"smtpPassword,omitempty"` // only on input
	HasSMTPPass  bool       `json:"hasSmtpPassword"`
	InboxOnly    bool       `json:"inboxOnly"`
	Since        string     `json:"since"`
	DeleteAfter  bool       `json:"deleteAfter"`
	Enabled      bool       `json:"enabled"`
	Status       string     `json:"status"`
	LastError    string     `json:"lastError"`
	LastRun      *time.Time `json:"lastRun,omitempty"`
	Imported     int        `json:"imported"`
	Interval     int        `json:"interval"` // minutes between checks
	TwoWay       bool       `json:"twoWay"`   // read/flag/delete/archive synced both ways
	KeepDays     int        `json:"keepDays"` // with DeleteAfter: letters younger than this stay on the source server
	Progress     string     `json:"progress,omitempty"`
	Remaining    int64      `json:"remaining"`      // letters still waiting on the source (last pass)
	RemainBytes  int64      `json:"remainingBytes"` // and their size

	passEnc, smtpPassEnc []byte
}

const extCols = `id, mailbox, address, display_name, protocol, host, port, security, username, password_enc, smtp_host, smtp_port, smtp_security,
	smtp_username, smtp_password_enc, inbox_only, COALESCE(to_char(since, 'YYYY-MM-DD'), ''), delete_after, enabled, status, last_error, last_run, imported, interval_minutes, keep_days, two_way`

func scanExt(row interface{ Scan(...any) error }) (extAccount, error) {
	var a extAccount
	err := row.Scan(&a.ID, &a.Mailbox, &a.Address, &a.DisplayName, &a.Protocol, &a.Host, &a.Port, &a.Security, &a.Username, &a.passEnc,
		&a.SMTPHost, &a.SMTPPort, &a.SMTPSecurity, &a.SMTPUsername, &a.smtpPassEnc, &a.InboxOnly, &a.Since, &a.DeleteAfter, &a.Enabled,
		&a.Status, &a.LastError, &a.LastRun, &a.Imported, &a.Interval, &a.KeepDays, &a.TwoWay)
	a.HasSMTPPass = len(a.smtpPassEnc) > 0
	return a, err
}

// live progress of running syncs, shown in the admin list
var (
	extProgress   = map[int64]string{}
	extRunning    = map[int64]bool{}
	extProgressMu sync.Mutex
)

func setProgress(id int64, p string) {
	extProgressMu.Lock()
	extProgress[id] = p
	extProgressMu.Unlock()
}

// ---------- connections ----------

func dialRemote(host string, port int, security string) (net.Conn, error) {
	addr := net.JoinHostPort(host, strconv.Itoa(port))
	d := &net.Dialer{Timeout: 20 * time.Second, Control: denyPrivate} // external servers only, never our own internal ports
	// IPv4 first: from this server the IPv6 route to some providers (Gmail) is much slower.
	// COLLECTOR_NETWORK=tcp lets the system choose.
	nets := []string{env("COLLECTOR_NETWORK", "tcp4"), "tcp"}
	var err error
	for _, n := range nets {
		var c net.Conn
		if security == "ssl" {
			c, err = tls.DialWithDialer(d, n, addr, &tls.Config{ServerName: host})
		} else {
			c, err = d.Dial(n, addr)
		}
		if err == nil {
			return c, nil
		}
	}
	return nil, err
}

func remoteIMAP(a extAccount, pass string) (*client.Client, error) {
	conn, err := dialRemote(a.Host, a.Port, a.Security)
	if err != nil {
		return nil, err
	}
	c, err := client.New(conn)
	if err != nil {
		return nil, err
	}
	c.Timeout = 10 * time.Minute // one batch of up to ~8 MB, even on a slow route
	if a.Security == "starttls" {
		if err := c.StartTLS(&tls.Config{ServerName: a.Host}); err != nil {
			c.Logout()
			return nil, err
		}
	}
	if err := c.Login(a.Username, pass); err != nil {
		c.Logout()
		return nil, fmt.Errorf("login failed: %w", err)
	}
	return c, nil
}

// pop3 is a minimal POP3 client: enough to list and fetch letters.
type pop3 struct {
	conn net.Conn
	r    *bufio.Reader
}

func remotePOP3(a extAccount, pass string) (*pop3, error) {
	conn, err := dialRemote(a.Host, a.Port, a.Security)
	if err != nil {
		return nil, err
	}
	p := &pop3{conn: conn, r: bufio.NewReader(conn)}
	_ = conn.SetDeadline(time.Now().Add(10 * time.Minute))
	if _, err := p.line(); err != nil {
		conn.Close()
		return nil, err
	}
	if a.Security == "starttls" {
		if _, err := p.cmd("STLS"); err != nil {
			conn.Close()
			return nil, err
		}
		tc := tls.Client(conn, &tls.Config{ServerName: a.Host})
		p.conn, p.r = tc, bufio.NewReader(tc)
	}
	if _, err := p.cmd("USER " + a.Username); err != nil {
		p.conn.Close()
		return nil, err
	}
	if _, err := p.cmd("PASS " + pass); err != nil {
		p.conn.Close()
		return nil, fmt.Errorf("login failed: %w", err)
	}
	return p, nil
}

func (p *pop3) line() (string, error) {
	l, err := p.r.ReadString('\n')
	if err != nil {
		return "", err
	}
	l = strings.TrimRight(l, "\r\n")
	if strings.HasPrefix(l, "-ERR") {
		return "", errors.New(strings.TrimSpace(strings.TrimPrefix(l, "-ERR")))
	}
	return l, nil
}

func (p *pop3) cmd(c string) (string, error) {
	if _, err := fmt.Fprintf(p.conn, "%s\r\n", c); err != nil {
		return "", err
	}
	return p.line()
}

// multi reads a dot-terminated multi-line answer.
func (p *pop3) multi() ([]byte, error) {
	var b bytes.Buffer
	for {
		l, err := p.r.ReadString('\n')
		if err != nil {
			return nil, err
		}
		if l == ".\r\n" || l == ".\n" {
			return b.Bytes(), nil
		}
		if strings.HasPrefix(l, "..") {
			l = l[1:]
		}
		b.WriteString(l)
	}
}

func (p *pop3) close() { _, _ = p.cmd("QUIT"); p.conn.Close() }

// ---------- worker ----------

func (s *Server) runCollector(ctx context.Context) {
	// Every minute: accounts whose own interval has passed since the last check.
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			rows, err := s.db.Query(ctx, `SELECT `+extCols+` FROM external_accounts
				WHERE enabled AND mailbox IN (SELECT address FROM mailboxes WHERE active AND deleted_at IS NULL)
				  AND (last_run IS NULL OR last_run <= now() - make_interval(mins => interval_minutes) + interval '20 seconds') ORDER BY id`)
			if err != nil {
				continue
			}
			var list []extAccount
			for rows.Next() {
				if a, err := scanExt(rows); err == nil {
					list = append(list, a)
				}
			}
			rows.Close()
			// Each account on its own: a long Gmail import must not hold up the other mailboxes
			// (their letters waited up to 50 minutes behind it). syncExternal skips an account
			// whose previous pass is still running.
			for _, a := range list {
				go s.syncExternal(ctx, a)
			}
		}
	}
}

// syncExternal runs one collection pass; it never runs twice at once for the same account.
func (s *Server) syncExternal(ctx context.Context, a extAccount) {
	extProgressMu.Lock()
	if extRunning[a.ID] {
		extProgressMu.Unlock()
		return
	}
	extRunning[a.ID] = true
	extProgressMu.Unlock()
	defer func() {
		extProgressMu.Lock()
		delete(extRunning, a.ID)
		delete(extProgress, a.ID)
		extProgressMu.Unlock()
	}()
	pass, err := s.unseal(a.passEnc)
	if err == nil {
		if a.Status == "new" {
			_, _ = s.db.Exec(ctx, `UPDATE external_accounts SET status='importing' WHERE id=$1`, a.ID)
		}
		var n int
		if a.Protocol == "pop3" {
			n, err = s.syncPOP3(ctx, a, pass)
		} else {
			n, err = s.syncIMAP(ctx, a, pass)
		}
		if n > 0 && a.Protocol == "pop3" { // IMAP counts after every batch (copyFolder)
			_, _ = s.db.Exec(ctx, `UPDATE external_accounts SET imported = imported + $2 WHERE id=$1`, a.ID, n)
		}
		if n > 0 {
			s.hub.countsChanged(a.Mailbox)
		}
	}
	status, msg := "ok", ""
	if err != nil {
		status, msg = "error", truncate(err.Error(), 300)+authHint(a.Host+" "+a.SMTPHost, err)
		log.Printf("collector %s → %s: %v", a.Address, a.Mailbox, err)
	}
	_, _ = s.db.Exec(ctx, `UPDATE external_accounts SET status=$2, last_error=$3, last_run=now() WHERE id=$1`, a.ID, status, msg)
}

const extPerRun = 1000 // letters per pass; a big history continues on the next passes

func (s *Server) syncIMAP(ctx context.Context, a extAccount, pass string) (int, error) {
	rc, err := remoteIMAP(a, pass)
	if err != nil {
		return 0, err
	}
	defer rc.Logout()
	local, err := s.dialAs(a.Mailbox)
	if err != nil {
		return 0, err
	}
	defer func() { local.Logout() }() // the variable: it may be replaced by a new connection
	// Our side waits while the source sends a letter, sometimes for minutes: a long timeout, not the
	// usual 2 minutes, or the idle connection is dropped mid-import. Never none: an APPEND left
	// without an answer would stop the import for good (it hung for 9 hours once).
	local.Timeout = 15 * time.Minute

	ch := make(chan *imap.MailboxInfo, 64)
	done := make(chan error, 1)
	go func() { done <- rc.List("", "*", ch) }()
	var boxes []*imap.MailboxInfo
	for mb := range ch {
		boxes = append(boxes, mb)
	}
	if err := <-done; err != nil {
		return 0, err
	}
	// Inbox first, then the rest.
	rf := &remoteFolders{boxes: boxes}
	var twoWay bool
	_ = s.db.QueryRow(ctx, `SELECT two_way FROM external_accounts WHERE id=$1`, a.ID).Scan(&twoWay)
	// A Gmail that collected other mailboxes keeps their letters under a label named after the
	// address (info@firma.ru). When TMAIL has a mailbox with that address, those letters go into
	// it — before the Inbox, so a letter that is both there and in the Inbox lands only in the
	// mailbox it was sent to (the rest are skipped as already fetched, by Message-ID). Letters sent
	// from such an address (Gmail "send mail as") go to that mailbox's Sent.
	routes := s.labelRoutes(ctx, a, boxes)
	targets := map[string]*client.Client{}
	defer func() {
		for _, c := range targets {
			c.Logout()
		}
	}()
	lastUse := map[*client.Client]time.Time{}
	targetConn := func(addr string) (*client.Client, error) {
		if c := targets[addr]; c != nil {
			if time.Since(lastUse[c]) < time.Minute {
				return c, nil
			}
			if c2, err := s.aliveConn(c, addr); err == nil {
				targets[addr], lastUse[c2] = c2, time.Now()
				return c2, nil
			}
		}
		c, err := s.dialAs(addr)
		if err != nil {
			return nil, err
		}
		c.Timeout = 15 * time.Minute
		targets[addr], lastUse[c] = c, time.Now()
		return c, nil
	}
	var routed, inbox, rest, archive []*imap.MailboxInfo
	for _, mb := range boxes {
		switch {
		case routes[mb.Name] != "":
			routed = append(routed, mb)
		case strings.EqualFold(mb.Name, "INBOX"):
			inbox = append(inbox, mb)
		case a.InboxOnly:
		case hasAttr(mb.Attributes, imap.AllAttr):
			archive = append(archive, mb) // Gmail's "All mail": last, only what no other folder had (archived letters)
		default:
			rest = append(rest, mb)
		}
	}
	ordered := append(append(append(routed, inbox...), rest...), archive...)
	sendersRoute := func(raw []byte) (*client.Client, string) {
		if len(routes) == 0 {
			return nil, ""
		}
		m, err := mail.ReadMessage(bytes.NewReader(raw))
		if err != nil {
			return nil, ""
		}
		from, err := mail.ParseAddress(m.Header.Get("From"))
		if err != nil {
			return nil, ""
		}
		addr := strings.ToLower(from.Address)
		for _, t := range routes {
			if t == addr {
				if c, err := targetConn(t); err == nil {
					if sent, err := s.folders.byRole(c, t, "sent"); err == nil {
						return c, sent
					}
				}
			}
		}
		return nil, ""
	}
	total := 0
	for _, mb := range ordered {
		if hasAttr(mb.Attributes, imap.NoSelectAttr) || hasAttr(mb.Attributes, imap.FlaggedAttr) ||
			hasAttr(mb.Attributes, "\\Important") || hasAttr(mb.Attributes, "\\NonExistent") {
			continue // Gmail's "Starred", "Important" only repeat letters from other folders
		}
		// A connection unused for a while (another folder went to another mailbox meanwhile) may
		// be dead on our side while the server still waits: checked, renewed if needed.
		if local, err = s.aliveConn(local, a.Mailbox); err != nil {
			return total, err
		}
		into, dest := local, ""
		var route func([]byte) (*client.Client, string)
		if t := routes[mb.Name]; t != "" {
			if into, err = targetConn(t); err != nil {
				log.Printf("collector %s: mailbox %s: %v", a.Address, t, err)
				continue
			}
			dest = "INBOX"
		} else if hasAttr(mb.Attributes, imap.AllAttr) {
			dest, err = s.folders.byRole(local, a.Mailbox, "archive")
		} else {
			dest, err = s.localFolderFor(local, a.Mailbox, mb)
			if roleOf(mb) == "sent" {
				route = sendersRoute
			}
		}
		if err != nil {
			log.Printf("collector %s: folder %q: %v", a.Address, mb.Name, err)
			continue
		}
		n, err := s.copyFolder(ctx, a, rc, into, mb.Name, dest, extPerRun-total, route)
		total += n
		if err != nil {
			return total, fmt.Errorf("folder %s: %w", mb.Name, err)
		}
		if twoWay && into == local && route == nil {
			if _, err := rc.Select(mb.Name, true); err == nil {
				s.pullRemoteState(ctx, a, rc, local, rf, mb.Name)
			}
		}
		if a.DeleteAfter {
			s.cleanSource(ctx, a, rc, rf, mb.Name)
		}
		if total >= extPerRun {
			break
		}
	}
	return total, nil
}

// localFolderFor maps a remote folder to ours: special folders by role, the rest by name
// (created when missing).
func (s *Server) localFolderFor(local *client.Client, account string, mb *imap.MailboxInfo) (string, error) {
	role := roleOf(mb)
	if role == "" {
		// Common names of servers without special-use flags (English and Russian folder names:
		// detection data, not interface text).
		switch strings.ToLower(strings.TrimPrefix(strings.TrimPrefix(mb.Name, "INBOX"+mb.Delimiter), "INBOX.")) {
		case "sent", "sent items", "sent messages", "отправленные":
			role = "sent"
		case "drafts", "черновики":
			role = "drafts"
		case "trash", "deleted", "deleted items", "deleted messages", "корзина", "удаленные", "удалённые":
			role = "trash"
		case "spam", "junk", "junk e-mail", "спам":
			role = "junk"
		case "archive", "архив":
			role = "archive"
		}
	}
	if role != "" {
		return s.folders.byRole(local, account, role)
	}
	name := mb.Name
	if mb.Delimiter != "" {
		name = strings.TrimPrefix(name, "INBOX"+mb.Delimiter)
		name = strings.ReplaceAll(name, mb.Delimiter, " - ")
	}
	name = strings.TrimPrefix(name, "[Gmail] - ")
	name = strings.Map(func(r rune) rune {
		if strings.ContainsRune("/\\*%\"", r) {
			return '-'
		}
		if r == '.' {
			return '\u2024' // the mail store does not allow "." in names; "․" looks the same (info@firma․ru)
		}
		return r
	}, name)
	if folderExists(local, name) != nil {
		if err := local.Create(name); err != nil {
			return "", err
		}
		_ = local.Subscribe(name)
	}
	return name, nil
}

// route, when set, may send a letter to another mailbox and folder (letters "sent as" a routed address).
func (s *Server) copyFolder(ctx context.Context, a extAccount, rc, local *client.Client, remote, dest string, budget int, route func([]byte) (*client.Client, string)) (int, error) {
	if budget <= 0 {
		return 0, nil
	}
	mbox, err := rc.Select(remote, true)
	if err != nil {
		return 0, err
	}
	var validity, last int64
	err = s.db.QueryRow(ctx, `SELECT uidvalidity, last_uid FROM external_folders WHERE account_id=$1 AND remote=$2`, a.ID, remote).Scan(&validity, &last)
	if err != nil || validity != int64(mbox.UidValidity) {
		last = 0 // new folder, or the server renumbered it: start over (duplicates are skipped by Message-ID)
		_, _ = s.db.Exec(ctx, `INSERT INTO external_folders(account_id, remote, uidvalidity, last_uid) VALUES ($1,$2,$3,0)
			ON CONFLICT (account_id, remote) DO UPDATE SET uidvalidity=EXCLUDED.uidvalidity, last_uid=0`, a.ID, remote, int64(mbox.UidValidity))
	}
	if mbox.Messages == 0 {
		return 0, nil
	}
	crit := imap.NewSearchCriteria()
	crit.Uid = new(imap.SeqSet)
	crit.Uid.AddRange(uint32(last+1), 0)
	if a.Since != "" {
		if t, err := time.Parse("2006-01-02", a.Since); err == nil {
			crit.Since = t
		}
	}
	uids, err := rc.UidSearch(crit)
	if err != nil {
		return 0, err
	}
	var todo []uint32
	for _, u := range uids {
		if int64(u) > last {
			todo = append(todo, u)
		}
	}
	if len(todo) == 0 {
		return 0, nil
	}
	sortUIDs(todo)
	if len(todo) > budget {
		todo = todo[:budget]
	}
	_, _ = s.db.Exec(ctx, `UPDATE external_folders SET total=$3 WHERE account_id=$1 AND remote=$2`, a.ID, remote, len(uids))
	// How many bytes are still waiting there (sizes only, a few bytes per letter).
	if len(uids) > len(todo) {
		set := new(imap.SeqSet)
		set.AddNum(uids...)
		ch := make(chan *imap.Message, 256)
		done := make(chan error, 1)
		go func() { done <- rc.UidFetch(set, []imap.FetchItem{imap.FetchRFC822Size}, ch) }()
		var sum int64
		for m := range ch {
			sum += int64(m.Size)
		}
		if <-done == nil {
			_, _ = s.db.Exec(ctx, `UPDATE external_folders SET remaining_bytes=$3 WHERE account_id=$1 AND remote=$2`, a.ID, remote, sum)
		}
	} else {
		_, _ = s.db.Exec(ctx, `UPDATE external_folders SET remaining_bytes=0 WHERE account_id=$1 AND remote=$2`, a.ID, remote)
	}
	imported := 0
	section := &imap.BodySectionName{Peek: true}
	// Sizes and Message-IDs first (a few bytes each): letters we already have — the same letter
	// under several Gmail labels, or fetched before a break — are not downloaded again, and one
	// request never asks for more than ~8 MB (big attachments could outlast the command timeout).
	sizes := map[uint32]uint32{}
	known := map[uint32]bool{}
	{
		set := new(imap.SeqSet)
		set.AddNum(todo...)
		hdr := &imap.BodySectionName{Peek: true, BodyPartName: imap.BodyPartName{Specifier: imap.HeaderSpecifier, Fields: []string{"Message-ID"}}}
		ch := make(chan *imap.Message, 64)
		done := make(chan error, 1)
		go func() { done <- rc.UidFetch(set, []imap.FetchItem{imap.FetchUid, imap.FetchRFC822Size, hdr.FetchItem()}, ch) }()
		ids := map[uint32]string{}
		for m := range ch {
			sizes[m.Uid] = m.Size
			if lit := m.GetBody(hdr); lit != nil {
				if b, err := io.ReadAll(lit); err == nil {
					if mm, err := mail.ReadMessage(bytes.NewReader(append(b, '\r', '\n'))); err == nil {
						ids[m.Uid] = strings.TrimSpace(mm.Header.Get("Message-Id"))
					}
				}
			}
		}
		if err := <-done; err != nil {
			return 0, err
		}
		for u, id := range ids {
			if id == "" {
				continue
			}
			var x int
			if s.db.QueryRow(ctx, `SELECT 1 FROM external_seen WHERE account_id=$1 AND key=$2`, a.ID, id).Scan(&x) == nil {
				known[u] = true
				// Still remember where it lives here, for two-way sync of marks and deletions.
				_, _ = s.db.Exec(ctx, `INSERT INTO external_messages(account_id, remote_folder, remote_uid, uidvalidity, message_id, local_folder, seen, flagged)
					VALUES ($1,$2,$3,$4,$5,$6,false,false) ON CONFLICT (account_id, remote_folder, remote_uid) DO NOTHING`,
					a.ID, remote, int64(u), int64(mbox.UidValidity), id, dest)
			}
		}
	}
	const batchBytes = 32 << 20 // letters stream in one by one, so memory stays small
	for i := 0; i < len(todo); {
		j, total := i, uint32(0)
		for j < len(todo) && j-i < 200 && (j == i || total+sizes[todo[j]] <= batchBytes) {
			if !known[todo[j]] {
				total += sizes[todo[j]]
			}
			j++
		}
		batch := todo[i:j]
		batchStart := imported
		set := new(imap.SeqSet)
		for _, u := range batch {
			if sizes[u] < 50<<20 && !known[u] { // bigger letters are skipped: Gmail does not even send such
				set.AddNum(u)
			}
		}
		if !set.Empty() {
			// Each letter is stored as soon as it arrives; only one is kept in memory at a time.
			ch := make(chan *imap.Message, 1)
			done := make(chan error, 1)
			go func() {
				done <- rc.UidFetch(set, []imap.FetchItem{imap.FetchUid, imap.FetchFlags, imap.FetchInternalDate, imap.FetchRFC822Size, section.FetchItem()}, ch)
			}()
			var storeErr error
			for m := range ch {
				if storeErr != nil {
					continue // drain the rest of the answer
				}
				if lit := m.GetBody(section); lit != nil {
					raw, err := io.ReadAll(lit)
					if err == nil {
						into, folder := local, dest
						if route != nil {
							if c, f := route(raw); c != nil {
								into, folder = c, f
							}
						}
						ok, err := s.storeExternal(ctx, a, into, folder, raw, m.Flags, m.InternalDate, &remoteRef{remote, m.Uid, mbox.UidValidity})
						if err != nil {
							storeErr = err
							continue
						}
						if ok {
							imported++
						}
					}
				}
			}
			if err := <-done; err != nil {
				return imported, err
			}
			if storeErr != nil {
				return imported, storeErr
			}
		}
		if got := imported - batchStart; got > 0 { // the counter in the admin list grows as letters arrive
			_, _ = s.db.Exec(ctx, `UPDATE external_accounts SET imported = imported + $2 WHERE id=$1`, a.ID, got)
		}
		// The whole batch is in: letters fetched before a break are skipped next time (Message-ID).
		_, _ = s.db.Exec(ctx, `UPDATE external_folders SET last_uid=$3 WHERE account_id=$1 AND remote=$2`, a.ID, remote, int64(batch[len(batch)-1]))
		setProgress(a.ID, fmt.Sprintf("%s: %d of %d", remote, j, len(todo)))
		i = j
	}
	return imported, nil
}

// storeExternal appends one letter unless it was fetched before (same Message-ID).
type remoteRef struct {
	folder   string
	uid      uint32
	validity uint32
}

func (s *Server) storeExternal(ctx context.Context, a extAccount, local *client.Client, dest string, raw []byte, flags []string, date time.Time, ref *remoteRef) (bool, error) {
	key := ""
	if m, err := mail.ReadMessage(bytes.NewReader(raw)); err == nil {
		key = strings.TrimSpace(m.Header.Get("Message-Id"))
		if date.IsZero() {
			date, _ = m.Header.Date()
		}
	}
	if key == "" {
		key = fmt.Sprintf("size:%d:%s", len(raw), date.UTC().Format(time.RFC3339))
	}
	tag, err := s.db.Exec(ctx, `INSERT INTO external_seen(account_id, key) VALUES ($1,$2) ON CONFLICT DO NOTHING`, a.ID, key)
	if err != nil {
		return false, err
	}
	if ref != nil && strings.HasPrefix(key, "<") {
		// Remember where it lives there (also for letters already fetched from another folder,
		// e.g. a Gmail label), so changes can be synced both ways.
		seen, flagged := false, false
		for _, f := range flags {
			seen = seen || f == imap.SeenFlag
			flagged = flagged || f == imap.FlaggedFlag
		}
		_, _ = s.db.Exec(ctx, `INSERT INTO external_messages(account_id, remote_folder, remote_uid, uidvalidity, message_id, local_folder, seen, flagged)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8) ON CONFLICT (account_id, remote_folder, remote_uid) DO NOTHING`,
			a.ID, ref.folder, int64(ref.uid), int64(ref.validity), key, dest, seen, flagged)
	}
	if tag.RowsAffected() == 0 {
		return false, nil
	}
	keep := []string{}
	for _, f := range flags {
		switch f {
		case imap.SeenFlag, imap.AnsweredFlag, imap.FlaggedFlag, imap.DraftFlag:
			keep = append(keep, f)
		}
	}
	if date.IsZero() {
		date = time.Now()
	}
	raw, deliver := s.scanImported(ctx, a.Mailbox, "collector:"+a.Address, raw)
	if !deliver {
		return false, nil // quarantined; external_seen keeps it from being fetched again
	}
	if err := appendWithin(local, dest, keep, date, raw, 3*time.Minute); err != nil {
		_, _ = s.db.Exec(ctx, `DELETE FROM external_seen WHERE account_id=$1 AND key=$2`, a.ID, key)
		// Not stored here, so it must never count as "ours" when old letters are removed from the source.
		if ref != nil {
			_, _ = s.db.Exec(ctx, `DELETE FROM external_messages WHERE account_id=$1 AND remote_folder=$2 AND remote_uid=$3`, a.ID, ref.folder, int64(ref.uid))
		}
		// Which letter: to find it in the source if it fails again.
		log.Printf("collector %s: append %s (%d KB) to %s: %v", a.Address, key, len(raw)/1024, dest, err)
		return false, fmt.Errorf("could not save message %s (%d KB): %w", key, len(raw)/1024, err)
	}
	return true, nil
}

func (s *Server) syncPOP3(ctx context.Context, a extAccount, pass string) (int, error) {
	p, err := remotePOP3(a, pass)
	if err != nil {
		return 0, err
	}
	defer p.close()
	local, err := s.dialAs(a.Mailbox)
	if err != nil {
		return 0, err
	}
	defer local.Logout()
	// Our side waits while the source sends a letter, sometimes for minutes: a long timeout, not the
	// usual 2 minutes, or the idle connection is dropped mid-import. Never none: an APPEND left
	// without an answer would stop the import for good (it hung for 9 hours once).
	local.Timeout = 15 * time.Minute
	if _, err := p.cmd("UIDL"); err != nil {
		return 0, fmt.Errorf("the server does not support UIDL: %w", err)
	}
	list, err := p.multi()
	if err != nil {
		return 0, err
	}
	type item struct{ n, uidl string }
	var todo []item
	for _, l := range strings.Split(string(list), "\n") {
		f := strings.Fields(l)
		if len(f) != 2 {
			continue
		}
		var old bool
		if s.db.QueryRow(ctx, `SELECT created_at < now() - make_interval(days => $3) FROM external_seen WHERE account_id=$1 AND key=$2`, a.ID, "uidl:"+f[1], a.KeepDays).Scan(&old) == nil {
			if a.DeleteAfter && old {
				_, _ = p.cmd("DELE " + f[0])
			}
			continue
		}
		todo = append(todo, item{f[0], f[1]})
	}
	n := 0
	for i, it := range todo {
		if i >= extPerRun {
			break
		}
		if _, err := p.cmd("RETR " + it.n); err != nil {
			return n, err
		}
		raw, err := p.multi()
		if err != nil {
			return n, err
		}
		if ok, err := s.storeExternal(ctx, a, local, "INBOX", raw, nil, time.Time{}, nil); err != nil {
			return n, err
		} else if ok {
			n++
		}
		_, _ = s.db.Exec(ctx, `INSERT INTO external_seen(account_id, key) VALUES ($1,$2) ON CONFLICT DO NOTHING`, a.ID, "uidl:"+it.uidl)
		if a.DeleteAfter && a.KeepDays == 0 {
			_, _ = p.cmd("DELE " + it.n)
		}
		setProgress(a.ID, fmt.Sprintf("Inbox: %d of %d", i+1, len(todo)))
	}
	return n, nil
}

func sortUIDs(u []uint32) {
	for i := 1; i < len(u); i++ {
		for j := i; j > 0 && u[j] < u[j-1]; j-- {
			u[j], u[j-1] = u[j-1], u[j]
		}
	}
}

func sortMsgs(m []*imap.Message) {
	for i := 1; i < len(m); i++ {
		for j := i; j > 0 && m[j].Uid < m[j-1].Uid; j-- {
			m[j], m[j-1] = m[j-1], m[j]
		}
	}
}

// ---------- sending as an external address ----------

// submitExternal sends through the external mailbox's own SMTP server, so SPF/DKIM of that
// domain are right and the letter is not treated as forged.
func (s *Server) submitExternal(a extAccount, rcpts []string, msg []byte) error {
	pass, err := s.unseal(a.smtpPassEnc)
	if err != nil {
		return err
	}
	conn, err := dialRemote(a.SMTPHost, a.SMTPPort, a.SMTPSecurity)
	if err != nil {
		return err
	}
	c, err := smtp.NewClient(conn, a.SMTPHost)
	if err != nil {
		return err
	}
	defer c.Close()
	if err := c.Hello(s.cfg.Hostname); err != nil {
		return err
	}
	if a.SMTPSecurity == "starttls" {
		if err := c.StartTLS(&tls.Config{ServerName: a.SMTPHost}); err != nil {
			return err
		}
	}
	user := a.SMTPUsername
	if user == "" {
		user = a.Username
	}
	if err := c.Auth(smtp.PlainAuth("", user, pass, a.SMTPHost)); err != nil {
		return fmt.Errorf("SMTP login: %w", err)
	}
	if err := c.Mail(a.Address); err != nil {
		return err
	}
	for _, r := range rcpts {
		if err := c.Rcpt(r); err != nil {
			return fmt.Errorf("rcpt %s: %w", r, err)
		}
	}
	wc, err := c.Data()
	if err != nil {
		return err
	}
	if _, err := wc.Write(msg); err != nil {
		return err
	}
	if err := wc.Close(); err != nil {
		return err
	}
	return c.Quit()
}

// identityFor finds a send-as address the user may use (an external account with SMTP, bound to
// a mailbox the user can send from).
func (s *Server) identityFor(ctx context.Context, sess *Session, address string) (extAccount, Account, bool) {
	a, err := scanExt(s.db.QueryRow(ctx, `SELECT `+extCols+` FROM external_accounts WHERE lower(address)=lower($1) AND smtp_host <> '' AND smtp_password_enc IS NOT NULL`, address))
	if err != nil {
		return a, Account{}, false
	}
	acc, ok := sess.account(a.Mailbox)
	if !ok || !acc.CanSend {
		return a, acc, false
	}
	return a, acc, true
}

func smtpNewClient(conn net.Conn, host string) (*smtp.Client, error) {
	return smtp.NewClient(conn, host)
}
func tlsFor(host string) *tls.Config              { return &tls.Config{ServerName: host} }
func smtpPlain(user, pass, host string) smtp.Auth { return smtp.PlainAuth("", user, pass, host) }

// cleanSource removes letters already stored in TMAIL from one folder of the source server, keeping
// the last KeepDays there. Gmail: moved to its Trash (Gmail empties it after 30 days); elsewhere
// deleted. The mapping goes too, so this is not taken for a deletion to repeat in TMAIL.
func (s *Server) cleanSource(ctx context.Context, a extAccount, rc *client.Client, rf *remoteFolders, remote string) {
	trash := rf.byRole("trash")
	if remote == trash || remote == rf.byRole("junk") {
		return
	}
	if _, err := rc.Select(remote, false); err != nil {
		return
	}
	crit := imap.NewSearchCriteria()
	crit.Before = time.Now().AddDate(0, 0, -a.KeepDays+1)
	old, err := rc.UidSearch(crit)
	if err != nil || len(old) == 0 {
		return
	}
	isOld := map[uint32]bool{}
	for _, u := range old {
		isOld[u] = true
	}
	// Only letters we really have (fetched by this account).
	rows, err := s.db.Query(ctx, `SELECT remote_uid FROM external_messages WHERE account_id=$1 AND remote_folder=$2`, a.ID, remote)
	if err != nil {
		return
	}
	var done []int64
	for rows.Next() {
		var u int64
		if rows.Scan(&u) == nil && isOld[uint32(u)] {
			done = append(done, u)
		}
	}
	rows.Close()
	if len(done) == 0 {
		return
	}
	// In parts: tens of thousands of letters in one command time out on Gmail. Each part is
	// forgotten here right after it is moved, so nothing is taken for a deletion to repeat in TMAIL.
	moved := 0
	for i := 0; i < len(done); i += 1000 {
		part := done[i:min(i+1000, len(done))]
		set := new(imap.SeqSet)
		for _, u := range part {
			set.AddNum(uint32(u))
		}
		if trash != "" {
			err = rc.UidMove(set, trash)
		} else if err = rc.UidStore(set, imap.FormatFlagsOp(imap.AddFlags, true), []interface{}{imap.DeletedFlag}, nil); err == nil {
			err = uidExpunge(rc, set)
		}
		if err != nil {
			log.Printf("collector %s: clean %s: %v", a.Address, remote, err)
			break
		}
		_, _ = s.db.Exec(ctx, `DELETE FROM external_messages WHERE account_id=$1 AND remote_folder=$2 AND remote_uid = ANY($3)`, a.ID, remote, part)
		moved += len(part)
	}
	if moved == 0 {
		return
	}
	done = done[:moved]
	log.Printf("collector %s: removed %d old letters from %s", a.Address, len(done), remote)
}

// authHint explains a refused login for the big providers: they do not accept the account's
// normal password from mail programs, only an "app password".
func authHint(host string, err error) string {
	e := strings.ToLower(err.Error())
	if strings.Contains(e, "bandwidth") || strings.Contains(e, "exceeded") || strings.Contains(e, "too many simultaneous") || strings.Contains(e, "overquota") {
		return " — the source server has temporarily limited mail downloads (Gmail allows about 2.5 GB a day for mail programs). The collector will continue by itself once the limit is lifted, usually within a day."
	}
	if !strings.Contains(e, "invalid credentials") && !strings.Contains(e, "authenticationfailed") && !strings.Contains(e, "authentication failed") &&
		!strings.Contains(e, "535") && !strings.Contains(e, "not accepted") && !strings.Contains(e, "login failed") && !strings.Contains(e, "-err") {
		return ""
	}
	h := strings.ToLower(host)
	switch {
	case strings.Contains(h, "gmail") || strings.Contains(h, "google"):
		return " — Gmail does not accept the normal account password: an \"app password\" is needed. Turn on 2-Step Verification in the Google account and create a password at myaccount.google.com/apppasswords (16 letters); the username is the full @gmail.com address."
	case strings.Contains(h, "yandex"):
		return " — Yandex does not accept the normal password: create an \"app password\" (id.yandex.ru → Security → App passwords → Mail) and enable IMAP access under \"Mail clients\" in the mail settings."
	case strings.Contains(h, "mail.ru") || strings.Contains(h, "bk.ru") || strings.Contains(h, "list.ru") || strings.Contains(h, "inbox.ru"):
		return " — Mail.ru does not accept the normal password: create a \"password for an external application\" (Settings → Security → Passwords for external applications)."
	case strings.Contains(h, "outlook") || strings.Contains(h, "office365") || strings.Contains(h, "hotmail"):
		return " — Outlook/Microsoft often blocks password sign-in for mail programs: check that IMAP/SMTP is allowed in the mailbox settings, or use an app password."
	}
	return " — check the username and password; many mail services require a separate \"app password\" for mail programs."
}

// labelRoutes: remote folders (Gmail labels) named after a TMAIL mailbox address → that address.
func (s *Server) labelRoutes(ctx context.Context, a extAccount, boxes []*imap.MailboxInfo) map[string]string {
	out := map[string]string{}
	have := map[string]bool{}
	if rows, err := s.db.Query(ctx, `SELECT lower(address) FROM mailboxes WHERE active AND deleted_at IS NULL AND lower(address) <> lower($1)`, a.Mailbox); err == nil {
		for rows.Next() {
			var x string
			if rows.Scan(&x) == nil {
				have[x] = true
			}
		}
		rows.Close()
	}
	for _, mb := range boxes {
		name := strings.ToLower(mb.Name)
		if mb.Delimiter != "" {
			name = name[strings.LastIndex(name, strings.ToLower(mb.Delimiter))+1:]
		}
		if have[strings.TrimSpace(name)] && roleOf(mb) == "" {
			out[mb.Name] = strings.TrimSpace(name)
		}
	}
	return out
}

// appendWithin stores a letter, giving up after limit. go-imap waits for the server's "go ahead"
// for the letter's body without any timeout: when the connection breaks at that moment the wait
// never ends (the import stood still for 3 and 9 hours). The connection is closed instead; the
// next pass opens a new one and fetches the letter again.
func appendWithin(c *client.Client, dest string, flags []string, date time.Time, raw []byte, limit time.Duration) error {
	done := make(chan error, 1)
	go func() { done <- c.Append(dest, flags, date, bytes.NewReader(raw)) }()
	select {
	case err := <-done:
		return err
	case <-time.After(limit):
		_ = c.Terminate()
		return fmt.Errorf("the mail server did not respond within %s", limit)
	}
}

// aliveConn answers whether a connection to our mail store still works (NOOP within 20 s) and
// opens a new one when it does not. go-imap stops reading after a read timeout without closing the
// socket; the next APPEND then waits for the server's "go ahead" forever.
func (s *Server) aliveConn(c *client.Client, mailbox string) (*client.Client, error) {
	done := make(chan error, 1)
	go func() { done <- c.Noop() }()
	select {
	case err := <-done:
		if err == nil {
			return c, nil
		}
	case <-time.After(20 * time.Second):
	}
	_ = c.Terminate()
	n, err := s.dialAs(mailbox)
	if err != nil {
		return nil, err
	}
	n.Timeout = 15 * time.Minute
	return n, nil
}
