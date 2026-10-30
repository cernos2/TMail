package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/emersion/go-imap/client"
	"github.com/jackc/pgx/v5"
)

// Counterparty cards API. Cards are shared by the whole company; files and letters from them are
// listed only for the mailboxes the user has access to.

type crmParty struct {
	ID           int64             `json:"id"`
	Kind         string            `json:"kind"`
	Name         string            `json:"name"`
	Country      string            `json:"country"` // ISO 3166-1 alpha-2 of a company, "" when not known
	INN          string            `json:"inn"`     // the main tax / registration ID in its country
	IDs          map[string]string `json:"ids"`     // other identifiers: {"VAT": "DE123456789"}
	KPP          string            `json:"kpp"`
	OGRN         string            `json:"ogrn"`
	LegalAddress string            `json:"legalAddress"`
	BankName     string            `json:"bankName"`
	BIK          string            `json:"bik"`     // bank code: BIK, BIC/SWIFT, routing number, sort code…
	Account      string            `json:"account"` // account number or IBAN
	CorrAccount  string            `json:"corrAccount"`
	Website      string            `json:"website"`
	Position     string            `json:"position"`
	Notes        string            `json:"notes"`
	Phones       []string          `json:"phones"`
	Emails       []string          `json:"emails"`
	Domains      []string          `json:"domains"`
	Own          bool              `json:"own"`
	CompanyID    *int64            `json:"companyId,omitempty"`
	CompanyName  string            `json:"companyName,omitempty"`
	LastLetter   *time.Time        `json:"lastLetter,omitempty"`
	Letters      int               `json:"letters"`
	Pending      int               `json:"pending"`
	Addresses    []crmAddress      `json:"addresses,omitempty"`
	Contacts     []crmParty        `json:"contacts,omitempty"`
	Files        []crmFile         `json:"files,omitempty"`
	History      []crmLetter       `json:"history,omitempty"`
	Suggestions  []crmSuggest      `json:"suggestions,omitempty"`
	Companies    []crmRel          `json:"companies,omitempty"` // a person: the companies they write for
	Links        []crmRel          `json:"links,omitempty"`     // a company: suppliers, buyers, documents passed on
}

// crmRel: a linked card. Role for companies: supplier (it supplies this one), buyer (this one
// supplies it), via (its documents came through this one), forwarded (this one's came through it).
type crmRel struct {
	ID         int64      `json:"id"`
	Name       string     `json:"name"`
	Own        bool       `json:"own"`
	Role       string     `json:"role,omitempty"`
	Letters    int        `json:"letters"`
	LastLetter *time.Time `json:"lastLetter,omitempty"`
}
type crmAddress struct {
	ID      int64  `json:"id"`
	Kind    string `json:"kind"`
	Address string `json:"address"`
}
type crmFile struct {
	Account string    `json:"account"`
	Folder  string    `json:"folder"`
	UID     int64     `json:"uid"`
	Part    string    `json:"part"`
	Name    string    `json:"name"`
	Mime    string    `json:"mime"`
	Size    int64     `json:"size"`
	Kind    string    `json:"kind"`
	SentAt  time.Time `json:"sentAt"`
	Preview string    `json:"preview"`
}
type crmLetter struct {
	Account   string    `json:"account"`
	Folder    string    `json:"folder"`
	UID       int64     `json:"uid"`
	MessageID string    `json:"messageId"`
	Subject   string    `json:"subject"`
	SentAt    time.Time `json:"sentAt"`
}
type crmSuggest struct {
	ID     int64  `json:"id"`
	Field  string `json:"field"`
	Value  string `json:"value"`
	Source string `json:"source"`
	Target string `json:"target,omitempty"` // field "same": the name of the card it may be
}

const partyCols = `p.id, p.kind, p.name, p.country, p.ids::text, p.inn, p.kpp, p.ogrn, p.legal_address, p.bank_name, p.bik, p.account, p.corr_account, p.website,
	p.position, p.notes, p.phones, p.emails, p.domains, p.own, p.company_id, COALESCE(c.name, ''), p.last_letter,
	(SELECT count(*) FROM crm_letters l WHERE l.party_id = p.id), (SELECT count(*) FROM crm_suggestions g WHERE g.party_id = p.id AND g.status = 'pending')`

