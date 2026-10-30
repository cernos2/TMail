package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/mail"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
)

// Antivirus. Letters arriving over SMTP are scanned by Rspamd (services/rspamd/local.d/antivirus.conf),
// which refuses malware and writes heuristic findings into an X-Virus header. Everything that does not
// pass through Postfix is scanned here against the same clamd: letters fetched by the collector,
// outgoing letters written in the webmail and files uploaded as links.

const clamChunk = 64 << 10

// clamStreamMax must stay below StreamMaxLength in clamd.conf. A longer file cannot be checked
// whole and is reported as such (errTooLarge), never as clean.
const clamStreamMax = 2100<<20 - clamChunk

var errTooLarge = errors.New("file is too large for the antivirus scan")

var errClamDown = errors.New("antivirus is unavailable")

func clamAddr() string { return env("CLAMD_ADDR", "127.0.0.1:3310") }

func clamDial(ctx context.Context) (net.Conn, error) {
	d := net.Dialer{Timeout: 5 * time.Second}
	c, err := d.DialContext(ctx, "tcp", clamAddr())
	if err != nil {
		return nil, errClamDown
	}
	if dl, ok := ctx.Deadline(); ok {
		_ = c.SetDeadline(dl)
	} else {
		_ = c.SetDeadline(time.Now().Add(3 * time.Minute))
	}
	return c, nil
}

// clamScan streams r to clamd (INSTREAM) and returns the name of what it found, "" when clean.
// Files longer than clamStreamMax are refused with errTooLarge.
func clamScan(ctx context.Context, r io.Reader) (string, error) {
	c, err := clamDial(ctx)
	if err != nil {
		return "", err
	}
	defer c.Close()
	if _, err := c.Write([]byte("zINSTREAM\x00")); err != nil {
		return "", errClamDown
	}
	buf := make([]byte, clamChunk)
	var sent int64
	var werr error
	for werr == nil {
		n, rerr := r.Read(buf)
		if sent+int64(n) > clamStreamMax {
			return "", errTooLarge
		}
		if n > 0 {
			var hdr [4]byte
			binary.BigEndian.PutUint32(hdr[:], uint32(n))
			if _, werr = c.Write(hdr[:]); werr == nil {
				_, werr = c.Write(buf[:n])
			}
			sent += int64(n)
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			return "", rerr
		}
	}
	if werr != nil {
		return "", fmt.Errorf("clamd: %w", werr)
	}
	if _, err := c.Write([]byte{0, 0, 0, 0}); err != nil {
		return "", errClamDown
	}
	reply, err := io.ReadAll(io.LimitReader(c, 4096))
	if err != nil && len(reply) == 0 {
		return "", errClamDown
	}
	return parseClamReply(string(reply))
}

func parseClamReply(reply string) (string, error) {
	reply = strings.TrimRight(reply, "\x00\r\n ")
	reply = strings.TrimPrefix(reply, "stream: ")
	switch {
	case reply == "OK":
		return "", nil
	case strings.HasSuffix(reply, " FOUND"):
		return strings.TrimSuffix(reply, " FOUND"), nil
	default:
		return "", fmt.Errorf("clamd: %s", reply)
	}
}

func clamScanBytes(ctx context.Context, data []byte) (string, error) {
	return clamScan(ctx, bytes.NewReader(data))
}

// clamVersion: "ClamAV 1.5.4/28161/Thu Oct 22 07:52:01 2026".
func clamVersion(ctx context.Context) (string, error) {
	c, err := clamDial(ctx)
	if err != nil {
		return "", err
	}
	defer c.Close()
	if _, err := c.Write([]byte("zVERSION\x00")); err != nil {
		return "", errClamDown
	}
	b, _ := io.ReadAll(io.LimitReader(c, 512))
	v := strings.TrimRight(string(b), "\x00\r\n ")
	if v == "" {
		return "", errClamDown
	}
	return v, nil
}

// avKind sorts a finding the way Rspamd does (antivirus.conf patterns): malware is refused,
// the rest is a warning shown with the letter.
func avKind(name string) string {
	switch {
	case name == "":
		return ""
	case strings.Contains(name, " "): // Rspamd's own wording, e.g. "File is encrypted"
		if strings.Contains(strings.ToLower(name), "encrypted") {
			return "encrypted"
		}
		return ""
	case strings.HasPrefix(name, "Heuristics.OLE2.ContainsMacros"):
		return "macro"
	case strings.HasPrefix(name, "Heuristics.Encrypted"):
		return "encrypted"
	case strings.HasPrefix(name, "Heuristics.Limits"):
		return "unchecked" // too big or too deeply packed to be scanned whole
	case strings.HasPrefix(name, "Heuristics."):
		return "heuristic"
	case reSaneMalware.MatchString(name):
		return "virus"
	case strings.HasPrefix(name, "Sanesecurity.") || strings.HasPrefix(name, "Porcupine."):
		return "spam" // phishing / scam / junk signatures: heuristic, a score rather than a block
	default:
		return "virus"
	}
}

