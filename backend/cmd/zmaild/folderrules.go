package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap/client"
)

// User folders with a purpose: a description (the AI sorts letters by it) and a list of senders
// whose letters always go there. Auto-sorting is switched on per folder.

type folderRule struct {
	Account     string   `json:"account"`
	Folder      string   `json:"folder"`
	Description string   `json:"description"`
	Senders     []string `json:"senders"`
	Auto        bool     `json:"auto"`
	HideInbox   bool     `json:"hideInbox"`  // straight into the folder, never shown in the Inbox
	ExpireDays  int      `json:"expireDays"` // letters older than this go to Trash (0 = keep)
}

func folderExists(c *client.Client, name string) error {
	ch := make(chan *imap.MailboxInfo, 4)
	done := make(chan error, 1)
	go func() { done <- c.List("", name, ch) }()
	found := false
	for range ch {
		found = true
	}
	if err := <-done; err != nil {
		return err
	}
	if !found {
		return errors.New("no such folder")
	}
	return nil
}

func cleanSenders(in []string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, s := range in {
		for _, p := range strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ';' || r == ' ' || r == '\n' }) {
			p = strings.ToLower(strings.Trim(p, "<>\"' "))
			if p == "" || !strings.Contains(p, "@") && !strings.Contains(p, ".") {
				continue
			}
			if !strings.Contains(p, "@") {
				p = "@" + p // bare domain
			}
			if !seen[p] {
				seen[p] = true
				out = append(out, p)
			}
		}
	}
	return out
}

func validFolderName(n string) bool {
	return n != "" && len([]rune(n)) <= 60 && !strings.ContainsAny(n, "/\\*%\"\r\n\t") && !strings.EqualFold(n, "INBOX")
}

func (s *Server) handleFolderRules(w http.ResponseWriter, r *http.Request, sess *Session) {
	acc, ok := sess.account(r.URL.Query().Get("account"))
	if !ok {
		writeErr(w, http.StatusForbidden, "No access to the mailbox")
		return
	}
	rows, err := s.db.Query(r.Context(), `SELECT account, folder, description, senders, auto, hide_inbox, expire_days FROM folder_rules WHERE account=$1 ORDER BY folder`, acc.Address)
	if err != nil {
		serverErr(w, "folder rules", err)
		return
	}
	defer rows.Close()
	out := []folderRule{}
	for rows.Next() {
		var f folderRule
		if rows.Scan(&f.Account, &f.Folder, &f.Description, &f.Senders, &f.Auto, &f.HideInbox, &f.ExpireDays) == nil {
			out = append(out, f)
		}
	}
	writeJSON(w, http.StatusOK, out)
}

