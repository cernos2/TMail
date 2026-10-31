package main

import (
	"bufio"
	"bytes"
	"fmt"
	"github.com/emersion/go-message/charset"
	"html"
	"io"
	"log"
	"mime"
	"net/http"
	"net/url"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap/client"
	"github.com/emersion/go-message/textproto"
	"github.com/microcosm-cc/bluemonday"
)

type Attachment struct {
	Part    string `json:"part"`
	Name    string `json:"name"`
	Mime    string `json:"mime"`
	Size    uint32 `json:"size"`
	Inline  bool   `json:"inline"`         // can be shown in the browser (images, PDF)
	Preview string `json:"preview"`        // image | pdf | text | sheet | office | ""
	Risk    string `json:"risk,omitempty"` // exec | macro | html: ask before downloading
	// Embedded: an image shown in the letter's text (by Content-ID). Listed like in Gmail, but not
	// carried into a draft or a forward as a separate file: the text holds it already.
	Embedded bool `json:"embedded,omitempty"`
}

type FullMessage struct {
	Summary
	HTML        string            `json:"html"`
	RemoteImgs  bool              `json:"remoteImages"`
	Attachments []Attachment      `json:"attachments"`
	ReplyTo     []Addr            `json:"replyTo"`
	InReplyTo   string            `json:"inReplyTo"`
	References  string            `json:"references"`
	Spam        bool              `json:"spam"`
	Bcc         []Addr            `json:"bcc"`
	Text        string            `json:"text,omitempty"`        // plain text body (drafts, ?text=1)
	ReplyFolder string            `json:"replyFolder,omitempty"` // draft of a reply: which message it answers
	ReplyUID    string            `json:"replyUid,omitempty"`
	Auth        map[string]string `json:"auth"` // spf/dkim/dmarc → pass|fail|none…
	AV          []string          `json:"av"`   // antivirus warnings: macro | encrypted | heuristic | spam | virus
}

var headerFields = []string{"Message-ID", "In-Reply-To", "References", "Reply-To", "Authentication-Results", "X-Spam", "X-TMAIL-Reply", "X-Virus"}

