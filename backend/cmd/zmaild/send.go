package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"fmt"
	"html"
	"io"
	"log/slog"
	"mime"
	"mime/multipart"
	"net/http"
	"net/mail"
	"net/smtp"
	"net/textproto"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap/client"
	gomsg "github.com/emersion/go-message"
	gomail "github.com/emersion/go-message/mail"
)

const maxSendSize = 40 << 20 // 25 MB of attachments after base64; Postfix accepts up to 50 MB

// composeInput is a message from the composer (multipart form), used for sending and for drafts.
type composeInput struct {
	acc             Account
	ident           *extAccount // sending as an external address (its own SMTP)
	to, cc, bcc     []*mail.Address
	subject, text   string
	inReplyTo, refs string
	replyFolder     string
	replyUID        string
	html            string // body from the rich editor; inline images referenced as cid:
	files           []*multipart.FileHeader
	inline          []inlineUpload // images pasted into the body, uploaded now
	carry           []fileData     // server-side attachments (forwarded message or previous draft)
	carryInline     []fileData     // server-side inline images of the previous draft (by Content-ID)
	draftAccount    string
	draftUID        uint32
}

type inlineUpload struct {
	cid string
	fh  *multipart.FileHeader
}

var cidRe = regexp.MustCompile(`^[A-Za-z0-9._%+-]{1,120}@[A-Za-z0-9.-]{1,120}$`)

// parseCompose reads the composer form. needSend=false (drafts) allows mailboxes without send rights
// to be checked later and allows empty recipients.
func (s *Server) parseCompose(w http.ResponseWriter, r *http.Request, sess *Session, forSend bool) (*composeInput, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, maxSendSize+(1<<20))
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		writeErr(w, http.StatusRequestEntityTooLarge, "The message is too large: attachments together must not exceed 25 MB. Large files are sent as a link")
		return nil, false
	}
	f := r.MultipartForm.Value
	get := func(k string) string {
		if v := f[k]; len(v) > 0 {
			return strings.TrimSpace(v[0])
		}
		return ""
	}
	from := strings.ToLower(get("from"))
	acc, ok := sess.account(from)
	var ident *extAccount
	if !ok {
		// An external address connected in the collector: the letter belongs to its mailbox.
		if ea, mb, found := s.identityFor(r.Context(), sess, from); found {
			acc, ok, ident = mb, true, &ea
		}
	}
	if ok && ident == nil {
		if ea, found := s.externalFor(r.Context(), acc.Address); found {
			if ea.SMTPHost == "" || len(ea.smtpPassEnc) == 0 {
				if forSend {
					writeErr(w, http.StatusBadRequest, fmt.Sprintf("Sending (SMTP) is not set up for %s. An administrator can add it in \"Mail collector\"", acc.Address))
					return nil, false
				}
			} else {
				ident = &ea
			}
		}
	}
	if !ok || (forSend && !acc.CanSend) {
		writeErr(w, http.StatusForbidden, fmt.Sprintf("You cannot send from the address %s", from))
		return nil, false
	}
	in := &composeInput{acc: acc, ident: ident, subject: get("subject"), inReplyTo: get("inReplyTo"), refs: get("references"),
		replyFolder: get("replyFolder"), replyUID: get("replyUid"), files: r.MultipartForm.File["files"]}
	in.text = strings.ReplaceAll(get("text"), "\r\n", "\n")
	in.html = get("html")
	cids := f["inlineCid"]
	for i, fh := range r.MultipartForm.File["inline"] {
		if i < len(cids) && cidRe.MatchString(cids[i]) && strings.HasPrefix(fh.Header.Get("Content-Type"), "image/") {
			in.inline = append(in.inline, inlineUpload{cid: cids[i], fh: fh})
		}
	}
	var errs [3]error
	in.to, errs[0] = parseAddrs(get("to"))
	in.cc, errs[1] = parseAddrs(get("cc"))
	in.bcc, errs[2] = parseAddrs(get("bcc"))
	for _, err := range errs {
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return nil, false
		}
	}
	if forSend && len(in.to)+len(in.cc)+len(in.bcc) == 0 {
		writeErr(w, http.StatusBadRequest, "Specify at least one recipient")
		return nil, false
	}
	if da, du := strings.ToLower(get("draftAccount")), get("draftUid"); da != "" && du != "" {
		if _, ok := sess.account(da); ok {
			if n, err := strconv.ParseUint(du, 10, 32); err == nil {
				in.draftAccount, in.draftUID = da, uint32(n)
			}
		}
	}
	// Attachments that already live on the server: of a forwarded message or of the previous draft.
	if ca, cf, cu := strings.ToLower(get("carryAccount")), get("carryFolder"), get("carryUid"); ca != "" && cf != "" && cu != "" {
		if _, ok := sess.account(ca); !ok {
			writeErr(w, http.StatusForbidden, "No access to the message with the attachments")
			return nil, false
		}
		uid, perr := strconv.ParseUint(cu, 10, 32)
		var keep map[string]bool
		if _, listed := f["carryParts"]; listed {
			keep = map[string]bool{}
			for _, p := range strings.Split(get("carryParts"), ",") {
				if p = strings.TrimSpace(p); p != "" {
					keep[p] = true
				}
			}
		}
		keepCID := map[string]bool{}
		for _, id := range strings.Split(get("carryCids"), ",") {
			if id = strings.TrimSpace(id); cidRe.MatchString(id) {
				keepCID[id] = true
			}
		}
		if perr == nil && (keep == nil || len(keep) > 0 || len(keepCID) > 0) {
			c, err := s.dialAs(ca)
			if err != nil {
				serverErr(w, "carry dial", err)
				return nil, false
			}
			if _, err := c.Select(cf, true); err == nil {
				if keep == nil || len(keep) > 0 {
					in.carry, _ = collectAttachments(c, uint32(uid), keep)
				}
				if len(keepCID) > 0 {
					in.carryInline, _ = collectInline(c, uint32(uid), keepCID)
				}
			}
			_ = c.Logout()
		}
	}
	return in, true
}

