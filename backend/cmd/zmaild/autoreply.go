package main

import (
	"bufio"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

// Auto-reply ("I'm on vacation"): settings and text per mailbox. Loggi drafts the text from the
// reason and dates; Dovecot answers by a Sieve vacation script we install over ManageSieve, so it
// works while nobody has the mail open and answers each sender at most once a day.

type autoReply struct {
	Mailbox    string `json:"mailbox"`
	Enabled    bool   `json:"enabled"`
	Reason     string `json:"reason"`
	DateFrom   string `json:"dateFrom"` // YYYY-MM-DD or ""
	DateTo     string `json:"dateTo"`
	Substitute string `json:"substitute"`
	Notes      string `json:"notes"`
	Subject    string `json:"subject"`
	Body       string `json:"body"`
	UpdatedBy  string `json:"updatedBy,omitempty"`
}

var reasonNames = map[string]string{"vacation": "on vacation", "trip": "on a business trip", "sick": "on sick leave",
	"holiday": "public holidays / days off, the office is closed", "dayoff": "taking a day off", "other": "temporarily not answering email"}

func (s *Server) autoReplyAccount(w http.ResponseWriter, sess *Session, mailbox string) (Account, bool) {
	acc, ok := sess.account(mailbox)
	if !ok {
		writeErr(w, http.StatusForbidden, "No access to the mailbox")
		return acc, false
	}
	if !acc.CanSend {
		writeErr(w, http.StatusForbidden, "You have read-only access to this mailbox")
		return acc, false
	}
	return acc, true
}

func (s *Server) handleAutoReplyGet(w http.ResponseWriter, r *http.Request, sess *Session) {
	acc, ok := s.autoReplyAccount(w, sess, r.URL.Query().Get("mailbox"))
	if !ok {
		return
	}
	a := autoReply{Mailbox: acc.Address, Reason: "vacation"}
	var from, to *time.Time
	_ = s.db.QueryRow(r.Context(), `SELECT enabled, reason, date_from, date_to, substitute, notes, subject, body, updated_by FROM autoreplies WHERE mailbox=$1`, acc.Address).
		Scan(&a.Enabled, &a.Reason, &from, &to, &a.Substitute, &a.Notes, &a.Subject, &a.Body, &a.UpdatedBy)
	if from != nil {
		a.DateFrom = from.Format("2006-01-02")
	}
	if to != nil {
		a.DateTo = to.Format("2006-01-02")
	}
	// Past the end date the reply is off by itself (the script checks dates); show that.
	if a.Enabled && a.DateTo != "" && a.DateTo < time.Now().Format("2006-01-02") {
		a.Enabled = false
	}
	writeJSON(w, http.StatusOK, a)
}

func parseDay(s string) (*time.Time, error) {
	if s == "" {
		return nil, nil
	}
	t, err := time.Parse("2006-01-02", s)
	return &t, err
}

func (s *Server) handleAutoReplySave(w http.ResponseWriter, r *http.Request, sess *Session) {
	var in autoReply
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "Invalid data")
		return
	}
	acc, ok := s.autoReplyAccount(w, sess, in.Mailbox)
	if !ok {
		return
	}
	from, err1 := parseDay(in.DateFrom)
	to, err2 := parseDay(in.DateTo)
	if err1 != nil || err2 != nil || from != nil && to != nil && to.Before(*from) {
		writeErr(w, http.StatusBadRequest, "Check the dates")
		return
	}
	in.Subject, in.Body = strings.TrimSpace(in.Subject), strings.TrimSpace(in.Body)
	if in.Enabled && in.Body == "" {
		writeErr(w, http.StatusBadRequest, "Write the auto-reply text (or ask Loggi)")
		return
	}
	if in.Subject == "" {
		in.Subject = "Auto-reply"
	}
	if err := s.installVacation(acc.Address, in, from, to); err != nil {
		serverErr(w, "install vacation", err)
		return
	}
	if _, err := s.db.Exec(r.Context(), `INSERT INTO autoreplies(mailbox, enabled, reason, date_from, date_to, substitute, notes, subject, body, updated_by, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,now()) ON CONFLICT (mailbox) DO UPDATE SET enabled=EXCLUDED.enabled, reason=EXCLUDED.reason,
		date_from=EXCLUDED.date_from, date_to=EXCLUDED.date_to, substitute=EXCLUDED.substitute, notes=EXCLUDED.notes, subject=EXCLUDED.subject,
		body=EXCLUDED.body, updated_by=EXCLUDED.updated_by, updated_at=now()`,
		acc.Address, in.Enabled, in.Reason, from, to, in.Substitute, in.Notes, in.Subject, in.Body, sess.Login); err != nil {
		serverErr(w, "save autoreply", err)
		return
	}
	s.audit(r.Context(), sess.UserID, sess.Login, clientIP(r), "autoreply.save", acc.Address, map[string]any{"enabled": in.Enabled})
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// POST /api/autoreply/draft — Loggi writes the text from the reason, dates and substitute.
func (s *Server) handleAutoReplyDraft(w http.ResponseWriter, r *http.Request, sess *Session) {
	var in autoReply
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "Invalid data")
		return
	}
	acc, ok := s.autoReplyAccount(w, sess, in.Mailbox)
	if !ok {
		return
	}
	p, err := s.loadPrompt(r.Context(), "autoreply")
	if err != nil || !p.Enabled {
		writeErr(w, http.StatusNotFound, "The auto-reply prompt is disabled in the admin panel")
		return
	}
	reason := reasonNames[in.Reason]
	if reason == "" {
		reason = in.Reason
	}
	day := func(d string) string {
		t, err := time.Parse("2006-01-02", d)
		if err != nil {
			return ""
		}
		return t.Format("January 2, 2006")
	}
	if in.DateFrom != "" || in.DateTo != "" {
		reason += fmt.Sprintf(" (from %s to %s inclusive; I will answer messages after %s)", or(day(in.DateFrom), "today"), or(day(in.DateTo), "—"), or(day(nextDay(in.DateTo)), "my return"))
	}
	if strings.TrimSpace(in.Substitute) != "" {
		reason += ". For urgent matters, please contact: " + strings.TrimSpace(in.Substitute)
	}
	text, err := s.complete(r.Context(), p, fillTemplate(p.Template, aiInput{Instructions: reason, Mailbox: acc.Address, Draft: in.Notes}, sess.Name))
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	subject := "Auto-reply: " + strings.TrimSuffix(strings.SplitN(reasonNames[in.Reason], ",", 2)[0], ".")
	if in.DateTo != "" {
		subject += " until " + day(in.DateTo)
	}
	writeJSON(w, http.StatusOK, map[string]string{"body": text, "subject": subject})
}