func (s *Server) handleMessage(w http.ResponseWriter, r *http.Request, sess *Session) {
	q := r.URL.Query()
	uid64, err := strconv.ParseUint(q.Get("uid"), 10, 32)
	if err != nil || uid64 == 0 {
		writeErr(w, http.StatusBadRequest, "No message specified")
		return
	}
	uid := uint32(uid64)
	s.withAccount(w, r, sess, q.Get("account"), false, func(c *client.Client, acc Account) {
		folder, err := s.resolveFolder(c, acc.Address, r)
		if err != nil {
			writeErr(w, http.StatusNotFound, "Folder not found")
			return
		}
		if _, err := c.Select(folder, false); err != nil {
			writeErr(w, http.StatusNotFound, "Folder not found")
			return
		}
		set := new(imap.SeqSet)
		set.AddNum(uid)
		hdr := &imap.BodySectionName{BodyPartName: imap.BodyPartName{Specifier: imap.HeaderSpecifier, Fields: headerFields}, Peek: true}
		items := []imap.FetchItem{imap.FetchUid, imap.FetchEnvelope, imap.FetchFlags, imap.FetchInternalDate, imap.FetchRFC822Size, imap.FetchBodyStructure, hdr.FetchItem()}
		m, err := fetchOne(c, set, items)
		if err != nil {
			serverErr(w, "fetch message", err)
			return
		}
		if m == nil {
			writeErr(w, http.StatusNotFound, "Message not found. It may have been moved or deleted")
			return
		}
		full := FullMessage{Summary: summaryOf(m, acc.Address, folder), Auth: map[string]string{}, ReplyTo: []Addr{}, AV: []string{}}
		if lit := firstBody(m); lit != nil {
			if h, err := textproto.ReadHeader(bufio.NewReader(lit)); err == nil {
				full.InReplyTo = strings.TrimSpace(h.Get("In-Reply-To"))
				full.References = strings.TrimSpace(h.Get("References"))
				full.Spam = strings.EqualFold(strings.TrimSpace(h.Get("X-Spam")), "yes")
				full.Auth = parseAuthResults(h.Values("Authentication-Results"))
				full.AV = avFlags(h.Values("X-Virus"))
				if rf, ru, ok := strings.Cut(h.Get("X-TMAIL-Reply"), "|"); ok {
					full.ReplyFolder, full.ReplyUID = rf, ru
				}
			}
		}
		full.Bcc = []Addr{}
		if m.Envelope != nil {
			for _, a := range m.Envelope.ReplyTo {
				full.ReplyTo = append(full.ReplyTo, addrOf(a))
			}
			for _, a := range m.Envelope.Bcc {
				full.Bcc = append(full.Bcc, addrOf(a))
			}
		}

		bs := m.BodyStructure
		full.Attachments = attachmentsOf(bs)
		htmlPart, textPart := bodyParts(bs)
		cidURL := func(cid string) string {
			return "/api/cid?" + url.Values{"account": {acc.Address}, "folder": {folder}, "uid": {strconv.Itoa(int(uid))}, "cid": {cid}}.Encode()
		}
		switch {
		case htmlPart != nil:
			raw, err := fetchPart(c, uid, htmlPart.path)
			if err != nil {
				serverErr(w, "fetch html part", err)
				return
			}
			full.HTML, full.RemoteImgs = sanitizeHTML(string(decodePart(raw, htmlPart.part.Encoding, htmlPart.part.Params["charset"])), cidURL)
		case textPart != nil:
			raw, err := fetchPart(c, uid, textPart.path)
			if err != nil {
				serverErr(w, "fetch text part", err)
				return
			}
			full.HTML = textToHTML(string(decodePart(raw, textPart.part.Encoding, textPart.part.Params["charset"])))
		}

		wantText := q.Get("text") == "1"
		if !wantText {
			if d, err := s.folders.byRole(c, acc.Address, "drafts"); err == nil && d == folder {
				wantText = true
			}
		}
		if wantText && textPart != nil {
			if raw, err := fetchPart(c, uid, textPart.path); err == nil {
				full.Text = string(decodePart(raw, textPart.part.Encoding, textPart.part.Params["charset"]))
			}
		}
		if sb, ok := s.sentByFor(r.Context(), acc.Address, []string{full.MessageID})[normMsgID(full.MessageID)]; ok {
			full.SentBy = &sb
		}
		if !full.Seen && q.Get("peek") != "1" {
			if err := c.UidStore(set, imap.FormatFlagsOp(imap.AddFlags, true), []interface{}{imap.SeenFlag}, nil); err == nil {
				full.Seen = true
				s.hub.countsChanged(acc.Address)
			}
		}
		writeJSON(w, http.StatusOK, full)
	})
}

func fetchOne(c *client.Client, set *imap.SeqSet, items []imap.FetchItem) (*imap.Message, error) {
	ch := make(chan *imap.Message, 1)
	done := make(chan error, 1)
	go func() { done <- c.UidFetch(set, items, ch) }()
	var m *imap.Message
	for x := range ch {
		m = x
	}
	return m, <-done
}

func fetchPart(c *client.Client, uid uint32, path []int) ([]byte, error) {
	set := new(imap.SeqSet)
	set.AddNum(uid)
	sec := &imap.BodySectionName{BodyPartName: imap.BodyPartName{Path: path}, Peek: true}
	m, err := fetchOne(c, set, []imap.FetchItem{imap.FetchUid, sec.FetchItem()})
	if err != nil {
		return nil, err
	}
	if m == nil {
		return nil, fmt.Errorf("message %d not found", uid)
	}
	lit := firstBody(m)
	if lit == nil {
		return nil, fmt.Errorf("part %v missing", path)
	}
	return io.ReadAll(lit)
}

