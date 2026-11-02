package main

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/emersion/go-vcard"
	"github.com/jackc/pgx/v5"
)

// Counterparties as vCards: the phones' address book (CardDAV) and import/export of .vcf files.

type vcParty struct {
	ID        int64
	Kind      string
	Name      string
	Position  string
	Notes     string
	Website   string
	Phones    []string
	Emails    []string
	Company   string
	Href      string
	Addresses []string
	UID       string
}

const vcPartySelect = `SELECT p.id, p.kind, p.name, p.position, p.notes, p.website, p.phones, p.emails, COALESCE(c.name, ''),
	COALESCE(p.dav_href, p.id::text || '.vcf'),
	COALESCE((SELECT array_agg(a.address ORDER BY a.id) FROM crm_addresses a WHERE a.party_id = p.id), '{}'),
	COALESCE(p.dav_uid, 'tmail-crm-' || p.id)
	FROM crm_parties p LEFT JOIN crm_parties c ON c.id = p.company_id`

func scanVCParty(row pgx.Row) (vcParty, error) {
	var p vcParty
	err := row.Scan(&p.ID, &p.Kind, &p.Name, &p.Position, &p.Notes, &p.Website, &p.Phones, &p.Emails, &p.Company, &p.Href, &p.Addresses, &p.UID)
	return p, err
}