func scanParty(row pgx.Row) (crmParty, error) {
	var p crmParty
	var ids string
	err := row.Scan(&p.ID, &p.Kind, &p.Name, &p.Country, &ids, &p.INN, &p.KPP, &p.OGRN, &p.LegalAddress, &p.BankName, &p.BIK, &p.Account, &p.CorrAccount, &p.Website,
		&p.Position, &p.Notes, &p.Phones, &p.Emails, &p.Domains, &p.Own, &p.CompanyID, &p.CompanyName, &p.LastLetter, &p.Letters, &p.Pending)
	_ = json.Unmarshal([]byte(ids), &p.IDs)
	if p.IDs == nil {
		p.IDs = map[string]string{}
	}
	return p, err
}

func userAccounts(sess *Session) []string {
	out := make([]string, len(sess.Accounts))
	for i, a := range sess.Accounts {
		out[i] = a.Address
	}
	return out
}

func (s *Server) handleCrmList(w http.ResponseWriter, r *http.Request, sess *Session) {
	q := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
	kind := r.URL.Query().Get("kind")
	args := []any{}
	where := []string{"true"}
	if q != "" {
		args = append(args, "%"+q+"%")
		where = append(where, "p.search LIKE $1")
	}
	if kind == "company" || kind == "person" {
		args = append(args, kind)
		where = append(where, "p.kind = $"+strconv.Itoa(len(args)))
	}
	rows, err := s.db.Query(r.Context(), `SELECT `+partyCols+` FROM crm_parties p LEFT JOIN crm_parties c ON c.id = p.company_id
		WHERE `+strings.Join(where, " AND ")+` ORDER BY p.last_letter DESC NULLS LAST, p.name LIMIT 500`, args...)
	if err != nil {
		serverErr(w, "crm list", err)
		return
	}
	defer rows.Close()
	out := []crmParty{}
	for rows.Next() {
		if p, err := scanParty(rows); err == nil {
			out = append(out, p)
		}
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) loadParty(r *http.Request, sess *Session, id int64) (crmParty, error) {
	ctx := r.Context()
	p, err := scanParty(s.db.QueryRow(ctx, `SELECT `+partyCols+` FROM crm_parties p LEFT JOIN crm_parties c ON c.id = p.company_id WHERE p.id=$1`, id))
	if err != nil {
		return p, err
	}
	accs := userAccounts(sess)
	if rows, err := s.db.Query(ctx, `SELECT id, kind, address FROM crm_addresses WHERE party_id=$1 ORDER BY id`, id); err == nil {
		for rows.Next() {
			var a crmAddress
			if rows.Scan(&a.ID, &a.Kind, &a.Address) == nil {
				p.Addresses = append(p.Addresses, a)
			}
		}
		rows.Close()
	}
	rels := func(q string) (out []crmRel) {
		if rows, err := s.db.Query(ctx, q, id); err == nil {
			for rows.Next() {
				var x crmRel
				if rows.Scan(&x.ID, &x.Name, &x.Own, &x.Role, &x.Letters, &x.LastLetter) == nil {
					out = append(out, x)
				}
			}
			rows.Close()
		}
		return
	}
	if p.Kind == "company" {
		// People who write for it (from any company link), the most active first.
		if rows, err := s.db.Query(ctx, `SELECT `+partyCols+` FROM crm_person_companies l JOIN crm_parties p ON p.id = l.person_id
			LEFT JOIN crm_parties c ON c.id = p.company_id WHERE l.company_id=$1 ORDER BY l.letters DESC, p.name`, id); err == nil {
			for rows.Next() {
				if c, err := scanParty(rows); err == nil {
					p.Contacts = append(p.Contacts, c)
				}
			}
			rows.Close()
		}
		for i := range p.Contacts { // letters for THIS company, not all of the person's
			_ = s.db.QueryRow(ctx, `SELECT letters FROM crm_person_companies WHERE person_id=$1 AND company_id=$2`, p.Contacts[i].ID, id).Scan(&p.Contacts[i].Letters)
		}
		p.Links = rels(`SELECT c.id, c.name, c.own, CASE WHEN l.role='supplier' THEN 'supplier' ELSE 'via' END, l.letters, l.last_letter
				FROM crm_company_links l JOIN crm_parties c ON c.id = l.from_id WHERE l.to_id=$1
			UNION ALL SELECT c.id, c.name, c.own, CASE WHEN l.role='supplier' THEN 'buyer' ELSE 'forwarded' END, l.letters, l.last_letter
				FROM crm_company_links l JOIN crm_parties c ON c.id = l.to_id WHERE l.from_id=$1
			ORDER BY 5 DESC, 2`)
	} else {
		p.Companies = rels(`SELECT c.id, c.name, c.own, '', l.letters, l.last_letter FROM crm_person_companies l JOIN crm_parties c ON c.id = l.company_id
			WHERE l.person_id=$1 ORDER BY l.letters DESC, l.last_letter DESC NULLS LAST`)
	}
	// A company card also shows the files and letters of the people who write only for it.
	ids := `(SELECT $1::bigint UNION SELECT person_id FROM crm_person_companies l WHERE company_id=$1
		AND NOT EXISTS (SELECT 1 FROM crm_person_companies o WHERE o.person_id=l.person_id AND o.company_id<>$1))`
	if rows, err := s.db.Query(ctx, `SELECT DISTINCT ON (account, message_id, part) account, folder, uid, part, name, mime, size, kind, COALESCE(sent_at, created_at)
		FROM crm_files WHERE party_id IN `+ids+` AND account = ANY($2) ORDER BY account, message_id, part`, id, accs); err == nil {
		for rows.Next() {
			var f crmFile
			if rows.Scan(&f.Account, &f.Folder, &f.UID, &f.Part, &f.Name, &f.Mime, &f.Size, &f.Kind, &f.SentAt) == nil {
				f.Preview = previewKind(f.Mime, f.Name)
				p.Files = append(p.Files, f)
			}
		}
		rows.Close()
	}
	if rows, err := s.db.Query(ctx, `SELECT DISTINCT ON (account, message_id) account, folder, uid, message_id, subject, sent_at
		FROM crm_letters WHERE party_id IN `+ids+` AND account = ANY($2) ORDER BY account, message_id`, id, accs); err == nil {
		for rows.Next() {
			var l crmLetter
			if rows.Scan(&l.Account, &l.Folder, &l.UID, &l.MessageID, &l.Subject, &l.SentAt) == nil {
				p.History = append(p.History, l)
			}
		}
		rows.Close()
	}
	sortByTime(p.Files, func(f crmFile) time.Time { return f.SentAt })
	sortByTime(p.History, func(l crmLetter) time.Time { return l.SentAt })
	if len(p.History) > 200 {
		p.History = p.History[:200]
	}
	if rows, err := s.db.Query(ctx, `SELECT id, field, value, source FROM crm_suggestions WHERE party_id=$1 AND status='pending' ORDER BY id`, id); err == nil {
		for rows.Next() {
			var g crmSuggest
			if rows.Scan(&g.ID, &g.Field, &g.Value, &g.Source) == nil {
				// The source is the subject of a letter that may sit in someone else's mailbox.
				if !sess.IsAdmin {
					g.Source = ""
				}
				if g.Field == "same" {
					if s.db.QueryRow(ctx, `SELECT name FROM crm_parties WHERE id=$1`, g.Value).Scan(&g.Target) != nil {
						continue // that card is gone
					}
				}
				p.Suggestions = append(p.Suggestions, g)
			}
		}
		rows.Close()
	}
	return p, nil
}

func sortByTime[T any](xs []T, at func(T) time.Time) {
	for i := 1; i < len(xs); i++ {
		for j := i; j > 0 && at(xs[j]).After(at(xs[j-1])); j-- {
			xs[j], xs[j-1] = xs[j-1], xs[j]
		}
	}
}

func (s *Server) handleCrmGet(w http.ResponseWriter, r *http.Request, sess *Session) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	p, err := s.loadParty(r, sess, id)
	if errors.Is(err, pgx.ErrNoRows) {
		writeErr(w, http.StatusNotFound, "Card not found")
		return
	}
	if err != nil {
		serverErr(w, "crm get", err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

// GET /api/crm/lookup?email= — the card for a sender (person first, then company by domain).
func (s *Server) handleCrmLookup(w http.ResponseWriter, r *http.Request, sess *Session) {
	email := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("email")))
	var id, person int64
	_ = s.db.QueryRow(r.Context(), `SELECT COALESCE(company_id, id), id FROM crm_parties WHERE kind='person' AND emails @> ARRAY[$1::text] LIMIT 1`, email).Scan(&id, &person)
	if id == 0 {
		if at := strings.LastIndex(email, "@"); at > 0 && !publicDomains[email[at+1:]] {
			_ = s.db.QueryRow(r.Context(), `SELECT id FROM crm_parties WHERE kind='company' AND domains @> ARRAY[$1::text] LIMIT 1`, email[at+1:]).Scan(&id)
		}
	}
	if id == 0 {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	p, err := scanParty(s.db.QueryRow(r.Context(), `SELECT `+partyCols+` FROM crm_parties p LEFT JOIN crm_parties c ON c.id = p.company_id WHERE p.id=$1`, id))
	if err != nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if person != 0 && person != id {
		// The person who wrote, with their own phone and position, next to their company.
		if c, err := scanParty(s.db.QueryRow(r.Context(), `SELECT `+partyCols+` FROM crm_parties p LEFT JOIN crm_parties c ON c.id = p.company_id WHERE p.id=$1`, person)); err == nil {
			p.Contacts = []crmParty{c}
		}
	}
	writeJSON(w, http.StatusOK, p)
}

func (s *Server) handleCrmSave(w http.ResponseWriter, r *http.Request, sess *Session) {
	var in crmParty
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 128<<10)).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "Invalid data")
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || (in.Kind != "company" && in.Kind != "person") {
		writeErr(w, http.StatusBadRequest, "A company name or a person's full name is required")
		return
	}
	// The main ID: checked as a Russian INN for a Russian company (or one of unknown country on a
	// server in Russia), otherwise only normalised. A valid INN marks a company of unknown country as Russian.
	if in.Kind == "company" {
		id, cc := cleanTaxID(in.INN, in.Country)
		if strings.TrimSpace(in.INN) != "" && id == "" {
			writeErr(w, http.StatusBadRequest, "The INN is wrong: its checksum does not match")
			return
		}
		in.INN, in.Country = id, cc
		in.IDs = cleanIDs(in.IDs, id)
	} else {
		in.Country, in.IDs = normCountry(in.Country), nil
	}
	clean := func(xs []string, f func(string) string) []string {
		out := []string{}
		for _, x := range xs {
			if v := f(strings.TrimSpace(x)); v != "" {
				out = append(out, v)
			}
		}
		return out
	}
	lower := func(v string) string { return strings.ToLower(v) }
	phones, emails, domains := clean(in.Phones, normPhone), clean(in.Emails, lower), clean(in.Domains, lower)
	ctx := r.Context()
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	vals := []any{in.Kind, in.Name, in.INN, in.KPP, in.OGRN, in.LegalAddress, in.BankName, in.BIK, in.Account, in.CorrAccount, in.Website, in.Position, in.Notes, phones, emails, domains, in.CompanyID,
		in.Country, idsJSON(in.IDs)}
	var err error
	if id == 0 {
		err = s.db.QueryRow(ctx, `INSERT INTO crm_parties(kind, name, inn, kpp, ogrn, legal_address, bank_name, bik, account, corr_account, website, position, notes, phones, emails, domains, company_id, country, ids, source)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19::text::jsonb,'manual') RETURNING id`, vals...).Scan(&id)
	} else {
		_, err = s.db.Exec(ctx, `UPDATE crm_parties SET kind=$1, name=$2, inn=$3, kpp=$4, ogrn=$5, legal_address=$6, bank_name=$7, bik=$8, account=$9, corr_account=$10,
			website=$11, position=$12, notes=$13, phones=$14, emails=$15, domains=$16, company_id=$17, country=$18, ids=$19::text::jsonb, updated_at=now() WHERE id=$20`, append(vals, id)...)
	}
	if err != nil {
		serverErr(w, "crm save", err)
		return
	}
	if in.Kind == "person" && in.CompanyID != nil && *in.CompanyID > 0 {
		_, _ = s.db.Exec(ctx, `INSERT INTO crm_person_companies(person_id, company_id) VALUES ($1,$2) ON CONFLICT DO NOTHING`, id, *in.CompanyID)
	}
	s.crmReindex(ctx, id)
	s.audit(ctx, sess.UserID, sess.Login, clientIP(r), "crm.save", in.Name, nil)
	p, _ := s.loadParty(r, sess, id)
	writeJSON(w, http.StatusOK, p)
}

