package main

import (
	"archive/tar"
	"archive/zip"
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/mail"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap/client"
)

// Import of a Google Takeout mail archive (MBOX, also inside the .tgz/.zip as downloaded), for a
// mailbox too big for IMAP (Gmail gives programs ~2.5 GB a day). Each letter carries its Gmail
// labels (X-Gmail-Labels) and is filed like the collector files it: a label named after a TMAIL
// mailbox → that mailbox; Inbox, Sent, Drafts, Spam, Trash → the folders of those roles; another
// label → a folder of that name; no label → Archive. Letters already brought by the collector of the
// same Gmail account (and the other way round) are recognised by Message-ID and stored once.

const importDir = "/import" // /opt/zmail/import on the server

type mboxJob struct {
	mu                                   sync.Mutex
	Running                              bool
	File, Current                        string
	Read, Stored, Skipped, Errors, Virus int
	Bytes                                int64
	Started, Finished                    time.Time
	LastError                            string
	ByMailbox                            map[string]int
}

var mboxImport = &mboxJob{}

func (j *mboxJob) snapshot() map[string]any {
	j.mu.Lock()
	defer j.mu.Unlock()
	return map[string]any{"running": j.Running, "file": j.File, "read": j.Read, "stored": j.Stored, "skipped": j.Skipped,
		"errors": j.Errors, "virus": j.Virus, "bytes": j.Bytes, "started": j.Started, "finished": j.Finished, "lastError": j.LastError, "byMailbox": j.ByMailbox}
}