// POST /api/folders — create a folder (with its rule); PUT /api/folders — change the rule
// (and rename the folder when "rename" is given).
func (s *Server) handleFolderSave(w http.ResponseWriter, r *http.Request, sess *Session) {
	var in struct {
		folderRule
		Rename string `json:"rename"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "Invalid data")
		return
	}
	create := r.Method == http.MethodPost
	name := strings.TrimSpace(in.Folder)
	newName := strings.TrimSpace(in.Rename)
	if create {
		newName = name
	}
	if newName != "" && !validFolderName(newName) {
		writeErr(w, http.StatusBadRequest, "Folder name: up to 60 characters, without / \\ * % or quotes")
		return
	}
	senders := cleanSenders(in.Senders)
	desc := strings.TrimSpace(in.Description)
	if len([]rune(desc)) > 1000 {
		writeErr(w, http.StatusBadRequest, "The description is too long")
		return
	}
	s.withAccount(w, r, sess, in.Account, false, func(c *client.Client, acc Account) {
		switch {
		case create:
			if folderExists(c, name) == nil {
				writeErr(w, http.StatusConflict, "This folder already exists")
				return
			}
			if err := c.Create(name); err != nil {
				serverErr(w, "create folder", err)
				return
			}
			_ = c.Subscribe(name)
		case newName != "" && newName != name:
			if folderExists(c, newName) == nil {
				writeErr(w, http.StatusConflict, "This folder already exists")
				return
			}
			if err := c.Rename(name, newName); err != nil {
				serverErr(w, "rename folder", err)
				return
			}
			_ = c.Subscribe(newName)
			_, _ = s.db.Exec(r.Context(), `DELETE FROM folder_rules WHERE account=$1 AND folder=$2`, acc.Address, name)
			name = newName
		default:
			if folderExists(c, name) != nil {
				writeErr(w, http.StatusNotFound, "Folder not found")
				return
			}
		}
		if in.HideInbox {
			in.Auto = true // hiding from the Inbox is a kind of auto-sorting
		}
		var before []string
		_ = s.db.QueryRow(r.Context(), `SELECT senders FROM folder_rules WHERE account=$1 AND folder=$2`, acc.Address, name).Scan(&before)
		if in.ExpireDays < 0 || in.ExpireDays > 3650 {
			in.ExpireDays = 0
		}
		if _, err := s.db.Exec(r.Context(), `INSERT INTO folder_rules(account, folder, description, senders, auto, hide_inbox, expire_days, created_by) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
			ON CONFLICT (account, folder) DO UPDATE SET description=EXCLUDED.description, senders=EXCLUDED.senders, auto=EXCLUDED.auto, hide_inbox=EXCLUDED.hide_inbox,
			expire_days=EXCLUDED.expire_days, updated_at=now()`,
			acc.Address, name, desc, senders, in.Auto, in.HideInbox, in.ExpireDays, sess.Login); err != nil {
			serverErr(w, "save folder rule", err)
			return
		}
		s.audit(r.Context(), sess.UserID, sess.Login, clientIP(r), "folder.save", acc.Address+"/"+name, map[string]any{"auto": in.Auto})
		if in.ExpireDays > 0 { // old letters go at once, not at the next hourly round
			go func(account, folder string, days int) {
				if n, err := s.expireFolder(account, folder, days); err == nil && n > 0 {
					log.Printf("folder expiry %s/%s: %d letters older than %d days to Trash", account, folder, n, days)
					s.hub.countsChanged(account)
				}
			}(acc.Address, name, in.ExpireDays)
		}
		if added := newSenders(before, senders); in.Auto && len(added) > 0 {
			go s.fileBySenders(acc.Address, name, added) // their letters already in the Inbox go there too
		}
		s.hub.countsChanged(acc.Address)
		writeJSON(w, http.StatusOK, folderRule{Account: acc.Address, Folder: name, Description: desc, Senders: senders, Auto: in.Auto, HideInbox: in.HideInbox, ExpireDays: in.ExpireDays})
	})
}

// DELETE /api/folders?account=&folder= — only empty folders, so nothing is lost by accident.
func (s *Server) handleFolderDelete(w http.ResponseWriter, r *http.Request, sess *Session) {
	name := r.URL.Query().Get("folder")
	s.withAccount(w, r, sess, r.URL.Query().Get("account"), false, func(c *client.Client, acc Account) {
		st, err := c.Status(name, []imap.StatusItem{imap.StatusMessages})
		if err != nil {
			writeErr(w, http.StatusNotFound, "Folder not found")
			return
		}
		if roleOfName(c, name) != "" {
			writeErr(w, http.StatusBadRequest, "A system folder cannot be deleted")
			return
		}
		if st.Messages > 0 {
			writeErr(w, http.StatusConflict, "The folder contains messages. Move them first")
			return
		}
		if err := c.Delete(name); err != nil {
			serverErr(w, "delete folder", err)
			return
		}
		_, _ = s.db.Exec(r.Context(), `DELETE FROM folder_rules WHERE account=$1 AND folder=$2`, acc.Address, name)
		s.audit(r.Context(), sess.UserID, sess.Login, clientIP(r), "folder.delete", acc.Address+"/"+name, nil)
		s.hub.countsChanged(acc.Address)
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	})
}

func roleOfName(c *client.Client, name string) string {
	if strings.EqualFold(name, "INBOX") {
		return "inbox"
	}
	ch := make(chan *imap.MailboxInfo, 4)
	done := make(chan error, 1)
	go func() { done <- c.List("", name, ch) }()
	role := ""
	for mb := range ch {
		role = roleOf(mb)
	}
	<-done
	return role
}

// newSenders: those in now that were not in before.
func newSenders(before, now []string) []string {
	had := map[string]bool{}
	for _, x := range before {
		had[x] = true
	}
	var out []string
	for _, x := range now {
		if !had[x] {
			out = append(out, x)
		}
	}
	return out
}

// POST /api/folders/sender {account, folder, sender} — "all letters from this sender go to this
// folder", straight from a letter: the sender (address or @domain) joins the folder's rule (and
// leaves any other folder's rule of the mailbox), and the letters already in the Inbox move there.
func (s *Server) handleFolderSender(w http.ResponseWriter, r *http.Request, sess *Session) {
	var in struct {
		Account string `json:"account"`
		Folder  string `json:"folder"`
		Sender  string `json:"sender"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "Invalid data")
		return
	}
	snd := cleanSenders([]string{in.Sender})
	if len(snd) != 1 {
		writeErr(w, http.StatusBadRequest, "A sender address or a domain is required")
		return
	}
	s.withAccount(w, r, sess, in.Account, false, func(c *client.Client, acc Account) {
		if folderExists(c, in.Folder) != nil || roleOfName(c, in.Folder) != "" {
			writeErr(w, http.StatusBadRequest, "Choose one of your own folders (not a system one)")
			return
		}
		ctx := r.Context()
		if _, err := s.db.Exec(ctx, `UPDATE folder_rules SET senders = array_remove(senders, $2), updated_at=now() WHERE account=$1 AND folder<>$3 AND $2 = ANY(senders)`,
			acc.Address, snd[0], in.Folder); err != nil {
			serverErr(w, "folder sender", err)
			return
		}
		if _, err := s.db.Exec(ctx, `INSERT INTO folder_rules(account, folder, description, senders, auto, hide_inbox, created_by) VALUES ($1,$2,'',ARRAY[$3],true,false,$4)
			ON CONFLICT (account, folder) DO UPDATE SET senders = CASE WHEN $3 = ANY(folder_rules.senders) THEN folder_rules.senders ELSE array_append(folder_rules.senders, $3) END,
			auto = true, updated_at = now()`, acc.Address, in.Folder, snd[0], sess.Login); err != nil {
			serverErr(w, "folder sender", err)
			return
		}
		s.audit(ctx, sess.UserID, sess.Login, clientIP(r), "folder.save", acc.Address+"/"+in.Folder, map[string]any{"sender": snd[0]})
		go s.fileBySenders(acc.Address, in.Folder, snd)
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "sender": snd[0]})
	})
}

