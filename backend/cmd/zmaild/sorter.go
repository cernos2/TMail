package main

import (
	"context"
	"fmt"
	"html"
	"log"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap/client"
)

// Auto-sorting: new Inbox letters of mailboxes that have folders with auto-sorting switched on are
// moved by sender (address or @domain) or, failing that, by the AI comparing the letter with the
// folders' descriptions. Runs on the server, so it works even when nobody has the mail open.
// Letters already in the Inbox when sorting is first switched on are left where they are.

const sortEvery = 45 * time.Second

var reNum = regexp.MustCompile(`\d+`)

func (s *Server) runSorter(ctx context.Context) {
	t := time.NewTicker(sortEvery)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			// Every active mailbox: first the counterparty cards, then folder sorting.
			rows, err := s.db.Query(ctx, `SELECT address FROM mailboxes WHERE active ORDER BY address`)
			if err != nil {
				continue
			}
			var accounts []string
			for rows.Next() {
				var a string
				if rows.Scan(&a) == nil {
					accounts = append(accounts, a)
				}
			}
			rows.Close()
			for _, a := range accounts {
				if err := s.crmScan(ctx, a); err != nil {
					log.Printf("crm %s: %v", a, err)
				}
				if err := s.sortAccount(ctx, a); err != nil {
					log.Printf("sorter %s: %v", a, err)
				}
			}
		}
	}
}

func (s *Server) sortAccount(ctx context.Context, account string) error {
	_, err := s.sortInbox(ctx, account)
	return err
}

var sortLocks sync.Map // account → *sync.Mutex: the timer and a new-mail event must not sort at once

// sortInbox files new Inbox letters and returns where each moved letter went (UID → folder).
func (s *Server) sortInbox(ctx context.Context, account string) (map[uint32]string, error) {
	mu, _ := sortLocks.LoadOrStore(account, &sync.Mutex{})
	mu.(*sync.Mutex).Lock()
	defer mu.(*sync.Mutex).Unlock()
	movedTo := map[uint32]string{}
	rules, err := s.autoRules(ctx, account)
	if err != nil || len(rules) == 0 {
		return movedTo, err
	}
	c, err := s.dialAs(account)
	if err != nil {
		return movedTo, err
	}
	defer c.Logout()
	mb, err := c.Select("INBOX", false)
	if err != nil {
		return movedTo, err
	}
	var validity, last int64
	err = s.db.QueryRow(ctx, `SELECT uidvalidity, last_uid FROM sort_state WHERE account=$1`, account).Scan(&validity, &last)
	if err != nil || validity != int64(mb.UidValidity) {
		// First run (or the Inbox was recreated): start from now on.
		_, err := s.db.Exec(ctx, `INSERT INTO sort_state(account, uidvalidity, last_uid) VALUES ($1,$2,$3)
			ON CONFLICT (account) DO UPDATE SET uidvalidity=EXCLUDED.uidvalidity, last_uid=EXCLUDED.last_uid`, account, int64(mb.UidValidity), int64(mb.UidNext)-1)
		return movedTo, err
	}
	if int64(mb.UidNext)-1 <= last {
		return movedTo, nil
	}
	crit := imap.NewSearchCriteria()
	crit.Uid = new(imap.SeqSet)
	crit.Uid.AddRange(uint32(last+1), 0)
	uids, err := c.UidSearch(crit)
	if err != nil {
		return movedTo, err
	}
	var fresh []uint32
	for _, u := range uids {
		if int64(u) > last {
			fresh = append(fresh, u)
		}
	}
	if len(fresh) == 0 {
		return movedTo, nil
	}
	sort.Slice(fresh, func(i, j int) bool { return fresh[i] < fresh[j] })
	if len(fresh) > 300 {
		fresh = fresh[:300] // the rest on the next round
	}
	msgs, err := fetchSummaries(c, account, "INBOX", fresh)
	if err != nil {
		return movedTo, err
	}
	byUID := map[uint32]Summary{}
	for _, m := range msgs {
		byUID[m.UID] = m
	}
	moved := false
	for _, uid := range fresh {
		m, ok := byUID[uid]
		// Sender rules apply to any letter; the AI only reads fresh unread ones (not the archive
		// fetched from Gmail: tens of thousands of old letters would keep Loggi busy for days).
		useAI := !m.Seen && time.Since(m.Date) < 3*24*time.Hour
		if ok {
			if folder, reason := s.pickFolder(ctx, c, account, m, rules, useAI); folder != "" {
				set := new(imap.SeqSet)
				set.AddNum(uid)
				if err := c.UidMove(set, folder); err != nil {
					log.Printf("sorter %s: move %d to %s: %v", account, uid, folder, err)
				} else {
					moved = true
					movedTo[uid] = folder
					_, _ = s.db.Exec(ctx, `INSERT INTO sort_log(account, folder, subject, sender, reason) VALUES ($1,$2,$3,$4,$5)`,
						account, folder, truncate(m.Subject, 200), m.From.Email, reason)
				}
			}
		}
		_, _ = s.db.Exec(ctx, `UPDATE sort_state SET last_uid=$2 WHERE account=$1`, account, int64(uid))
	}
	if moved {
		s.hub.countsChanged(account)
	}
	return movedTo, nil
}