// GET /api/admin/import-mbox — files waiting in the import folder and the job state.
// POST /api/admin/import-mbox {account, file} — start (account: the Gmail collector's address).
func (s *Server) adminImportMbox(w http.ResponseWriter, r *http.Request, sess *Session) {
	if r.Method == http.MethodGet {
		var files []map[string]any
		entries, _ := os.ReadDir(importDir)
		for _, e := range entries {
			if fi, err := e.Info(); err == nil && !e.IsDir() {
				files = append(files, map[string]any{"name": e.Name(), "size": fi.Size()})
			}
		}
		writeJSON(w, http.StatusOK, map[string]any{"files": files, "job": mboxImport.snapshot()})
		return
	}
	var in struct {
		Account string `json:"account"`
		File    string `json:"file"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "Invalid data")
		return
	}
	path := filepath.Join(importDir, filepath.Base(in.File))
	if _, err := os.Stat(path); err != nil {
		writeErr(w, http.StatusNotFound, "File not found in the import folder")
		return
	}
	var a extAccount
	if err := s.db.QueryRow(r.Context(), `SELECT id, mailbox, address FROM external_accounts WHERE lower(address)=lower($1)`, in.Account).Scan(&a.ID, &a.Mailbox, &a.Address); err != nil {
		writeErr(w, http.StatusNotFound, "No mail collector for this address. Connect the mailbox in \"Mail collector\" first")
		return
	}
	j := mboxImport
	j.mu.Lock()
	if j.Running {
		j.mu.Unlock()
		writeErr(w, http.StatusConflict, "Import is already running")
		return
	}
	j.Running, j.File, j.Current, j.Started, j.Finished, j.LastError = true, filepath.Base(path), "", time.Now(), time.Time{}, ""
	j.Read, j.Stored, j.Skipped, j.Errors, j.Virus, j.Bytes, j.ByMailbox = 0, 0, 0, 0, 0, 0, map[string]int{}
	j.mu.Unlock()
	s.auditAdmin(r, sess, "import.mbox", a.Address, map[string]any{"file": filepath.Base(path)})
	go s.runMboxImport(a, path)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) runMboxImport(a extAccount, path string) {
	j := mboxImport
	err := s.importArchive(a, path)
	j.mu.Lock()
	j.Running, j.Finished, j.Current = false, time.Now(), ""
	if err != nil {
		j.LastError = err.Error()
	}
	log.Printf("mbox import %s: read %d, stored %d, already there %d, errors %d (%v)", j.File, j.Read, j.Stored, j.Skipped, j.Errors, err)
	j.mu.Unlock()
}

// importArchive reads .mbox files straight from the archive, without unpacking it to disk.
func (s *Server) importArchive(a extAccount, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	imp := newMboxImporter(s, a)
	defer imp.close()
	low := strings.ToLower(path)
	switch {
	case strings.HasSuffix(low, ".mbox"):
		return imp.readMbox(f)
	case strings.HasSuffix(low, ".tgz") || strings.HasSuffix(low, ".tar.gz"):
		gz, err := gzip.NewReader(bufio.NewReaderSize(f, 1<<20))
		if err != nil {
			return err
		}
		tr := tar.NewReader(gz)
		for {
			h, err := tr.Next()
			if err == io.EOF {
				return nil
			}
			if err != nil {
				return err
			}
			if strings.HasSuffix(strings.ToLower(h.Name), ".mbox") {
				if err := imp.readMbox(tr); err != nil {
					return err
				}
			}
		}
	case strings.HasSuffix(low, ".zip"):
		st, _ := f.Stat()
		zr, err := zip.NewReader(f, st.Size())
		if err != nil {
			return err
		}
		for _, zf := range zr.File {
			if !strings.HasSuffix(strings.ToLower(zf.Name), ".mbox") {
				continue
			}
			rc, err := zf.Open()
			if err != nil {
				return err
			}
			err = imp.readMbox(rc)
			rc.Close()
			if err != nil {
				return err
			}
		}
		return nil
	}
	return fmt.Errorf("a .mbox, .tgz or .zip file from Google Takeout is needed")
}

type mboxImporter struct {
	s       *Server
	a       extAccount
	routes  map[string]bool // TMAIL mailboxes a label may name
	conns   map[string]*client.Client
	folders map[string]map[string]string // mailbox → role → folder
}

func newMboxImporter(s *Server, a extAccount) *mboxImporter {
	imp := &mboxImporter{s: s, a: a, routes: map[string]bool{}, conns: map[string]*client.Client{}, folders: map[string]map[string]string{}}
	if rows, err := s.db.Query(context.Background(), `SELECT lower(address) FROM mailboxes WHERE active AND deleted_at IS NULL`); err == nil {
		for rows.Next() {
			var x string
			if rows.Scan(&x) == nil {
				imp.routes[x] = true
			}
		}
		rows.Close()
	}
	return imp
}

func (imp *mboxImporter) close() {
	for _, c := range imp.conns {
		c.Logout()
	}
}

func (imp *mboxImporter) conn(mailbox string) (*client.Client, error) {
	if c := imp.conns[mailbox]; c != nil && c.State() != imap.LogoutState {
		return c, nil
	}
	c, err := imp.s.dialAs(mailbox)
	if err != nil {
		return nil, err
	}
	c.Timeout = 15 * time.Minute
	imp.conns[mailbox] = c
	return c, nil
}

// reMboxFrom: the line that starts a letter in Takeout's MBOX ("From 1700000000000000000@xxx Mon Jan 01 …").
var reMboxFrom = regexp.MustCompile(`^From \S+ +\w{3} \w{3} +\d{1,2} \d\d:\d\d:\d\d `)

func (imp *mboxImporter) readMbox(r io.Reader) error {
	br := bufio.NewReaderSize(r, 1<<20)
	var cur bytes.Buffer
	started := false
	flush := func() {
		if started && cur.Len() > 0 {
			imp.one(bytes.TrimRight(cur.Bytes(), "\r\n"))
		}
		cur.Reset()
	}
	for {
		line, err := br.ReadBytes('\n')
		if len(line) > 0 {
			if bytes.HasPrefix(line, []byte("From ")) && reMboxFrom.Match(line) {
				flush()
				started = true
			} else if started {
				if bytes.HasPrefix(line, []byte(">From ")) { // mboxrd escaping
					line = line[1:]
				}
				cur.Write(line)
			}
		}
		if err == io.EOF {
			flush()
			return nil
		}
		if err != nil {
			return err
		}
	}
}

// Gmail system labels as Takeout writes them (the account's language, Russian or English):
// detection data, not interface text.
var gmailRoles = map[string]string{
	"входящие": "inbox", "inbox": "inbox", "отправленные": "sent", "sent": "sent", "черновики": "drafts", "drafts": "drafts",
	"спам": "junk", "spam": "junk", "корзина": "trash", "trash": "trash",
}
var gmailIgnore = map[string]bool{"непрочитанные": true, "unread": true, "помеченные": true, "starred": true, "важное": true, "important": true,
	"открытые": true, "opened": true, "архивированные": true, "archived": true}

func (imp *mboxImporter) one(raw []byte) {
	j := mboxImport
	j.mu.Lock()
	j.Read++
	j.Bytes += int64(len(raw))
	j.mu.Unlock()
	m, err := mail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		imp.fail(fmt.Errorf("cannot parse a message: %w", err))
		return
	}
	labels := splitLabels(decodeWords(m.Header.Get("X-Gmail-Labels")))
	role, user, target := "", "", ""
	unread, starred := false, false
	for _, l := range labels {
		low := strings.ToLower(l)
		switch { // label names in Russian and English: detection data
		case low == "чат" || low == "chat":
			j.mu.Lock()
			j.Skipped++
			j.mu.Unlock()
			return // Hangouts chats are not letters
		case low == "непрочитанные" || low == "unread":
			unread = true
		case low == "помеченные" || low == "starred":
			starred = true
		case gmailRoles[low] != "":
			if role == "" || role == "inbox" {
				role = gmailRoles[low]
			}
		case gmailIgnore[low] || strings.HasPrefix(low, "категория") || strings.HasPrefix(low, "category"):
		case imp.routes[low] && low != strings.ToLower(imp.a.Mailbox):
			target = low
		default:
			if user == "" {
				user = l
			}
		}
	}
	// Sent "as" another of our addresses (Gmail "send mail as") goes to that mailbox's Sent.
	if role == "sent" {
		if from, err := mail.ParseAddress(m.Header.Get("From")); err == nil && imp.routes[strings.ToLower(from.Address)] {
			target = strings.ToLower(from.Address)
		}
	}
	mailbox := imp.a.Mailbox
	if target != "" {
		mailbox = target
		if role == "" { // like the collector: a letter collected for that address is in its Inbox
			role, user = "inbox", ""
		}
	}
	date, _ := m.Header.Date()
	key := strings.TrimSpace(m.Header.Get("Message-Id"))
	if key == "" {
		key = fmt.Sprintf("size:%d:%s", len(raw), date.UTC().Format(time.RFC3339))
	}
	ctx := context.Background()
	tag, err := imp.s.db.Exec(ctx, `INSERT INTO external_seen(account_id, key) VALUES ($1,$2) ON CONFLICT DO NOTHING`, imp.a.ID, key)
	if err != nil {
		imp.fail(err)
		return
	}
	if tag.RowsAffected() == 0 { // the collector (or an earlier run) already has it
		j.mu.Lock()
		j.Skipped++
		j.mu.Unlock()
		return
	}
	forget := func() {
		_, _ = imp.s.db.Exec(ctx, `DELETE FROM external_seen WHERE account_id=$1 AND key=$2`, imp.a.ID, key)
	}
	c, err := imp.conn(mailbox)
	if err != nil {
		forget()
		imp.fail(err)
		return
	}
	folder, err := imp.folderFor(c, mailbox, role, user)
	if err != nil {
		forget()
		imp.fail(err)
		return
	}
	var flags []string
	if !unread {
		flags = append(flags, imap.SeenFlag)
	}
	if starred {
		flags = append(flags, imap.FlaggedFlag)
	}
	if role == "drafts" {
		flags = append(flags, imap.DraftFlag)
	}
	if date.IsZero() {
		date = time.Now()
	}
	body, deliver := imp.s.scanImported(ctx, mailbox, "takeout:"+imp.a.Address, raw)
	if !deliver {
		j.mu.Lock()
		j.Virus++
		j.mu.Unlock()
		return
	}
	j.mu.Lock()
	j.Current = mailbox + " / " + folder
	j.mu.Unlock()
	err = appendWithin(c, folder, flags, date, body, 3*time.Minute)
	if err != nil { // one more try on a new connection
		delete(imp.conns, mailbox)
		if c, err = imp.conn(mailbox); err == nil {
			err = appendWithin(c, folder, flags, date, body, 3*time.Minute)
		}
	}
	if err != nil {
		forget()
		imp.fail(fmt.Errorf("%s: %w", key, err))
		return
	}
	j.mu.Lock()
	j.Stored++
	j.ByMailbox[mailbox+" / "+folder]++
	n := j.Read
	j.mu.Unlock()
	if n%2000 == 0 {
		log.Printf("mbox import: %d read, %d stored", n, j.Stored)
		imp.s.hub.countsChanged(mailbox)
	}
}

func (imp *mboxImporter) fail(err error) {
	j := mboxImport
	j.mu.Lock()
	j.Errors++
	j.LastError = truncate(err.Error(), 300)
	j.mu.Unlock()
}

// folderFor: the folder of a role in the mailbox, a folder named after a user label (created when
// missing; "." is not allowed by the mail store), or Archive.
func (imp *mboxImporter) folderFor(c *client.Client, mailbox, role, label string) (string, error) {
	if imp.folders[mailbox] == nil {
		imp.folders[mailbox] = map[string]string{}
	}
	key := role
	switch {
	case role != "" && role != "inbox":
	case label != "":
		key = "label:" + label
	case role == "inbox":
		key = "inbox"
	default:
		key = "archive"
	}
	if f := imp.folders[mailbox][key]; f != "" {
		return f, nil
	}
	var f string
	var err error
	switch {
	case key == "inbox":
		f = "INBOX"
	case strings.HasPrefix(key, "label:"):
		f = strings.Map(func(r rune) rune {
			switch {
			case strings.ContainsRune("\\*%\"", r):
				return '-'
			case r == '.':
				return '\u2024'
			}
			return r
		}, strings.ReplaceAll(label, "/", " - "))
		if folderExists(c, f) != nil {
			if err = c.Create(f); err == nil {
				_ = c.Subscribe(f)
			}
		}
	default:
		f, err = imp.s.folders.byRole(c, mailbox, key)
	}
	if err != nil {
		return "", err
	}
	imp.folders[mailbox][key] = f
	return f, nil
}

// splitLabels: "Inbox,\"Clients, VIP\",Unread" → the labels (quoted ones may hold commas).
func splitLabels(h string) []string {
	var out []string
	var cur strings.Builder
	quoted := false
	for _, r := range h {
		switch {
		case r == '"':
			quoted = !quoted
		case r == ',' && !quoted:
			if t := strings.TrimSpace(cur.String()); t != "" {
				out = append(out, t)
			}
			cur.Reset()
		default:
			cur.WriteRune(r)
		}
	}
	if t := strings.TrimSpace(cur.String()); t != "" {
		out = append(out, t)
	}
	return out
}