var reSaneMalware = regexp.MustCompile(`(?i)^(Sanesecurity\.(Badmacro|Foxhole|Malware|MalwareHash|Rogue)|Porcupine\.Malware)`)

// ---------- warnings shown with a letter ----------

// avFlags turns X-Virus values (written by Rspamd or by the collector) into warning kinds.
func avFlags(values []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, v := range values {
		for _, name := range strings.Split(v, ",") {
			// Rspamd writes some findings as "File is encrypted: Heuristics.Encrypted.Zip".
			if i := strings.LastIndex(name, ": "); i >= 0 {
				name = name[i+2:]
			}
			if k := avKind(strings.TrimSpace(name)); k != "" && !seen[k] {
				seen[k] = true
				out = append(out, k)
			}
		}
	}
	return out
}

var riskyExt = map[string]string{}

func init() {
	for _, e := range strings.Fields(".exe .scr .com .pif .bat .cmd .vbs .vbe .js .jse .wsf .wsh .wsc .hta .ps1 .psm1 .lnk .msi .msp .msc .cpl .jar .reg .scf .inf .chm .application .gadget .appx .msix .iso .img .vhd .vhdx .xll .sct .dll .sys") {
		riskyExt[e] = "exec"
	}
	for _, e := range strings.Fields(".docm .dotm .xlsm .xltm .xlam .pptm .potm .ppam .ppsm .sldm") {
		riskyExt[e] = "macro"
	}
	for _, e := range strings.Fields(".html .htm .shtml .svg .url") {
		riskyExt[e] = "html"
	}
}

// attachmentRisk judges a file by its name: executables, Office files with macros, web pages
// (fake login forms) and double extensions like "invoice.pdf.exe".
func attachmentRisk(name string) string {
	ext := strings.ToLower(filepath.Ext(strings.TrimSpace(name)))
	return riskyExt[ext]
}

// setVirusHeader replaces any X-Virus header of a raw letter with ours (or just removes it).
func setVirusHeader(raw []byte, value string) []byte {
	end := bytes.Index(raw, []byte("\r\n\r\n"))
	nl := "\r\n"
	if end < 0 {
		end = bytes.Index(raw, []byte("\n\n"))
		nl = "\n"
	}
	if end < 0 {
		return raw
	}
	head, body := raw[:end+len(nl)], raw[end+len(nl):]
	var out bytes.Buffer
	if value != "" {
		out.WriteString("X-Virus: " + value + nl)
	}
	skip := false
	for _, line := range bytes.SplitAfter(head, []byte("\n")) {
		if len(line) == 0 {
			continue
		}
		if line[0] == ' ' || line[0] == '\t' {
			if !skip {
				out.Write(line)
			}
			continue
		}
		skip = len(line) >= 8 && strings.EqualFold(string(line[:8]), "x-virus:")
		if !skip {
			out.Write(line)
		}
	}
	out.Write(body)
	return out.Bytes()
}

// ---------- collector: scan before delivery ----------

// scanImported checks a letter fetched from an external mailbox. Malware goes to quarantine
// (deliver=false); findings that are only suspicious are written into X-Virus for the reader.
// reHasFiles: the letter carries a file (attachment, picture, document) — only those are scanned
// while importing; plain text letters are many and cannot carry malware. Every file is checked
// again before anyone downloads or previews it (avGate), so nothing unchecked is handed out.
var reHasFiles = regexp.MustCompile(`(?i)(filename\*?=|content-disposition:\s*attachment|content-type:\s*(application|image|audio|video|message)/)`)

func (s *Server) scanImported(ctx context.Context, mailbox, source string, raw []byte) (out []byte, deliver bool) {
	if !reHasFiles.Match(raw) {
		return setVirusHeader(raw, ""), true
	}
	sctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	found, err := clamScanBytes(sctx, raw)
	if err != nil {
		// Fail open: an antivirus outage must not stop the mail. The letter is marked so the
		// reader can see it was not checked.
		log.Printf("antivirus: %s: %v", mailbox, err)
		return setVirusHeader(raw, ""), true
	}
	if avKind(found) == "virus" {
		if s.quarantine(ctx, mailbox, source, found, raw) == nil {
			return nil, false
		}
	}
	return setVirusHeader(raw, found), true
}

