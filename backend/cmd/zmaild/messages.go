package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"html"
	"io"
	"mime/quotedprintable"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap/client"
	"github.com/emersion/go-message/charset"
	"github.com/emersion/go-message/textproto"
)

type Addr struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

type Summary struct {
	Account        string       `json:"account"`
	Folder         string       `json:"folder"`
	UID            uint32       `json:"uid"`
	From           Addr         `json:"from"`
	To             []Addr       `json:"to"`
	Cc             []Addr       `json:"cc,omitempty"`
	Subject        string       `json:"subject"`
	Date           time.Time    `json:"date"`
	Seen           bool         `json:"seen"`
	Flagged        bool         `json:"flagged"`
	Answered       bool         `json:"answered"`
	HasAttachments bool         `json:"hasAttachments"`
	Attachments    []Attachment `json:"attachments,omitempty"` // shown as chips in the list, like Gmail
	Size           uint32       `json:"size"`
	Preview        string       `json:"preview"`
	MessageID      string       `json:"messageId"`
	InReplyTo      string       `json:"inReplyTo,omitempty"`
	References     []string     `json:"refs,omitempty"` // for grouping into conversations
	Labels         []int64      `json:"labels,omitempty"`
	SentBy         *SentBy      `json:"sentBy,omitempty"`
	Dmarc          string       `json:"dmarc,omitempty"` // our Rspamd's verdict: pass | fail | …
}

// resolveFolder accepts ?folder=<name> or ?role=inbox|sent|drafts|junk|trash|archive.
func (s *Server) resolveFolder(c *client.Client, account string, r *http.Request) (string, error) {
	if f := r.URL.Query().Get("folder"); f != "" {
		return f, nil
	}
	role := r.URL.Query().Get("role")
	if role == "" {
		role = "inbox"
	}
	return s.folders.byRole(c, account, role)
}

func (s *Server) handleMessages(w http.ResponseWriter, r *http.Request, sess *Session) {
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	before, _ := strconv.ParseUint(q.Get("before"), 10, 32)

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
		// The filters (dates, unread, flagged) without the searched text: fresh criteria each time.
		base := func() *imap.SearchCriteria {
			crit := imap.NewSearchCriteria()
			if d, err := time.Parse("2006-01-02", q.Get("since")); err == nil {
				crit.Since = d
			}
			if d, err := time.Parse("2006-01-02", q.Get("until")); err == nil {
				crit.Before = d.AddDate(0, 0, 1)
			}
			if q.Get("unseen") == "1" {
				crit.WithoutFlags = append(crit.WithoutFlags, imap.SeenFlag)
			}
			if q.Get("flagged") == "1" {
				crit.WithFlags = append(crit.WithFlags, imap.FlaggedFlag)
			}
			return crit
		}
		text := strings.TrimSpace(q.Get("q"))
		if id, err := strconv.ParseInt(q.Get("label"), 10, 64); err == nil && id > 0 {
			// A label spans folders: search every folder except Trash, Junk and Drafts.
			crit := base()
			if text != "" {
				crit = withHeaderText(crit, text) // headers only: the full-text index in every folder would hang
			}
			crit.WithFlags = append(crit.WithFlags, labelKeyword(id))
			msgs, err := s.searchAllFolders(c, acc.Address, crit, limit)
			if err != nil {
				serverErr(w, "label search", err)
				return
			}
			s.attachSentBy(r.Context(), acc.Address, msgs)
			writeJSON(w, http.StatusOK, map[string]any{"folder": "", "messages": msgs, "hasMore": false})
			return
		}
		if before == 1 {
			writeJSON(w, http.StatusOK, map[string]any{"folder": folder, "messages": []Summary{}, "hasMore": false})
			return
		}
		var uids []uint32
		partial, indexed := false, false
		page := limit // the index gives one page; with filters it gives all, they are applied after
		if hasFilters(base()) {
			page = 0
		}
		switch {
		case text == "":
			crit := base()
			if before > 1 {
				crit.Uid = new(imap.SeqSet)
				crit.Uid.AddRange(1, uint32(before-1))
			}
			uids, err = c.UidSearch(crit)
		case reAddressQuery.MatchString(text):
			// Sender or recipient: headers only, without the full-text index (see dialSearch).
			uids, indexed, err = s.quickSearch(r.Context(), acc.Address, folder, text, base, uint32(before), page)
		default:
			// Subject, addresses and text at once; attachments within a time budget (see searchText).
			uids, partial, indexed, err = s.searchText(r.Context(), acc.Address, folder, text, base, uint32(before), page)
		}
		if err != nil {
			serverErr(w, "search", err)
			return
		}
		if indexed && hasFilters(base()) {
			// The index knows no flags: the filters (unread, flagged, period) come from Dovecot.
			if keep, err := c.UidSearch(base()); err == nil {
				uids = intersectUIDs(uids, keep)
			}
		}
		if before > 1 && text != "" { // a further page of the same search: the result is cached
			kept := uids[:0:0]
			for _, u := range uids {
				if uint64(u) < before {
					kept = append(kept, u)
				}
			}
			uids = kept
		}
		sort.Slice(uids, func(i, j int) bool { return uids[i] < uids[j] })
		hasMore := len(uids) > limit
		if hasMore {
			uids = uids[len(uids)-limit:]
		}
		msgs, err := fetchSummaries(c, acc.Address, folder, uids)
		if err != nil {
			serverErr(w, "fetch summaries", err)
			return
		}
		s.attachSentBy(r.Context(), acc.Address, msgs)
		writeJSON(w, http.StatusOK, map[string]any{"folder": folder, "messages": msgs, "hasMore": hasMore, "partial": partial, "indexed": indexed})
	})
}

