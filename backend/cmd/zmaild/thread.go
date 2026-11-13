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
// Inbox, Sent, Archive and the own folders of the same mailbox (and the folder it was opened from),
// oldest first.

var reMsgID = regexp.MustCompile(`<[^<>\s]+>`)

func msgIDs(h string) []string { return reMsgID.FindAllString(h, -1) }

const maxThread = 100

// Links followed one step per round for conversations whose letters do not name their root;
// each round searches all folders once.
const (
	maxThreadRounds = 8
	threadRoundIDs  = 20
)

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
		base := baseSubject(m.Subject)
		folders := []string{folder}
		for _, role := range []string{"inbox", "sent", "archive"} {
			if f, err := s.folders.byRole(c, acc.Address, role); err == nil && f != folder {
				folders = append(folders, f)
			}
		}
		// The own folders too: auto-sorting files a reply there while the rest stays in the Inbox.
		if all, err := listFolders(c, false); err == nil {
			for _, f := range all {
				if f.Role == "" && f.Name != folder {
					folders = append(folders, f.Name)
				}
			}
		}

		// Round 1: everything whose Message-ID is one we know, or that refers to the root or to this
		// message. A well-formed conversation is complete after it: every reply names the root first.
		// Some senders (CRM systems) give only In-Reply-To and no References; their replies link one
		// step back each, so the next rounds follow those links from the letters just found.
		var terms []*imap.SearchCriteria
		for id := range ids {
			terms = append(terms, headerCrit("Message-ID", id))
		}
		terms = append(terms, headerCrit("References", root))
		if m.MessageID != "" {
			terms = append(terms, headerCrit("In-Reply-To", m.MessageID))
		}
		searched := map[string]bool{root: true, m.MessageID: true}
		var all []Summary
		seen := map[string]bool{}    // letters kept, by Message-ID (the same letter in two folders: once)
		fetched := map[string]bool{} // folder|uid already read
		for round := 0; round < maxThreadRounds && len(terms) > 0 && len(all) < maxThread; round++ {
			found := s.threadRound(c, acc.Address, folders, orAll(terms), fetched)
			before := len(all)
			for _, x := range found {
				// Like Gmail: a letter belongs to the conversation when it refers to one of its letters
				// AND has the same subject without Re:/Fwd: — the subject alone never joins, a new one splits.
				if baseSubject(x.Subject) != base || !linked(x, ids) {
					continue
				}
				k := x.MessageID
				if k == "" {
					k = x.Folder + "|" + strconv.Itoa(int(x.UID))
				}
				if !seen[k] {
					seen[k] = true
					all = append(all, x)
				}
			}
			// Everything found so far names the same root: nothing more to follow.
			if anchored(all, root) || len(all) == before {
				break
			}
			terms = nil
			for _, x := range all[before:] {
				for _, id := range append([]string{x.MessageID, x.InReplyTo}, x.References...) {
					if id == "" {
						continue
					}
					ids[id] = true
					if searched[id] || len(terms) >= 3*threadRoundIDs {
						continue
					}
					searched[id] = true
					terms = append(terms, headerCrit("Message-ID", id), headerCrit("In-Reply-To", id), headerCrit("References", id))
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
// Spam tags put before the subject by other servers ("***SPAM*** Re: Order") count as prefixes too.
var reSubjPrefix = regexp.MustCompile(`(?i)^\s*(((re|fwd?|aw|wg|отв|пересл|ответ)(\[\d+\])?\s*:|\*+\s*spam\s*\*+|\[spam\])\s*)+`)

// baseSubject is the subject without Re:/Fwd: prefixes, lower-cased, with spaces collapsed.
func baseSubject(s string) string {
	return strings.ToLower(strings.Join(strings.Fields(reSubjPrefix.ReplaceAllString(s, "")), " "))
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

// linked: the letter refers to, or is, one of the conversation's letters.
func linked(x Summary, ids map[string]bool) bool {
	for _, id := range append([]string{x.MessageID, x.InReplyTo}, x.References...) {
		if id != "" && ids[id] {
			return true
		}
	}
	return false
}

// anchored: every letter is the root or names it first in References, and the root itself is
// the start of the conversation, not a reply to something else.
func anchored(all []Summary, root string) bool {
	for _, x := range all {
		if x.MessageID == root {
			if x.InReplyTo != "" || len(x.References) > 0 {
				return false
			}
		} else if len(x.References) == 0 || x.References[0] != root {
			return false
		}
	}
	return true
}

// threadRound searches the folders (each on its own session, all at once) and reads the letters
// not read yet.
func (s *Server) threadRound(c *client.Client, account string, folders []string, crit *imap.SearchCriteria, fetched map[string]bool) []Summary {
	hits := make([][]uint32, len(folders))
	var wg sync.WaitGroup
	for i, f := range folders {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// Searched on sessions without the full-text index: a subject like "New order received" is
			// in 60 000 letters, the index takes ~25 s for that and the header cache ~2 s (see dialSearch).
			if u, err := s.searchHeaders(account, f, crit); err == nil {
				hits[i] = u
			}
		}()
	}
	wg.Wait()
	var out []Summary
	for i, f := range folders {
		var uids []uint32
		for _, u := range hits[i] {
			if k := f + "|" + strconv.Itoa(int(u)); !fetched[k] {
				fetched[k] = true
				uids = append(uids, u)
			}
		}
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
		// Folder by folder in the order given: the copy in the folder the conversation was opened
		// from comes first and is the one kept.
		if found, err := fetchSummaries(c, account, f, uids); err == nil {
			out = append(out, found...)
		}
	}
	return out
}