func (s *Server) vcParties(ctx context.Context, where string, args ...any) ([]vcParty, error) {
	rows, err := s.db.Query(ctx, vcPartySelect+" "+where, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []vcParty
	for rows.Next() {
		p, err := scanVCParty(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// vcSafe: a component of a structured value (N, ORG, ADR) must not contain the separator.
func vcSafe(s string) string { return strings.ReplaceAll(strings.TrimSpace(s), ";", ",") }

// partyCard builds the vCard 3.0 of a counterparty (3.0: what iPhones and Outlook read best).
func partyCard(p vcParty) vcard.Card {
	c := vcard.Card{}
	c.SetValue(vcard.FieldVersion, "3.0")
	uid := p.UID
	if uid == "" {
		uid = "tmail-crm-" + strconv.FormatInt(p.ID, 10)
	}
	c.SetValue(vcard.FieldUID, uid)
	c.SetValue(vcard.FieldFormattedName, p.Name)
	if p.Kind == "company" {
		c.Set(vcard.FieldName, &vcard.Field{Value: ";;;;"})
		c.SetValue(vcard.FieldOrganization, vcSafe(p.Name))
		c.SetValue("X-ABSHOWAS", "COMPANY")
	} else {
		// TMAIL keeps one "full name" line: phones show it whole as the given name.
		c.Set(vcard.FieldName, &vcard.Field{Value: ";" + vcSafe(p.Name) + ";;;"})
		if p.Company != "" {
			c.SetValue(vcard.FieldOrganization, vcSafe(p.Company))
		}
		if p.Position != "" {
			c.SetValue(vcard.FieldTitle, p.Position)
		}
	}
	for _, ph := range p.Phones {
		c.Add(vcard.FieldTelephone, &vcard.Field{Value: ph, Params: vcard.Params{vcard.ParamType: {"WORK", "VOICE"}}})
	}
	for _, e := range p.Emails {
		c.Add(vcard.FieldEmail, &vcard.Field{Value: e, Params: vcard.Params{vcard.ParamType: {"INTERNET", "WORK"}}})
	}
	for _, a := range p.Addresses {
		c.Add(vcard.FieldAddress, &vcard.Field{Value: ";;" + vcSafe(a) + ";;;;", Params: vcard.Params{vcard.ParamType: {"WORK"}}})
	}
	if p.Website != "" {
		c.SetValue(vcard.FieldURL, p.Website)
	}
	if p.Notes != "" {
		c.SetValue(vcard.FieldNote, p.Notes)
	}
	return c
}

func encodeCard(c vcard.Card) string {
	var b bytes.Buffer
	_ = vcard.NewEncoder(&b).Encode(c)
	return b.String()
}

func cardETag(data string) string {
	h := sha1.Sum([]byte(data))
	return hex.EncodeToString(h[:10])
}

// cardFields reads what TMAIL keeps from a vCard.
func cardFields(c vcard.Card) (p vcParty) {
	p.Kind = "person"
	p.Name = strings.TrimSpace(c.PreferredValue(vcard.FieldFormattedName))
	org := ""
	if v := c.PreferredValue(vcard.FieldOrganization); v != "" {
		org = strings.TrimSpace(strings.Split(v, ";")[0])
	}
	if n := c.Name(); p.Name == "" && n != nil {
		p.Name = strings.Join(strings.Fields(strings.Join([]string{n.HonorificPrefix, n.GivenName, n.AdditionalName, n.FamilyName}, " ")), " ")
	}
	if strings.EqualFold(c.Value("X-ABSHOWAS"), "COMPANY") || c.Kind() == vcard.KindOrganization || (p.Name == "" && org != "") {
		p.Kind = "company"
		if org != "" {
			p.Name = org
		}
	} else {
		p.Company = org
	}
	p.Position = strings.TrimSpace(c.PreferredValue(vcard.FieldTitle))
	p.Notes = c.PreferredValue(vcard.FieldNote)
	p.Website = strings.TrimSpace(c.PreferredValue(vcard.FieldURL))
	for _, v := range c.Values(vcard.FieldTelephone) {
		if v = normPhone(strings.TrimPrefix(strings.TrimSpace(v), "tel:")); v != "" {
			p.Phones = append(p.Phones, v)
		}
	}
	for _, v := range c.Values(vcard.FieldEmail) {
		if v = strings.ToLower(strings.TrimSpace(v)); v != "" {
			p.Emails = append(p.Emails, v)
		}
	}
	for _, a := range c.Addresses() {
		line := strings.Join(strings.Fields(strings.Join([]string{a.StreetAddress, a.ExtendedAddress, a.Locality, a.Region, a.PostalCode, a.Country}, " ")), " ")
		if line != "" {
			p.Addresses = append(p.Addresses, line)
		}
	}
	return p
}

// savePartyCard creates or updates a counterparty from a vCard. id=0 creates; href is the name a
// phone gave the new contact.
func (s *Server) savePartyCard(ctx context.Context, id int64, href string, c vcard.Card) (int64, error) {
	p := cardFields(c)
	if p.Name == "" {
		if len(p.Emails) > 0 {
			p.Name = p.Emails[0]
		} else if len(p.Phones) > 0 {
			p.Name = p.Phones[0]
		} else {
			return 0, fmt.Errorf("the contact has no name")
		}
	}
	var companyID *int64
	if p.Kind == "person" && p.Company != "" {
		var cid int64
		err := s.db.QueryRow(ctx, `SELECT id FROM crm_parties WHERE kind='company' AND lower(name)=lower($1) ORDER BY id LIMIT 1`, p.Company).Scan(&cid)
		if err != nil {
			err = s.db.QueryRow(ctx, `INSERT INTO crm_parties(kind, name, source) VALUES ('company', $1, 'manual') RETURNING id`, truncate(p.Company, 300)).Scan(&cid)
		}
		if err == nil {
			companyID = &cid
			s.crmReindex(ctx, cid)
		}
	}
	if p.Phones == nil {
		p.Phones = []string{}
	}
	if p.Emails == nil {
		p.Emails = []string{}
	}
	var err error
	if id == 0 {
		var hrefArg, uidArg any
		if href != "" {
			hrefArg = href
			if u := strings.TrimSpace(c.Value(vcard.FieldUID)); u != "" && len(u) <= 200 {
				uidArg = u
			}
		}
		err = s.db.QueryRow(ctx, `INSERT INTO crm_parties(kind, name, position, notes, website, phones, emails, company_id, dav_href, dav_uid, source)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,'manual') RETURNING id`,
			p.Kind, truncate(p.Name, 300), p.Position, p.Notes, p.Website, p.Phones, p.Emails, companyID, hrefArg, uidArg).Scan(&id)
	} else {
		_, err = s.db.Exec(ctx, `UPDATE crm_parties SET kind=$2, name=$3, position=$4, notes=$5, website=$6, phones=$7, emails=$8,
			company_id=COALESCE($9, CASE WHEN $2='company' THEN NULL ELSE company_id END), updated_at=now() WHERE id=$1`,
			id, p.Kind, truncate(p.Name, 300), p.Position, p.Notes, p.Website, p.Phones, p.Emails, companyID)
	}
	if err != nil {
		return 0, err
	}
	if companyID != nil {
		_, _ = s.db.Exec(ctx, `INSERT INTO crm_person_companies(person_id, company_id) VALUES ($1,$2) ON CONFLICT DO NOTHING`, id, *companyID)
	}
	for _, a := range p.Addresses {
		_, _ = s.db.Exec(ctx, `INSERT INTO crm_addresses(party_id, kind, address) VALUES ($1,'post',$2) ON CONFLICT DO NOTHING`, id, truncate(a, 500))
	}
	s.crmReindex(ctx, id)
	return id, nil
}

// GET /api/crm/vcard[?ids=1,2,3] — counterparties as one .vcf file (all of them without ids).
func (s *Server) handleCrmVCardExport(w http.ResponseWriter, r *http.Request, sess *Session) {
	where, args := "ORDER BY p.kind DESC, lower(p.name)", []any{}
	if q := r.URL.Query().Get("ids"); q != "" {
		var ids []int64
		for _, x := range strings.Split(q, ",") {
			if n, err := strconv.ParseInt(strings.TrimSpace(x), 10, 64); err == nil {
				ids = append(ids, n)
			}
		}
		where, args = "WHERE p.id = ANY($1) ORDER BY lower(p.name)", []any{ids}
	}
	list, err := s.vcParties(r.Context(), where, args...)
	if err != nil {
		serverErr(w, "vcard export", err)
		return
	}
	var b bytes.Buffer
	for _, p := range list {
		b.WriteString(encodeCard(partyCard(p)))
	}
	s.audit(r.Context(), sess.UserID, sess.Login, clientIP(r), "crm.vcard_export", strconv.Itoa(len(list)), nil)
	w.Header().Set("Content-Type", "text/vcard; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="contacts-%s.vcf"`, time.Now().Format("2006-01-02")))
	_, _ = w.Write(b.Bytes())
}

// POST /api/crm/vcard (multipart "file") — contacts from a .vcf file (a phone, Outlook, Google).
// A contact with an e-mail or phone already known updates that card instead of making a duplicate.
func (s *Server) handleCrmVCardImport(w http.ResponseWriter, r *http.Request, sess *Session) {
	r.Body = http.MaxBytesReader(w, r.Body, 20<<20)
	f, _, err := r.FormFile("file")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "Choose a .vcf file")
		return
	}
	defer f.Close()
	ctx := r.Context()
	dec := vcard.NewDecoder(f)
	created, updated, skipped := 0, 0, 0
	for n := 0; n < 20000; n++ {
		c, err := dec.Decode()
		if err == io.EOF {
			break
		}
		if err != nil {
			if n == 0 {
				writeErr(w, http.StatusBadRequest, "This is not a vCard (.vcf) file")
				return
			}
			skipped++
			break
		}
		p := cardFields(c)
		var id int64
		if len(p.Emails) > 0 || len(p.Phones) > 0 {
			_ = s.db.QueryRow(ctx, `SELECT id FROM crm_parties WHERE kind=$1 AND (emails && $2 OR phones && $3) ORDER BY id LIMIT 1`,
				p.Kind, nonNil(p.Emails), nonNil(p.Phones)).Scan(&id)
		}
		if id == 0 && p.Name != "" {
			_ = s.db.QueryRow(ctx, `SELECT id FROM crm_parties WHERE kind=$1 AND lower(name)=lower($2) ORDER BY id LIMIT 1`, p.Kind, p.Name).Scan(&id)
		}
		if id > 0 {
			// Keep what the card already has; add phones and e-mails from the file.
			old, err := s.vcParties(ctx, "WHERE p.id=$1", id)
			if err == nil && len(old) == 1 {
				merged := partyCard(mergeParty(old[0], p))
				if _, err := s.savePartyCard(ctx, id, "", merged); err == nil {
					updated++
					continue
				}
			}
			skipped++
			continue
		}
		if _, err := s.savePartyCard(ctx, 0, "", c); err != nil {
			skipped++
			continue
		}
		created++
	}
	s.audit(ctx, sess.UserID, sess.Login, clientIP(r), "crm.vcard_import", "", map[string]any{"created": created, "updated": updated, "skipped": skipped})
	writeJSON(w, http.StatusOK, map[string]int{"created": created, "updated": updated, "skipped": skipped})
}

func nonNil(xs []string) []string {
	if xs == nil {
		return []string{}
	}
	return xs
}

// mergeParty: the existing card with what the imported one adds (empty fields filled, new phones
// and e-mails appended).
func mergeParty(old, add vcParty) vcParty {
	m := old
	fill := func(dst *string, v string) {
		if *dst == "" {
			*dst = v
		}
	}
	fill(&m.Position, add.Position)
	fill(&m.Website, add.Website)
	fill(&m.Company, add.Company)
	if add.Notes != "" && !strings.Contains(m.Notes, add.Notes) {
		m.Notes = strings.TrimSpace(m.Notes + "\n" + add.Notes)
	}
	appendNew := func(dst []string, xs []string) []string {
		for _, x := range xs {
			found := false
			for _, y := range dst {
				if strings.EqualFold(x, y) {
					found = true
				}
			}
			if !found {
				dst = append(dst, x)
			}
		}
		return dst
	}
	m.Phones = appendNew(m.Phones, add.Phones)
	m.Emails = appendNew(m.Emails, add.Emails)
	m.Addresses = appendNew(nil, add.Addresses)
	return m
}