// previews: list previews of recent letters (~60 000, a few MB), the oldest forgotten first.
var previews = &previewCache{m: map[string]string{}, max: 60000}

type previewCache struct {
	mu    sync.Mutex
	m     map[string]string
	order []string
	max   int
}

func (p *previewCache) get(k string) (string, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	v, ok := p.m[k]
	return v, ok
}

func (p *previewCache) put(k, v string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if _, ok := p.m[k]; !ok {
		p.order = append(p.order, k)
	}
	p.m[k] = v
	if len(p.order) > p.max {
		drop := p.order[:len(p.order)-p.max]
		for _, d := range drop {
			delete(p.m, d)
		}
		p.order = append([]string(nil), p.order[len(drop):]...)
	}
}

// fetchSummaries returns list rows (newest first) for the given UIDs of the selected folder.
func fetchSummaries(c *client.Client, account, folder string, uids []uint32) ([]Summary, error) {
	out := []Summary{}
	if len(uids) == 0 {
		return out, nil
	}
	set := new(imap.SeqSet)
	set.AddNum(uids...)
	authHdr := &imap.BodySectionName{BodyPartName: imap.BodyPartName{Specifier: imap.HeaderSpecifier, Fields: []string{"Authentication-Results", "References"}}, Peek: true}
	items := []imap.FetchItem{imap.FetchUid, imap.FetchEnvelope, imap.FetchFlags, imap.FetchInternalDate, imap.FetchRFC822Size, imap.FetchBodyStructure, authHdr.FetchItem()}
	ch := make(chan *imap.Message, 16)
	done := make(chan error, 1)
	go func() { done <- c.UidFetch(set, items, ch) }()

	type previewSrc struct {
		path    []int
		enc, cs string
		html    bool
	}
	srcs := map[uint32]previewSrc{}
	idx := map[uint32]int{}
	for m := range ch {
		sm := summaryOf(m, account, folder)
		if lit := firstBody(m); lit != nil {
			if h, err := textproto.ReadHeader(bufio.NewReader(lit)); err == nil {
				sm.Dmarc = parseAuthResults(h.Values("Authentication-Results"))["dmarc"]
				sm.References = msgIDs(h.Get("References"))
			}
		}
		if m.BodyStructure != nil {
			if p, enc, cs, isHTML := previewPart(m.BodyStructure); p != nil {
				srcs[m.Uid] = previewSrc{p, enc, cs, isHTML}
			}
		}
		idx[m.Uid] = len(out)
		out = append(out, sm)
	}
	if err := <-done; err != nil {
		return nil, err
	}

	// A letter's preview never changes: those made before come from memory (the slowest part of
	// a list was reading the start of every letter from the disk).
	var validity uint32
	if mb := c.Mailbox(); mb != nil && mb.Name == folder {
		validity = mb.UidValidity
	}
	pkey := func(uid uint32) string {
		return account + "\x00" + folder + "\x00" + strconv.FormatUint(uint64(validity), 10) + "\x00" + strconv.FormatUint(uint64(uid), 10)
	}
	if validity != 0 {
		for uid := range srcs {
			if p, ok := previews.get(pkey(uid)); ok {
				out[idx[uid]].Preview = p
				delete(srcs, uid)
			}
		}
	}

	// Then those the search index keeps (old letters found by a search).
	if len(srcs) > 0 {
		left := make([]uint32, 0, len(srcs))
		for uid := range srcs {
			left = append(left, uid)
		}
		for uid, p := range storedPreviews(account, folder, left) {
			out[idx[uid]].Preview = p
			if validity != 0 {
				previews.put(pkey(uid), p)
			}
			delete(srcs, uid)
		}
	}

	// Fetch the first bytes of the text part, grouped by part path (one FETCH per distinct path).
	groups := map[string][]uint32{}
	for uid, src := range srcs {
		groups[pathString(src.path)] = append(groups[pathString(src.path)], uid)
	}
	for _, guids := range groups {
		src := srcs[guids[0]]
		// Letters often begin with kilobytes of <style>: take enough to reach the text.
		size := 4000 // some "plain text" parts also start with a style sheet
		if src.html {
			size = 12000
		}
		sec := &imap.BodySectionName{BodyPartName: imap.BodyPartName{Path: src.path}, Peek: true, Partial: []int{0, size}}
		gset := new(imap.SeqSet)
		gset.AddNum(guids...)
		gch := make(chan *imap.Message, 16)
		gdone := make(chan error, 1)
		go func() { gdone <- c.UidFetch(gset, []imap.FetchItem{imap.FetchUid, sec.FetchItem()}, gch) }()
		for m := range gch {
			lit := firstBody(m)
			if lit == nil {
				continue
			}
			raw, _ := io.ReadAll(lit)
			ps := srcs[m.Uid]
			if i, ok := idx[m.Uid]; ok {
				out[i].Preview = makePreview(raw, ps.enc, ps.cs, ps.html)
				if validity != 0 {
					previews.put(pkey(m.Uid), out[i].Preview)
				}
			}
		}
		if err := <-gdone; err != nil {
			return nil, err
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UID > out[j].UID })
	return out, nil
}