type partRef struct {
	path []int
	part *imap.BodyStructure
}

// bodyParts returns the HTML and plain-text bodies (not attachments) of a message.
func bodyParts(bs *imap.BodyStructure) (htmlPart, textPart *partRef) {
	if bs == nil {
		return nil, nil
	}
	bs.Walk(func(p []int, part *imap.BodyStructure) bool {
		if len(part.Parts) > 0 || len(p) == 0 {
			return true
		}
		if strings.EqualFold(part.MIMEType, "message") {
			return false // do not descend into forwarded messages
		}
		if !strings.EqualFold(part.MIMEType, "text") || isAttachmentPart(part) {
			return true
		}
		switch strings.ToLower(part.MIMESubType) {
		case "html":
			if htmlPart == nil {
				htmlPart = &partRef{append([]int(nil), p...), part}
			}
		case "plain":
			if textPart == nil {
				textPart = &partRef{append([]int(nil), p...), part}
			}
		}
		return true
	})
	return
}

func attachmentsOf(bs *imap.BodyStructure) []Attachment {
	out := []Attachment{}
	if bs == nil {
		return out
	}
	bs.Walk(func(p []int, part *imap.BodyStructure) bool {
		if len(p) == 0 || len(part.Parts) > 0 && !strings.EqualFold(part.MIMEType, "message") {
			return true
		}
		embedded := part.Id != "" && strings.EqualFold(part.MIMEType, "image") && !isAttachmentPart(part)
		if !isAttachmentPart(part) && !embedded {
			return true
		}
		name := partFilename(part)
		mt := strings.ToLower(part.MIMEType + "/" + part.MIMESubType)
		if name == "" {
			name = "attachment"
			if exts, _ := mime.ExtensionsByType(mt); len(exts) > 0 {
				name += exts[0]
			}
			if mt == "message/rfc822" {
				name = "message.eml"
			}
		}
		out = append(out, Attachment{Part: pathString(p), Name: name, Mime: mt, Size: decodedSize(part), Inline: inlineSafe(mt), Preview: previewKind(mt, name), Risk: attachmentRisk(name), Embedded: embedded})
		return !strings.EqualFold(part.MIMEType, "message")
	})
	return out
}

// fileAttachments: the attachments without the images embedded in the text.
func fileAttachments(atts []Attachment) []Attachment {
	out := atts[:0:0]
	for _, a := range atts {
		if !a.Embedded {
			out = append(out, a)
		}
	}
	return out
}

// inlineSafe: types the browser may display from our origin without risk of running scripts.
func inlineSafe(mt string) bool {
	switch mt {
	case "image/png", "image/jpeg", "image/gif", "image/webp", "application/pdf":
		return true
	}
	return false
}

var textExts = map[string]bool{".txt": true, ".csv": true, ".log": true, ".md": true, ".json": true, ".xml": true,
	".yaml": true, ".yml": true, ".ini": true, ".conf": true, ".cfg": true, ".tsv": true}

// decodedSize estimates the file size from the encoded part size IMAP reports: base64 stores
// 57 bytes in each 78-byte line (76 characters + CRLF).
func decodedSize(p *imap.BodyStructure) uint32 {
	if strings.EqualFold(p.Encoding, "base64") {
		return uint32(uint64(p.Size) * 57 / 78)
	}
	return p.Size
}

// previewKind tells the browser how an attachment can be previewed.
func previewKind(mt, name string) string {
	switch {
	case strings.HasPrefix(mt, "image/") && inlineSafe(mt):
		return "image"
	case mt == "application/pdf":
		return "pdf"
	case strings.HasPrefix(mt, "text/") && mt != "text/html", mt == "application/json", mt == "application/xml",
		textExts[strings.ToLower(filepath.Ext(name))]:
		return "text"
	case sheetExts[strings.ToLower(filepath.Ext(name))]:
		return "sheet"
	case officeExts[strings.ToLower(filepath.Ext(name))]:
		return "office"
	}
	return ""
}