func (s *Server) handleCrmDelete(w http.ResponseWriter, r *http.Request, sess *Session) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	var name string
	_ = s.db.QueryRow(r.Context(), `DELETE FROM crm_parties WHERE id=$1 RETURNING name`, id).Scan(&name)
	s.audit(r.Context(), sess.UserID, sess.Login, clientIP(r), "crm.delete", name, nil)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleCrmAddress(w http.ResponseWriter, r *http.Request, sess *Session) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if r.Method == http.MethodDelete {
		_, _ = s.db.Exec(r.Context(), `DELETE FROM crm_addresses WHERE id=$1`, id)
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
		return
	}
	var in crmAddress
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&in); err != nil || len([]rune(strings.TrimSpace(in.Address))) < 5 {
		writeErr(w, http.StatusBadRequest, "Enter an address")
		return
	}
	_, _ = s.db.Exec(r.Context(), `INSERT INTO crm_addresses(party_id, kind, address) VALUES ($1,'delivery',$2) ON CONFLICT DO NOTHING`, id, strings.TrimSpace(in.Address))
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// POST /api/crm/suggestions/{id}?accept=1|0
func (s *Server) handleCrmSuggestion(w http.ResponseWriter, r *http.Request, sess *Session) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	var party int64
	var field, value string
	if err := s.db.QueryRow(r.Context(), `UPDATE crm_suggestions SET status=CASE WHEN $2 THEN 'accepted' ELSE 'rejected' END WHERE id=$1 RETURNING party_id, field, value`,
		id, r.URL.Query().Get("accept") == "1").Scan(&party, &field, &value); err != nil {
		writeErr(w, http.StatusNotFound, "Suggestion not found")
		return
	}
	if r.URL.Query().Get("accept") == "1" && field == "same" { // "this is the same company": the cards become one
		into, _ := strconv.ParseInt(value, 10, 64)
		if err := s.mergeParties(r.Context(), party, into); err != nil {
			serverErr(w, "crm merge", err)
			return
		}
		s.audit(r.Context(), sess.UserID, sess.Login, clientIP(r), "crm.merge", strconv.FormatInt(party, 10)+" → "+value, nil)
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "open": into})
		return
	}
	if r.URL.Query().Get("accept") == "1" && crmFields[field] {
		_, _ = s.db.Exec(r.Context(), `UPDATE crm_parties SET `+field+`=$2, updated_at=now() WHERE id=$1`, party, value)
		s.crmReindex(r.Context(), party)
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "open": party})
}