// hiddenFolders: folders of the mailbox whose letters must never appear in the Inbox.
func (s *Server) hiddenFolders(ctx context.Context, account string) map[string]bool {
	out := map[string]bool{}
	if rows, err := s.db.Query(ctx, `SELECT folder FROM folder_rules WHERE account=$1 AND auto AND hide_inbox`, account); err == nil {
		for rows.Next() {
			var f string
			if rows.Scan(&f) == nil {
				out[f] = true
			}
		}
		rows.Close()
	}
	return out
}

func (s *Server) autoRules(ctx context.Context, account string) ([]folderRule, error) {
	rows, err := s.db.Query(ctx, `SELECT account, folder, description, senders, auto FROM folder_rules WHERE account=$1 AND auto ORDER BY folder`, account)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []folderRule
	for rows.Next() {
		var f folderRule
		if rows.Scan(&f.Account, &f.Folder, &f.Description, &f.Senders, &f.Auto) == nil {
			out = append(out, f)
		}
	}
	return out, nil
}

// pickFolder: sender rules first (exact address, then @domain), then the AI by description.
func (s *Server) pickFolder(ctx context.Context, c *client.Client, account string, m Summary, rules []folderRule, useAI bool) (string, string) {
	from := strings.ToLower(m.From.Email)
	for _, r := range rules {
		for _, snd := range r.Senders {
			// "@ozon.ru" also covers its subdomains: seller.ozon.ru, sender.ozon.ru…
			if snd == from || strings.HasPrefix(snd, "@") && (strings.HasSuffix(from, snd) || strings.HasSuffix(from, "."+snd[1:])) {
				return r.Folder, "sender"
			}
		}
	}
	if !useAI {
		return "", ""
	}
	var described []folderRule
	for _, r := range rules {
		if strings.TrimSpace(r.Description) != "" {
			described = append(described, r)
		}
	}
	if len(described) == 0 || !s.aiConfigured() {
		return "", ""
	}
	p, err := s.loadPrompt(ctx, "sort")
	if err != nil || !p.Enabled {
		return "", ""
	}
	var list strings.Builder
	for i, r := range described {
		fmt.Fprintf(&list, "%d. %s — %s\n", i+1, r.Folder, r.Description)
	}
	in := aiInput{From: m.From.Email, Subject: m.Subject, Letter: letterText(c, m), Folders: list.String(), Mailbox: account}
	if m.From.Name != "" {
		in.From = m.From.Name + " <" + m.From.Email + ">"
	}
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	ans, err := s.complete(ctx, p, fillTemplate(p.Template, in, ""))
	if err != nil {
		log.Printf("sorter %s: ai: %v", account, err)
		return "", ""
	}
	n, _ := strconv.Atoi(reNum.FindString(ans))
	if n < 1 || n > len(described) {
		return "", ""
	}
	return described[n-1].Folder, "ai"
}

// letterText returns up to ~4000 characters of the letter's text for the AI.
func letterText(c *client.Client, m Summary) string {
	set := new(imap.SeqSet)
	set.AddNum(m.UID)
	msg, err := fetchOne(c, set, []imap.FetchItem{imap.FetchUid, imap.FetchBodyStructure})
	if err != nil || msg == nil || msg.BodyStructure == nil {
		return m.Preview
	}
	path, enc, cs, isHTML := previewPart(msg.BodyStructure)
	if path == nil {
		return m.Preview
	}
	raw, err := fetchPart(c, m.UID, path)
	if err != nil {
		return m.Preview
	}
	t := string(decodePart(raw, enc, cs))
	if isHTML {
		t = html.UnescapeString(reTag.ReplaceAllString(reStyleScript.ReplaceAllString(t, " "), " "))
	}
	t = strings.TrimSpace(reSpaces.ReplaceAllString(t, " "))
	if r := []rune(t); len(r) > 4000 {
		t = string(r[:4000])
	}
	return t
}

