package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"sort"
	"time"

	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap/client"
)

// The Inbox as one feed: letters filed into the user's own folders (by hand or by auto-sorting)
// stay in the Inbox list, like Gmail labels, unless the folder is marked "don't show these letters
// in the Inbox". System folders (Sent, Drafts, Archive, Spam, Trash…) are not part of it.
//
// Every folder is read newest UID first; the folders are merged by date. The cursor holds, for each
// folder not yet finished, the UID below which its next page starts.

// feedFolders: the Inbox and the own folders shown with it; nil when the mailbox has no such folder.
func (s *Server) feedFolders(ctx context.Context, c *client.Client, account string) ([]string, error) {
	all, err := listFolders(c, false)
	if err != nil {
		return nil, err
	}
	hidden := map[string]bool{}
	if rows, err := s.db.Query(ctx, `SELECT folder FROM folder_rules WHERE account=$1 AND hide_inbox`, account); err == nil {
		for rows.Next() {
			var f string
			if rows.Scan(&f) == nil {
				hidden[f] = true
			}
		}
		rows.Close()
	}
	var inbox string
	var own []string
	for _, f := range all {
		switch {
		case f.Role == "inbox":
			inbox = f.Name
		case f.Role == "" && !hidden[f.Name]:
			own = append(own, f.Name)
		}
	}
	if inbox == "" || len(own) == 0 {
		return nil, nil
	}
	return append([]string{inbox}, own...), nil
}

type feedItem struct {
	uid  uint32
	date time.Time
	mid  string
}

func decodeFeedCursor(s string) map[string]uint32 {
	if s == "" {
		return nil
	}
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return nil
	}
	var m map[string]uint32
	if json.Unmarshal(b, &m) != nil {
		return nil
	}
	return m
}

// inboxFeed returns a page of the feed and the cursor of the next one ("" = no more).
func (s *Server) inboxFeed(c *client.Client, account string, folders []string, base func() *imap.SearchCriteria, cursor string, limit int) ([]Summary, string, error) {
	bounds := decodeFeedCursor(cursor) // nil: the first page
	type stream struct {
		folder string
		items  []feedItem // newest UID first
		pos    int
		more   bool // letters below the read ones
	}
	var streams []*stream
	for _, f := range folders {
		bound, ok := bounds[f]
		if bounds != nil && !ok {
			continue // this folder is finished
		}
		mbox, err := c.Select(f, true)
		if err != nil {
			continue
		}
		var uids []uint32
		if bounds == nil && !hasFilters(base()) {
			uids, err = lastUIDs(c, mbox.Messages, uint32(limit)+1)
		} else {
			if bounds != nil && bound <= 1 {
				continue
			}
			crit := base()
			if bounds != nil {
				crit.Uid = new(imap.SeqSet)
				crit.Uid.AddRange(1, bound-1)
			}
			uids, err = c.UidSearch(crit)
		}
		if err != nil {
			return nil, "", err
		}
		sort.Slice(uids, func(i, j int) bool { return uids[i] > uids[j] })
		st := &stream{folder: f, more: len(uids) > limit+1}
		if bounds == nil && !hasFilters(base()) {
			st.more = mbox.Messages > uint32(len(uids))
		}
		if len(uids) > limit+1 {
			uids = uids[:limit+1]
		}
		if len(uids) > 0 {
			set := new(imap.SeqSet)
			set.AddNum(uids...)
			ch := make(chan *imap.Message, 16)
			done := make(chan error, 1)
			go func() { done <- c.UidFetch(set, []imap.FetchItem{imap.FetchUid, imap.FetchInternalDate, imap.FetchEnvelope}, ch) }()
			got := map[uint32]feedItem{}
			for m := range ch {
				it := feedItem{uid: m.Uid, date: m.InternalDate}
				if m.Envelope != nil {
					it.mid = m.Envelope.MessageId
					if it.date.IsZero() {
						it.date = m.Envelope.Date
					}
				}
				got[m.Uid] = it
			}
			if err := <-done; err != nil {
				return nil, "", err
			}
			for _, u := range uids {
				if it, ok := got[u]; ok {
					st.items = append(st.items, it)
				}
			}
		}
		if len(st.items) > 0 || st.more {
			streams = append(streams, st)
		}
	}

	// Merge: the newest of the folders' next letters, until the page is full. A folder whose read
	// letters run out while it has older ones ends the page: nothing past them can be compared.
	chosen := map[string][]uint32{}
	seen := map[string]bool{} // the same letter in two folders: shown once
	n := 0
	for n < limit {
		var best *stream
		stop := false
		for _, st := range streams {
			if st.pos >= len(st.items) {
				if st.more {
					stop = true
				}
				continue
			}
			if best == nil || st.items[st.pos].date.After(best.items[best.pos].date) {
				best = st
			}
		}
		if stop || best == nil {
			break
		}
		it := best.items[best.pos]
		best.pos++
		if it.mid != "" && seen[it.mid] {
			continue
		}
		if it.mid != "" {
			seen[it.mid] = true
		}
		chosen[best.folder] = append(chosen[best.folder], it.uid)
		n++
	}

	next := map[string]uint32{}
	for _, st := range streams {
		switch {
		case st.pos < len(st.items):
			// the next page starts at its first letter not shown
			next[st.folder] = st.items[st.pos].uid + 1
		case st.more && len(st.items) > 0:
			next[st.folder] = st.items[len(st.items)-1].uid
		case st.more:
			if b, ok := bounds[st.folder]; ok {
				next[st.folder] = b
			}
		}
	}

	out := []Summary{}
	for _, f := range folders {
		uids := chosen[f]
		if len(uids) == 0 {
			continue
		}
		if _, err := c.Select(f, true); err != nil {
			continue
		}
		sort.Slice(uids, func(i, j int) bool { return uids[i] < uids[j] })
		msgs, err := fetchSummaries(c, account, f, uids)
		if err != nil {
			return nil, "", err
		}
		out = append(out, msgs...)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Date.After(out[j].Date) })
	if len(next) == 0 {
		return out, "", nil
	}
	b, _ := json.Marshal(next)
	return out, base64.RawURLEncoding.EncodeToString(b), nil
}
