package main

import (
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap/client"
)

// Conversations: a message plus everything that references the same first message, collected from
// Inbox, Sent and Archive of the same mailbox (and the folder it was opened from), oldest first.

var reMsgID = regexp.MustCompile(`<[^<>\s]+>`)

func msgIDs(h string) []string { return reMsgID.FindAllString(h, -1) }

const maxThread = 100

func (s *Server) handleThread(w http.ResponseWriter, r *http.Request, sess *Session) {
	q := r.URL.Query()
	uid, err := strconv.ParseUint(q.Get("uid"), 10, 32)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "No message specified")
		return
	}
	s.withAccount(w, r, sess, q.Get("account"), false, func(c *client.Client, acc Account) {
		folder, err := s.resolveFolder(c, acc.Address, r)
		if err != nil {
			writeErr(w, http.StatusNotFound, "Folder not found")
			return
		}
		if _, err := c.Select(folder, true); err != nil {
			writeErr(w, http.StatusNotFound, "Folder not found")
			return
		}
		self, err := fetchSummaries(c, acc.Address, folder, []uint32{uint32(uid)})
		if err != nil || len(self) == 0 {
			writeErr(w, http.StatusNotFound, "Message not found")
			return
		}
		m := self[0]
		ids := map[string]bool{}
		for _, id := range append([]string{m.MessageID, m.InReplyTo}, m.References...) {
			if id != "" && len(ids) < 30 {
				ids[id] = true
			}
		}
		root := m.MessageID
		if len(m.References) > 0 {
			root = m.References[0]
		} else if m.InReplyTo != "" {
			root = m.InReplyTo
		}
		if len(ids) == 0 {
			s.attachSentBy(r.Context(), acc.Address, self)
			writeJSON(w, http.StatusOK, map[string]any{"messages": self})
			return
		}
		// Everything whose Message-ID is one we know, or that refers to the root or to this message.
		var terms []*imap.SearchCriteria
		for id := range ids {
			terms = append(terms, headerCrit("Message-ID", id))
		}
		terms = append(terms, headerCrit("References", root))
		if m.MessageID != "" {
			terms = append(terms, headerCrit("In-Reply-To", m.MessageID))
		}
		base := baseSubject(m.Subject)
		if len([]rune(base)) >= 3 {
			c := imap.NewSearchCriteria()
			c.Header.Add("Subject", base)
			terms = append(terms, c)
		}
		crit := orAll(terms)
		mine := participants(m)
		// Subject matches count only when they share a correspondent with this letter.
		related := func(x Summary) bool {
			for _, id := range append([]string{x.MessageID, x.InReplyTo}, x.References...) {
				if id != "" && (ids[id] || id == root || id == m.MessageID) {
					return true
				}
			}
			if base == "" || baseSubject(x.Subject) != base {
				return false
			}
			for e := range participants(x) {
				if mine[e] {
					return true
				}
			}
			return false
		}

		folders := []string{folder}
		for _, role := range []string{"inbox", "sent", "archive"} {
			if f, err := s.folders.byRole(c, acc.Address, role); err == nil && f != folder {
				folders = append(folders, f)
			}
		}
		// Searched on sessions without the full-text index: a subject like "New order received" is in
		// 60 000 letters, the index takes ~25 s for that and the header cache ~2 s (see dialSearch).
		// All folders at once, each on its own session.
		hits := make([][]uint32, len(folders))
		var wg sync.WaitGroup
		for i, f := range folders {
			wg.Add(1)
			go func() {
				defer wg.Done()
				if u, err := s.searchHeaders(acc.Address, f, crit); err == nil {
					hits[i] = u
				}
			}()
		}
		wg.Wait()
		var all []Summary
		seen := map[string]bool{}
		for i, f := range folders {
			uids := hits[i]
			if len(uids) == 0 {
				continue
			}
			if _, err := c.Select(f, true); err != nil {
				continue
			}
			sort.Slice(uids, func(i, j int) bool { return uids[i] > uids[j] })
			if len(uids) > maxThread {
				uids = uids[:maxThread]
			}
			found, err := fetchSummaries(c, acc.Address, f, uids)
			if err != nil {
				continue
			}
			// The same letter can be in two folders (sent to ourselves): keep the first one seen,
			// which is the copy in the folder the conversation was opened from.
			for _, x := range found {
				if !related(x) {
					continue
				}
				k := x.MessageID
				if k == "" {
					k = f + "|" + strconv.Itoa(int(x.UID))
				}
				if !seen[k] {
					seen[k] = true
					all = append(all, x)
				}
			}
		}
		sort.SliceStable(all, func(i, j int) bool {
			if !all[i].Date.Equal(all[j].Date) {
				return all[i].Date.Before(all[j].Date)
			}
			return all[i].Folder == all[j].Folder && all[i].UID < all[j].UID
		})
		if len(all) > maxThread {
			all = all[len(all)-maxThread:]
		}
		s.attachSentBy(r.Context(), acc.Address, all)
		writeJSON(w, http.StatusOK, map[string]any{"messages": all})
	})
}

// Reply and forward prefixes in several languages, Russian included: detection data.
var reSubjPrefix = regexp.MustCompile(`(?i)^\s*((re|fwd?|aw|wg|отв|пересл|ответ)(\[\d+\])?\s*:\s*)+`)

// baseSubject is the subject without Re:/Fwd: prefixes, lower-cased, with spaces collapsed.
func baseSubject(s string) string {
	return strings.ToLower(strings.Join(strings.Fields(reSubjPrefix.ReplaceAllString(s, "")), " "))
}

func participants(m Summary) map[string]bool {
	out := map[string]bool{}
	for _, a := range append(append([]Addr{m.From}, m.To...), m.Cc...) {
		if e := strings.ToLower(a.Email); e != "" && e != strings.ToLower(m.Account) {
			out[e] = true
		}
	}
	return out
}

func headerCrit(k, v string) *imap.SearchCriteria {
	c := imap.NewSearchCriteria()
	c.Header.Add(k, v)
	return c
}

// orAll folds criteria into nested ORs (IMAP's OR takes exactly two keys).
func orAll(cs []*imap.SearchCriteria) *imap.SearchCriteria {
	if len(cs) == 1 {
		return cs[0]
	}
	mid := len(cs) / 2
	out := imap.NewSearchCriteria()
	out.Or = [][2]*imap.SearchCriteria{{orAll(cs[:mid]), orAll(cs[mid:])}}
	return out
}