// buildMessage renders the MIME message:
//
//	multipart/mixed            (only with attachments)
//	  multipart/related        (only with images in the body)
//	    multipart/alternative  text/plain + text/html
//	    image/* inline, Content-ID
//	  attachments
//
// Drafts keep Bcc and a private X-TMAIL-Reply header (which message is being answered); sent mail has neither.
func buildMessage(in *composeInput, draft bool) ([]byte, string, error) {
	var h gomail.Header
	h.SetDate(time.Now())
	fromAddr := &gomail.Address{Name: in.acc.Name, Address: in.acc.Address}
	if in.ident != nil {
		fromAddr = &gomail.Address{Name: in.ident.DisplayName, Address: in.ident.Address}
	}
	h.SetAddressList("From", []*gomail.Address{fromAddr})
	if len(in.to) > 0 {
		h.SetAddressList("To", toGoMail(in.to))
	}
	if len(in.cc) > 0 {
		h.SetAddressList("Cc", toGoMail(in.cc))
	}
	if draft && len(in.bcc) > 0 {
		h.SetAddressList("Bcc", toGoMail(in.bcc))
	}
	h.SetSubject(in.subject)
	msgID := newMessageID(fromAddr.Address)
	h.Set("Message-Id", "<"+msgID+">")
	if in.inReplyTo != "" {
		h.Set("In-Reply-To", in.inReplyTo)
		h.Set("References", strings.TrimSpace(in.refs+" "+in.inReplyTo))
	}
	if draft && in.replyFolder != "" && in.replyUID != "" {
		h.Set("X-TMAIL-Reply", in.replyFolder+"|"+in.replyUID)
	}
	h.Set("User-Agent", "TMAIL")
	h.Set("MIME-Version", "1.0")

	htmlBody := plainToHTML(in.text)
	if strings.TrimSpace(in.html) != "" {
		htmlBody = `<div style="font-family:Arial,Helvetica,sans-serif;font-size:14px;line-height:1.5">` + composePolicy.Sanitize(in.html) + `</div>`
	}

	part := func(w *gomsg.Writer, ct, cte string, extra map[string]string, body io.Reader) error {
		var ph gomsg.Header
		ph.Set("Content-Type", ct)
		ph.Set("Content-Transfer-Encoding", cte)
		for k, v := range extra {
			ph.Set(k, v)
		}
		pw, err := w.CreatePart(ph)
		if err != nil {
			return err
		}
		if _, err := io.Copy(pw, body); err != nil {
			return err
		}
		return pw.Close()
	}
	nested := func(w *gomsg.Writer, ct string, fill func(*gomsg.Writer) error) error {
		var ph gomsg.Header
		ph.Set("Content-Type", ct)
		nw, err := w.CreatePart(ph)
		if err != nil {
			return err
		}
		if err := fill(nw); err != nil {
			return err
		}
		return nw.Close()
	}
	writeAlt := func(w *gomsg.Writer) error {
		if err := part(w, "text/plain; charset=utf-8", "quoted-printable", nil, strings.NewReader(in.text)); err != nil {
			return err
		}
		return part(w, "text/html; charset=utf-8", "quoted-printable", nil, strings.NewReader(htmlBody))
	}
	fileHeader := func(disp, name, cid string) map[string]string {
		x := map[string]string{"Content-Disposition": mime.FormatMediaType(disp, map[string]string{"filename": name})}
		if cid != "" {
			x["Content-ID"] = "<" + cid + ">"
		}
		return x
	}
	withName := func(ct, name string) string {
		mt, params, err := mime.ParseMediaType(ct)
		if err != nil {
			mt, params = "application/octet-stream", map[string]string{}
		}
		params["name"] = name
		return mime.FormatMediaType(mt, params)
	}
	writeRelated := func(w *gomsg.Writer) error {
		if err := nested(w, "multipart/alternative", writeAlt); err != nil {
			return err
		}
		for _, fd := range in.carryInline {
			if err := part(w, withName(fd.Mime, fd.Name), "base64", fileHeader("inline", fd.Name, fd.Cid), bytes.NewReader(fd.Data)); err != nil {
				return err
			}
		}
		for _, up := range in.inline {
			src, err := up.fh.Open()
			if err != nil {
				return err
			}
			name := filepath.Base(up.fh.Filename)
			err = part(w, withName(up.fh.Header.Get("Content-Type"), name), "base64", fileHeader("inline", name, up.cid), src)
			src.Close()
			if err != nil {
				return err
			}
		}
		return nil
	}
	hasInline := len(in.inline)+len(in.carryInline) > 0
	hasFiles := len(in.files)+len(in.carry) > 0

	var buf bytes.Buffer
	top := "multipart/alternative"
	switch {
	case hasFiles:
		top = "multipart/mixed"
	case hasInline:
		top = "multipart/related"
	}
	h.Set("Content-Type", top)
	mw, err := gomsg.CreateWriter(&buf, h.Header)
	if err != nil {
		return nil, "", err
	}
	switch top {
	case "multipart/alternative":
		err = writeAlt(mw)
	case "multipart/related":
		err = writeRelated(mw)
	default:
		if hasInline {
			err = nested(mw, "multipart/related", writeRelated)
		} else {
			err = nested(mw, "multipart/alternative", writeAlt)
		}
		for _, fd := range in.carry {
			if err != nil {
				break
			}
			err = part(mw, withName(fd.Mime, fd.Name), "base64", fileHeader("attachment", fd.Name, ""), bytes.NewReader(fd.Data))
		}
		for _, fh := range in.files {
			if err != nil {
				break
			}
			ct := fh.Header.Get("Content-Type")
			if ct == "" {
				ct = mime.TypeByExtension(filepath.Ext(fh.Filename))
			}
			if ct == "" {
				ct = "application/octet-stream"
			}
			src, oerr := fh.Open()
			if oerr != nil {
				err = oerr
				break
			}
			name := filepath.Base(fh.Filename)
			err = part(mw, withName(ct, name), "base64", fileHeader("attachment", name, ""), src)
			src.Close()
		}
	}
	if err != nil {
		return nil, "", err
	}
	if err := mw.Close(); err != nil {
		return nil, "", err
	}
	return buf.Bytes(), msgID, nil
}