const maxTextPreview = 2 << 20

func hasNonASCII(b []byte) bool {
	for _, c := range b {
		if c >= 0x80 {
			return true
		}
	}
	return false
}

// textForPreview decodes a text attachment for display: declared charset, otherwise UTF-8 or,
// if the bytes are not valid UTF-8, Windows-1251 (common for Russian files from Windows).
func textForPreview(data []byte, cs string) string {
	if len(data) > maxTextPreview {
		data = data[:maxTextPreview]
	}
	data = bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))
	// IMAP reports us-ascii when a file declares no charset, so a declared ASCII/UTF-8 that
	// doesn't match the bytes is treated as unknown.
	lc := strings.ToLower(cs)
	if (cs == "" || lc == "us-ascii" || lc == "utf-8" || lc == "utf8") && !utf8.Valid(data) {
		cs = "windows-1251"
	} else if lc == "us-ascii" {
		cs = ""
	} else if utf8.Valid(data) && hasNonASCII(data) {
		cs = "" // valid UTF-8 with Cyrillic is almost certainly UTF-8, whatever the sender declared
	}
	if cs != "" && !strings.EqualFold(cs, "utf-8") {
		if r, err := charset.Reader(cs, bytes.NewReader(data)); err == nil {
			if conv, err := io.ReadAll(r); err == nil {
				data = conv
			}
		}
	}
	return string(bytes.ToValidUTF8(data, []byte("\uFFFD")))
}

// ---------- attachments and inline images ----------

func (s *Server) handleAttachment(w http.ResponseWriter, r *http.Request, sess *Session) {
	q := r.URL.Query()
	path, ok := parsePath(q.Get("part"))
	uid, err := strconv.ParseUint(q.Get("uid"), 10, 32)
	if !ok || err != nil {
		writeErr(w, http.StatusBadRequest, "No attachment specified")
		return
	}
	s.servePart(w, r, sess, uint32(uid), func(bs *imap.BodyStructure) ([]int, *imap.BodyStructure) {
		var found *imap.BodyStructure
		bs.Walk(func(p []int, part *imap.BodyStructure) bool {
			if pathString(p) == pathString(path) {
				found = part
				return false
			}
			return found == nil
		})
		return path, found
	}, q.Get("download") == "1")
}

func (s *Server) handleCID(w http.ResponseWriter, r *http.Request, sess *Session) {
	q := r.URL.Query()
	cid := strings.Trim(q.Get("cid"), "<>")
	uid, err := strconv.ParseUint(q.Get("uid"), 10, 32)
	if cid == "" || err != nil {
		writeErr(w, http.StatusBadRequest, "No image specified")
		return
	}
	s.servePart(w, r, sess, uint32(uid), func(bs *imap.BodyStructure) ([]int, *imap.BodyStructure) {
		var path []int
		var found *imap.BodyStructure
		bs.Walk(func(p []int, part *imap.BodyStructure) bool {
			if found == nil && len(p) > 0 && strings.EqualFold(strings.Trim(part.Id, "<>"), cid) {
				path, found = append([]int(nil), p...), part
			}
			return found == nil
		})
		return path, found
	}, false)
}

