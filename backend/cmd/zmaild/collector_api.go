package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Admin API of the mail collector and the list of send-as addresses for the composer.

func (s *Server) adminExtList(w http.ResponseWriter, r *http.Request, sess *Session) {
	rows, err := s.db.Query(r.Context(), `SELECT `+extCols+` FROM external_accounts ORDER BY mailbox, address`)
	if err != nil {
		serverErr(w, "external list", err)
		return
	}
	defer rows.Close()
	out := []extAccount{}
	left := map[int64][2]int64{}
	if lr, err := s.db.Query(r.Context(), `SELECT account_id, sum(total), sum(remaining_bytes) FROM external_folders GROUP BY account_id`); err == nil {
		for lr.Next() {
			var id, n, b int64
			if lr.Scan(&id, &n, &b) == nil {
				left[id] = [2]int64{n, b}
			}
		}
		lr.Close()
	}
	extProgressMu.Lock()
	defer extProgressMu.Unlock()
	for rows.Next() {
		if a, err := scanExt(rows); err == nil {
			a.Progress = extProgress[a.ID]
			a.Remaining, a.RemainBytes = left[a.ID][0], left[a.ID][1]
			if extRunning[a.ID] && a.Progress == "" {
				a.Progress = "connecting…"
			}
			out = append(out, a)
		}
	}
	writeJSON(w, http.StatusOK, out)
}

func readExt(w http.ResponseWriter, r *http.Request) (extAccount, bool) {
	var a extAccount
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&a); err != nil {
		writeErr(w, http.StatusBadRequest, "Invalid data")
		return a, false
	}
	a.Address = strings.ToLower(strings.TrimSpace(a.Address))
	a.Mailbox = a.Address // the external mailbox becomes its own mailbox in TMAIL
	a.Host, a.SMTPHost = strings.TrimSpace(a.Host), strings.TrimSpace(a.SMTPHost)
	a.Username, a.SMTPUsername = strings.TrimSpace(a.Username), strings.TrimSpace(a.SMTPUsername)
	if a.Protocol != "pop3" {
		a.Protocol = "imap"
	}
	for _, sec := range []*string{&a.Security, &a.SMTPSecurity} {
		if *sec != "ssl" && *sec != "starttls" && *sec != "none" {
			*sec = "ssl"
		}
	}
	switch {
	case !emailRe.MatchString(a.Address):
		writeErr(w, http.StatusBadRequest, "Specify the external mailbox address")
	case a.Host == "" || a.Port <= 0 || a.Port > 65535 || a.Username == "":
		writeErr(w, http.StatusBadRequest, "Specify the server, port and username")
	case a.SMTPHost != "" && (a.SMTPPort <= 0 || a.SMTPPort > 65535):
		writeErr(w, http.StatusBadRequest, "Check the SMTP port")
	case a.Interval != 0 && (a.Interval < 1 || a.Interval > 1440):
		writeErr(w, http.StatusBadRequest, "Check interval: from 1 minute to 1 day")
	case a.Since != "" && func() bool { _, err := time.Parse("2006-01-02", a.Since); return err != nil }():
		writeErr(w, http.StatusBadRequest, "Invalid date")
	default:
		return a, true
	}
	return a, false
}

