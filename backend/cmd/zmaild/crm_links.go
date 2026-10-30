package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/emersion/go-imap"
)

// Companies are separate cards tied by links: a person may write for several companies (each
// letter counts for the company it names), a company supplies another one (by its invoices) or
// passes on another one's documents. Variants of one name ("NORDTECH", "NordTech.kom", NordTech)
// are one card: companyKey, plus the names confirmed by merging cards by hand (crm_aliases).

// findCompany: the card of a company by INN, then by a known name variant, then by its name key.
// A card with another INN is never taken by name.
func (s *Server) findCompany(ctx context.Context, inn, name string) int64 {
	var id int64
	if inn != "" {
		_ = s.db.QueryRow(ctx, `SELECT id FROM crm_parties WHERE kind='company' AND inn=$1 ORDER BY own DESC, id LIMIT 1`, inn).Scan(&id)
	}
	key := companyKey(name)
	if id != 0 || len([]rune(key)) < 2 {
		return id
	}
	_ = s.db.QueryRow(ctx, `SELECT a.party_id FROM crm_aliases a JOIN crm_parties p ON p.id=a.party_id WHERE a.key=$1 AND (p.inn='' OR $2='' OR p.inn=$2)`, key, inn).Scan(&id)
	if id != 0 {
		return id
	}
	rows, err := s.db.Query(ctx, `SELECT id, name FROM crm_parties WHERE kind='company' AND (inn='' OR $1='') ORDER BY own DESC, inn DESC, id`, inn)
	if err != nil {
		return 0
	}
	defer rows.Close()
	for rows.Next() {
		var n string
		if rows.Scan(&id, &n) == nil && companyKey(n) == key {
			return id
		}
	}
	return 0
}

// linkPerson ties the person to the company; the count is of letters both cards have (so analysing a
// letter again does not count it twice). crm_parties.company_id keeps the main company (most letters).
func (s *Server) linkPerson(ctx context.Context, person, company int64) {
	if person == 0 || company == 0 || person == company {
		return
	}
	_, _ = s.db.Exec(ctx, `INSERT INTO crm_person_companies(person_id, company_id) VALUES ($1,$2) ON CONFLICT DO NOTHING`, person, company)
	_, _ = s.db.Exec(ctx, `UPDATE crm_person_companies l SET letters = x.n, last_letter = x.last FROM (SELECT count(*) AS n, max(a.sent_at) AS last
		FROM crm_letters a JOIN crm_letters b ON a.account=b.account AND a.message_id=b.message_id WHERE a.party_id=$1 AND b.party_id=$2) x
		WHERE l.person_id=$1 AND l.company_id=$2`, person, company)
	s.refreshMainCompany(ctx, person)
}

func (s *Server) refreshMainCompany(ctx context.Context, person int64) {
	_, _ = s.db.Exec(ctx, `UPDATE crm_parties SET company_id = (SELECT company_id FROM crm_person_companies WHERE person_id=$1
		ORDER BY letters DESC, last_letter DESC NULLS LAST LIMIT 1) WHERE id=$1 AND EXISTS (SELECT 1 FROM crm_person_companies WHERE person_id=$1)`, person)
	s.crmReindex(ctx, person)
}

// personCompanies: the companies a person is known to write for.
func (s *Server) personCompanies(ctx context.Context, person int64) []int64 {
	var out []int64
	rows, err := s.db.Query(ctx, `SELECT company_id FROM crm_person_companies WHERE person_id=$1 ORDER BY letters DESC`, person)
	if err != nil {
		return nil
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		if rows.Scan(&id) == nil {
			out = append(out, id)
		}
	}
	return out
}

// linkCompanies: role "supplier" (from supplies to) or "via" (documents of from came through to),
// counted like linkPerson by the letters both cards have.
func (s *Server) linkCompanies(ctx context.Context, from, to int64, role string) {
	if from == 0 || to == 0 || from == to {
		return
	}
	_, _ = s.db.Exec(ctx, `INSERT INTO crm_company_links(from_id, to_id, role) VALUES ($1,$2,$3) ON CONFLICT DO NOTHING`, from, to, role)
	_, _ = s.db.Exec(ctx, `UPDATE crm_company_links l SET letters = x.n, last_letter = x.last FROM (SELECT count(*) AS n, max(a.sent_at) AS last
		FROM crm_letters a JOIN crm_letters b ON a.account=b.account AND a.message_id=b.message_id WHERE a.party_id=$1 AND b.party_id=$2) x
		WHERE l.from_id=$1 AND l.to_id=$2 AND l.role=$3`, from, to, role)
}

