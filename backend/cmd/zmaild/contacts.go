package main

import (
	"context"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/emersion/go-imap"
	"github.com/jackc/pgx/v5"
)

// The address book is filled from the mailboxes themselves: senders of INBOX messages and
// recipients of Sent messages. Sync is incremental (last UID per folder), so nothing is counted twice.

const (
	contactSyncEvery   = 2 * time.Minute
	contactBackfillMax = 3000
)

type contactSyncer struct {
	mu      sync.Mutex
	running map[string]bool
	last    map[string]time.Time
}

func newContactSyncer() *contactSyncer {
	return &contactSyncer{running: map[string]bool{}, last: map[string]time.Time{}}
}

// syncSoon starts a background sync for each mailbox unless one ran recently.
func (s *Server) syncContactsSoon(accounts []Account) {
	for _, a := range accounts {
		addr := a.Address
		cs := s.contacts
		cs.mu.Lock()
		if cs.running[addr] || time.Since(cs.last[addr]) < contactSyncEvery {
			cs.mu.Unlock()
			continue
		}
		cs.running[addr] = true
		cs.mu.Unlock()
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			defer cancel()
			if err := s.syncContacts(ctx, addr); err != nil {
				slog.Warn("contact sync", "mailbox", addr, "err", err)
			}
			cs.mu.Lock()
			cs.running[addr] = false
			cs.last[addr] = time.Now()
			cs.mu.Unlock()
		}()
	}
}

type contactAgg struct {
	name       string
	sent, recv int
	last       time.Time
}

func (s *Server) syncContacts(ctx context.Context, mailbox string) error {
	var mailboxID int64
	var inV, inU, sentV, sentU int64
	if err := s.db.QueryRow(ctx, `SELECT m.id, coalesce(cs.inbox_validity,0), coalesce(cs.inbox_uid,0), coalesce(cs.sent_validity,0), coalesce(cs.sent_uid,0)
		FROM mailboxes m LEFT JOIN contact_sync cs ON cs.mailbox_id = m.id WHERE m.address = $1`, mailbox).
		Scan(&mailboxID, &inV, &inU, &sentV, &sentU); err != nil {
		return err
	}
	c, err := s.dialAs(mailbox)
	if err != nil {
		return err
	}
	defer c.Logout()

	agg := map[string]*contactAgg{}
	add := func(a *imap.Address, when time.Time, sent bool) {
		ad := addrOf(a)
		if !emailRe.MatchString(ad.Email) || ad.Email == mailbox {
			return
		}
		x := agg[ad.Email]
		if x == nil {
			x = &contactAgg{}
			agg[ad.Email] = x
		}
		if ad.Name != "" && !strings.EqualFold(ad.Name, ad.Email) {
			x.name = ad.Name
		}
		if sent {
			x.sent++
		} else {
			x.recv++
		}
		if when.After(x.last) {
			x.last = when
		}
	}
	scan := func(role string, validity, lastUID int64, sent bool) (int64, int64, error) {
		name, err := s.folders.byRole(c, mailbox, role)
		if err != nil {
			return validity, lastUID, nil // no such folder: nothing to learn
		}
		st, err := c.Select(name, true)
		if err != nil {
			return validity, lastUID, err
		}
		if int64(st.UidValidity) != validity {
			lastUID = 0
		}
		if int64(st.UidNext) <= lastUID+1 {
			return int64(st.UidValidity), lastUID, nil
		}
		crit := imap.NewSearchCriteria()
		set := new(imap.SeqSet)
		set.AddRange(uint32(lastUID+1), 0)
		crit.Uid = set
		uids, err := c.UidSearch(crit)
		if err != nil {
			return validity, lastUID, err
		}
		sort.Slice(uids, func(i, j int) bool { return uids[i] < uids[j] })
		var fresh []uint32
		for _, u := range uids {
			if int64(u) > lastUID {
				fresh = append(fresh, u)
			}
		}
		if len(fresh) > contactBackfillMax {
			fresh = fresh[len(fresh)-contactBackfillMax:]
		}
		if len(fresh) == 0 {
			return int64(st.UidValidity), lastUID, nil
		}
		fset := new(imap.SeqSet)
		fset.AddNum(fresh...)
		ch := make(chan *imap.Message, 64)
		done := make(chan error, 1)
		go func() {
			done <- c.UidFetch(fset, []imap.FetchItem{imap.FetchUid, imap.FetchEnvelope, imap.FetchInternalDate}, ch)
		}()
		maxUID := lastUID
		for m := range ch {
			if m.Envelope != nil {
				if sent {
					for _, a := range append(append([]*imap.Address{}, m.Envelope.To...), m.Envelope.Cc...) {
						add(a, m.InternalDate, true)
					}
				} else {
					for _, a := range m.Envelope.From {
						add(a, m.InternalDate, false)
					}
				}
			}
			if int64(m.Uid) > maxUID {
				maxUID = int64(m.Uid)
			}
		}
		if err := <-done; err != nil {
			return validity, lastUID, err
		}
		return int64(st.UidValidity), maxUID, nil
	}
	nInV, nInU, err := scan("inbox", inV, inU, false)
	if err != nil {
		return err
	}
	nSentV, nSentU, err := scan("sent", sentV, sentU, true)
	if err != nil {
		return err
	}

	return pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		// All contacts in one round trip (a batch), not one query each.
		var b pgx.Batch
		for email, x := range agg {
			if x.last.IsZero() {
				x.last = time.Now()
			}
			b.Queue(`
				INSERT INTO contacts(mailbox_id, email, name, sent_count, recv_count, last_at, search) VALUES ($1,$2,$3,$4,$5,$6,$7)
				ON CONFLICT (mailbox_id, email) DO UPDATE SET
				  name = CASE WHEN EXCLUDED.name <> '' THEN EXCLUDED.name ELSE contacts.name END,
				  search = CASE WHEN EXCLUDED.name <> '' THEN EXCLUDED.search ELSE contacts.search END,
				  sent_count = contacts.sent_count + EXCLUDED.sent_count,
				  recv_count = contacts.recv_count + EXCLUDED.recv_count,
				  last_at = greatest(contacts.last_at, EXCLUDED.last_at)`,
				mailboxID, email, truncate(x.name, 200), x.sent, x.recv, x.last, searchKey(email, x.name))
		}
		if b.Len() > 0 {
			if err := tx.SendBatch(ctx, &b).Close(); err != nil {
				return err
			}
		}
		_, err := tx.Exec(ctx, `
			INSERT INTO contact_sync(mailbox_id, inbox_validity, inbox_uid, sent_validity, sent_uid, synced_at) VALUES ($1,$2,$3,$4,$5,now())
			ON CONFLICT (mailbox_id) DO UPDATE SET inbox_validity=EXCLUDED.inbox_validity, inbox_uid=EXCLUDED.inbox_uid,
			  sent_validity=EXCLUDED.sent_validity, sent_uid=EXCLUDED.sent_uid, synced_at=now()`,
			mailboxID, nInV, nInU, nSentV, nSentU)
		return err
	})
}