func (s *Server) handleSend(w http.ResponseWriter, r *http.Request, sess *Session) {
	in, ok := s.parseCompose(w, r, sess, true)
	if !ok {
		return
	}
	msg, msgID, err := buildMessage(in, false)
	if err != nil {
		serverErr(w, "build message", err)
		return
	}
	if len(msg) > maxSendSize {
		writeErr(w, http.StatusRequestEntityTooLarge, "The message is too large: attachments together must not exceed 25 MB. Large files are sent as a link")
		return
	}
	// Programs are refused as attachments (Rspamd would bounce them anyway); they go as a file link.
	for _, f := range in.files {
		if attachmentRisk(f.Filename) == "exec" {
			writeErr(w, http.StatusBadRequest, fmt.Sprintf("%q is an executable file: mail servers do not accept such attachments. Send it as a link.", f.Filename))
			return
		}
	}
	for _, f := range in.carry {
		if attachmentRisk(f.Name) == "exec" {
			writeErr(w, http.StatusBadRequest, fmt.Sprintf("%q is an executable file: mail servers do not accept such attachments. Send it as a link.", f.Name))
			return
		}
	}
	{ // the whole letter, inline pictures and carried parts included
		ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second) // the user is waiting
		found, err := clamScanBytes(ctx, msg)
		cancel()
		if err != nil {
			slog.Warn("antivirus on send", "err", err) // fail open, like for incoming mail
		} else if avKind(found) == "virus" {
			s.audit(r.Context(), sess.UserID, sess.Login, clientIP(r), "av.send_blocked", in.acc.Address, map[string]any{"virus": found})
			writeErr(w, http.StatusUnprocessableEntity, fmt.Sprintf("A virus was found in an attachment (%s). The message was not sent. Check the file on your computer.", found))
			return
		}
	}
	acc := in.acc
	rcpts := append(append(emails(in.to), emails(in.cc)...), emails(in.bcc)...)
	var err2 error
	if in.ident != nil {
		err2 = s.submitExternal(*in.ident, rcpts, msg)
		if err2 == nil {
			go s.afterExternalSend(*in.ident, msgID, msg)
		}
	} else {
		err2 = s.submit(acc.Address, rcpts, msg)
	}
	if err := err2; err != nil {
		slog.Error("smtp submit", "from", acc.Address, "err", err)
		writeErr(w, http.StatusBadGateway, sendErrorText(err))
		return
	}
	slog.Info("message sent", "from", acc.Address, "login", sess.Login, "rcpts", len(rcpts), "size", len(msg))
	s.recordSent(r.Context(), sess, acc.Address, msgID, in.subject, rcpts, len(msg))

	// Copy to Sent, mark the original as answered, remove the draft it was written in.
	// The copy's place goes back to the page: "Message sent" opens it.
	var sentFolder string
	var sentUID uint32
	if c, err := s.dialAs(acc.Address); err == nil {
		if sent, err := s.folders.byRole(c, acc.Address, "sent"); err == nil {
			if err := appendWithin(c, sent, []string{imap.SeenFlag}, time.Now(), msg, 2*time.Minute); err != nil {
				slog.Warn("append to Sent", "err", err)
			} else if _, err := c.Select(sent, true); err == nil {
				crit := imap.NewSearchCriteria()
				crit.Header.Add("Message-Id", msgID)
				if uids, err := c.UidSearch(crit); err == nil && len(uids) > 0 {
					sentFolder, sentUID = sent, uids[len(uids)-1]
				}
			}
		}
		if in.replyFolder != "" && in.replyUID != "" {
			markAnswered(c, in.replyFolder, in.replyUID)
		}
		_ = c.Logout()
	}
	if in.draftUID > 0 {
		s.deleteDraft(in.draftAccount, in.draftUID)
	}
	s.hub.countsChanged(acc.Address)
	writeJSON(w, http.StatusOK, map[string]any{"ok": "1", "messageId": msgID, "account": acc.Address, "folder": sentFolder, "uid": sentUID})
}

