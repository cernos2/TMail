package main

import (
	"context"
	"fmt"
	"html"
	"io"
	"log"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap/client"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Search index in the database (msg_index). Dovecot's own full-text search reads the whole index
// for a common word (20–40 s in 90 000 letters) and knows no word forms; the database answers in
// tens of milliseconds and finds other forms of a word (Russian inflections) and the letter yo written as ye. A background job
// indexes new letters every two minutes and forgets the deleted and moved ones. Until a folder's
// first pass is done its search goes to Dovecot as before; the letters that came after the last
// pass are searched there by subject and addresses. The text of attachments stays with Dovecot:
// it is searched in the background and added to the answer (see searchText).

const (
	idxBatch   = 200
	idxTextMax = 20000 // characters of a letter's text that go into the index
	idxPartMax = 65536 // bytes of the text part fetched for that
)

// Word forms are matched with the PostgreSQL text-search dictionary of the server's language
// (договора = договор in Russian, invoices = invoice in English); languages PostgreSQL has no
// dictionary for use "simple" (exact words, still prefix search). SEARCH_LANGUAGE overrides it.
var searchConfig = func() string {
	known := map[string]string{"ar": "arabic", "hy": "armenian", "eu": "basque", "ca": "catalan", "da": "danish", "nl": "dutch",
		"en": "english", "fi": "finnish", "fr": "french", "de": "german", "el": "greek", "hi": "hindi", "hu": "hungarian",
		"id": "indonesian", "ga": "irish", "it": "italian", "lt": "lithuanian", "ne": "nepali", "nb": "norwegian",
		"pt-BR": "portuguese", "pt-PT": "portuguese", "ro": "romanian", "ru": "russian", "sr": "serbian", "es": "spanish",
		"sv": "swedish", "ta": "tamil", "tr": "turkish"}
	if v := env("SEARCH_LANGUAGE", ""); v != "" {
		for _, c := range known {
			if c == v {
				return v
			}
		}
		if v == "simple" {
			return v
		}
	}
	if c, ok := known[serverLocale().Lang]; ok {
		return c
	}
	return "simple"
}()

func (s *Server) runSearchIndex(ctx context.Context) {
	select {
	case <-ctx.Done():
		return
	case <-time.After(30 * time.Second):
	}
	// Built with another dictionary (the server's language changed): the index is made anew.
	var was string
	_ = s.db.QueryRow(ctx, `SELECT value->>'config' FROM app_state WHERE key='search_index'`).Scan(&was)
	if was != searchConfig {
		if was != "" {
			log.Printf("search index: dictionary %s → %s, indexing anew", was, searchConfig)
			_, _ = s.db.Exec(ctx, `TRUNCATE msg_index, msg_index_state`)
		}
		_, _ = s.db.Exec(ctx, `INSERT INTO app_state(key, value) VALUES ('search_index', jsonb_build_object('config', $1::text))
			ON CONFLICT (key) DO UPDATE SET value=EXCLUDED.value, updated_at=now()`, searchConfig)
	}
	t := time.NewTicker(2 * time.Minute)
	defer t.Stop()
	for {
		s.indexAll(ctx)
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

func (s *Server) indexAll(ctx context.Context) {
	rows, err := s.db.Query(ctx, `SELECT address FROM mailboxes WHERE active AND deleted_at IS NULL ORDER BY address`)
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
	if len(accounts) == 0 {
		return
	}
	_, _ = s.db.Exec(ctx, `DELETE FROM msg_index WHERE account <> ALL($1)`, accounts)
	_, _ = s.db.Exec(ctx, `DELETE FROM msg_index_state WHERE account <> ALL($1)`, accounts)
	for _, a := range accounts {
		if ctx.Err() != nil {
			return
		}
		if err := s.indexAccount(ctx, a); err != nil {
			log.Printf("search index %s: %v", a, err)
		}
	}
}

func (s *Server) indexAccount(ctx context.Context, account string) error {
	c, err := s.dialSearch(account) // no full-text plugin: fetching never wakes Dovecot's indexer
	if err != nil {
		return err
	}
	defer c.Logout()
	c.Timeout = 5 * time.Minute

	ch := make(chan *imap.MailboxInfo, 32)
	done := make(chan error, 1)
	go func() { done <- c.List("", "*", ch) }()
	var folders []string
	for mb := range ch {
		if hasAttr(mb.Attributes, imap.NoSelectAttr) {
			continue
		}
		if r := roleOf(mb); r == "trash" || r == "junk" {
			continue // spam and deleted letters: searched by Dovecot when asked
		}
		folders = append(folders, mb.Name)
	}
	if err := <-done; err != nil {
		return err
	}
	_, _ = s.db.Exec(ctx, `DELETE FROM msg_index WHERE account=$1 AND folder <> ALL($2)`, account, folders)
	_, _ = s.db.Exec(ctx, `DELETE FROM msg_index_state WHERE account=$1 AND folder <> ALL($2)`, account, folders)
	for _, f := range folders {
		if ctx.Err() != nil {
			return nil
		}
		if err := s.indexFolder(ctx, c, account, f); err != nil {
			log.Printf("search index %s/%s: %v", account, f, err)
		}
	}
	return nil
}

func (s *Server) indexFolder(ctx context.Context, c *client.Client, account, folder string) error {
	st, err := c.Select(folder, true)
	if err != nil {
		return err
	}
	var validity, last int64
	err = s.db.QueryRow(ctx, `SELECT uidvalidity, last_uid FROM msg_index_state WHERE account=$1 AND folder=$2`, account, folder).Scan(&validity, &last)
	if err != nil || validity != int64(st.UidValidity) {
		if _, err := s.db.Exec(ctx, `DELETE FROM msg_index WHERE account=$1 AND folder=$2`, account, folder); err != nil {
			return err
		}
		last = 0
		if _, err := s.db.Exec(ctx, `INSERT INTO msg_index_state(account, folder, uidvalidity) VALUES ($1,$2,$3)
			ON CONFLICT (account, folder) DO UPDATE SET uidvalidity=EXCLUDED.uidvalidity, last_uid=0, complete=false, updated_at=now()`,
			account, folder, int64(st.UidValidity)); err != nil {
			return err
		}
	}

	if st.UidNext == 0 || int64(st.UidNext)-1 > last {
		crit := imap.NewSearchCriteria()
		crit.Uid = new(imap.SeqSet)
		crit.Uid.AddRange(uint32(last+1), 0)
		uids, err := c.UidSearch(crit)
		if err != nil {
			return err
		}
		fresh := uids[:0]
		for _, u := range uids {
			if int64(u) > last {
				fresh = append(fresh, u)
			}
		}
		started, count := time.Now(), 0
		for i := 0; i < len(fresh); i += idxBatch {
			if ctx.Err() != nil {
				return nil
			}
			chunk := fresh[i:min(i+idxBatch, len(fresh))]
			if err := s.indexUIDs(ctx, c, account, folder, chunk); err != nil {
				return err
			}
			count += len(chunk)
			if _, err := s.db.Exec(ctx, `UPDATE msg_index_state SET last_uid=$3, updated_at=now() WHERE account=$1 AND folder=$2`,
				account, folder, int64(chunk[len(chunk)-1])); err != nil {
				return err
			}
			time.Sleep(20 * time.Millisecond) // leave the disk to the people reading their mail
		}
		if count >= 1000 {
			log.Printf("search index %s/%s: %d letters in %s", account, folder, count, time.Since(started).Round(time.Second))
		}
	}

	// Rows indexed before previews were kept: once more, the newest first.
	for ctx.Err() == nil {
		rows, err := s.db.Query(ctx, `SELECT uid FROM msg_index WHERE account=$1 AND folder=$2 AND preview IS NULL
			ORDER BY uid DESC LIMIT $3`, account, folder, idxBatch)
		if err != nil {
			return err
		}
		var old []uint32
		for rows.Next() {
			var u int64
			if rows.Scan(&u) == nil {
				old = append(old, uint32(u))
			}
		}
		rows.Close()
		if len(old) == 0 {
			break
		}
		if err := s.indexUIDs(ctx, c, account, folder, old); err != nil {
			return err
		}
		// Letters gone from the folder meanwhile: their rows are removed below.
		_, _ = s.db.Exec(ctx, `UPDATE msg_index SET preview='' WHERE account=$1 AND folder=$2 AND uid = ANY($3) AND preview IS NULL`,
			account, folder, uidList(old))
		time.Sleep(20 * time.Millisecond)
	}

	// Letters deleted or moved away: more rows than letters in the folder.
	var rows int64
	if err := s.db.QueryRow(ctx, `SELECT count(*) FROM msg_index WHERE account=$1 AND folder=$2`, account, folder).Scan(&rows); err != nil {
		return err
	}
	if rows > int64(st.Messages) {
		all, err := c.UidSearch(imap.NewSearchCriteria())
		if err != nil {
			return err
		}
		ids := make([]int64, len(all))
		for i, u := range all {
			ids[i] = int64(u)
		}
		if _, err := s.db.Exec(ctx, `DELETE FROM msg_index WHERE account=$1 AND folder=$2 AND uid <> ALL($3)`, account, folder, ids); err != nil {
			return err
		}
	}
	_, err = s.db.Exec(ctx, `UPDATE msg_index_state SET complete=true, updated_at=now() WHERE account=$1 AND folder=$2`, account, folder)
	return err
}

type idxDoc struct {
	uid     uint32
	date    time.Time
	hdr     string
	text    string
	preview string
	path    []int
	enc, cs string
	isHTML  bool
}

// indexUIDs reads the envelope and the text of the given letters and stores them in msg_index.
func (s *Server) indexUIDs(ctx context.Context, c *client.Client, account, folder string, uids []uint32) error {
	set := new(imap.SeqSet)
	set.AddNum(uids...)
	ch := make(chan *imap.Message, 32)
	done := make(chan error, 1)
	go func() {
		done <- c.UidFetch(set, []imap.FetchItem{imap.FetchUid, imap.FetchEnvelope, imap.FetchInternalDate, imap.FetchBodyStructure}, ch)
	}()
	docs := map[uint32]*idxDoc{}
	var order []uint32
	for m := range ch {
		sm := summaryOf(m, account, folder)
		var h strings.Builder
		h.WriteString(sm.Subject)
		for _, a := range append(append([]Addr{sm.From}, sm.To...), sm.Cc...) {
			h.WriteString(" " + a.Name + " " + a.Email)
		}
		for _, a := range sm.Attachments {
			h.WriteString(" " + a.Name)
		}
		d := &idxDoc{uid: m.Uid, date: sm.Date, hdr: normSearch(h.String())}
		if m.BodyStructure != nil {
			d.path, d.enc, d.cs, d.isHTML = previewPart(m.BodyStructure)
		}
		docs[m.Uid] = d
		order = append(order, m.Uid)
	}
	if err := <-done; err != nil {
		return err
	}

	// The text parts, one fetch per part number (most letters have theirs at 1 or 1.1).
	groups := map[string][]uint32{}
	for _, u := range order {
		if d := docs[u]; d.path != nil {
			k := pathString(d.path)
			groups[k] = append(groups[k], u)
		}
	}
	for k, list := range groups {
		path, _ := parsePath(k)
		gs := new(imap.SeqSet)
		gs.AddNum(list...)
		sec := &imap.BodySectionName{BodyPartName: imap.BodyPartName{Path: path}, Peek: true, Partial: []int{0, idxPartMax}}
		ch := make(chan *imap.Message, 32)
		done := make(chan error, 1)
		go func() { done <- c.UidFetch(gs, []imap.FetchItem{imap.FetchUid, sec.FetchItem()}, ch) }()
		for m := range ch {
			d := docs[m.Uid]
			lit := firstBody(m)
			if d == nil || lit == nil {
				continue
			}
			raw, _ := io.ReadAll(lit)
			body := string(decodePart(raw, d.enc, d.cs))
			d.text = indexText(body, d.isHTML)
			d.preview = previewText(body, d.isHTML)
		}
		if err := <-done; err != nil {
			return err
		}
	}

	b := &pgx.Batch{}
	for _, u := range order {
		d := docs[u]
		var date *time.Time
		if !d.date.IsZero() {
			date = &d.date
		}
		b.Queue(`INSERT INTO msg_index(account, folder, uid, date, hdr, tsv, preview)
			VALUES ($1, $2, $3, $4, $5, to_tsvector('`+searchConfig+`', $5) || strip(to_tsvector('`+searchConfig+`', $6)), $7)
			ON CONFLICT (account, folder, uid) DO UPDATE SET date=EXCLUDED.date, hdr=EXCLUDED.hdr, tsv=EXCLUDED.tsv, preview=EXCLUDED.preview`,
			account, folder, int64(u), date, d.hdr, d.text, strings.ReplaceAll(d.preview, "\x00", ""))
	}
	return s.db.SendBatch(ctx, b).Close()
}

// indexText: the readable text of a letter's body for the index.
func indexText(t string, isHTML bool) string {
	if isHTML || reLooksHTML.MatchString(t) {
		t = reComment.ReplaceAllString(t, " ")
		t = reStyleScript.ReplaceAllString(t, " ")
		t = reTag.ReplaceAllString(t, " ")
		t = html.UnescapeString(t)
	}
	t = reFiller.ReplaceAllString(t, " ")
	t = strings.TrimSpace(reSpaces.ReplaceAllString(t, " "))
	if utf8.RuneCountInString(t) > idxTextMax {
		t = string([]rune(t)[:idxTextMax])
	}
	return normSearch(t)
}

// normSearch: lower case, the Cyrillic yo as ye (people write either), no invalid bytes and no
// zero bytes (the database refuses those in text).
func normSearch(s string) string {
	s = strings.ToLower(strings.ToValidUTF8(s, " "))
	return strings.NewReplacer("ё", "е", "\x00", " ").Replace(s)
}

var reSearchWord = regexp.MustCompile(`[\pL\pN]+`)

// indexSearch finds the letters of an indexed folder whose subject, addresses, attachment names
// or text contain every word of the query in any form (a Russian noun in any case), or whose
// header line contains the query as typed (a part of an address or a number). ok=false: the
// folder is not indexed yet. Letters newer than the last pass are searched in Dovecot's headers.
// page > 0: only the newest page+1 hits below before (one page of the list: milliseconds instead
// of half a second for a word in 30 000 letters).
func (s *Server) indexSearch(ctx context.Context, account, folder, text string, base func() *imap.SearchCriteria, before uint32, page int) (uids []uint32, ok bool, err error) {
	var last int64
	var complete bool
	err = s.db.QueryRow(ctx, `SELECT last_uid, complete FROM msg_index_state WHERE account=$1 AND folder=$2`, account, folder).Scan(&last, &complete)
	if err != nil || !complete {
		return nil, false, nil
	}
	uids, err = s.indexQuery(ctx, account, folder, text, before, page)
	if err != nil {
		return nil, false, err
	}
	if len(uids) == 0 {
		// Typed in the wrong layout: "ljujdjh" is the Russian word for "contract".
		if alt := ruLayout(text); alt != "" {
			if more, err := s.indexQuery(ctx, account, folder, alt, before, page); err == nil {
				uids = more
			}
		}
	}
	tail := withHeaderText(base(), text)
	tail.Uid = new(imap.SeqSet)
	tail.Uid.AddRange(uint32(last+1), 0)
	if fresh, err := s.searchHeaders(account, folder, tail); err == nil {
		for _, u := range fresh {
			if int64(u) > last {
				uids = append(uids, u)
			}
		}
	}
	return uids, true, nil
}

func (s *Server) indexQuery(ctx context.Context, account, folder, text string, before uint32, page int) ([]uint32, error) {
	norm := normSearch(strings.TrimSpace(text))
	var terms []string
	words := reSearchWord.FindAllString(norm, 12)
	if reAddressQuery.MatchString(text) {
		words = nil // an address or a domain: where it stands in the headers, not mentioned in a text
	}
	for _, w := range words {
		if utf8.RuneCountInString(w) >= 2 {
			terms = append(terms, w+":*")
		}
	}
	like := "%" + strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`).Replace(norm) + "%"
	q := `SELECT uid FROM msg_index WHERE account=$1 AND folder=$2 AND (hdr LIKE $3`
	args := []any{account, folder, like}
	if len(terms) > 0 {
		q += ` OR tsv @@ to_tsquery('` + searchConfig + `', $4)`
		args = append(args, strings.Join(terms, " & "))
	}
	q += `)`
	if before > 1 {
		args = append(args, int64(before))
		q += fmt.Sprintf(` AND uid < $%d`, len(args))
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if page > 0 {
		// A common word: walking the folder from the newest letter finds a page at once. A rare
		// one: that walk reads the whole folder, the word's own index is faster then.
		limit := fmt.Sprintf(` ORDER BY uid DESC LIMIT %d`, page+1)
		if out, err := s.idxRows(ctx, "SET LOCAL statement_timeout = 120", q+limit, args); err == nil {
			return out, nil
		}
		return s.idxRows(ctx, "SET LOCAL enable_indexscan = off; SET LOCAL enable_seqscan = off", q+limit, args)
	}
	return s.idxRows(ctx, "SET LOCAL enable_seqscan = off", q, args)
}

// idxRows runs a search of the index with the given planner settings, in its own transaction.
func (s *Server) idxRows(ctx context.Context, settings, q string, args []any) ([]uint32, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	for _, set := range strings.Split(settings, ";") {
		if _, err := tx.Exec(ctx, set); err != nil {
			return nil, err
		}
	}
	rows, err := tx.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []uint32
	for rows.Next() {
		var u int64
		if err := rows.Scan(&u); err != nil {
			return nil, err
		}
		out = append(out, uint32(u))
	}
	return out, rows.Err()
}

// ruLayout turns a query typed with the English layout into Russian: "ljujdjh" → the Russian word
// for "contract". The two strings below are the keyboard layouts (data).
// Empty when the query is not only Latin letters: an address, a number or Cyrillic is no slip.
// It is tried only when the query as typed found nothing.
func ruLayout(s string) string {
	const en = "qwertyuiop[]asdfghjkl;'zxcvbnm,.`"
	const ru = "йцукенгшщзхъфывапролджэячсмитьбюё"
	ruRunes := []rune(ru)
	var b strings.Builder
	latin := 0
	for _, r := range strings.ToLower(s) {
		if i := strings.IndexRune(en, r); i >= 0 {
			b.WriteRune(ruRunes[i])
			if r >= 'a' && r <= 'z' {
				latin++
			}
			continue
		}
		if r == ' ' {
			b.WriteRune(r)
			continue
		}
		return "" // digits, @, Cyrillic: not a layout slip
	}
	if latin < 3 {
		return ""
	}
	return b.String()
}

func hasFilters(c *imap.SearchCriteria) bool {
	return !c.Since.IsZero() || !c.Before.IsZero() || len(c.WithFlags) > 0 || len(c.WithoutFlags) > 0
}

func intersectUIDs(a, b []uint32) []uint32 {
	in := make(map[uint32]bool, len(b))
	for _, u := range b {
		in[u] = true
	}
	out := a[:0:0]
	for _, u := range a {
		if in[u] {
			out = append(out, u)
		}
	}
	return out
}

func uidList(uids []uint32) []int64 {
	out := make([]int64, len(uids))
	for i, u := range uids {
		out[i] = int64(u)
	}
	return out
}

// indexDB: the database for the previews kept in the index (see storedPreviews); set at start.
var indexDB *pgxpool.Pool

// storedPreviews: the list previews the indexer keeps for the given letters of a folder.
func storedPreviews(account, folder string, uids []uint32) map[uint32]string {
	out := map[uint32]string{}
	if indexDB == nil || len(uids) == 0 {
		return out
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	rows, err := indexDB.Query(ctx, `SELECT uid, preview FROM msg_index WHERE account=$1 AND folder=$2 AND uid = ANY($3) AND preview <> ''`,
		account, folder, uidList(uids))
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var u int64
		var p string
		if rows.Scan(&u, &p) == nil {
			out[uint32(u)] = p
		}
	}
	return out
}