func summaryOf(m *imap.Message, account, folder string) Summary {
	sm := Summary{Account: account, Folder: folder, UID: m.Uid, Date: m.InternalDate, Size: m.Size, To: []Addr{}}
	for _, f := range m.Flags {
		switch f {
		case imap.SeenFlag:
			sm.Seen = true
		case imap.FlaggedFlag:
			sm.Flagged = true
		case imap.AnsweredFlag:
			sm.Answered = true
		default:
			if id, ok := labelOfKeyword(f); ok {
				sm.Labels = append(sm.Labels, id)
			}
		}
	}
	if e := m.Envelope; e != nil {
		sm.Subject = e.Subject
		sm.MessageID = e.MessageId
		if ids := msgIDs(e.InReplyTo); len(ids) > 0 {
			sm.InReplyTo = ids[0]
		}
		if len(e.From) > 0 {
			sm.From = addrOf(e.From[0])
		}
		for _, a := range e.To {
			sm.To = append(sm.To, addrOf(a))
		}
		for _, a := range e.Cc {
			sm.Cc = append(sm.Cc, addrOf(a))
		}
		if sm.Date.IsZero() {
			sm.Date = e.Date
		}
	}
	if m.BodyStructure != nil {
		if atts := attachmentsOf(m.BodyStructure); len(atts) > 0 {
			sm.HasAttachments, sm.Attachments = true, atts
		}
	}
	return sm
}