// handleSaveDraft stores the composer state as a message in the Drafts folder and replaces the
// previous version. Returns the new draft's UID and its attachments.
func (s *Server) handleSaveDraft(w http.ResponseWriter, r *http.Request, sess *Session) {
	in, ok := s.parseCompose(w, r, sess, false)
	if !ok {
		return
	}
	msg, msgID, err := buildMessage(in, true)
	if err != nil {
		serverErr(w, "build draft", err)
		return
	}
	if len(msg) > maxSendSize {
		writeErr(w, http.StatusRequestEntityTooLarge, "The draft is too large: attachments together must not exceed 25 MB. Large files are sent as a link")
		return
	}
	c, err := s.dialAs(in.acc.Address)
	if err != nil {
		serverErr(w, "draft dial", err)
		return
	}
	defer c.Logout()
	drafts, err := s.folders.byRole(c, in.acc.Address, "drafts")
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "The mailbox has no Drafts folder")
		return
	}
	if err := appendWithin(c, drafts, []string{imap.DraftFlag, imap.SeenFlag}, time.Now(), msg, 2*time.Minute); err != nil {
		serverErr(w, "append draft", err)
		return
	}
	// Find the UID of the message we just stored (no UIDPLUS in go-imap v1).
	if _, err := c.Select(drafts, false); err != nil {
		serverErr(w, "select drafts", err)
		return
	}
	crit := imap.NewSearchCriteria()
	crit.Header = textproto.MIMEHeader{"Message-Id": {msgID}}
	uids, err := c.UidSearch(crit)
	if err != nil || len(uids) == 0 {
		serverErr(w, "find draft", fmt.Errorf("uid not found: %v", err))
		return
	}
	uid := uids[len(uids)-1]
	var atts []Attachment
	set := new(imap.SeqSet)
	set.AddNum(uid)
	if m, err := fetchOne(c, set, []imap.FetchItem{imap.FetchUid, imap.FetchBodyStructure}); err == nil && m != nil {
		atts = attachmentsOf(m.BodyStructure)
	}
	// Remove the previous version (it may live in another mailbox if the sender was changed).
	if in.draftUID > 0 && !(in.draftAccount == in.acc.Address && in.draftUID == uid) {
		if in.draftAccount == in.acc.Address {
			removeDraftUID(c, in.draftUID)
		} else {
			s.deleteDraft(in.draftAccount, in.draftUID)
		}
	}
	s.hub.countsChanged(in.acc.Address)
	writeJSON(w, http.StatusOK, map[string]any{"account": in.acc.Address, "folder": drafts, "uid": uid, "attachments": atts, "savedAt": time.Now()})
}

