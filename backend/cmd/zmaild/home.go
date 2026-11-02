package main

import (
	"context"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/emersion/go-imap"
)

// The home page: the latest files that came in or went out, across all of the user's mailboxes
// (the newest letters of Inbox and Sent are looked at, not the whole mailbox).

type homeFile struct {
	Account string     `json:"account"`
	Folder  string     `json:"folder"`
	UID     uint32     `json:"uid"`
	Date    time.Time  `json:"date"`
	From    Addr       `json:"from"`
	Subject string     `json:"subject"`
	Sent    bool       `json:"sent"`
	File    Attachment `json:"file"`
}

var homeCache sync.Map // user id → homeCached

type homeCached struct {
	at    time.Time
	files []homeFile
}

// GET /api/home/files
func (s *Server) handleHomeFiles(w http.ResponseWriter, r *http.Request, sess *Session) {
	if c, ok := homeCache.Load(sess.UserID); ok && time.Since(c.(homeCached).at) < 2*time.Minute && r.URL.Query().Get("fresh") == "" {
		writeJSON(w, http.StatusOK, c.(homeCached).files)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	var mu sync.Mutex
	var all []homeFile
	var wg sync.WaitGroup
	sem := make(chan struct{}, 4)
	for i, a := range sess.Accounts {
		if i >= 12 {
			break
		}
		wg.Add(1)
		go func(acc string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			files := s.recentFiles(ctx, acc)
			mu.Lock()
			all = append(all, files...)
			mu.Unlock()
		}(a.Address)
	}
	wg.Wait()
	sort.Slice(all, func(i, j int) bool { return all[i].Date.After(all[j].Date) })
	// The same file sent around (a reply quoting it): only its newest copy.
	seen := map[string]bool{}
	out := []homeFile{}
	for _, f := range all {
		k := strings.ToLower(f.File.Name) + "|" + f.Subject
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, f)
		if len(out) >= 24 {
			break
		}
	}
	homeCache.Store(sess.UserID, homeCached{time.Now(), out})
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) recentFiles(ctx context.Context, account string) []homeFile {
	c, err := s.dialAs(account)
	if err != nil {
		return nil
	}
	defer c.Logout()
	var out []homeFile
	for _, role := range []string{"inbox", "sent"} {
		if ctx.Err() != nil {
			break
		}
		folder, err := s.folders.byRole(c, account, role)
		if err != nil {
			continue
		}
		mbox, err := c.Select(folder, true)
		if err != nil || mbox.Messages == 0 {
			continue
		}
		set := new(imap.SeqSet)
		n := uint32(80)
		set.AddRange(max(1, mbox.Messages-min(mbox.Messages, n)+1), mbox.Messages)
		ch := make(chan *imap.Message, 16)
		done := make(chan error, 1)
		go func() {
			done <- c.Fetch(set, []imap.FetchItem{imap.FetchUid, imap.FetchEnvelope, imap.FetchBodyStructure, imap.FetchInternalDate}, ch)
		}()
		for m := range ch {
			if m.BodyStructure == nil || m.Envelope == nil {
				continue
			}
			atts := fileAttachments(attachmentsOf(m.BodyStructure))
			if len(atts) == 0 {
				continue
			}
			from := Addr{}
			if len(m.Envelope.From) > 0 {
				from = addrOf(m.Envelope.From[0])
			}
			date := m.Envelope.Date
			if date.IsZero() {
				date = m.InternalDate
			}
			for _, a := range atts {
				if strings.HasSuffix(strings.ToLower(a.Name), ".ics") || a.Size < 1024 {
					continue // invitations and signature trifles
				}
				out = append(out, homeFile{Account: account, Folder: folder, UID: m.Uid, Date: date, From: from,
					Subject: m.Envelope.Subject, Sent: role == "sent", File: a})
			}
		}
		<-done
	}
	return out
}