func (s *Server) quarantine(ctx context.Context, mailbox, source, virus string, raw []byte) error {
	c, err := s.s3()
	if err != nil {
		log.Printf("quarantine %s: no storage: %v", mailbox, err)
		return err
	}
	from, subject := "", ""
	if m, err := mail.ReadMessage(bytes.NewReader(raw)); err == nil {
		from, subject = decodeWords(m.Header.Get("From")), decodeWords(m.Header.Get("Subject"))
	}
	key := "quarantine/" + time.Now().Format("2006/01/") + newToken() + ".eml"
	if _, err := c.PutObject(ctx, s3Bucket(), key, bytes.NewReader(raw), int64(len(raw)), minio.PutObjectOptions{ContentType: "message/rfc822"}); err != nil {
		log.Printf("quarantine %s: %v", mailbox, err)
		return err
	}
	_, err = s.db.Exec(ctx, `INSERT INTO av_quarantine(mailbox, source, sender, subject, virus, size, s3_key) VALUES ($1,$2,$3,$4,$5,$6,$7)`,
		mailbox, source, truncate(from, 300), truncate(subject, 300), virus, len(raw), key)
	if err == nil {
		log.Printf("quarantine: %s ← %s: %s", mailbox, from, virus)
		s.audit(ctx, 0, "antivirus", "", "av.quarantine", mailbox, map[string]any{"virus": virus, "from": from, "subject": subject})
	}
	return err
}

// ---------- admin: status, quarantine ----------

type quarantineItem struct {
	ID       int64      `json:"id"`
	Mailbox  string     `json:"mailbox"`
	Source   string     `json:"source"`
	Sender   string     `json:"sender"`
	Subject  string     `json:"subject"`
	Virus    string     `json:"virus"`
	Size     int64      `json:"size"`
	Created  time.Time  `json:"created"`
	Released *time.Time `json:"released"`
	By       string     `json:"releasedBy"`
}

type avRejected struct {
	Time    time.Time `json:"time"`
	Sender  string    `json:"sender"`
	Rcpt    []string  `json:"rcpt"`
	Subject string    `json:"subject"`
	Reason  string    `json:"reason"`
	Virus   string    `json:"virus"`
}

func (s *Server) adminAntivirus(w http.ResponseWriter, r *http.Request, sess *Session) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	out := map[string]any{"ok": false, "quarantine": []quarantineItem{}, "rejected": []avRejected{}}
	if v, err := clamVersion(ctx); err == nil {
		out["ok"] = true
		out["version"] = v
		if parts := strings.SplitN(v, "/", 3); len(parts) == 3 {
			out["engine"], out["signatures"], out["updated"] = parts[0], parts[1], parts[2]
			// The mirror we update from can lag; more than two days without new signatures is worth a look.
			if t, err := time.Parse("Mon Jan _2 15:04:05 2006", strings.TrimSpace(parts[2])); err == nil {
				out["ageHours"] = int(time.Since(t).Hours())
				out["stale"] = time.Since(t) > 48*time.Hour
			}
		}
	} else {
		out["error"] = err.Error()
	}
	items := []quarantineItem{}
	if rows, err := s.db.Query(ctx, `SELECT id, mailbox, source, sender, subject, virus, size, created_at, released_at, COALESCE(released_by,'')
		FROM av_quarantine ORDER BY created_at DESC LIMIT 200`); err == nil {
		for rows.Next() {
			var q quarantineItem
			if rows.Scan(&q.ID, &q.Mailbox, &q.Source, &q.Sender, &q.Subject, &q.Virus, &q.Size, &q.Created, &q.Released, &q.By) == nil {
				items = append(items, q)
			}
		}
		rows.Close()
	}
	out["quarantine"] = items
	out["rejected"] = rspamdRejected(ctx)
	writeJSON(w, http.StatusOK, out)
}