func or(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

func nextDay(d string) string {
	t, err := time.Parse("2006-01-02", d)
	if err != nil {
		return ""
	}
	return t.AddDate(0, 0, 1).Format("2006-01-02")
}

// ---------- Sieve ----------

func sieveString(s string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s) + `"`
}

func sieveText(s string) string {
	var b strings.Builder
	b.WriteString("text:\r\n")
	for _, line := range strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n") {
		if strings.HasPrefix(line, ".") {
			line = "." + line
		}
		b.WriteString(line + "\r\n")
	}
	b.WriteString(".\r\n")
	return b.String()
}

func vacationScript(mailbox string, a autoReply, from, to *time.Time) string {
	var b strings.Builder
	b.WriteString("# TMAIL auto-reply — managed by the webmail, edits here are overwritten\r\n")
	b.WriteString(`require ["vacation", "date", "relational"];` + "\r\n")
	var conds []string
	if from != nil {
		conds = append(conds, `currentdate :zone "+0300" :value "ge" "date" `+sieveString(from.Format("2006-01-02")))
	}
	if to != nil {
		conds = append(conds, `currentdate :zone "+0300" :value "le" "date" `+sieveString(to.Format("2006-01-02")))
	}
	vac := fmt.Sprintf("vacation :days 1 :subject %s :from %s :addresses [%s] %s;", sieveString(a.Subject), sieveString(mailbox), sieveString(mailbox), sieveText(a.Body))
	if len(conds) > 0 {
		b.WriteString("if allof(" + strings.Join(conds, ", ") + ") {\r\n  " + vac + "\r\n}\r\n")
	} else {
		b.WriteString(vac + "\r\n")
	}
	return b.String()
}

