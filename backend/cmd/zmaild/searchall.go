package main

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap/client"
)

// Searching the whole mailbox, like Gmail: Inbox, Sent, Drafts, Archive and every own folder at
// once (not Trash and Spam), newest first. The search index holds all those folders, so one
// query answers; letters that arrived after the index last ran, and folders it has not finished,
// are searched by their headers in Dovecot. Pages go back in time: the cursor is a date.

type foundRef struct {
	folder string
	uid    uint32
	date   time.Time
}

// searchEverywhere returns a page of letters matching text, older than beforeMs (0 = newest),
// and the cursor of the next page (0 = no more).
func (s *Server) searchEverywhere(ctx context.Context, c *client.Client, account, text string, base func() *imap.SearchCriteria, beforeMs int64, limit int) ([]Summary, int64, error) {
	folders, err := listFolders(c, false)
	if err != nil {
		return nil, 0, err
	}
	var names []string
	for _, f := range folders {
		if f.Role != "trash" && f.Role != "junk" {
			names = append(names, f.Name)
		}
	}
	type state struct {
		last     int64
		complete bool
	}
	states := map[string]state{}
	if rows, err := s.db.Query(ctx, `SELECT folder, last_uid, complete FROM msg_index_state WHERE account=$1`, account); err == nil {
		for rows.Next() {
			var f string
			var st state
			if rows.Scan(&f, &st.last, &st.complete) == nil {
				states[f] = st
			}
		}
		rows.Close()
	}
	crit := base()
	var before time.Time
	if beforeMs > 0 {
		before = time.UnixMilli(beforeMs)
	}

	// The index: every indexed folder in one query.
	var indexed []string
	for _, n := range names {
		if states[n].complete {
			indexed = append(indexed, n)
		}
	}
	var refs []foundRef
	if len(indexed) > 0 {
		run := func(t string) error {
			cond, args := indexMatch(t, []any{account, indexed})
			q := `SELECT folder, uid, COALESCE(date, 'epoch') FROM msg_index WHERE account=$1 AND folder = ANY($2) AND ` + cond
			if !before.IsZero() {
				args = append(args, before)
				q += fmt.Sprintf(` AND date < $%d`, len(args))
			}
			if !crit.Since.IsZero() {
				args = append(args, crit.Since)
				q += fmt.Sprintf(` AND date >= $%d`, len(args))
			}
			if !crit.Before.IsZero() {
				args = append(args, crit.Before)
				q += fmt.Sprintf(` AND date < $%d`, len(args))
			}
			// With flag filters (unread, flagged) more rows are read: some are dropped below.
			n := limit + 1
			if len(crit.WithFlags)+len(crit.WithoutFlags) > 0 {
				n = limit * 20
			}
			q += fmt.Sprintf(` ORDER BY date DESC NULLS LAST LIMIT %d`, n)
			c2, cancel := context.WithTimeout(ctx, 15*time.Second)
			defer cancel()
			tx, err := s.db.Begin(c2)
			if err != nil {
				return err
			}
			defer tx.Rollback(c2)
			_, _ = tx.Exec(c2, "SET LOCAL enable_seqscan = off")
			rows, err := tx.Query(c2, q, args...)
			if err != nil {
				return err
			}
			defer rows.Close()
			for rows.Next() {
				var r foundRef
				var u int64
				if rows.Scan(&r.folder, &u, &r.date) == nil {
					r.uid = uint32(u)
					refs = append(refs, r)
				}
			}
			return rows.Err()
		}
		if err := run(text); err != nil {
			return nil, 0, err
		}
		if len(refs) == 0 {
			if alt := ruLayout(text); alt != "" { // typed in the wrong keyboard layout
				_ = run(alt)
			}
		}
	}

	// Newer than the index, and folders the index has not finished: their headers in Dovecot.
	// (Only on the first page: older pages come from the index alone.)
	if beforeMs == 0 {
		for _, n := range names {
			st := states[n]
			hc := withHeaderText(base(), text)
			if st.complete {
				hc.Uid = new(imap.SeqSet)
				hc.Uid.AddRange(uint32(st.last+1), 0)
			}
			uids, err := s.searchHeaders(account, n, hc)
			if err != nil || len(uids) == 0 {
				continue
			}
			sort.Slice(uids, func(i, j int) bool { return uids[i] > uids[j] })
			for i, u := range uids {
				if i >= limit || (st.complete && int64(u) <= st.last) {
					break
				}
				refs = append(refs, foundRef{folder: n, uid: u, date: time.Now()}) // fresh: dated by the fetch below
			}
		}
	}

	// Letters per folder, with the flag filters applied there.
	byFolder := map[string][]uint32{}
	for _, r := range refs {
		byFolder[r.folder] = append(byFolder[r.folder], r.uid)
	}
	out := []Summary{} // never null: the page lists it
	for f, uids := range byFolder {
		if _, err := c.Select(f, true); err != nil {
			continue
		}
		if len(crit.WithFlags)+len(crit.WithoutFlags) > 0 {
			fc := base()
			fc.Uid = new(imap.SeqSet)
			fc.Uid.AddNum(uids...)
			if keep, err := c.UidSearch(fc); err == nil {
				uids = keep
			}
		}
		if len(uids) == 0 {
			continue
		}
		sort.Slice(uids, func(i, j int) bool { return uids[i] < uids[j] })
		msgs, err := fetchSummaries(c, account, f, uids)
		if err != nil {
			continue
		}
		out = append(out, msgs...)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Date.After(out[j].Date) })
	// The same letter found twice (index and the header search): once.
	seen := map[string]bool{}
	uniq := out[:0]
	for _, m := range out {
		k := m.Folder + "\x00" + fmt.Sprint(m.UID)
		if !seen[k] {
			seen[k] = true
			uniq = append(uniq, m)
		}
	}
	out = uniq
	var cursor int64
	if len(out) > limit {
		out = out[:limit]
	}
	if len(refs) > limit && len(out) > 0 {
		cursor = out[len(out)-1].Date.UnixMilli()
	}
	return out, cursor, nil
}
