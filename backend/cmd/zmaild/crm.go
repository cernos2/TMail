package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap/client"
)

// Counterparty cards are filled from letters: each new incoming letter (and, on request, older ones)
// is read by Loggi together with requisites-like attachments, and what it finds is merged into the
// sender's company and person cards. A value that contradicts the card becomes a suggestion.

var publicDomains = map[string]bool{"gmail.com": true, "googlemail.com": true, "mail.ru": true, "bk.ru": true, "inbox.ru": true, "list.ru": true,
	"internet.ru": true, "yandex.ru": true, "ya.ru": true, "yandex.com": true, "yandex.by": true, "yandex.kz": true, "rambler.ru": true, "lenta.ru": true,
	"outlook.com": true, "hotmail.com": true, "live.com": true, "icloud.com": true, "me.com": true, "yahoo.com": true, "gmx.com": true, "gmx.de": true,
	"proton.me": true, "protonmail.com": true, "aol.com": true, "zoho.com": true, "qip.ru": true, "ukr.net": true, "tut.by": true, "mail.com": true}

// The patterns below hold Russian words on purpose: they detect Russian document names, quote
// headers and forward markers in letters (detection data, not interface text).
var (
	reNoReply = regexp.MustCompile(`(?i)^(no-?reply|do-?not-?reply|mailer-daemon|postmaster|notifications?|news(letter)?|info-?rassylka|robot|bounce)[@+.-]`)
	reDocName = regexp.MustCompile(`(?i)реквиз|карточк|card|счет|счёт|invoice|договор|contract|инн|requisit|сч\.|спецификац|коммерческ|кп`)
	reDocExt  = regexp.MustCompile(`(?i)\.(pdf|docx?|odt|rtf|xlsx?|ods|jpe?g|png)$`)
	// Where the quoted or forwarded part of a letter begins: data below it is not the sender's.
	reQuoteCut = regexp.MustCompile(`(?i)(-{3,}\s*(original message|forwarded message|исходное сообщение|пересылаемое сообщение|переадресованное сообщение)|\n\s*>|\b\d{1,2}[ .]\S+[ .]\d{4}.{0,60}(пишет|wrote)\s*:|\bОт:\s.{0,120}\b(Отправлено|Дата|Sent):|\bFrom:\s.{0,120}\b(Sent|Date):|\n[^\n]{0,120}<[^>\s]+@[^>\s]+>\s*:\s*\n|\n[^\n]{0,40}\d{4}\s*г\.[^\n]{0,40}\d{1,2}:\d{2}[^\n]{0,120}:\s*\n)`)
	reDigits   = regexp.MustCompile(`\D`)
	// A company card / requisites document (not an invoice or contract).
	reCardDoc = regexp.MustCompile(`(?i)реквиз|карточк|card|requisit|анкет`)
	// A forwarded letter: whatever documents it carries belong to someone else, not the sender.
	reFwdSubject = regexp.MustCompile(`(?i)^\s*((fwd?|fw|пересл\S*|tr)\s*:\s*)+`)
	reFwdBody    = regexp.MustCompile(`(?i)(-{3,}\s*(forwarded message|пересылаемое сообщение|переадресованное сообщение)|начало переадресованного сообщения)`)
)

const crmPerRound = 10 // letters per mailbox per worker round; the rest next time

type crmJob struct {
	mu                       sync.Mutex
	Running                  bool      `json:"running"`
	Done, Total, Found, Errs int       `json:"-"`
	Started                  time.Time `json:"-"`
	Current                  string    `json:"-"`
}

func (j *crmJob) snapshot() map[string]any {
	j.mu.Lock()
	defer j.mu.Unlock()
	return map[string]any{"running": j.Running, "done": j.Done, "total": j.Total, "found": j.Found, "errors": j.Errs, "started": j.Started, "current": j.Current}
}

// ---------- worker ----------

// crmScan analyses new Inbox letters of one mailbox (called from the mail worker before sorting).
func (s *Server) crmScan(ctx context.Context, account string) error {
	if !s.aiConfigured() {
		return nil
	}
	if p, err := s.loadPrompt(ctx, "extract"); err != nil || !p.Enabled {
		return nil
	}
	c, err := s.dialAs(account)
	if err != nil {
		return err
	}
	defer c.Logout()
	c.Timeout = 15 * time.Minute // local Dovecot; Loggi may think longer than the idle limit between commands
	mb, err := c.Select("INBOX", true)
	if err != nil {
		return err
	}
	var validity, last int64
	err = s.db.QueryRow(ctx, `SELECT uidvalidity, last_uid FROM crm_state WHERE account=$1`, account).Scan(&validity, &last)
	if err != nil || validity != int64(mb.UidValidity) {
		// Older letters are analysed only by the explicit "go through old mail" action.
		_, err := s.db.Exec(ctx, `INSERT INTO crm_state(account, uidvalidity, last_uid) VALUES ($1,$2,$3)
			ON CONFLICT (account) DO UPDATE SET uidvalidity=EXCLUDED.uidvalidity, last_uid=EXCLUDED.last_uid`, account, int64(mb.UidValidity), int64(mb.UidNext)-1)
		return err
	}
	if int64(mb.UidNext)-1 <= last {
		return nil
	}
	crit := imap.NewSearchCriteria()
	crit.Uid = new(imap.SeqSet)
	crit.Uid.AddRange(uint32(last+1), 0)
	uids, err := c.UidSearch(crit)
	if err != nil {
		return err
	}
	var fresh []uint32
	for _, u := range uids {
		if int64(u) > last {
			fresh = append(fresh, u)
		}
	}
	if len(fresh) > crmPerRound {
		fresh = fresh[:crmPerRound]
	}
	if len(fresh) == 0 {
		return nil
	}
	msgs, err := fetchSummaries(c, account, "INBOX", fresh)
	if err != nil {
		return err
	}
	maxUID := last
	for _, m := range msgs {
		// Old letters brought in by the mail collector (an archive from Gmail) are not read by Loggi
		// one by one here — that would take days; "process old mail" in the admin does them.
		if time.Since(m.Date) > 3*24*time.Hour {
			if int64(m.UID) > maxUID {
				maxUID = int64(m.UID)
			}
			continue
		}
		if _, err := s.crmProcess(ctx, c, account, "INBOX", m, false); err != nil {
			log.Printf("crm %s uid %d: %v", account, m.UID, err)
		}
		if int64(m.UID) > maxUID {
			maxUID = int64(m.UID)
		}
	}
	_, err = s.db.Exec(ctx, `UPDATE crm_state SET last_uid=$2 WHERE account=$1`, account, maxUID)
	return err
}