func (s *Server) handleDeleteDraft(w http.ResponseWriter, r *http.Request, sess *Session) {
	var in struct {
		Account string `json:"account"`
		UID     uint32 `json:"uid"`
	}
	if err := readJSON(r, &in); err != nil || in.UID == 0 {
		writeErr(w, http.StatusBadRequest, "No draft specified")
		return
	}
	if _, ok := sess.account(in.Account); !ok {
		writeErr(w, http.StatusForbidden, "No access to the mailbox")
		return
	}
	s.trashDraft(strings.ToLower(in.Account), in.UID)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// trashDraft moves a discarded draft to Trash (kept 30 days there, like any deleted letter).
// Replaced or sent drafts are removed with deleteDraft instead.
func (s *Server) trashDraft(account string, uid uint32) {
	c, err := s.dialAs(account)
	if err != nil {
		return
	}
	defer c.Logout()
	drafts, err1 := s.folders.byRole(c, account, "drafts")
	trash, err2 := s.folders.byRole(c, account, "trash")
	if err1 != nil || err2 != nil {
		return
	}
	if _, err := c.Select(drafts, false); err != nil {
		return
	}
	set := new(imap.SeqSet)
	set.AddNum(uid)
	if err := c.UidMove(set, trash); err != nil {
		removeDraftUID(c, uid)
	}
	s.hub.countsChanged(account)
}

// deleteDraft removes a draft permanently from the Drafts folder of the mailbox.
func (s *Server) deleteDraft(account string, uid uint32) {
	c, err := s.dialAs(account)
	if err != nil {
		return
	}
	defer c.Logout()
	drafts, err := s.folders.byRole(c, account, "drafts")
	if err != nil {
		return
	}
	if _, err := c.Select(drafts, false); err != nil {
		return
	}
	removeDraftUID(c, uid)
	s.hub.countsChanged(account)
}

// removeDraftUID expects the Drafts folder to be selected read-write.
func removeDraftUID(c *client.Client, uid uint32) {
	set := new(imap.SeqSet)
	set.AddNum(uid)
	if err := c.UidStore(set, imap.FormatFlagsOp(imap.AddFlags, true), []interface{}{imap.DeletedFlag}, nil); err == nil {
		_ = uidExpunge(c, set)
	}
}

func markAnswered(c *client.Client, folder, uid string) {
	n, err := strconv.ParseUint(uid, 10, 32)
	if err != nil {
		return
	}
	if _, err := c.Select(folder, false); err != nil {
		return
	}
	set := new(imap.SeqSet)
	set.AddNum(uint32(n))
	_ = c.UidStore(set, imap.FormatFlagsOp(imap.AddFlags, true), []interface{}{imap.AnsweredFlag}, nil)
}

func (s *Server) submit(from string, rcpts []string, msg []byte) error {
	conn, err := tls.Dial("tcp", s.cfg.SMTPAddr, &tls.Config{ServerName: s.cfg.Hostname})
	if err != nil {
		return err
	}
	c, err := smtp.NewClient(conn, s.cfg.Hostname)
	if err != nil {
		return err
	}
	defer c.Close()
	if err := c.Auth(smtp.PlainAuth("", from+"*"+s.cfg.MasterUser, s.cfg.MasterPass, s.cfg.Hostname)); err != nil {
		return fmt.Errorf("auth: %w", err)
	}
	if err := c.Mail(from); err != nil {
		return err
	}
	for _, r := range rcpts {
		if err := c.Rcpt(r); err != nil {
			return fmt.Errorf("rcpt %s: %w", r, err)
		}
	}
	wc, err := c.Data()
	if err != nil {
		return err
	}
	if _, err := wc.Write(msg); err != nil {
		return err
	}
	if err := wc.Close(); err != nil {
		return err
	}
	return c.Quit()
}

func parseAddrs(s string) ([]*mail.Address, error) {
	s = strings.TrimSpace(strings.ReplaceAll(s, ";", ","))
	if s == "" {
		return nil, nil
	}
	list, err := mail.ParseAddressList(s)
	if err != nil {
		return nil, fmt.Errorf("Could not parse the addresses: %s", s)
	}
	return list, nil
}

func toGoMail(in []*mail.Address) []*gomail.Address {
	out := make([]*gomail.Address, len(in))
	for i, a := range in {
		out[i] = &gomail.Address{Name: a.Name, Address: a.Address}
	}
	return out
}

func emails(in []*mail.Address) []string {
	out := make([]string, len(in))
	for i, a := range in {
		out[i] = a.Address
	}
	return out
}

func newMessageID(from string) string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	domain := from[strings.LastIndex(from, "@")+1:]
	return fmt.Sprintf("%d.%s@%s", time.Now().UnixNano(), hex.EncodeToString(b), domain)
}