// rspamdRejected: letters refused at SMTP time for a virus or an executable attachment,
// from Rspamd's recent history (controller on loopback).
func rspamdRejected(ctx context.Context) []avRejected {
	out := []avRejected{}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "http://127.0.0.1:11334/history", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return out
	}
	defer resp.Body.Close()
	var h struct {
		Rows []struct {
			Unix    float64  `json:"unix_time"`
			Sender  string   `json:"sender_mime"`
			Rcpt    []string `json:"rcpt_mime"`
			Subject string   `json:"subject"`
			Action  string   `json:"action"`
			Symbols map[string]struct {
				Options []string `json:"options"`
			} `json:"symbols"`
		} `json:"rows"`
	}
	if json.NewDecoder(io.LimitReader(resp.Body, 16<<20)).Decode(&h) != nil {
		return out
	}
	for _, row := range h.Rows {
		if row.Action != "reject" {
			continue
		}
		x := avRejected{Time: time.Unix(int64(row.Unix), 0), Sender: row.Sender, Rcpt: row.Rcpt, Subject: row.Subject}
		for _, sym := range []string{"CLAM_VIRUS", "CLAM_SANE_MALWARE"} {
			if v, ok := row.Symbols[sym]; ok {
				x.Reason, x.Virus = "virus", strings.Join(v.Options, ", ")
			}
		}
		if x.Reason == "" {
			for _, sym := range []string{"MIME_BAD_EXTENSION", "MIME_DOUBLE_BAD_EXTENSION"} {
				if v, ok := row.Symbols[sym]; ok {
					x.Reason, x.Virus = "exec", strings.Join(v.Options, ", ")
				}
			}
		}
		if x.Reason != "" {
			out = append(out, x)
		}
	}
	return out
}

// POST /api/admin/antivirus/test — the EICAR test string must be detected.
func (s *Server) adminAntivirusTest(w http.ResponseWriter, r *http.Request, sess *Session) {
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	eicar := []byte(`X5O!P%@AP[4\PZX54(P^)7CC)7}$EICAR` + `-STANDARD-ANTIVIRUS-TEST-FILE!$H+H*`)
	start := time.Now()
	found, err := clamScanBytes(ctx, eicar)
	if err != nil {
		writeErr(w, http.StatusServiceUnavailable, fmt.Sprintf("The antivirus is not responding: %v", err))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"detected": found != "", "virus": found, "ms": time.Since(start).Milliseconds()})
}