// POST /api/crm/extract {account, folder, uid} — analyse the open letter right now.
func (s *Server) handleCrmExtract(w http.ResponseWriter, r *http.Request, sess *Session) {
	var in struct {
		Account, Folder string
		UID             uint32
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "Invalid data")
		return
	}
	s.withAccount(w, r, sess, in.Account, false, func(c *client.Client, acc Account) {
		c.Timeout = 15 * time.Minute // reading attachments and Loggi's answer may take minutes
		if _, err := c.Select(in.Folder, true); err != nil {
			writeErr(w, http.StatusNotFound, "Folder not found")
			return
		}
		msgs, err := fetchSummaries(c, acc.Address, in.Folder, []uint32{in.UID})
		if err != nil || len(msgs) == 0 {
			writeErr(w, http.StatusNotFound, "Message not found")
			return
		}
		if !s.aiConfigured() {
			writeErr(w, http.StatusServiceUnavailable, "AI is not connected")
			return
		}
		id, err := s.crmProcess(r.Context(), c, acc.Address, in.Folder, msgs[0], true)
		if err != nil {
			writeErr(w, http.StatusBadGateway, fmt.Sprintf("Could not process the message: %v", err))
			return
		}
		if id == 0 {
			writeErr(w, http.StatusUnprocessableEntity, "The message has no counterparty details (looks like a newsletter or a notification)")
			return
		}
		writeJSON(w, http.StatusOK, map[string]int64{"id": id})
	})
}