// handleContacts returns address suggestions across all mailboxes the user works with.
func (s *Server) handleContacts(w http.ResponseWriter, r *http.Request, sess *Session) {
	s.syncContactsSoon(sess.Accounts)
	q := strings.ToLower(strings.Join(strings.Fields(r.URL.Query().Get("q")), " "))
	type hit struct {
		Email    string    `json:"email"`
		Name     string    `json:"name"`
		Sent     int       `json:"sent"`
		Recv     int       `json:"recv"`
		Last     time.Time `json:"last"`
		Company  string    `json:"company,omitempty"`
		Position string    `json:"position,omitempty"`
	}
	out := []hit{}
	if q == "" || len(q) > 100 {
		writeJSON(w, http.StatusOK, out)
		return
	}
	esc := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(q)
	addrs := make([]string, len(sess.Accounts))
	for i, a := range sess.Accounts {
		addrs[i] = a.Address
	}
	rows, err := s.db.Query(r.Context(), `
		SELECT c.email,
		       coalesce((array_agg(c.name ORDER BY c.last_at DESC) FILTER (WHERE c.name <> ''))[1], ''),
		       sum(c.sent_count)::int, sum(c.recv_count)::int, max(c.last_at)
		FROM contacts c JOIN mailboxes m ON m.id = c.mailbox_id
		WHERE m.address = ANY($1) AND c.search LIKE '%' || $2 || '%'
		GROUP BY c.email
		ORDER BY max(c.last_at) DESC NULLS LAST, sum(c.sent_count) * 3 + sum(c.recv_count) DESC
		LIMIT 10`, addrs, esc)
	if err != nil {
		serverErr(w, "contacts", err)
		return
	}
	out, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (hit, error) {
		var h hit
		err := row.Scan(&h.Email, &h.Name, &h.Sent, &h.Recv, &h.Last)
		return h, err
	})
	if err != nil {
		serverErr(w, "contacts scan", err)
		return
	}
	// Counterparty cards: typing a company name ("Service") offers the people who work there.
	crmHits := []hit{}
	if crows, err := s.db.Query(r.Context(), `
		SELECT e, p.name, COALESCE(c.name, CASE WHEN p.kind = 'company' THEN p.name ELSE '' END), p.position, p.last_letter
		FROM crm_parties p LEFT JOIN crm_parties c ON c.id = p.company_id, unnest(p.emails) e
		WHERE p.search LIKE '%' || $1 || '%' OR c.search LIKE '%' || $1 || '%'
		ORDER BY (c.search LIKE $1 || '%' OR p.search LIKE $1 || '%') DESC, p.last_letter DESC NULLS LAST
		LIMIT 10`, esc); err == nil {
		for crows.Next() {
			var h hit
			var last *time.Time
			if crows.Scan(&h.Email, &h.Name, &h.Company, &h.Position, &last) == nil {
				if last != nil {
					h.Last = *last
				}
				if h.Company == h.Name {
					h.Name = ""
				}
				crmHits = append(crmHits, h)
			}
		}
		crows.Close()
	}
	byEmail := map[string]int{}
	for i, h := range out {
		byEmail[h.Email] = i
	}
	merged := []hit{}
	for _, h := range crmHits {
		if i, ok := byEmail[h.Email]; ok {
			h.Sent, h.Recv = out[i].Sent, out[i].Recv
			if out[i].Last.After(h.Last) {
				h.Last = out[i].Last
			}
			if h.Name == "" {
				h.Name = out[i].Name
			}
			out[i].Email = "" // replaced by the richer card entry
		}
		merged = append(merged, h)
	}
	for _, h := range out {
		if h.Email != "" {
			merged = append(merged, h)
		}
	}
	// Whoever letters were last exchanged with comes first.
	sort.SliceStable(merged, func(i, j int) bool { return merged[i].Last.After(merged[j].Last) })
	if len(merged) > 10 {
		merged = merged[:10]
	}
	writeJSON(w, http.StatusOK, merged)
}

// searchKey is what suggestions match against: name words and the address, lowercased with
// Unicode rules (PostgreSQL in the Alpine image lowercases only ASCII).
func searchKey(email, name string) string {
	clean := strings.NewReplacer(`"`, " ", "'", " ", "«", " ", "»", " ", "(", " ", ")", " ", ",", " ").Replace(name)
	return strings.ToLower(strings.Join(strings.Fields(clean+" "+email), " "))
}