func addrOf(a *imap.Address) Addr {
	email := a.MailboxName
	if a.HostName != "" {
		email += "@" + a.HostName
	}
	return Addr{Name: strings.TrimSpace(a.PersonalName), Email: strings.ToLower(email)}
}

func firstBody(m *imap.Message) imap.Literal {
	for _, lit := range m.Body {
		if lit != nil {
			return lit
		}
	}
	return nil
}

func pathString(p []int) string {
	parts := make([]string, len(p))
	for i, n := range p {
		parts[i] = strconv.Itoa(n)
	}
	return strings.Join(parts, ".")
}

func parsePath(s string) ([]int, bool) {
	if s == "" {
		return nil, false
	}
	var out []int
	for _, p := range strings.Split(s, ".") {
		n, err := strconv.Atoi(p)
		if err != nil || n <= 0 {
			return nil, false
		}
		out = append(out, n)
	}
	return out, true
}

// isAttachmentPart: files the user should see in the attachment list.
// Images embedded in the HTML by Content-ID are not listed.
func isAttachmentPart(p *imap.BodyStructure) bool {
	if strings.EqualFold(p.Disposition, "attachment") {
		return true
	}
	mt := strings.ToLower(p.MIMEType)
	inlineImage := p.Id != "" && mt == "image"
	if name := partFilename(p); name != "" {
		return !inlineImage
	}
	switch mt {
	case "text", "multipart":
		return false
	case "message":
		return true
	}
	return !inlineImage && p.Id == ""
}

// previewPart picks the first non-attachment text/plain part, falling back to text/html.
func previewPart(bs *imap.BodyStructure) (path []int, enc, cs string, isHTML bool) {
	var htmlPath []int
	var htmlEnc, htmlCS string
	bs.Walk(func(p []int, part *imap.BodyStructure) bool {
		if path != nil || len(part.Parts) > 0 || len(p) == 0 {
			return path == nil
		}
		if !strings.EqualFold(part.MIMEType, "text") || isAttachmentPart(part) {
			return true
		}
		switch strings.ToLower(part.MIMESubType) {
		case "plain":
			path, enc, cs = append([]int(nil), p...), part.Encoding, part.Params["charset"]
		case "html":
			if htmlPath == nil {
				htmlPath, htmlEnc, htmlCS = append([]int(nil), p...), part.Encoding, part.Params["charset"]
			}
		}
		return true
	})
	if path == nil && htmlPath != nil {
		return htmlPath, htmlEnc, htmlCS, true
	}
	return path, enc, cs, false
}

// decodeTransfer undoes base64/quoted-printable and returns the exact bytes of the part.
// Use it for attachments and images: their content must not be altered.
// Truncated input (previews) is tolerated.
func decodeTransfer(raw []byte, enc string) []byte {
	switch strings.ToLower(enc) {
	case "base64":
		clean := bytes.Map(func(r rune) rune {
			if r == '\r' || r == '\n' || r == ' ' || r == '\t' {
				return -1
			}
			return r
		}, raw)
		clean = clean[:len(clean)-len(clean)%4]
		b := make([]byte, base64.StdEncoding.DecodedLen(len(clean)))
		n, _ := base64.StdEncoding.Decode(b, clean)
		return b[:n]
	case "quoted-printable":
		b, _ := io.ReadAll(quotedprintable.NewReader(bytes.NewReader(raw)))
		return b
	}
	return raw
}

// decodePart decodes a text body part to valid UTF-8 (transfer encoding + charset).
// Never use it for attachments: it rewrites bytes that are not valid text.
func decodePart(raw []byte, enc, cs string) []byte {
	b := decodeTransfer(raw, enc)
	cs = strings.ToLower(strings.TrimSpace(cs))
	if cs != "" && cs != "utf-8" && cs != "us-ascii" && cs != "utf8" {
		if r, err := charset.Reader(cs, bytes.NewReader(b)); err == nil {
			if conv, err := io.ReadAll(r); err == nil || len(conv) > 0 {
				b = conv
			}
		}
	}
	return bytes.ToValidUTF8(b, nil)
}