// ---------- admin ----------

func (s *Server) adminCrmStatus(w http.ResponseWriter, r *http.Request, sess *Session) {
	var companies, people, files, letters int
	_ = s.db.QueryRow(r.Context(), `SELECT count(*) FILTER (WHERE kind='company'), count(*) FILTER (WHERE kind='person') FROM crm_parties`).Scan(&companies, &people)
	_ = s.db.QueryRow(r.Context(), `SELECT (SELECT count(*) FROM crm_files), (SELECT count(*) FROM crm_processed)`).Scan(&files, &letters)
	writeJSON(w, http.StatusOK, map[string]any{"companies": companies, "people": people, "files": files, "analysed": letters, "job": s.crm.snapshot()})
}

func (s *Server) adminCrmBackfill(w http.ResponseWriter, r *http.Request, sess *Session) {
	var in struct {
		Days int `json:"days"`
	}
	_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10)).Decode(&in)
	if in.Days < 1 || in.Days > 730 {
		in.Days = 90
	}
	if !s.aiConfigured() {
		writeErr(w, http.StatusServiceUnavailable, "AI is not connected")
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
	go s.crmBackfill(in.Days)
	s.audit(r.Context(), sess.UserID, sess.Login, clientIP(r), "crm.backfill", strconv.Itoa(in.Days), nil)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// POST /api/crm/parties/{id}/self — "This is us": the company (name, tax ID, mail domains) and its people
// are our side: never counterparties, letters from them are read only for the companies they name.
// The cards stay, marked as ours, with their links (our suppliers, clients, colleagues).
func (s *Server) handleCrmSelf(w http.ResponseWriter, r *http.Request, sess *Session) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	ctx := r.Context()
	var kind, name, inn string
	var domains, emails []string
	if err := s.db.QueryRow(ctx, `SELECT kind, name, inn, domains, emails FROM crm_parties WHERE id=$1`, id).Scan(&kind, &name, &inn, &domains, &emails); err != nil {
		writeErr(w, http.StatusNotFound, "Card not found")
		return
	}
	add := func(k, v string) {
		if v = strings.TrimSpace(v); v != "" {
			_, _ = s.db.Exec(ctx, `INSERT INTO crm_self(kind, value, created_by) VALUES ($1,$2,$3) ON CONFLICT DO NOTHING`, k, v, sess.Login)
			forgetOwn()
		}
	}
	ids := []int64{id}
	if kind == "company" {
		add("name", name)
		add("inn", inn)
		for _, d := range domains {
			if !publicDomains[strings.ToLower(d)] {
				add("domain", strings.ToLower(d))
			}
		}
		// Its people (writing only for it) are our colleagues.
		if rows, err := s.db.Query(ctx, `SELECT p.id, p.emails FROM crm_person_companies l JOIN crm_parties p ON p.id=l.person_id WHERE l.company_id=$1
			AND NOT EXISTS (SELECT 1 FROM crm_person_companies o WHERE o.person_id=l.person_id AND o.company_id<>$1)`, id); err == nil {
			for rows.Next() {
				var pid int64
				var em []string
				if rows.Scan(&pid, &em) == nil {
					ids = append(ids, pid)
					emails = append(emails, em...)
				}
			}
			rows.Close()
		}
	}
	for _, e := range emails {
		add("email", strings.ToLower(e))
	}
	_, _ = s.db.Exec(ctx, `UPDATE crm_parties SET own=true, updated_at=now() WHERE id = ANY($1)`, ids)
	_, _ = s.db.Exec(ctx, `DELETE FROM crm_suggestions WHERE party_id = ANY($1) AND status='pending'`, ids)
	s.audit(ctx, sess.UserID, sess.Login, clientIP(r), "crm.self", name, map[string]any{"cards": len(ids), "emails": emails, "domains": domains})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "marked": len(ids)})
}