// runFolderExpiry moves letters older than a folder's "delete after N days" to Trash (Dovecot
// empties Trash after 30 days). Checked every hour; the age is from when the letter arrived.
func (s *Server) runFolderExpiry(ctx context.Context) {
	t := time.NewTicker(time.Hour)
	defer t.Stop()
	for {
		s.expireFolders(ctx)
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

func (s *Server) expireFolders(ctx context.Context) {
	rows, err := s.db.Query(ctx, `SELECT account, folder, expire_days FROM folder_rules WHERE expire_days > 0
		AND account IN (SELECT address FROM mailboxes WHERE active AND deleted_at IS NULL) ORDER BY account`)
	if err != nil {
		return
	}
	type rule struct {
		account, folder string
		days            int
	}
	var list []rule
	for rows.Next() {
		var x rule
		if rows.Scan(&x.account, &x.folder, &x.days) == nil {
			list = append(list, x)
		}
	}
	rows.Close()
	for _, x := range list {
		n, err := s.expireFolder(x.account, x.folder, x.days)
		if err != nil {
			log.Printf("folder expiry %s/%s: %v", x.account, x.folder, err)
		} else if n > 0 {
			log.Printf("folder expiry %s/%s: %d letters older than %d days to Trash", x.account, x.folder, n, x.days)
			s.audit(ctx, 0, "system", "", "folder.expire", x.account+"/"+x.folder, map[string]any{"letters": n, "days": x.days})
			s.hub.countsChanged(x.account)
		}
	}
}

func (s *Server) expireFolder(account, folder string, days int) (int, error) {
	c, err := s.dialAs(account)
	if err != nil {
		return 0, err
	}
	defer c.Logout()
	c.Timeout = 15 * time.Minute // thousands of letters at once
	trash, err := s.folders.byRole(c, account, "trash")
	if err != nil || trash == "" || trash == folder || roleOfName(c, folder) == "trash" {
		return 0, err
	}
	if _, err := c.Select(folder, false); err != nil {
		return 0, err
	}
	crit := imap.NewSearchCriteria()
	crit.Before = time.Now().AddDate(0, 0, -days) // INTERNALDATE: when it arrived
	uids, err := c.UidSearch(crit)
	if err != nil || len(uids) == 0 {
		return 0, err
	}
	moved := 0
	for i := 0; i < len(uids); i += 1000 {
		set := new(imap.SeqSet)
		set.AddNum(uids[i:min(i+1000, len(uids))]...)
		if err := c.UidMove(set, trash); err != nil {
			return moved, err
		}
		moved += min(1000, len(uids)-i)
	}
	return moved, nil
}