var (
	reStyleScript = regexp.MustCompile(`(?is)<(style|script|head|title)[^>]*>.*?(</(style|script|head|title)>|$)`) // also cut off by the preview's length
	reComment     = regexp.MustCompile(`(?s)<!--.*?(-->|$)`)
	reTag         = regexp.MustCompile(`(?s)<[^>]*>`)
	reTagTail     = regexp.MustCompile(`<[^>]*$`) // a tag cut off at the end
	reCSSRule     = regexp.MustCompile(`(?s)[^{}<>]{0,300}\{[^{}]*:[^{}]*\}`)
	reCSSEmpty    = regexp.MustCompile(`(?s)[^{}<>]{0,300}\{\s*\}`)          // what is left of @media { … } after its rules
	reCSSOpen     = regexp.MustCompile(`(?s)[^{}<>]{0,300}\{[^{}]*:[^{}]*$`) // a rule cut off at the end
	reLooksHTML   = regexp.MustCompile(`(?i)<(html|head|body|div|table|p|br|span|style|!doctype|meta)[\s>/]`)
	reCSSComment  = regexp.MustCompile(`(?s)/\*.*?(\*/|$)`)
	// Invisible fillers mailers pad previews with: ⠀ (braille blank), combining grapheme joiner,
	// zero-width spaces and joiners, soft hyphen, BOM.
	reFiller    = regexp.MustCompile("[\u2800\u034f\u200b-\u200f\u00ad\ufeff\u2060\u180e]+")
	reSpaces    = regexp.MustCompile(`\s+`)
	reQuoteLine = regexp.MustCompile(`(?m)^>.*$`)
)

func makePreview(raw []byte, enc, cs string, isHTML bool) string {
	return previewText(string(decodePart(raw, enc, cs)), isHTML)
}

// previewText: the readable start of a letter for the list — no tags, styles, CSS or comments,
// even when the part is cut in the middle of them or a "plain text" part is really HTML.
func previewText(t string, isHTML bool) string {
	if isHTML || reLooksHTML.MatchString(t) {
		t = reComment.ReplaceAllString(t, " ")
		t = reStyleScript.ReplaceAllString(t, " ")
		t = reTag.ReplaceAllString(t, " ")
		t = reTagTail.ReplaceAllString(t, " ")
		t = html.UnescapeString(t)
	} else {
		t = reQuoteLine.ReplaceAllString(t, " ")
	}
	// Style sheets that leaked into the text: «body { margin: 0; … }», «*{box-sizing:…}», @media.
	t = reFiller.ReplaceAllString(t, " ")
	if strings.Contains(t, "/*") {
		t = reCSSComment.ReplaceAllString(t, " ")
	}
	if strings.Contains(t, "{") {
		for i := 0; i < 3; i++ {
			t = reCSSEmpty.ReplaceAllString(reCSSRule.ReplaceAllString(t, " "), " ")
		}
		t = reCSSOpen.ReplaceAllString(t, " ")
		t = strings.NewReplacer("{", " ", "}", " ").Replace(t)
	}
	t = strings.TrimSpace(reSpaces.ReplaceAllString(t, " "))
	if utf8.RuneCountInString(t) > 180 {
		t = string([]rune(t)[:180]) + "…"
	}
	return t
}

// attachSentBy marks messages written from the webmail with the login of their author.
func (s *Server) attachSentBy(ctx context.Context, mailbox string, msgs []Summary) {
	ids := make([]string, len(msgs))
	for i, m := range msgs {
		ids[i] = m.MessageID
	}
	by := s.sentByFor(ctx, mailbox, ids)
	for i := range msgs {
		if sb, ok := by[normMsgID(msgs[i].MessageID)]; ok {
			sb := sb
			msgs[i].SentBy = &sb
		}
	}
}