const sieveName = "tmail-autoreply"

// installVacation writes and activates the script (or deactivates it) over ManageSieve.
func (s *Server) installVacation(mailbox string, a autoReply, from, to *time.Time) error {
	host, _, _ := net.SplitHostPort(s.cfg.IMAPAddr)
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(host, env("SIEVE_PORT", "4190")), 10*time.Second)
	if err != nil {
		return err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(30 * time.Second))
	rd := bufio.NewReader(conn)
	if _, err := sieveResp(rd); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(conn, "STARTTLS\r\n"); err != nil {
		return err
	}
	if _, err := sieveResp(rd); err != nil {
		return err
	}
	tc := tls.Client(conn, &tls.Config{ServerName: s.cfg.Hostname})
	_ = tc.SetDeadline(time.Now().Add(30 * time.Second))
	rd = bufio.NewReader(tc)
	if _, err := sieveResp(rd); err != nil { // capabilities after TLS
		return err
	}
	cmd := func(c string) error {
		if _, err := tc.Write([]byte(c)); err != nil {
			return err
		}
		_, err := sieveResp(rd)
		return err
	}
	auth := base64.StdEncoding.EncodeToString([]byte(mailbox + "\x00" + s.cfg.MasterUser + "\x00" + s.cfg.MasterPass))
	if err := cmd(`AUTHENTICATE "PLAIN" "` + auth + "\"\r\n"); err != nil {
		// Some setups accept the "user*master" login form instead of SASL authorization.
		auth = base64.StdEncoding.EncodeToString([]byte("\x00" + mailbox + "*" + s.cfg.MasterUser + "\x00" + s.cfg.MasterPass))
		if err := cmd(`AUTHENTICATE "PLAIN" "` + auth + "\"\r\n"); err != nil {
			return fmt.Errorf("managesieve login: %w", err)
		}
	}
	if !a.Enabled {
		_ = cmd("SETACTIVE \"\"\r\n")
		_ = cmd("DELETESCRIPT \"" + sieveName + "\"\r\n")
		_ = cmd("LOGOUT\r\n")
		return nil
	}
	script := vacationScript(mailbox, a, from, to)
	if err := cmd(fmt.Sprintf("PUTSCRIPT \"%s\" {%d+}\r\n%s\r\n", sieveName, len(script), script)); err != nil {
		return fmt.Errorf("putscript: %w", err)
	}
	if err := cmd("SETACTIVE \"" + sieveName + "\"\r\n"); err != nil {
		return fmt.Errorf("setactive: %w", err)
	}
	_ = cmd("LOGOUT\r\n")
	return nil
}

// sieveResp reads lines until OK / NO / BYE.
func sieveResp(rd *bufio.Reader) (string, error) {
	var all strings.Builder
	for {
		line, err := rd.ReadString('\n')
		if err != nil {
			return all.String(), err
		}
		all.WriteString(line)
		up := strings.ToUpper(line)
		switch {
		case strings.HasPrefix(up, "OK"):
			return all.String(), nil
		case strings.HasPrefix(up, "NO"), strings.HasPrefix(up, "BYE"):
			return all.String(), errors.New(strings.TrimSpace(all.String()))
		}
		// A literal {n} in an error: read its bytes so the next line is in sync.
		if i := strings.LastIndex(line, "{"); i >= 0 && strings.HasSuffix(strings.TrimSpace(line), "}") {
			var n int
			if _, err := fmt.Sscanf(strings.TrimSpace(line[i:]), "{%d}", &n); err == nil && n > 0 && n < 1<<20 {
				buf := make([]byte, n)
				if _, err := io.ReadFull(rd, buf); err != nil {
					return all.String(), err
				}
				all.Write(buf)
			}
		}
	}
}