// ownCompany: our company card for a colleague's letter — by the mail domain or the address of a
// person linked to it; with a single own company, that one.
func (s *Server) ownCompany(ctx context.Context, from, domain string) int64 {
	var id int64
	_ = s.db.QueryRow(ctx, `SELECT c.id FROM crm_parties c WHERE c.own AND c.kind='company' AND (c.domains @> ARRAY[$2::text] OR EXISTS
		(SELECT 1 FROM crm_person_companies l JOIN crm_parties p ON p.id=l.person_id WHERE l.company_id=c.id AND p.emails @> ARRAY[$1::text])) ORDER BY c.id LIMIT 1`, from, domain).Scan(&id)
	if id == 0 {
		_ = s.db.QueryRow(ctx, `SELECT min(id) FROM crm_parties WHERE own AND kind='company' HAVING count(*) = 1`).Scan(&id)
	}
	return id
}

// mergeParties moves everything of card src into dst (same kind) and removes src. The name of
// src is remembered as a variant of dst's.
func (s *Server) mergeParties(ctx context.Context, src, dst int64) error {
	if src == dst {
		return nil
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var kind, kind2, name string
	if err := tx.QueryRow(ctx, `SELECT kind, name FROM crm_parties WHERE id=$1`, src).Scan(&kind, &name); err != nil {
		return err
	}
	if err := tx.QueryRow(ctx, `SELECT kind FROM crm_parties WHERE id=$1`, dst).Scan(&kind2); err != nil {
		return err
	}
	if kind != kind2 {
		return errMergeKinds
	}
	fill := []string{}
	for f := range crmFields {
		fill = append(fill, f+` = CASE WHEN d.`+f+` = '' THEN s.`+f+` ELSE d.`+f+` END`)
	}
	for _, q := range []string{
		`UPDATE crm_parties d SET ` + strings.Join(fill, ", ") + `,
			phones = ARRAY(SELECT DISTINCT unnest(d.phones || s.phones)), emails = ARRAY(SELECT DISTINCT unnest(d.emails || s.emails)),
			domains = ARRAY(SELECT DISTINCT unnest(d.domains || s.domains)), own = d.own OR s.own, ids = s.ids || d.ids,
			last_letter = GREATEST(d.last_letter, s.last_letter), updated_at = now()
			FROM crm_parties s WHERE d.id=$2 AND s.id=$1`,
		`INSERT INTO crm_letters(party_id, account, folder, uid, message_id, subject, sent_at, incoming)
			SELECT $2, account, folder, uid, message_id, subject, sent_at, incoming FROM crm_letters WHERE party_id=$1 ON CONFLICT DO NOTHING`,
		`INSERT INTO crm_files(party_id, account, folder, uid, part, message_id, name, mime, size, kind, sent_at, created_at)
			SELECT $2, account, folder, uid, part, message_id, name, mime, size, kind, sent_at, created_at FROM crm_files WHERE party_id=$1 ON CONFLICT DO NOTHING`,
		`INSERT INTO crm_addresses(party_id, kind, address) SELECT $2, kind, address FROM crm_addresses WHERE party_id=$1 ON CONFLICT DO NOTHING`,
		// people ↔ companies, from either side
		`INSERT INTO crm_person_companies(person_id, company_id, letters, last_letter)
			SELECT CASE WHEN person_id=$1 THEN $2 ELSE person_id END, CASE WHEN company_id=$1 THEN $2 ELSE company_id END, letters, last_letter
			FROM crm_person_companies WHERE person_id=$1 OR company_id=$1
			ON CONFLICT (person_id, company_id) DO UPDATE SET letters = crm_person_companies.letters + EXCLUDED.letters,
			last_letter = GREATEST(crm_person_companies.last_letter, EXCLUDED.last_letter)`,
		`INSERT INTO crm_company_links(from_id, to_id, role, letters, last_letter)
			SELECT CASE WHEN from_id=$1 THEN $2 ELSE from_id END, CASE WHEN to_id=$1 THEN $2 ELSE to_id END, role, letters, last_letter
			FROM crm_company_links WHERE (from_id=$1 OR to_id=$1) AND from_id <> $2 AND to_id <> $2
			ON CONFLICT (from_id, to_id, role) DO UPDATE SET letters = crm_company_links.letters + EXCLUDED.letters,
			last_letter = GREATEST(crm_company_links.last_letter, EXCLUDED.last_letter)`,
		`UPDATE crm_parties SET company_id=$2 WHERE company_id=$1`,
		`UPDATE crm_processed SET party_id=$2 WHERE party_id=$1`,
		`UPDATE crm_aliases SET party_id=$2 WHERE party_id=$1`,
		`DELETE FROM crm_suggestions WHERE field='same' AND value IN ($1::bigint::text, $2::bigint::text) AND party_id IN ($1, $2)`,
		`UPDATE crm_suggestions SET value=$2::bigint::text WHERE field='same' AND value=$1::bigint::text`,
		`DELETE FROM crm_parties WHERE id=$1 AND $2::bigint IS NOT NULL`,
	} {
		if _, err := tx.Exec(ctx, q, src, dst); err != nil {
			return err
		}
	}
	if k := companyKey(name); kind == "company" && k != "" {
		if _, err := tx.Exec(ctx, `INSERT INTO crm_aliases(key, party_id) VALUES ($1,$2) ON CONFLICT (key) DO UPDATE SET party_id=$2`, k, dst); err != nil {
			return err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	if kind == "person" {
		s.refreshMainCompany(ctx, dst)
	} else {
		rows, _ := s.db.Query(ctx, `SELECT person_id FROM crm_person_companies WHERE company_id=$1`, dst)
		var people []int64
		for rows != nil && rows.Next() {
			var p int64
			if rows.Scan(&p) == nil {
				people = append(people, p)
			}
		}
		if rows != nil {
			rows.Close()
		}
		for _, p := range people {
			s.refreshMainCompany(ctx, p)
		}
	}
	s.crmReindex(ctx, dst)
	return nil
}

var errMergeKinds = errors.New("Only a company can be merged with a company, or a person with a person")

// crmDedupe merges company cards whose names are variants of one name (and INNs do not differ).
func (s *Server) crmDedupe(ctx context.Context) {
	type co struct {
		id        int64
		name, inn string
		own       bool
	}
	rows, err := s.db.Query(ctx, `SELECT id, name, inn, own FROM crm_parties WHERE kind='company' ORDER BY own DESC, (inn<>'') DESC, id`)
	if err != nil {
		return
	}
	groups := map[string][]co{}
	var order []string
	for rows.Next() {
		var c co
		if rows.Scan(&c.id, &c.name, &c.inn, &c.own) == nil {
			k := companyKey(c.name)
			if len([]rune(k)) < 3 {
				continue
			}
			if groups[k] == nil {
				order = append(order, k)
			}
			groups[k] = append(groups[k], c)
		}
	}
	rows.Close()
	merged := 0
	for _, k := range order {
		g := groups[k]
		inns := map[string]bool{}
		for _, c := range g {
			if c.inn != "" {
				inns[c.inn] = true
			}
		}
		if len(g) < 2 || len(inns) > 1 { // two INNs under one name: really two companies
			continue
		}
		for _, c := range g[1:] {
			if err := s.mergeParties(ctx, c.id, g[0].id); err != nil {
				log.Printf("crm dedupe %d→%d: %v", c.id, g[0].id, err)
				continue
			}
			merged++
		}
	}
	if merged > 0 {
		log.Printf("crm dedupe: %d cards merged", merged)
	}
}

// crmEnsureOwn: our companies marked "This is us" (crm_self) have a card with the "own" mark; the
// cards of addresses marked as ours are tied to it.
func (s *Server) crmEnsureOwn(ctx context.Context) {
	var names, inns, domains, emails []string
	rows, err := s.db.Query(ctx, `SELECT kind, value FROM crm_self ORDER BY created_at`)
	if err != nil {
		return
	}
	for rows.Next() {
		var k, v string
		if rows.Scan(&k, &v) == nil {
			switch k {
			case "name":
				names = append(names, v)
			case "inn":
				inns = append(inns, v)
			case "domain":
				domains = append(domains, v)
			case "email":
				emails = append(emails, v)
			}
		}
	}
	rows.Close()
	if len(names) == 0 {
		return
	}
	// One card per own name; the INN and domains go to the first one (that is how "This is us" stores them).
	var first int64
	for i, n := range names {
		inn := ""
		if i == 0 && len(inns) > 0 {
			inn = inns[0]
		}
		id := s.findCompany(ctx, inn, n)
		if id == 0 {
			if s.db.QueryRow(ctx, `INSERT INTO crm_parties(kind, name, inn, own, source) VALUES ('company', $1, $2, true, 'manual') RETURNING id`, n, inn).Scan(&id) != nil {
				continue
			}
		}
		_, _ = s.db.Exec(ctx, `UPDATE crm_parties SET own=true WHERE id=$1`, id)
		if i == 0 {
			first = id
			s.crmAddList(ctx, id, "domains", domains, strings.ToLower)
			if inn != "" {
				_, _ = s.db.Exec(ctx, `UPDATE crm_parties SET inn=$2 WHERE id=$1 AND inn=''`, id, inn)
			}
		}
		s.crmReindex(ctx, id)
	}
	for _, e := range emails {
		var pid int64
		if s.db.QueryRow(ctx, `SELECT id FROM crm_parties WHERE kind='person' AND emails @> ARRAY[$1::text] LIMIT 1`, e).Scan(&pid) != nil {
			if s.db.QueryRow(ctx, `INSERT INTO crm_parties(kind, name, emails, source) VALUES ('person', $1, $2, 'manual') RETURNING id`, e, []string{e}).Scan(&pid) != nil {
				continue
			}
		}
		_, _ = s.db.Exec(ctx, `INSERT INTO crm_person_companies(person_id, company_id) VALUES ($1,$2) ON CONFLICT DO NOTHING`, pid, first)
		_, _ = s.db.Exec(ctx, `UPDATE crm_parties SET own=true WHERE id=$1`, pid)
		s.refreshMainCompany(ctx, pid)
	}
}

// POST /api/crm/parties/{id}/merge {into} — the two cards are one company (or one person).
func (s *Server) handleCrmMerge(w http.ResponseWriter, r *http.Request, sess *Session) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	var in struct {
		Into int64 `json:"into"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10)).Decode(&in); err != nil || in.Into == 0 || in.Into == id {
		writeErr(w, http.StatusBadRequest, "Choose the card to merge with")
		return
	}
	var a, b, innA, innB string
	if s.db.QueryRow(r.Context(), `SELECT name, inn FROM crm_parties WHERE id=$1`, id).Scan(&a, &innA) != nil ||
		s.db.QueryRow(r.Context(), `SELECT name, inn FROM crm_parties WHERE id=$1`, in.Into).Scan(&b, &innB) != nil {
		writeErr(w, http.StatusNotFound, "Card not found")
		return
	}
	if innA != "" && innB != "" && innA != innB {
		writeErr(w, http.StatusConflict, fmt.Sprintf("The companies have different INNs (%s and %s): they are different legal entities. The link between them is visible on the cards without merging.", innA, innB))
		return
	}
	if err := s.mergeParties(r.Context(), id, in.Into); err != nil {
		if errors.Is(err, errMergeKinds) {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		serverErr(w, "crm merge", err)
		return
	}
	s.audit(r.Context(), sess.UserID, sess.Login, clientIP(r), "crm.merge", a+" → "+b, nil)
	p, _ := s.loadParty(r, sess, in.Into)
	writeJSON(w, http.StatusOK, p)
}

// POST /api/admin/crm/reprocess {emails} — analyse again every Inbox letter from these addresses
// (after the rules changed, or after "This is us"). Runs in the background as the CRM job.
func (s *Server) adminCrmReprocess(w http.ResponseWriter, r *http.Request, sess *Session) {
	var in struct {
		Emails []string `json:"emails"`
		Skip   int      `json:"skip"` // letters already done by an interrupted run
		// Letters analysed after this moment are not analysed again (an interrupted run resumed).
		After time.Time `json:"after"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&in); err != nil || len(in.Emails) == 0 {
		writeErr(w, http.StatusBadRequest, "Specify the addresses")
		return
	}
	s.crm.mu.Lock()
	if s.crm.Running {
		s.crm.mu.Unlock()
		writeErr(w, http.StatusConflict, "Processing is already running")
		return
	}
	s.crm.Running, s.crm.Done, s.crm.Total, s.crm.Found, s.crm.Errs, s.crm.Started = true, 0, 0, 0, 0, time.Now()
	s.crm.mu.Unlock()
	if in.After.IsZero() {
		in.After = time.Now()
	}
	// Remembered until done: a restart of the server continues the job (see crmResume).
	if b, err := json.Marshal(crmJobState{Emails: in.Emails, After: in.After}); err == nil {
		_, _ = s.db.Exec(r.Context(), `INSERT INTO app_state (key, value) VALUES ('crm_reprocess', $1)
			ON CONFLICT (key) DO UPDATE SET value=EXCLUDED.value, updated_at=now()`, b)
	}
	go s.crmReprocess(in.Emails, in.Skip, in.After)
	s.audit(r.Context(), sess.UserID, sess.Login, clientIP(r), "crm.backfill", strings.Join(in.Emails, ", "), nil)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

type crmJobState struct {
	Emails []string  `json:"emails"`
	After  time.Time `json:"after"` // letters analysed after this moment are done already
}

// crmResume continues a re-analysis interrupted by a restart: the letters it already did were
// analysed after its start, so they are passed over.
func (s *Server) crmResume(ctx context.Context) {
	var st crmJobState
	if err := s.db.QueryRow(ctx, `SELECT value FROM app_state WHERE key='crm_reprocess'`).Scan(&st); err != nil || len(st.Emails) == 0 {
		return
	}
	s.crm.mu.Lock()
	if s.crm.Running {
		s.crm.mu.Unlock()
		return
	}
	s.crm.Running, s.crm.Done, s.crm.Total, s.crm.Found, s.crm.Errs, s.crm.Started = true, 0, 0, 0, 0, time.Now()
	s.crm.mu.Unlock()
	log.Printf("crm reprocess: resumed after a restart (%d addresses, done since %s)", len(st.Emails), st.After.Format(time.RFC3339))
	s.crmReprocess(st.Emails, 0, st.After)
}

func (s *Server) crmReprocess(emails []string, skip int, after time.Time) {
	j := &s.crm
	ctx := context.Background()
	defer func() { j.mu.Lock(); j.Running = false; j.Current = ""; j.mu.Unlock() }()
	// Finished (not interrupted by a restart): nothing to resume any more.
	defer func() { _, _ = s.db.Exec(ctx, `DELETE FROM app_state WHERE key='crm_reprocess'`) }()
	rows, err := s.db.Query(ctx, `SELECT address FROM mailboxes WHERE active ORDER BY address`)
	if err != nil {
		return
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
		c, err := s.dialAs(a)
		if err != nil {
			continue
		}
		c.Timeout = 15 * time.Minute // local Dovecot; Loggi may think for minutes between commands
		if _, err := c.Select("INBOX", true); err != nil {
			c.Logout()
			continue
		}
		seen := map[uint32]bool{}
		var uids []uint32
		for _, e := range emails {
			crit := imap.NewSearchCriteria()
			crit.Header.Add("From", strings.TrimSpace(e))
			if found, err := s.searchHeaders(a, "INBOX", crit); err == nil { // without the full-text index: seconds, not minutes
				for _, u := range found {
					if !seen[u] {
						seen[u] = true
						uids = append(uids, u)
					}
				}
			}
		}
		if skip > 0 {
			n := min(skip, len(uids))
			uids, skip = uids[n:], skip-n
		}
		j.mu.Lock()
		j.Total += len(uids)
		j.mu.Unlock()
		for i := 0; i < len(uids); i += 20 {
			batch := uids[i:min(i+20, len(uids))]
			msgs, err := fetchSummaries(c, a, "INBOX", batch)
			if err != nil { // the connection broke: one more try on a new one
				c.Logout()
				if c, err = s.dialAs(a); err == nil {
					c.Timeout = 15 * time.Minute
					if _, err = c.Select("INBOX", true); err == nil {
						msgs, err = fetchSummaries(c, a, "INBOX", batch)
					}
				}
				if err != nil {
					log.Printf("crm reprocess %s: %v", a, err)
					break
				}
			}
			// Letters of the batch analysed after the start already: one query for the whole batch.
			done := map[string]bool{}
			if !after.IsZero() {
				ids := make([]string, 0, len(msgs))
				for _, m := range msgs {
					ids = append(ids, m.MessageID)
				}
				if rows, err := s.db.Query(ctx, `SELECT message_id FROM crm_processed WHERE account=$1 AND message_id = ANY($2) AND created_at > $3`, a, ids, after); err == nil {
					for rows.Next() {
						var id string
						if rows.Scan(&id) == nil {
							done[id] = true
						}
					}
					rows.Close()
				}
			}
			for _, m := range msgs {
				if done[m.MessageID] {
					j.mu.Lock()
					j.Done++
					j.mu.Unlock()
					continue
				}
				j.mu.Lock()
				j.Current = a + ": " + truncate(m.Subject, 80)
				j.mu.Unlock()
				id, err := s.crmProcess(ctx, c, a, "INBOX", m, true)
				j.mu.Lock()
				j.Done++
				if err != nil {
					j.Errs++
				} else if id > 0 {
					j.Found++
				}
				j.mu.Unlock()
			}
		}
		if c != nil {
			c.Logout()
		}
	}
}