func (s *Server) adminExtSave(w http.ResponseWriter, r *http.Request, sess *Session) {
	a, ok := readExt(w, r)
	if !ok {
		return
	}
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if id == 0 {
		if err := s.ensureExternalMailbox(r.Context(), a.Address, a.DisplayName, sess.UserID); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	if a.Interval == 0 {
		a.Interval = 2
	}
	if a.KeepDays < 0 || a.KeepDays > 3650 {
		a.KeepDays = 90
	}
	var since any
	if a.Since != "" {
		since = a.Since
	}
	ctx := r.Context()
	var err error
	if id == 0 {
		if a.Password == "" {
			writeErr(w, http.StatusBadRequest, "Enter the external mailbox password")
			return
		}
		pe, _ := s.seal(a.Password)
		var spe any
		if a.SMTPPassword != "" {
			spe, _ = s.seal(a.SMTPPassword)
		} else if a.SMTPHost != "" && (a.SMTPUsername == "" || a.SMTPUsername == a.Username) {
			spe = pe // same login for SMTP
		}
		err = s.db.QueryRow(ctx, `INSERT INTO external_accounts(mailbox, address, display_name, protocol, host, port, security, username, password_enc,
			smtp_host, smtp_port, smtp_security, smtp_username, smtp_password_enc, inbox_only, since, delete_after, enabled, created_by, interval_minutes, keep_days, two_way)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22) RETURNING id`,
			a.Mailbox, a.Address, a.DisplayName, a.Protocol, a.Host, a.Port, a.Security, a.Username, pe,
			a.SMTPHost, a.SMTPPort, a.SMTPSecurity, a.SMTPUsername, spe, a.InboxOnly, since, a.DeleteAfter, a.Enabled, sess.Login, a.Interval, a.KeepDays, a.TwoWay).Scan(&id)
	} else {
		_, err = s.db.Exec(ctx, `UPDATE external_accounts SET mailbox=$2, address=$3, display_name=$4, protocol=$5, host=$6, port=$7, security=$8, username=$9,
			smtp_host=$10, smtp_port=$11, smtp_security=$12, smtp_username=$13, inbox_only=$14, since=$15, delete_after=$16, enabled=$17, interval_minutes=$18, keep_days=$19, two_way=$20 WHERE id=$1`,
			id, a.Mailbox, a.Address, a.DisplayName, a.Protocol, a.Host, a.Port, a.Security, a.Username,
			a.SMTPHost, a.SMTPPort, a.SMTPSecurity, a.SMTPUsername, a.InboxOnly, since, a.DeleteAfter, a.Enabled, a.Interval, a.KeepDays, a.TwoWay)
		if err == nil && a.Password != "" {
			pe, _ := s.seal(a.Password)
			_, err = s.db.Exec(ctx, `UPDATE external_accounts SET password_enc=$2, status=CASE WHEN status='error' THEN 'ok' ELSE status END WHERE id=$1`, id, pe)
		}
		if err == nil && a.SMTPPassword != "" {
			spe, _ := s.seal(a.SMTPPassword)
			_, err = s.db.Exec(ctx, `UPDATE external_accounts SET smtp_password_enc=$2 WHERE id=$1`, id, spe)
		}
	}
	if err != nil {
		if strings.Contains(err.Error(), "duplicate") {
			writeErr(w, http.StatusConflict, "This mailbox is already connected")
			return
		}
		serverErr(w, "external save", err)
		return
	}
	s.audit(ctx, sess.UserID, sess.Login, clientIP(r), "collector.save", a.Address+" → "+a.Mailbox, nil)
	writeJSON(w, http.StatusOK, map[string]int64{"id": id})
}

func (s *Server) adminExtDelete(w http.ResponseWriter, r *http.Request, sess *Session) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	var addr string
	_ = s.db.QueryRow(r.Context(), `DELETE FROM external_accounts WHERE id=$1 RETURNING address`, id).Scan(&addr)
	s.audit(r.Context(), sess.UserID, sess.Login, clientIP(r), "collector.delete", addr, nil)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// POST /api/admin/collector/test — check the entered settings without saving.
func (s *Server) adminExtTest(w http.ResponseWriter, r *http.Request, sess *Session) {
	a, ok := readExt(w, r)
	if !ok {
		return
	}
	id, _ := strconv.ParseInt(r.URL.Query().Get("id"), 10, 64)
	if a.Password == "" && id > 0 {
		if old, err := scanExt(s.db.QueryRow(r.Context(), `SELECT `+extCols+` FROM external_accounts WHERE id=$1`, id)); err == nil {
			a.Password, _ = s.unseal(old.passEnc)
			if a.SMTPPassword == "" && len(old.smtpPassEnc) > 0 {
				a.SMTPPassword, _ = s.unseal(old.smtpPassEnc)
			}
		}
	}
	res := map[string]string{}
	if a.Protocol == "pop3" {
		if p, err := remotePOP3(a, a.Password); err != nil {
			res["mail"] = "Error: " + err.Error() + authHint(a.Host, err)
		} else {
			st, _ := p.cmd("STAT")
			res["mail"] = "Connected. " + strings.TrimPrefix(st, "+OK ")
			p.close()
		}
	} else if c, err := remoteIMAP(a, a.Password); err != nil {
		res["mail"] = "Error: " + err.Error() + authHint(a.Host, err)
	} else {
		mb, err := c.Select("INBOX", true)
		if err == nil {
			res["mail"] = fmt.Sprintf("Connected, %d messages in Inbox", mb.Messages)
		} else {
			res["mail"] = "Signed in, but could not open Inbox: " + err.Error()
		}
		c.Logout()
	}
	if a.SMTPHost != "" {
		pass := a.SMTPPassword
		if pass == "" {
			pass = a.Password
		}
		a.smtpPassEnc, _ = s.seal(pass)
		if err := s.smtpLoginCheck(a); err != nil {
			res["smtp"] = "Error: " + err.Error() + authHint(a.SMTPHost, err)
		} else {
			res["smtp"] = "Sending: signed in"
		}
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) smtpLoginCheck(a extAccount) error {
	pass, err := s.unseal(a.smtpPassEnc)
	if err != nil {
		return err
	}
	conn, err := dialRemote(a.SMTPHost, a.SMTPPort, a.SMTPSecurity)
	if err != nil {
		return err
	}
	c, err := smtpNewClient(conn, a.SMTPHost)
	if err != nil {
		return err
	}
	defer c.Close()
	if err := c.Hello(s.cfg.Hostname); err != nil {
		return err
	}
	if a.SMTPSecurity == "starttls" {
		if err := c.StartTLS(tlsFor(a.SMTPHost)); err != nil {
			return err
		}
	}
	user := a.SMTPUsername
	if user == "" {
		user = a.Username
	}
	return c.Auth(smtpPlain(user, pass, a.SMTPHost))
}

// POST /api/admin/collector/{id}/sync — collect now.
func (s *Server) adminExtSync(w http.ResponseWriter, r *http.Request, sess *Session) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	a, err := scanExt(s.db.QueryRow(r.Context(), `SELECT `+extCols+` FROM external_accounts WHERE id=$1`, id))
	if err != nil {
		writeErr(w, http.StatusNotFound, "Not found")
		return
	}
	go s.syncExternal(context.Background(), a)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// GET /api/identities — external addresses the user can send from.
func (s *Server) handleIdentities(w http.ResponseWriter, r *http.Request, sess *Session) {
	rows, err := s.db.Query(r.Context(), `SELECT address, display_name, mailbox FROM external_accounts
		WHERE smtp_host <> '' AND smtp_password_enc IS NOT NULL AND enabled ORDER BY address`)
	if err != nil {
		serverErr(w, "identities", err)
		return
	}
	defer rows.Close()
	out := []map[string]string{}
	for rows.Next() {
		var addr, name, mb string
		if rows.Scan(&addr, &name, &mb) == nil {
			if acc, ok := sess.account(mb); ok && acc.CanSend {
				out = append(out, map[string]string{"address": addr, "name": name, "mailbox": mb})
			}
		}
	}
	writeJSON(w, http.StatusOK, out)
}