// POST /api/admin/antivirus/quarantine/{id}?action=release|delete
func (s *Server) adminQuarantineAction(w http.ResponseWriter, r *http.Request, sess *Session) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	var mailbox, key, virus string
	if err := s.db.QueryRow(r.Context(), `SELECT mailbox, s3_key, virus FROM av_quarantine WHERE id=$1`, id).Scan(&mailbox, &key, &virus); err != nil {
		writeErr(w, http.StatusNotFound, "Message not found in quarantine")
		return
	}
	c, err := s.s3()
	if err != nil {
		writeErr(w, http.StatusServiceUnavailable, "Storage is unavailable")
		return
	}
	switch r.URL.Query().Get("action") {
	case "delete":
		_ = c.RemoveObject(r.Context(), s3Bucket(), key, minio.RemoveObjectOptions{})
		_, _ = s.db.Exec(r.Context(), `DELETE FROM av_quarantine WHERE id=$1`, id)
		s.audit(r.Context(), sess.UserID, sess.Login, clientIP(r), "av.delete", mailbox, map[string]any{"virus": virus})
	case "release":
		obj, err := c.GetObject(r.Context(), s3Bucket(), key, minio.GetObjectOptions{})
		if err != nil {
			writeErr(w, http.StatusBadGateway, "Could not read the message from quarantine")
			return
		}
		raw, err := io.ReadAll(io.LimitReader(obj, 200<<20))
		obj.Close()
		if err != nil {
			writeErr(w, http.StatusBadGateway, "Could not read the message from quarantine")
			return
		}
		local, err := s.dialAs(mailbox)
		if err != nil {
			serverErr(w, "quarantine release", err)
			return
		}
		defer local.Logout()
		// Released by an administrator, but the reader still sees what the antivirus found.
		if err := appendWithin(local, "INBOX", nil, time.Now(), setVirusHeader(raw, virus), 5*time.Minute); err != nil {
			serverErr(w, "quarantine release", err)
			return
		}
		_, _ = s.db.Exec(r.Context(), `UPDATE av_quarantine SET released_at=now(), released_by=$2 WHERE id=$1`, id, sess.Login)
		s.audit(r.Context(), sess.UserID, sess.Login, clientIP(r), "av.release", mailbox, map[string]any{"virus": virus})
		s.hub.countsChanged(mailbox)
	default:
		writeErr(w, http.StatusBadRequest, "Unknown action")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// runQuarantineCleanup removes quarantined letters after 30 days.
func (s *Server) runQuarantineCleanup(ctx context.Context) {
	t := time.NewTicker(6 * time.Hour)
	defer t.Stop()
	for {
		if c, err := s.s3(); err == nil {
			if rows, err := s.db.Query(ctx, `DELETE FROM av_quarantine WHERE created_at < now() - interval '30 days' RETURNING s3_key`); err == nil {
				var keys []string
				for rows.Next() {
					var k string
					if rows.Scan(&k) == nil {
						keys = append(keys, k)
					}
				}
				rows.Close()
				for _, k := range keys {
					_ = c.RemoveObject(ctx, s3Bucket(), k, minio.RemoveObjectOptions{})
				}
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// ---------- downloads: only files the antivirus has checked ----------

// fileVerdict scans a file unless the same bytes were checked recently (clean verdicts are
// re-checked after a day, when the signatures have changed; malware stays malware).
func (s *Server) fileVerdict(ctx context.Context, data []byte) (string, error) {
	sum := sha256.Sum256(data)
	hash := hex.EncodeToString(sum[:])
	var found string
	if s.db.QueryRow(ctx, `SELECT found FROM av_scans WHERE hash=$1 AND (found <> '' OR scanned_at > now() - interval '1 day')`, hash).Scan(&found) == nil {
		return found, nil
	}
	sctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	found, err := clamScanBytes(sctx, data)
	if errors.Is(err, errTooLarge) {
		found, err = "Heuristics.Limits.Exceeded.Size", nil
	}
	if err != nil {
		return "", err
	}
	_, _ = s.db.Exec(ctx, `INSERT INTO av_scans(hash, found) VALUES ($1,$2) ON CONFLICT (hash) DO UPDATE SET found=EXCLUDED.found, scanned_at=now()`, hash, found)
	return found, nil
}

// avGate hands out a file only with a verdict from the antivirus: while it is busy or down the
// download waits, malware is never given out.
func (s *Server) avGate(w http.ResponseWriter, r *http.Request, sess *Session, name string, data []byte) bool {
	found, err := s.fileVerdict(r.Context(), data)
	if err != nil {
		log.Printf("antivirus gate %q: %v", name, err)
		w.Header().Set("Retry-After", "30")
		writeErr(w, http.StatusServiceUnavailable, fmt.Sprintf("The antivirus has not checked %q yet. The file can be downloaded after the check. Try again in a minute.", name))
		return false
	}
	switch avKind(found) {
	case "virus":
		s.audit(r.Context(), sess.UserID, sess.Login, clientIP(r), "av.download_blocked", name, map[string]any{"virus": found})
		writeErr(w, http.StatusForbidden, fmt.Sprintf("A virus was found in %q (%s). Download is blocked.", name, found))
		return false
	case "encrypted", "unchecked":
		writeErr(w, http.StatusForbidden, lockedText(name, avKind(found)))
		return false
	case "heuristic":
		s.audit(r.Context(), sess.UserID, sess.Login, clientIP(r), "av.download_blocked", name, map[string]any{"virus": found})
		writeErr(w, http.StatusForbidden, fmt.Sprintf("The antivirus considers %q suspicious (%s). Download is blocked.", name, found))
		return false
	}
	return true
}

// Files the antivirus cannot look into are treated as not checked: no download, no preview.
func lockedText(name, kind string) string {
	if kind == "heuristic" {
		return fmt.Sprintf("The antivirus considers %q suspicious, so download is blocked.", name)
	}
	if kind == "encrypted" {
		return fmt.Sprintf("%q is password-protected: the antivirus cannot check its contents, so download is blocked. Ask the sender to send the file without a password.", name)
	}
	return fmt.Sprintf("%q is too large or packed too many times: the antivirus cannot check it completely, so download is blocked.", name)
}

// serveScan answers ?as=scan: the verdict for one attachment, so the webmail can say
// "checked by antivirus" next to it and open the download only when it is clean.
func (s *Server) serveScan(w http.ResponseWriter, r *http.Request, data []byte) {
	found, err := s.fileVerdict(r.Context(), data)
	status := "clean"
	switch k := avKind(found); {
	case err != nil:
		status = "pending"
	case k == "virus":
		status = "virus"
	case k == "encrypted" || k == "unchecked" || k == "heuristic":
		status = "locked"
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": status, "virus": found, "kind": avKind(found)})
}