// fileBySenders moves every Inbox letter from the rule's senders into its folder at once — the
// letters that came before the rule was made (or were fetched from Gmail): the sorter itself only
// looks at new ones. The search runs without the full-text index (see dialSearch).
func (s *Server) fileBySenders(account, folder string, senders []string) {
	if len(senders) == 0 {
		return
	}
	mu, _ := sortLocks.LoadOrStore(account, &sync.Mutex{})
	mu.(*sync.Mutex).Lock()
	defer mu.(*sync.Mutex).Unlock()
	sc, err := s.dialSearch(account)
	if err != nil {
		log.Printf("file by senders %s: %v", account, err)
		return
	}
	defer sc.Logout()
	if _, err := sc.Select("INBOX", true); err != nil {
		return
	}
	seen := map[uint32]bool{}
	var uids []uint32
	for _, snd := range senders {
		crit := imap.NewSearchCriteria()
		crit.Header.Add("From", snd)
		if strings.HasPrefix(snd, "@") { // the domain and its subdomains
			sub := imap.NewSearchCriteria()
			sub.Header.Add("From", "."+snd[1:])
			crit = imap.NewSearchCriteria()
			crit.Or = [][2]*imap.SearchCriteria{{headerCrit("From", snd), sub}}
		}
		found, err := sc.UidSearch(crit)
		if err != nil {
			log.Printf("file by senders %s %s: %v", account, snd, err)
			continue
		}
		for _, u := range found {
			if !seen[u] {
				seen[u] = true
				uids = append(uids, u)
			}
		}
	}
	if len(uids) == 0 {
		return
	}
	c, err := s.dialAs(account)
	if err != nil {
		return
	}
	defer c.Logout()
	c.Timeout = 15 * time.Minute // moving thousands of letters at once
	if _, err := c.Select("INBOX", false); err != nil {
		return
	}
	moved := 0
	for i := 0; i < len(uids); i += 1000 {
		set := new(imap.SeqSet)
		set.AddNum(uids[i:min(i+1000, len(uids))]...)
		if err := c.UidMove(set, folder); err != nil {
			log.Printf("file by senders %s → %s: %v", account, folder, err)
			break
		}
		moved += min(1000, len(uids)-i)
		s.hub.countsChanged(account)
	}
	ctx := context.Background()
	_, _ = s.db.Exec(ctx, `INSERT INTO sort_log(account, folder, subject, sender, reason) VALUES ($1,$2,$3,$4,'sender')`,
		account, folder, fmt.Sprintf("Previously received messages moved: %d", moved), strings.Join(senders, ", "))
	log.Printf("file by senders %s → %s: %d letters", account, folder, moved)
}

// fileBySenderRules moves the given new Inbox letters whose sender has a folder rule (no AI) and
// returns where each went.
func (s *Server) fileBySenderRules(account string, msgs []Summary) map[uint32]string {
	movedTo := map[uint32]string{}
	ctx := context.Background()
	rules, err := s.autoRules(ctx, account)
	if err != nil || len(rules) == 0 {
		return movedTo
	}
	byFolder := map[string][]Summary{}
	for _, m := range msgs {
		if f, _ := s.pickFolder(ctx, nil, account, m, rules, false); f != "" {
			byFolder[f] = append(byFolder[f], m)
		}
	}
	if len(byFolder) == 0 {
		return movedTo
	}
	mu, _ := sortLocks.LoadOrStore(account, &sync.Mutex{})
	mu.(*sync.Mutex).Lock()
	defer mu.(*sync.Mutex).Unlock()
	c, err := s.dialAs(account)
	if err != nil {
		return movedTo
	}
	defer c.Logout()
	if _, err := c.Select("INBOX", false); err != nil {
		return movedTo
	}
	for f, list := range byFolder {
		set := new(imap.SeqSet)
		for _, m := range list {
			set.AddNum(m.UID)
		}
		if err := c.UidMove(set, f); err != nil {
			log.Printf("sorter %s: move new to %s: %v", account, f, err)
			continue
		}
		for _, m := range list {
			movedTo[m.UID] = f
			_, _ = s.db.Exec(ctx, `INSERT INTO sort_log(account, folder, subject, sender, reason) VALUES ($1,$2,$3,$4,'sender')`,
				account, f, truncate(m.Subject, 200), m.From.Email)
		}
	}
	s.hub.countsChanged(account)
	return movedTo
}