func (s *Server) servePart(w http.ResponseWriter, r *http.Request, sess *Session, uid uint32,
	pick func(*imap.BodyStructure) ([]int, *imap.BodyStructure), forceDownload bool) {
	s.withAccount(w, r, sess, r.URL.Query().Get("account"), false, func(c *client.Client, acc Account) {
		folder, err := s.resolveFolder(c, acc.Address, r)
		if err != nil {
			writeErr(w, http.StatusNotFound, "Folder not found")
			return
		}
		if _, err := c.Select(folder, true); err != nil {
			writeErr(w, http.StatusNotFound, "Folder not found")
			return
		}
		set := new(imap.SeqSet)
		set.AddNum(uid)
		m, err := fetchOne(c, set, []imap.FetchItem{imap.FetchUid, imap.FetchBodyStructure})
		if err != nil || m == nil || m.BodyStructure == nil {
			writeErr(w, http.StatusNotFound, "Message not found")
			return
		}
		path, part := pick(m.BodyStructure)
		if part == nil {
			writeErr(w, http.StatusNotFound, "Attachment not found")
			return
		}
		raw, err := fetchPart(c, uid, path)
		if err != nil {
			serverErr(w, "fetch attachment", err)
			return
		}
		data := decodeTransfer(raw, part.Encoding) // exact file bytes
		if r.URL.Query().Get("as") == "scan" {
			s.serveScan(w, r, data)
			return
		}
		// Nothing of a file — download, preview, recognised text — before the antivirus has
		// checked it. Only our own thumbnail picture is drawn right away.
		as := r.URL.Query().Get("as")
		if (as != "thumb" || previewKind(strings.ToLower(part.MIMEType+"/"+part.MIMESubType), partFilename(part)) != "image") &&
			!s.avGate(w, r, sess, partFilename(part), data) {
			return
		}
		if r.URL.Query().Get("as") == "text" {
			name := partFilename(part)
			if previewKind(strings.ToLower(part.MIMEType+"/"+part.MIMESubType), name) != "text" {
				writeErr(w, http.StatusBadRequest, "This file cannot be shown as text")
				return
			}
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox")
			w.Header().Set("Cache-Control", "private, no-cache")
			_, _ = io.WriteString(w, textForPreview(data, part.Params["charset"]))
			return
		}
		mt := strings.ToLower(part.MIMEType + "/" + part.MIMESubType)
		name := partFilename(part)
		if as := r.URL.Query().Get("as"); as == "ocr" || as == "page" {
			k := previewKind(mt, name)
			if k != "image" && k != "pdf" {
				writeErr(w, http.StatusBadRequest, "Text cannot be recognized in this file")
				return
			}
			if k == "pdf" && !strings.HasSuffix(strings.ToLower(name), ".pdf") {
				name += ".pdf"
			}
			if k == "image" && filepath.Ext(name) == "" {
				name += map[string]string{"image/png": ".png", "image/gif": ".gif", "image/webp": ".webp"}[mt]
				if filepath.Ext(name) == "" {
					name += ".jpg"
				}
			}
			if as == "page" {
				n, _ := strconv.Atoi(r.URL.Query().Get("n"))
				png, err := s.officePage(r.Context(), name, data, max(n, 1), 1600)
				if err != nil {
					writeErr(w, http.StatusUnprocessableEntity, "Could not show the page")
					return
				}
				w.Header().Set("Content-Type", "image/png")
				w.Header().Set("Cache-Control", "private, max-age=86400")
				_, _ = w.Write(png)
				return
			}
			out, err := s.officeRenderQ(r.Context(), "ocr", "", name, data)
			if err != nil {
				log.Printf("ocr %q: %v", name, err)
				writeErr(w, http.StatusUnprocessableEntity, "Could not recognize the text")
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Cache-Control", "private, max-age=86400")
			_, _ = w.Write(out)
			return
		}
		if r.URL.Query().Get("as") == "thumb" {
			s.serveThumb(w, r, mt, name, data)
			return
		}
		if r.URL.Query().Get("as") == "html" {
			if previewKind(mt, name) != "sheet" {
				writeErr(w, http.StatusBadRequest, "This file cannot be shown as a spreadsheet")
				return
			}
			out, err := s.sheetHTML(r.Context(), name, data)
			if err != nil {
				log.Printf("sheet preview %q: %v", name, err)
				writeErr(w, http.StatusUnprocessableEntity, "Could not prepare the spreadsheet preview. Download it instead")
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; sandbox")
			w.Header().Set("Cache-Control", "private, no-cache")
			_, _ = w.Write(out)
			return
		}
		if r.URL.Query().Get("as") == "pdf" {
			if k := previewKind(mt, name); k != "office" && k != "sheet" {
				writeErr(w, http.StatusBadRequest, "This document cannot be shown")
				return
			}
			pdf, err := s.officeToPDF(r.Context(), name, data)
			if err != nil {
				log.Printf("office preview %q: %v", name, err)
				writeErr(w, http.StatusUnprocessableEntity, "Could not prepare the document preview. Download it instead")
				return
			}
			w.Header().Set("Content-Type", "application/pdf")
			w.Header().Set("Content-Disposition", mime.FormatMediaType("inline", map[string]string{"filename": strings.TrimSuffix(name, filepath.Ext(name)) + ".pdf"}))
			w.Header().Set("Cache-Control", "private, max-age=3600")
			w.Header().Set("Content-Length", strconv.Itoa(len(pdf)))
			_, _ = w.Write(pdf)
			return
		}
		if name == "" {
			name = "attachment"
		}
		disp := "attachment"
		if inlineSafe(mt) && !forceDownload {
			disp = "inline"
			w.Header().Set("Content-Type", mt)
		} else {
			w.Header().Set("Content-Type", "application/octet-stream")
		}
		w.Header().Set("Content-Disposition", mime.FormatMediaType(disp, map[string]string{"filename": name}))
		w.Header().Set("Cache-Control", "private, max-age=3600")
		w.Header().Set("Content-Length", strconv.Itoa(len(data)))
		_, _ = w.Write(data)
	})
}

// ---------- HTML ----------

var policy = func() *bluemonday.Policy {
	p := bluemonday.UGCPolicy()
	p.AllowStyling()
	p.AllowAttrs("style").Globally()
	p.AllowAttrs("bgcolor", "align", "valign", "width", "height", "border", "cellpadding", "cellspacing", "color", "face", "size", "dir").Globally()
	p.AllowElements("font", "center", "style")
	p.AllowUnsafe(true) // keeps <style> blocks; the message is shown in a script-less sandboxed iframe with its own CSP
	p.AllowURLSchemes("http", "https", "mailto", "tel", "cid", "data")
	p.RequireNoReferrerOnLinks(true)
	p.AddTargetBlankToFullyQualifiedLinks(true)
	p.AllowAttrs("data-tmail-sig", "data-tmail-quote", "data-tmail-fwd", "data-tmail-files").Globally()
	return p
}()

// sheetPolicy keeps the tables and cell formatting of a spreadsheet converted by LibreOffice.
var sheetPolicy = func() *bluemonday.Policy {
	p := bluemonday.NewPolicy()
	p.AllowElements("table", "thead", "tbody", "tfoot", "tr", "td", "th", "colgroup", "col", "caption", "h1", "h2", "p", "br", "b", "i", "u", "s", "font", "span", "div", "a", "hr")
	p.AllowAttrs("colspan", "rowspan", "width", "height", "align", "valign", "bgcolor", "border", "cellspacing", "cellpadding", "span", "sdval", "sdnum").Globally()
	p.AllowAttrs("color", "face", "size").OnElements("font")
	p.AllowAttrs("style").Globally()
	p.AllowStyling()
	return p
}()

// composePolicy cleans HTML written in our editor before it is sent or stored.
var composePolicy = func() *bluemonday.Policy {
	p := bluemonday.UGCPolicy()
	p.AllowStyling()
	p.AllowAttrs("style").Globally()
	p.AllowURLSchemes("http", "https", "mailto", "tel", "cid")
	p.AllowAttrs("data-tmail-sig", "data-tmail-quote", "data-tmail-fwd", "data-tmail-files").Globally()
	// Link cards for large files (email-safe tables) and formatting from the editor toolbar: <font> for size/face/colour, width on resized images
	// (Outlook ignores CSS width on <img>), alignment on blocks.
	p.AllowElements("font")
	p.AllowAttrs("color", "face", "size").OnElements("font")
	p.AllowAttrs("width", "height").OnElements("img")
	p.AllowAttrs("align", "valign", "width", "height", "cellpadding", "cellspacing", "border", "bgcolor").Globally()
	return p
}()

var (
	reCidSrc    = regexp.MustCompile(`(?i)(src\s*=\s*["']?)cid:([^"'\s>]+)`)
	reRemoteImg = regexp.MustCompile(`(?i)(src\s*=\s*["']?\s*https?:|url\(\s*["']?\s*https?:|background\s*=\s*["']?\s*https?:)`)
	reURL       = regexp.MustCompile(`https?://[^\s<>"']+[^\s<>"'.,;:!?)\]]`)
)

func sanitizeHTML(in string, cidURL func(string) string) (string, bool) {
	out := stripOwnURLs(policy.Sanitize(in))
	out = reCidSrc.ReplaceAllStringFunc(out, func(m string) string {
		sub := reCidSrc.FindStringSubmatch(m)
		return sub[1] + html.EscapeString(cidURL(sub[2]))
	})
	// Pictures from our own large-file links do not count as tracking images.
	check := out
	if h := env("FILES_HOSTNAME", ""); h != "" {
		check = strings.ReplaceAll(check, "https://"+h+"/", "/")
	}
	return out, reRemoteImg.MatchString(check)
}

// A letter must not point at our own API (e.g. <img src="/api/avatar?email=…"> as a read receipt,
// or CSS url(https://mail…/api/…)). Runs before cid: images get their attachment URLs.
var reOwnURL = regexp.MustCompile(`(?i)((?:src|background|href)\s*=\s*["']?|url\(\s*["']?)\s*((?:https?:)?//[^/"'\s)>]+)?(/[^"'\s)>]*)`)

func stripOwnURLs(in string) string {
	host := strings.ToLower(env("MAIL_HOSTNAME", "localhost"))
	return reOwnURL.ReplaceAllStringFunc(in, func(m string) string {
		sub := reOwnURL.FindStringSubmatch(m)
		if sub[2] != "" {
			h := strings.ToLower(sub[2][strings.Index(sub[2], "//")+2:])
			if i := strings.LastIndex(h, "@"); i >= 0 {
				h = h[i+1:]
			}
			if j := strings.Index(h, ":"); j >= 0 {
				h = h[:j]
			}
			if h != host {
				return m // someone else's site
			}
		}
		if strings.HasPrefix(strings.ToLower(sub[1]), "href") && sub[2] == "" && !strings.HasPrefix(sub[3], "/api/") {
			return m
		}
		return sub[1] + "about:blank"
	})
}

func textToHTML(t string) string {
	esc := html.EscapeString(t)
	esc = reURL.ReplaceAllStringFunc(esc, func(u string) string {
		return `<a href="` + u + `" target="_blank" rel="noopener noreferrer">` + u + `</a>`
	})
	return `<div style="white-space:pre-wrap;word-wrap:break-word;font-family:inherit">` + esc + `</div>`
}

// parseAuthResults keeps the verdicts our own Rspamd wrote (spf=, dkim=, dmarc=).
var reAuth = regexp.MustCompile(`(?i)\b(spf|dkim|dmarc)=(\w+)`)

func parseAuthResults(values []string) map[string]string {
	out := map[string]string{}
	for _, v := range values {
		for _, m := range reAuth.FindAllStringSubmatch(v, -1) {
			k := strings.ToLower(m[1])
			if _, ok := out[k]; !ok {
				out[k] = strings.ToLower(m[2])
			}
		}
	}
	return out
}