// plainToHTML turns the composed text into simple HTML; quoted lines become a blockquote.
func plainToHTML(t string) string {
	var b strings.Builder
	b.WriteString(`<div style="font-family:Arial,Helvetica,sans-serif;font-size:14px;line-height:1.5">`)
	inQuote := false
	for _, line := range strings.Split(t, "\n") {
		q := strings.HasPrefix(line, ">")
		if q && !inQuote {
			b.WriteString(`<blockquote style="margin:0 0 0 .8ex;border-left:2px solid #ccc;padding-left:1ex;color:#555">`)
			inQuote = true
		} else if !q && inQuote {
			b.WriteString(`</blockquote>`)
			inQuote = false
		}
		if q {
			line = strings.TrimPrefix(strings.TrimPrefix(line, ">"), " ")
		}
		b.WriteString(html.EscapeString(line))
		b.WriteString("<br>")
	}
	if inQuote {
		b.WriteString(`</blockquote>`)
	}
	b.WriteString(`</div>`)
	return b.String()
}

var reRcptErr = regexp.MustCompile(`rcpt ([^\s:]+): (\d{3})`)

// sendErrorText explains a refusal of the mail server in plain words, naming the address.
func sendErrorText(err error) string {
	e := err.Error()
	low := strings.ToLower(e)
	if m := reRcptErr.FindStringSubmatch(e); m != nil {
		addr := m[1]
		switch {
		case strings.Contains(low, "user unknown") || strings.Contains(low, "recipient address rejected"):
			return fmt.Sprintf("The address %s does not exist or the mailbox is disabled. The message was not sent. Check the address.", addr)
		case strings.Contains(low, "relay access denied"):
			return fmt.Sprintf("The server did not accept the address %s (relaying denied). The message was not sent.", addr)
		case strings.HasPrefix(m[2], "4"):
			return fmt.Sprintf("The address %s is temporarily not accepting mail. Try sending a little later.", addr)
		}
		return fmt.Sprintf("The server refused the address %s. The message was not sent. Check the address.", addr)
	}
	switch {
	case strings.Contains(e, "552") || strings.Contains(low, "size"):
		return "The message is too large to send. Send large files as a link."
	case strings.Contains(low, "virus") || strings.Contains(low, "executable"):
		return "The mail server rejected an attachment (a virus or an executable file). The message was not sent."
	case strings.Contains(low, "authentication") || strings.Contains(low, "535"):
		return "The sending server did not accept the mailbox username or password. Check the sending settings."
	}
	return "The sending server did not accept the message. Try again; if it happens again, tell your administrator."
}