// crmBackfill goes through older Inbox letters of every mailbox (admin action).
func (s *Server) crmBackfill(days int) {
	j := &s.crm
	ctx := context.Background()
	defer func() { j.mu.Lock(); j.Running = false; j.Current = ""; j.mu.Unlock() }()
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
	type item struct {
		account string
		uids    []uint32
	}
	var work []item
	for _, a := range accounts {
		c, err := s.dialAs(a)
		if err != nil {
			continue
		}
		if _, err := c.Select("INBOX", true); err == nil {
			crit := imap.NewSearchCriteria()
			crit.Since = time.Now().AddDate(0, 0, -days)
			if uids, err := c.UidSearch(crit); err == nil && len(uids) > 0 {
				if len(uids) > 3000 {
					uids = uids[len(uids)-3000:]
				}
				work = append(work, item{a, uids})
				j.mu.Lock()
				j.Total += len(uids)
				j.mu.Unlock()
			}
		}
		c.Logout()
	}
	for _, w := range work {
		c, err := s.dialAs(w.account)
		if err != nil {
			continue
		}
		c.Timeout = 15 * time.Minute
		if _, err := c.Select("INBOX", true); err != nil {
			c.Logout()
			continue
		}
		for i := 0; i < len(w.uids); i += 20 {
			batch := w.uids[i:min(i+20, len(w.uids))]
			msgs, err := fetchSummaries(c, w.account, "INBOX", batch)
			if err != nil {
				break
			}
			for _, m := range msgs {
				j.mu.Lock()
				j.Current = w.account + ": " + truncate(m.Subject, 80)
				j.mu.Unlock()
				id, err := s.crmProcess(ctx, c, w.account, "INBOX", m, false)
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
		c.Logout()
	}
}

// ---------- analysis of one letter ----------

type crmExtract struct {
	Kind string `json:"kind"`
	// SenderCompany: is the company the sender's own organisation (signature, mail domain), or a
	// third party whose invoice or document the sender passes on? Asked of the model explicitly.
	SenderCompany *bool `json:"sender_company"`
	// BuyerINN / BuyerName: the payer named in an invoice — never the owner of its bank details.
	// BuyerINN is its main tax ID (buyer_tax_id; buyer_inn in prompts written before other countries).
	BuyerINN     string `json:"buyer_inn"`
	BuyerTaxID   string `json:"buyer_tax_id"`
	BuyerCountry string `json:"buyer_country"`
	BuyerName    string `json:"buyer_name"`
	// The company: INN is its main tax / registration ID in its country (tax_id; inn in older
	// prompts), BIK the bank code (bank_code; bik), OtherIDs its other identifiers {"label": "value"}.
	// KPP, OGRN and CorrAccount exist only for Russian companies.
	Company struct {
		Name         string          `json:"name"`
		Country      string          `json:"country"`
		INN          string          `json:"inn"`
		TaxID        string          `json:"tax_id"`
		OtherIDs     json.RawMessage `json:"other_ids"`
		KPP          string          `json:"kpp"`
		OGRN         string          `json:"ogrn"`
		LegalAddress string          `json:"legal_address"`
		BankName     string          `json:"bank_name"`
		BIK          string          `json:"bik"`
		BankCode     string          `json:"bank_code"`
		Account      string          `json:"account"`
		CorrAccount  string          `json:"corr_account"`
		Website      string          `json:"website"`
		Phones       []string        `json:"phones"`
	} `json:"company"`
	Person struct {
		Name     string   `json:"name"`
		Position string   `json:"position"`
		Phones   []string `json:"phones"`
	} `json:"person"`
	Delivery    []string `json:"delivery_addresses"`
	Attachments []struct {
		Name string `json:"name"`
		Type string `json:"type"`
	} `json:"attachments"`
}

// unify brings the answer of either prompt version to the old field names: tax_id → inn,
// bank_code → bik, buyer_tax_id → buyer_inn (an old field already filled wins).
func (x *crmExtract) unify() {
	c := &x.Company
	if strings.TrimSpace(c.INN) == "" {
		c.INN = c.TaxID
	}
	if strings.TrimSpace(c.BIK) == "" {
		c.BIK = c.BankCode
	}
	if strings.TrimSpace(x.BuyerINN) == "" {
		x.BuyerINN = x.BuyerTaxID
	}
	c.Country, x.BuyerCountry = normCountry(c.Country), normCountry(x.BuyerCountry)
}

// Who "we" are changes rarely and is asked for every analysed letter: remembered for 30 seconds
// (and forgotten at once when a card is marked "This is us"). The maps are only read by the callers.
var ownCache struct {
	mu      sync.Mutex
	at      time.Time
	ident   ownIdent
	domains map[string]bool
}

func forgetOwn() {
	ownCache.mu.Lock()
	ownCache.at = time.Time{}
	ownCache.mu.Unlock()
}

func (s *Server) ownCached(ctx context.Context) (ownIdent, map[string]bool) {
	ownCache.mu.Lock()
	defer ownCache.mu.Unlock()
	if time.Since(ownCache.at) > 30*time.Second {
		ownCache.ident, ownCache.domains, ownCache.at = s.loadOwnIdentity(ctx), s.loadOwnDomains(ctx), time.Now()
	}
	return ownCache.ident, ownCache.domains
}

func (s *Server) ownIdentity(ctx context.Context) ownIdent {
	o, _ := s.ownCached(ctx)
	return o
}

func (s *Server) ownDomains(ctx context.Context) map[string]bool {
	_, d := s.ownCached(ctx)
	return d
}

func (s *Server) loadOwnDomains(ctx context.Context) map[string]bool {
	out := map[string]bool{}
	if rows, err := s.db.Query(ctx, `SELECT name FROM domains WHERE NOT external`); err == nil {
		for rows.Next() {
			var d string
			if rows.Scan(&d) == nil {
				out[strings.ToLower(d)] = true
			}
		}
		rows.Close()
	}
	return out
}

// crmProcess analyses one letter and returns the id of the card it went to (0 = skipped).
// force re-analyses letters seen before (the "update from this letter" button).
func (s *Server) crmProcess(ctx context.Context, c *client.Client, account, folder string, m Summary, force bool) (int64, error) {
	from := strings.ToLower(m.From.Email)
	at := strings.LastIndex(from, "@")
	if at < 0 || m.MessageID == "" || reNoReply.MatchString(from) {
		return 0, nil
	}
	domain := from[at+1:]
	own := s.ownIdentity(ctx)
	if s.ownDomains(ctx)[domain] || own.addrs[from] {
		return 0, nil // our own letter (any TMAIL mailbox, also a connected Gmail…): not a counterparty
	}
	// A colleague from our company marked "This is us": the letter is read only for the companies it
	// names (a supplier's invoice passed on…); the colleague goes to our company's card.
	colleague := own.domains[domain] || own.team[from] || own.isOwnName(m.From.Name)
	if !force {
		var x int
		if s.db.QueryRow(ctx, `SELECT 1 FROM crm_processed WHERE account=$1 AND message_id=$2`, account, m.MessageID).Scan(&x) == nil {
			return 0, nil
		}
	}
	p, err := s.loadPrompt(ctx, "extract")
	if err != nil || !p.Enabled {
		return 0, nil
	}

	// Letter text without the quoted history, plus requisites-like attachments.
	text := letterText(c, m)
	forward := reFwdSubject.MatchString(m.Subject) || reFwdBody.MatchString(text)
	if loc := reQuoteCut.FindStringIndex(text); loc != nil && loc[0] > 40 {
		text = text[:loc[0]]
	}
	var atts strings.Builder // the attachment list for the AI prompt
	var images []aiImage
	docs := 0
	var readDocs []string // documents shown to the model
	var all []Attachment
	set := new(imap.SeqSet)
	set.AddNum(m.UID)
	if msg, err := fetchOne(c, set, []imap.FetchItem{imap.FetchUid, imap.FetchBodyStructure}); err == nil && msg != nil && msg.BodyStructure != nil {
		all = attachmentsOf(msg.BodyStructure)
		for _, a := range all {
			fmt.Fprintf(&atts, "- %s (%s, %d KB)\n", a.Name, a.Mime, a.Size/1024)
			wanted := reDocExt.MatchString(a.Name) && a.Size < 8<<20 && (reDocName.MatchString(a.Name) || len(all) == 1 && !strings.HasPrefix(a.Mime, "image/"))
			if !wanted || docs >= 2 {
				continue
			}
			path, ok := parsePath(a.Part)
			if !ok {
				continue
			}
			raw, err := fetchPart(c, m.UID, path)
			if err != nil {
				continue
			}
			var enc string
			msg.BodyStructure.Walk(func(p []int, part *imap.BodyStructure) bool {
				if pathString(p) == a.Part {
					enc = part.Encoding
				}
				return true
			})
			data := decodeTransfer(raw, enc)
			if found, err := s.fileVerdict(ctx, data); err != nil || avKind(found) != "" && avKind(found) != "macro" {
				continue // not checked by the antivirus, or a finding: not read for the card
			}
			docs++
			readDocs = append(readDocs, a.Name)
			if strings.HasPrefix(a.Mime, "image/") || regexp.MustCompile(`(?i)\.(jpe?g|png)$`).MatchString(a.Name) {
				images = append(images, aiImage{Mime: a.Mime, Data: data})
				continue
			}
			name := a.Name
			if a.Mime == "application/pdf" && !strings.HasSuffix(strings.ToLower(name), ".pdf") {
				name += ".pdf"
			}
			t, _ := s.officeText(ctx, name, data)
			if len(strings.TrimSpace(t)) >= 150 {
				fmt.Fprintf(&atts, "\nText of the attachment \"%s\":\n%s\n", a.Name, clip(t, 6000))
			} else if pg, err := s.officePage(ctx, name, data, 1, 1400); err == nil {
				// A scan: show the first page to the model as a picture.
				images = append(images, aiImage{Mime: "image/png", Data: pg})
			}
		}
	}
	// A colleague's letter without attachments names no other company worth Loggi's time: only the
	// colleague goes to our company's card.
	if colleague && len(all) == 0 {
		_, _ = s.db.Exec(ctx, `INSERT INTO crm_processed(account, message_id) VALUES ($1,$2) ON CONFLICT (account, message_id) DO UPDATE SET created_at=now()`, account, m.MessageID)
		return s.crmMerge(ctx, crmExtract{}, m, account, folder, domain, nil, crmFlags{thirdParty: true, colleague: true, own: own})
	}
	who := m.From.Email
	if m.From.Name != "" {
		who = m.From.Name + " <" + m.From.Email + ">"
	}
	// The model is told who "we" are: our signatures in quotes and forwards are not the sender's.
	// The note is part of the AI prompt (text for the model, not interface text).
	where := ""
	if sc := serverCountry(); sc != "" {
		where = " This mail server works in the country " + sc + "; a company whose country the documents do not show is most likely from there."
	}
	note := "[Service note: our side is " + strings.Join(own.names, "; ") + ". These are not counterparties: their names, signatures, phones and company details do not belong to the sender. " +
		"If the letter forwards someone else's letter or third parties' documents, do not attribute the data of the forwarded part to the sender. " +
		"Add the field \"sender_company\" to the answer: true if the company in the answer is the sender's own organisation (their signature, the domain of their mail, their website); " +
		"false if it is another organisation: a supplier or a client from a forwarded or attached invoice, contract or document, or just a company mentioned. " +
		"An invoice names two companies: the supplier (who receives the payment) and the buyer (who pays). The bank, the account numbers and the bank code in an invoice are always the supplier's. " +
		"Return one company and do not mix the data of two: the name, tax ID, address and bank details must belong to one and the same organisation. " +
		"Give the company's country as an ISO 3166-1 alpha-2 code in the field \"country\" of the company. " +
		"If the letter or an attachment holds an invoice, also give the fields \"buyer_tax_id\", \"buyer_country\" and \"buyer_name\": the tax ID, country and name of the buyer (payer) of the invoice." + where + "]\n\n"
	in := aiInput{From: who, Subject: m.Subject, Letter: note + text, Attachments: atts.String(), Mailbox: account}
	ans, err := s.completeImages(ctx, p, fillTemplate(p.Template, in, ""), images)
	if err != nil {
		return 0, err
	}
	var x crmExtract
	if i, j := strings.Index(ans, "{"), strings.LastIndex(ans, "}"); i >= 0 && j > i {
		if err := json.Unmarshal([]byte(ans[i:j+1]), &x); err != nil {
			return 0, fmt.Errorf("bad JSON from model: %v", err)
		}
	}
	x.unify()
	_, _ = s.db.Exec(ctx, `INSERT INTO crm_processed(account, message_id) VALUES ($1,$2) ON CONFLICT (account, message_id) DO UPDATE SET created_at=now()`, account, m.MessageID)
	if x.Kind == "newsletter" || x.Kind == "none" || x.Kind == "" && x.Company.Name == "" && x.Person.Name == "" {
		return 0, nil
	}
	// The model still took our own signature for the sender's: drop what it found, keep only the
	// sender's address on the card.
	if pn := normName(x.Person.Name); pn != "" && own.names2[pn] {
		x = crmExtract{}
	}
	ownDoc := false
	if cn := normName(x.Company.Name); cn != "" && own.isOwnName(x.Company.Name) || own.isOwnTaxID(x.Company.INN) {
		x.Company = crmExtract{}.Company // our own company (e.g. the buyer of an invoice): not a counterparty
		ownDoc = true
	}
	// Third party: a forwarded letter, a colleague's letter, or the model says the company is not the sender's.
	thirdParty := forward || colleague || (x.SenderCompany != nil && !*x.SenderCompany)
	// The company's own website is on another domain than the sender's mail: not the sender's company.
	if h := siteHost(x.Company.Website); !thirdParty && h != "" && !publicDomains[domain] {
		if h != domain && !strings.HasSuffix(domain, "."+h) && !strings.HasSuffix(h, "."+domain) {
			thirdParty = true
		}
	}
	// The sender's domain already belongs to a company, and the letter names another one (other
	// INN or name): that is a document of a third party, whatever the model said.
	if !thirdParty && !publicDomains[domain] && (x.Company.INN != "" || x.Company.Name != "") {
		var ownerINN, ownerName string
		if s.db.QueryRow(ctx, `SELECT inn, name FROM crm_parties WHERE kind='company' AND domains @> ARRAY[$1::text] LIMIT 1`, domain).Scan(&ownerINN, &ownerName) == nil {
			inn := normTaxID(x.Company.INN)
			sameINN := inn != "" && inn == ownerINN
			sameName := x.Company.Name != "" && companyKey(x.Company.Name) == companyKey(ownerName)
			switch {
			case sameINN || sameName:
			case inn == "" || inn == ownerINN:
				// No tax ID: a brand or trade name of the domain's company ("Teremopt" from teremopt.ru is OOO "TEREM").
				x.Company.Name = ownerName
			default:
				thirdParty = true
			}
		}
	}
	// The company found is the buyer of an invoice: the bank details there are the supplier's.
	if inn := normTaxID(x.Company.INN); (inn != "" && inn == normTaxID(x.BuyerINN)) ||
		(x.BuyerName != "" && companyKey(x.Company.Name) == companyKey(x.BuyerName)) {
		x.Company.BankName, x.Company.BIK, x.Company.Account, x.Company.CorrAccount = "", "", "", ""
	}
	// Bank details and address of the sender's OWN company are taken only from the signature or
	// from a company card / requisites document — never from an invoice it passes on: an invoice
	// carries the supplier's bank, and the model tends to pin it on the buyer.
	if !thirdParty && len(readDocs) > 0 {
		cardDoc := false
		for _, n := range readDocs {
			cardDoc = cardDoc || reCardDoc.MatchString(n)
		}
		if !cardDoc {
			x.Company.BankName, x.Company.BIK, x.Company.Account, x.Company.CorrAccount, x.Company.LegalAddress = "", "", "", "", ""
		}
	}
	id, err := s.crmMerge(ctx, x, m, account, folder, domain, all, crmFlags{thirdParty: thirdParty, colleague: colleague, ownDoc: ownDoc, own: own})
	if err == nil {
		_, _ = s.db.Exec(ctx, `UPDATE crm_processed SET party_id=$3 WHERE account=$1 AND message_id=$2`, account, m.MessageID, id)
	}
	return id, err
}

// ---------- merging into cards ----------

// siteHost: "https://www.Firma.ru/about" → "firma.ru".
func siteHost(w string) string {
	w = strings.ToLower(strings.TrimSpace(w))
	if w == "" {
		return ""
	}
	if !strings.Contains(w, "://") {
		w = "http://" + w
	}
	u, err := url.Parse(w)
	if err != nil {
		return ""
	}
	h := strings.TrimPrefix(u.Hostname(), "www.")
	if socialHosts[h] {
		return "" // a Telegram / WhatsApp / VK page is not the company's site (nor its domain)
	}
	return h
}

var socialHosts = map[string]bool{"t.me": true, "telegram.me": true, "telegram.org": true, "wa.me": true, "whatsapp.com": true,
	"api.whatsapp.com": true, "chat.whatsapp.com": true, "vk.com": true, "vk.ru": true, "m.vk.com": true, "ok.ru": true,
	"instagram.com": true, "facebook.com": true, "youtube.com": true, "youtu.be": true, "rutube.ru": true, "dzen.ru": true,
	"max.ru": true, "viber.com": true, "twitter.com": true, "x.com": true, "tiktok.com": true, "linkedin.com": true}

func validINN(inn string) bool {
	d := reDigits.ReplaceAllString(inn, "")
	n := func(i int) int { return int(d[i] - '0') }
	check := func(w []int, upto int) int {
		sum := 0
		for i, k := range w {
			if i < upto {
				sum += k * n(i)
			}
		}
		return sum % 11 % 10
	}
	switch len(d) {
	case 10:
		return check([]int{2, 4, 10, 3, 5, 9, 4, 6, 8}, 9) == n(9)
	case 12:
		return check([]int{7, 2, 4, 10, 3, 5, 9, 4, 6, 8}, 10) == n(10) && check([]int{3, 7, 2, 4, 10, 3, 5, 9, 4, 6, 8}, 11) == n(11)
	}
	return false
}

// reExt finds a phone extension (Russian and English abbreviations: detection data). Extensions of
// +7 numbers are stored with the Russian abbreviation, the form already used by the saved cards (as
// the browser does, composables/phone.ts); others with "ext.".
var reExt = regexp.MustCompile(`(?i)\s*(доб\.?|добавочный|ext\.?|#)\s*(\d{1,6})\s*$`)

func normPhone(p string) string {
	ext := ""
	if m := reExt.FindStringSubmatch(p); m != nil {
		ext = m[2]
		p = p[:len(p)-len(m[0])]
	}
	if n := normPhoneMain(p); n != "" && ext != "" {
		if strings.HasPrefix(n, "+7(") {
			return n + " доб. " + ext
		}
		return n + " ext. " + ext
	} else if n != "" {
		return n
	}
	return ""
}

func normPhoneMain(p string) string {
	d := reDigits.ReplaceAllString(p, "")
	plus := strings.HasPrefix(strings.TrimSpace(p), "+")
	// The Russian and Kazakh way of writing without the country code (8XXXXXXXXXX or ten digits) is
	// read as +7 only where the server works there (or has no country set).
	sc := serverCountry()
	local7 := !plus && (sc == "" || sc == "RU" || sc == "KZ")
	if len(d) == 11 && (d[0] == '7' || d[0] == '8' && local7) {
		d = "7" + d[1:]
	} else if len(d) == 10 && local7 {
		d = "7" + d
	}
	if len(d) == 11 && d[0] == '7' && (plus || local7) {
		return fmt.Sprintf("+7(%s) %s-%s-%s", d[1:4], d[4:7], d[7:9], d[9:11])
	}
	if len(d) >= 6 {
		return strings.TrimSpace(p)
	}
	return ""
}

// normName makes OOO "Romashka" and OOO Romashka compare equal (the Cyrillic yo counts as ye).
func normName(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == ' ' {
			b.WriteRune(r)
		}
	}
	return strings.Join(strings.Fields(strings.NewReplacer("ё", "е").Replace(b.String())), " ")
}

type crmFlags struct {
	thirdParty bool // the company found is not the sender's: a supplier's invoice passed on, a forward
	colleague  bool // the sender is from our own company
	ownDoc     bool // the document is our own company's (our invoice passed on)
	own        ownIdent
}

// crmMerge puts what a letter says into the cards. The company the letter names gets its own card
// (by INN or a variant of its name); the sender is tied to the company they write for in this
// letter; companies are tied to each other by invoices (supplier → buyer) and by passing on each
// other's documents. A third party never gets the sender's domain, phones or person.
func (s *Server) crmMerge(ctx context.Context, x crmExtract, m Summary, account, folder, domain string, atts []Attachment, f crmFlags) (int64, error) {
	from := strings.ToLower(m.From.Email)
	public := publicDomains[domain]
	thirdParty := f.thirdParty
	if thirdParty {
		x.Person = crmExtract{}.Person // the people of a forwarded document are not the sender
		x.Delivery = nil
	}
	var companyID, personID int64
	domainCompany := func() (id int64) {
		if !public {
			_ = s.db.QueryRow(ctx, `SELECT id FROM crm_parties WHERE kind='company' AND domains @> ARRAY[$1::text] ORDER BY id LIMIT 1`, domain).Scan(&id)
		}
		return
	}

	// Company named in the letter: by its tax ID, then by a variant of its name, then by the mail domain.
	co := x.Company
	inn, country := cleanTaxID(co.INN, co.Country)
	otherIDs := cleanIDs(parseIDs(co.OtherIDs), inn)
	name := strings.TrimSpace(co.Name)
	if inn != "" || name != "" {
		companyID = s.findCompany(ctx, inn, name)
	}
	// No INN and a new spelling ("Terem Optika"), but its website is a known company's mail domain.
	if h := siteHost(co.Website); companyID == 0 && inn == "" && h != "" && !publicDomains[h] && !f.own.domains[h] {
		_ = s.db.QueryRow(ctx, `SELECT id FROM crm_parties WHERE kind='company' AND domains @> ARRAY[$1::text] ORDER BY id LIMIT 1`, h).Scan(&companyID)
	}
	if companyID == 0 && !thirdParty {
		companyID = domainCompany()
	}
	if h := siteHost(co.Website); h == "" || f.own.domains[h] || s.ownDomains(ctx)[h] {
		co.Website = "" // our own site in a document, or a messenger page, is not the company's site
	}
	_ = s.db.QueryRow(ctx, `SELECT id FROM crm_parties WHERE kind='person' AND emails @> ARRAY[$1::text] LIMIT 1`, from).Scan(&personID)
	// A new name without INN from a sender already writing for other companies: maybe a brand of
	// one of them, maybe really another company — a card of its own, and the question in it.
	var sameAs []int64
	if companyID == 0 && inn == "" && !thirdParty && name != "" && personID > 0 {
		sameAs = s.personCompanies(ctx, personID)
	}
	if companyID == 0 && name != "" {
		if err := s.db.QueryRow(ctx, `INSERT INTO crm_parties(kind, name) VALUES ('company', $1) RETURNING id`, name).Scan(&companyID); err != nil {
			return 0, err
		}
	}
	if companyID > 0 {
		vals := map[string]string{"inn": inn, "country": country, "legal_address": co.LegalAddress, "bank_name": co.BankName,
			"bik": normTaxID(co.BIK), "account": normTaxID(co.Account), "website": co.Website, "name": co.Name}
		if country == "RU" || country == "" { // KPP, OGRN and the correspondent account exist only in Russia
			vals["kpp"], vals["ogrn"], vals["corr_account"] = normTaxID(co.KPP), normTaxID(co.OGRN), normTaxID(co.CorrAccount)
		}
		s.crmSet(ctx, companyID, m.Subject, vals)
		s.crmAddIDs(ctx, companyID, otherIDs)
		s.crmAddList(ctx, companyID, "phones", co.Phones, normPhone)
		if !public && !thirdParty {
			s.crmAddList(ctx, companyID, "domains", []string{domain}, func(v string) string { return v })
		}
		for i, c := range sameAs {
			if i < 3 && c != companyID {
				_, _ = s.db.Exec(ctx, `INSERT INTO crm_suggestions(party_id, field, value, source) VALUES ($1,'same',$2,$3) ON CONFLICT DO NOTHING`,
					companyID, strconv.FormatInt(c, 10), truncate(m.Subject, 200))
			}
		}
	}

	// Person: by e-mail; created when there is a name (or no company to put the letter on).
	pname := strings.TrimSpace(x.Person.Name)
	if pname == "" {
		pname = strings.TrimSpace(m.From.Name)
	}
	if personID == 0 && (pname != "" || companyID == 0 || thirdParty) {
		if pname == "" {
			pname = from
		}
		if err := s.db.QueryRow(ctx, `INSERT INTO crm_parties(kind, name, emails, own) VALUES ('person', $1, $2, $3) RETURNING id`, pname, []string{from}, f.colleague).Scan(&personID); err != nil {
			return 0, err
		}
	}
	if personID > 0 {
		if pname != "" && pname != from { // a card named by its address gets the real name
			_, _ = s.db.Exec(ctx, `UPDATE crm_parties SET name=$2 WHERE id=$1 AND name = ANY(emails)`, personID, pname)
		}
		s.crmSet(ctx, personID, m.Subject, map[string]string{"position": x.Person.Position, "name": x.Person.Name})
		s.crmAddList(ctx, personID, "phones", x.Person.Phones, normPhone)
		s.crmAddList(ctx, personID, "emails", []string{from}, func(v string) string { return v })
	}

	// Whom the sender writes for in this letter: our company (a colleague), the company the letter
	// is from, the owner of the mail domain, or the only company the person is known to work for.
	var senderCo int64
	switch {
	case f.colleague:
		senderCo = s.ownCompany(ctx, from, domain)
	case !thirdParty && companyID > 0:
		senderCo = companyID
	default:
		senderCo = domainCompany()
	}
	if senderCo == 0 && personID > 0 {
		if cs := s.personCompanies(ctx, personID); len(cs) == 1 {
			senderCo = cs[0]
		}
	}

	// The buyer of an invoice in the letter: our company or a client.
	var buyerID int64
	binn, bcountry := cleanTaxID(x.BuyerINN, x.BuyerCountry)
	if bname := strings.TrimSpace(x.BuyerName); binn != "" || bname != "" {
		if f.own.isOwnTaxID(x.BuyerINN) || bname != "" && f.own.isOwnName(bname) {
			buyerID = s.ownCompany(ctx, "", "")
		} else if buyerID = s.findCompany(ctx, binn, bname); buyerID == 0 && binn != "" && bname != "" {
			_ = s.db.QueryRow(ctx, `INSERT INTO crm_parties(kind, name, inn, country) VALUES ('company', $1, $2, $3) RETURNING id`, bname, binn, bcountry).Scan(&buyerID)
		}
	}
	supplier := companyID
	if supplier == 0 && f.ownDoc {
		supplier = s.ownCompany(ctx, "", "") // our own invoice passed on to a client
	}

	main := companyID
	if main == 0 {
		main = senderCo
	}
	if main == 0 {
		main = personID
	}
	if main == 0 {
		return 0, nil
	}
	for _, a := range x.Delivery {
		if a = strings.TrimSpace(a); len([]rune(a)) >= 10 {
			_, _ = s.db.Exec(ctx, `INSERT INTO crm_addresses(party_id, address) SELECT $1, $2
				WHERE NOT EXISTS (SELECT 1 FROM crm_addresses WHERE party_id=$1 AND lower(address)=lower($2))`, main, a)
		}
	}
	kinds := map[string]string{}
	for _, a := range x.Attachments {
		kinds[a.Name] = a.Type
	}
	for _, a := range atts {
		k := kinds[a.Name]
		if k == "" {
			k = "other"
		}
		_, _ = s.db.Exec(ctx, `INSERT INTO crm_files(party_id, account, folder, uid, part, message_id, name, mime, size, kind, sent_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) ON CONFLICT DO NOTHING`, main, account, folder, int64(m.UID), a.Part, m.MessageID, a.Name, a.Mime, int64(a.Size), k, m.Date)
	}
	done := map[int64]bool{}
	for _, id := range []int64{companyID, senderCo, buyerID, supplier, personID} {
		if id > 0 && !done[id] {
			done[id] = true
			_, _ = s.db.Exec(ctx, `INSERT INTO crm_letters(party_id, account, folder, uid, message_id, subject, sent_at) VALUES ($1,$2,$3,$4,$5,$6,$7)
				ON CONFLICT DO NOTHING`, id, account, folder, int64(m.UID), m.MessageID, truncate(m.Subject, 300), m.Date)
			_, _ = s.db.Exec(ctx, `UPDATE crm_parties SET last_letter=GREATEST(COALESCE(last_letter, $2), $2), updated_at=now() WHERE id=$1`, id, m.Date)
			s.crmReindex(ctx, id)
		}
	}
	s.linkPerson(ctx, personID, senderCo)
	if supplier > 0 && buyerID > 0 {
		s.linkCompanies(ctx, supplier, buyerID, "supplier")
	}
	if thirdParty && companyID > 0 && senderCo > 0 && senderCo != buyerID {
		s.linkCompanies(ctx, companyID, senderCo, "via")
	}
	return main, nil
}

var crmFields = map[string]bool{"name": true, "inn": true, "country": true, "kpp": true, "ogrn": true, "legal_address": true, "bank_name": true, "bik": true,
	"account": true, "corr_account": true, "website": true, "position": true, "notes": true}

// crmSet fills empty fields; a different value for a filled field becomes a suggestion.
// The name is only suggested (the user may have renamed the card).
func (s *Server) crmSet(ctx context.Context, id int64, source string, vals map[string]string) {
	for f, v := range vals {
		v = strings.TrimSpace(v)
		if v == "" || !crmFields[f] {
			continue
		}
		var cur string
		if s.db.QueryRow(ctx, `SELECT `+f+` FROM crm_parties WHERE id=$1`, id).Scan(&cur) != nil {
			continue
		}
		if f == "account" { // a settlement account belongs to exactly one company
			var other int
			if s.db.QueryRow(ctx, `SELECT 1 FROM crm_parties WHERE account=$1 AND id<>$2 LIMIT 1`, v, id).Scan(&other) == nil {
				continue
			}
		}
		if cur == "" {
			_, _ = s.db.Exec(ctx, `UPDATE crm_parties SET `+f+`=$2 WHERE id=$1`, id, v)
		} else if normName(cur) != normName(v) && !(f == "name" && companyKey(cur) == companyKey(v)) { // "NordTech" = Nord Tech = OOO "NORDTECH"
			_, _ = s.db.Exec(ctx, `INSERT INTO crm_suggestions(party_id, field, value, source) VALUES ($1,$2,$3,$4) ON CONFLICT DO NOTHING`, id, f, v, truncate(source, 200))
		}
	}
}

// crmAddIDs adds the other identifiers of a company; a label it already has keeps its value.
func (s *Server) crmAddIDs(ctx context.Context, id int64, ids map[string]string) {
	if len(ids) > 0 {
		_, _ = s.db.Exec(ctx, `UPDATE crm_parties SET ids = $2::text::jsonb || ids WHERE id=$1`, id, idsJSON(ids))
	}
}

func (s *Server) crmAddList(ctx context.Context, id int64, field string, vals []string, norm func(string) string) {
	for _, v := range vals {
		if v = norm(strings.TrimSpace(v)); v != "" {
			_, _ = s.db.Exec(ctx, `UPDATE crm_parties SET `+field+` = array_append(`+field+`, $2) WHERE id=$1 AND NOT ($2 = ANY(`+field+`))`, id, v)
		}
	}
}

func (s *Server) crmReindex(ctx context.Context, id int64) {
	_, _ = s.db.Exec(ctx, `UPDATE crm_parties p SET search = lower(concat_ws(' ', p.name, p.inn, p.ogrn, p.website, p.position,
		(SELECT string_agg(v, ' ') FROM jsonb_each_text(p.ids) AS e(k, v)),
		array_to_string(p.emails, ' '), array_to_string(p.domains, ' '), array_to_string(p.phones, ' '),
		(SELECT c.name FROM crm_parties c WHERE c.id = p.company_id))) WHERE p.id=$1`, id)
}

type ownIdent struct {
	addrs   map[string]bool // every TMAIL mailbox and alias
	team    map[string]bool // addresses of colleagues marked "This is us"
	domains map[string]bool // mail domains of our own companies
	inns    map[string]bool // tax IDs (and other identifiers) of our own companies, normalised by normTaxID
	names   []string        // how "we" sign: mailbox and user names
	names2  map[string]bool // the same, normalised for comparison
	keys    []string        // keys of our own companies ("nordtech"): any name containing one is us
}

// isOwnTaxID: the identifier is one of our own companies'.
func (o ownIdent) isOwnTaxID(raw string) bool {
	k := normTaxID(raw)
	return k != "" && o.inns[k]
}

// isOwnName: "NordTech - online store", "NordTech Kom"… contain our company's name.
func (o ownIdent) isOwnName(n string) bool {
	k := companyKey(n)
	if k == "" {
		return false
	}
	if o.names2[k] || o.names2[normName(n)] {
		return true
	}
	for _, key := range o.keys {
		if strings.Contains(" "+k+" ", " "+key+" ") || strings.Contains(strings.ReplaceAll(k, " ", ""), strings.ReplaceAll(key, " ", "")) {
			return true
		}
	}
	return false
}

// loadOwnIdentity: everything that is "us" — never a counterparty.
func (s *Server) loadOwnIdentity(ctx context.Context) ownIdent {
	o := ownIdent{addrs: map[string]bool{}, team: map[string]bool{}, domains: map[string]bool{}, inns: map[string]bool{}, names2: map[string]bool{}}
	add := func(n string) {
		n = strings.TrimSpace(n)
		if len([]rune(n)) >= 4 && !o.names2[normName(n)] {
			o.names2[normName(n)] = true
			o.names = append(o.names, n)
		}
	}
	if rows, err := s.db.Query(ctx, `SELECT address, display_name FROM mailboxes UNION ALL SELECT address, display_name FROM external_accounts
		UNION ALL SELECT source, '' FROM aliases UNION ALL SELECT '', display_name FROM users`); err == nil {
		for rows.Next() {
			var a, n string
			if rows.Scan(&a, &n) == nil {
				if a != "" {
					o.addrs[strings.ToLower(a)] = true
				}
				add(n)
			}
		}
		rows.Close()
	}
	if rows, err := s.db.Query(ctx, `SELECT kind, value FROM crm_self`); err == nil {
		for rows.Next() {
			var k, v string
			if rows.Scan(&k, &v) == nil {
				switch k {
				case "name":
					add(v)
					o.names2[companyKey(v)] = true
					if k := companyKey(v); len([]rune(k)) >= 5 {
						o.keys = append(o.keys, k)
					}
				case "inn":
					if k := normTaxID(v); k != "" {
						o.inns[k] = true
					}
				case "signature": // how our side signs (set by an installation), no card of its own
					add(v)
				case "domain":
					o.domains[strings.ToLower(v)] = true
				case "email":
					o.team[strings.ToLower(v)] = true
				}
			}
		}
		rows.Close()
	}
	// The other identifiers (VAT number…) on the cards of our own companies; their main tax IDs are
	// in crm_self already ("This is us").
	if rows, err := s.db.Query(ctx, `SELECT ids::text FROM crm_parties WHERE own AND kind='company' AND ids <> '{}'::jsonb`); err == nil {
		for rows.Next() {
			var ids string
			if rows.Scan(&ids) == nil {
				var m map[string]string
				_ = json.Unmarshal([]byte(ids), &m)
				for _, v := range m {
					if k := normTaxID(v); len(k) >= 5 {
						o.inns[k] = true
					}
				}
			}
		}
		rows.Close()
	}
	return o
}

// Russian legal forms, branch words and the Cyrillic transliteration table: detection data for
// comparing company names, not interface text.
var (
	reLegalForm = regexp.MustCompile(`(?i)^(ооо|оао|зао|пао|ао|ип|тоо|нпо|пк|чп|ooo|llc|ltd|inc|интернет магазин|интернетмагазин|магазин|компания|фирма)\s+|\s+(ооо|llc|ltd|inc|gmbh)$`)
	reLegalLong = strings.NewReplacer("общество с ограниченной ответственностью", "ооо", "индивидуальный предприниматель", "ип",
		"публичное акционерное общество", "пао", "закрытое акционерное общество", "зао", "открытое акционерное общество", "оао", "акционерное общество", "ао",
		"торговый дом", "тд", "группа компаний", "гк", "научно производственное объединение", "нпо")
	reNameTLD = regexp.MustCompile(`(?i)\.(com|ru|рф|ком|net|org|su)([\s"'»]|$)`) // \b does not see Cyrillic
	reParens  = regexp.MustCompile(`\([^)]*\)`)
	// Not part of the name: a branch ("OP Vostok", "filial …"), a phone stuck to it, "online store", ".kom".
	reNameTail = regexp.MustCompile(`\s+((оп|филиал|обособленное подразделение)\s.*|[\d ]{5,}|интернет магазин|интернетмагазин|магазин|ком|com|shop|онлайн|online)$`)
	translit   = strings.NewReplacer("а", "a", "б", "b", "в", "v", "г", "g", "д", "d", "е", "e", "ё", "e", "э", "e", "ж", "zh", "з", "z", "и", "i", "й", "i",
		"ы", "i", "к", "k", "л", "l", "м", "m", "н", "n", "о", "o", "п", "p", "р", "r", "с", "s", "т", "t", "у", "u", "ф", "f", "х", "h", "ц", "c",
		"ч", "ch", "ш", "sh", "щ", "sch", "ъ", "", "ь", "", "ю", "yu", "я", "ya")
	// Latin spellings brought to the same letters: NordTech, Nord-Tech, Tsar spelled in Cyrillic or Latin…
	latinNorm = strings.NewReplacer("kh", "h", "ts", "c", "tz", "c", "ck", "k", "ph", "f", "x", "ks", "w", "v", "q", "k", "j", "i", "y", "i")
)

// companyKey: a company name for comparison. Legal form, quotes, case, spaces, ".ru"/".kom" and
// "online store" tails, branches and the alphabet do not matter: OOO "NORDTECH", "NordTech",
// Nord Tech and NordTech.kom, in Cyrillic or Latin letters, give the same key.
func companyKey(name string) string {
	n := reLegalLong.Replace(normName(strings.ReplaceAll(reNameTLD.ReplaceAllString(reParens.ReplaceAllString(name, " "), "$2"), "-", " ")))
	for i := 0; i < 3; i++ {
		n = strings.TrimSpace(reNameTail.ReplaceAllString(reLegalForm.ReplaceAllString(n, ""), ""))
	}
	return latinNorm.Replace(translit.Replace(strings.ReplaceAll(n, " ", "")))
}
