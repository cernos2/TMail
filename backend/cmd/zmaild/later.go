package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap/client"
)

// "Send later" and "Snooze". A scheduled letter waits, complete, in the folder Scheduled and the
// server sends it at its time (the browser may be closed); deleting it from the folder cancels
// it. A snoozed letter waits in the folder Snoozed and comes back to the top of its folder, unread,
// at the chosen time.

const (
	scheduledFolder = "Scheduled"
	snoozedFolder   = "Snoozed"
)

// ensureFolder creates a folder if it does not exist yet.
func ensureFolder(c *client.Client, name string) error {
	if folderExists(c, name) == nil {
		return nil
	}
	if err := c.Create(name); err != nil && folderExists(c, name) != nil {
		return err
	}
	_ = c.Subscribe(name)
	return nil
}

// uidByMessageID finds a letter in the selected folder by its Message-ID.
func uidByMessageID(c *client.Client, msgID string) (uint32, error) {
	crit := imap.NewSearchCriteria()
	crit.Header.Add("Message-Id", msgID)
	uids, err := c.UidSearch(crit)
	if err != nil {
		return 0, err
	}
	if len(uids) == 0 {
		return 0, errNotFound
	}
	return uids[len(uids)-1], nil
}

var errNotFound = errors.New("not found")

func fetchRawFlags(c *client.Client, uid uint32) ([]byte, []string, error) {
	set := new(imap.SeqSet)
	set.AddNum(uid)
	section := &imap.BodySectionName{Peek: true}
	ch := make(chan *imap.Message, 1)
	done := make(chan error, 1)
	go func() { done <- c.UidFetch(set, []imap.FetchItem{section.FetchItem(), imap.FetchFlags}, ch) }()
	var raw []byte
	var flags []string
	for m := range ch {
		if b := m.GetBody(section); b != nil {
			var buf bytes.Buffer
			_, _ = buf.ReadFrom(b)
			raw = buf.Bytes()
		}
		flags = m.Flags
	}
	if err := <-done; err != nil {
		return nil, nil, err
	}
	if raw == nil {
		return nil, nil, errNotFound
	}
	return raw, flags, nil
}

// setHeader replaces (or adds) one header field in a raw message.
func setHeader(raw []byte, name, value string) []byte {
	end := bytes.Index(raw, []byte("\r\n\r\n"))
	if end < 0 {
		return raw
	}
	head := string(raw[:end+2])
	lines := strings.SplitAfter(head, "\r\n")
	var out strings.Builder
	skipping, done := false, false
	for _, l := range lines {
		if l == "" {
			continue
		}
		if skipping && (strings.HasPrefix(l, " ") || strings.HasPrefix(l, "\t")) {
			continue // folded continuation of the replaced field
		}
		skipping = false
		if k, _, ok := strings.Cut(l, ":"); ok && strings.EqualFold(strings.TrimSpace(k), name) {
			if !done {
				out.WriteString(name + ": " + value + "\r\n")
				done = true
			}
			skipping = true
			continue
		}
		out.WriteString(l)
	}
	if !done {
		out.WriteString(name + ": " + value + "\r\n")
	}
	return append([]byte(out.String()+"\r\n"), raw[end+4:]...)
}

// ---------- send later ----------

// scheduleSend keeps a composed letter for sending at sendAt (called by handleSend).
func (s *Server) scheduleSend(w http.ResponseWriter, r *http.Request, sess *Session, in *composeInput, msg []byte, msgID, sendAtRaw string) {
	at, err := time.Parse(time.RFC3339, sendAtRaw)
	if err != nil || at.Before(time.Now().Add(30*time.Second)) || at.After(time.Now().AddDate(1, 0, 0)) {
		writeErr(w, http.StatusBadRequest, "Choose a time in the future (within a year)")
		return
	}
	acc := in.acc
	c, err := s.dialAs(acc.Address)
	if err != nil {
		serverErr(w, "schedule dial", err)
		return
	}
	defer c.Logout()
	if err := ensureFolder(c, scheduledFolder); err != nil {
		serverErr(w, "schedule folder", err)
		return
	}
	if err := appendWithin(c, scheduledFolder, []string{imap.SeenFlag}, time.Now(), msg, 2*time.Minute); err != nil {
		serverErr(w, "schedule append", err)
		return
	}
	var uid uint32
	if _, err := c.Select(scheduledFolder, true); err == nil {
		uid, _ = uidByMessageID(c, msgID)
	}
	var identID *int64
	if in.ident != nil {
		identID = &in.ident.ID
	}
	rcpts := append(append(emails(in.to), emails(in.cc)...), emails(in.bcc)...)
	if _, err := s.db.Exec(r.Context(), `INSERT INTO scheduled_messages(user_id, account, ident_id, message_id, subject, recipients, reply_folder, reply_uid, send_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`, sess.UserID, acc.Address, identID, msgID, truncate(in.subject, 300), rcpts, in.replyFolder, in.replyUID, at); err != nil {
		serverErr(w, "schedule save", err)
		return
	}
	if in.draftUID > 0 {
		s.deleteDraft(in.draftAccount, in.draftUID)
	}
	slog.Info("message scheduled", "from", acc.Address, "login", sess.Login, "at", at)
	s.hub.countsChanged(acc.Address)
	writeJSON(w, http.StatusOK, map[string]any{"ok": "1", "scheduled": at, "messageId": msgID, "account": acc.Address, "folder": scheduledFolder, "uid": uid})
}

type scheduledInfo struct {
	ID        int64     `json:"id"`
	Account   string    `json:"account"`
	MessageID string    `json:"messageId"`
	Subject   string    `json:"subject"`
	SendAt    time.Time `json:"sendAt"`
	Status    string    `json:"status"`
	Error     string    `json:"error,omitempty"`
}

// GET /api/scheduled — the user's letters waiting to be sent (and failed ones).
func (s *Server) handleScheduledList(w http.ResponseWriter, r *http.Request, sess *Session) {
	rows, err := s.db.Query(r.Context(), `SELECT id, account, message_id, subject, send_at, status, error FROM scheduled_messages
		WHERE account = ANY($1) AND status IN ('pending', 'sending', 'failed') ORDER BY send_at`, userAccounts(sess))
	if err != nil {
		serverErr(w, "scheduled list", err)
		return
	}
	defer rows.Close()
	out := []scheduledInfo{}
	for rows.Next() {
		var x scheduledInfo
		if rows.Scan(&x.ID, &x.Account, &x.MessageID, &x.Subject, &x.SendAt, &x.Status, &x.Error) == nil {
			out = append(out, x)
		}
	}
	snoozed := []map[string]any{}
	if srows, err := s.db.Query(r.Context(), `SELECT account, message_id, until, folder FROM snoozed_messages WHERE account = ANY($1) ORDER BY until`, userAccounts(sess)); err == nil {
		for srows.Next() {
			var acc, id, folder string
			var until time.Time
			if srows.Scan(&acc, &id, &until, &folder) == nil {
				snoozed = append(snoozed, map[string]any{"account": acc, "messageId": id, "until": until, "folder": folder})
			}
		}
		srows.Close()
	}
	writeJSON(w, http.StatusOK, map[string]any{"scheduled": out, "snoozed": snoozed})
}

// POST /api/scheduled/{id} {action: "cancel" | "now" | "reschedule", sendAt}
// cancel: the letter goes back to Drafts for editing.
func (s *Server) handleScheduledAction(w http.ResponseWriter, r *http.Request, sess *Session) {
	var in struct {
		Action string    `json:"action"`
		SendAt time.Time `json:"sendAt"`
	}
	if err := readJSON(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "Invalid request")
		return
	}
	ctx := r.Context()
	var id int64
	_, _ = fmt.Sscan(r.PathValue("id"), &id)
	var account, msgID, status string
	if err := s.db.QueryRow(ctx, `SELECT account, message_id, status FROM scheduled_messages WHERE id=$1 AND account = ANY($2)`,
		id, userAccounts(sess)).Scan(&account, &msgID, &status); err != nil {
		writeErr(w, http.StatusNotFound, "The letter is no longer scheduled")
		return
	}
	if status == "sending" || status == "sent" {
		writeErr(w, http.StatusConflict, "The letter is already being sent")
		return
	}
	switch in.Action {
	case "now":
		_, _ = s.db.Exec(ctx, `UPDATE scheduled_messages SET send_at=now(), status='pending', error='' WHERE id=$1`, id)
		go s.sendDueScheduled(context.Background())
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	case "reschedule":
		if in.SendAt.Before(time.Now().Add(30*time.Second)) || in.SendAt.After(time.Now().AddDate(1, 0, 0)) {
			writeErr(w, http.StatusBadRequest, "Choose a time in the future (within a year)")
			return
		}
		_, _ = s.db.Exec(ctx, `UPDATE scheduled_messages SET send_at=$2, status='pending', error='' WHERE id=$1`, id, in.SendAt)
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	case "cancel":
		if _, err := s.db.Exec(ctx, `UPDATE scheduled_messages SET status='cancelled' WHERE id=$1 AND status IN ('pending', 'failed')`, id); err != nil {
			serverErr(w, "scheduled cancel", err)
			return
		}
		folder, uid := "", uint32(0)
		if c, err := s.dialAs(account); err == nil {
			if _, err := c.Select(scheduledFolder, false); err == nil {
				if u, err := uidByMessageID(c, msgID); err == nil {
					if drafts, err := s.folders.byRole(c, account, "drafts"); err == nil {
						set := new(imap.SeqSet)
						set.AddNum(u)
						_ = c.UidStore(set, imap.FormatFlagsOp(imap.AddFlags, true), []interface{}{imap.DraftFlag}, nil)
						if c.UidMove(set, drafts) == nil {
							folder = drafts
							if _, err := c.Select(drafts, true); err == nil {
								uid, _ = uidByMessageID(c, msgID)
							}
						}
					}
				}
			}
			_ = c.Logout()
		}
		s.hub.countsChanged(account)
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "account": account, "folder": folder, "uid": uid})
	default:
		writeErr(w, http.StatusBadRequest, "Unknown action")
	}
}

func (s *Server) runLater(ctx context.Context) {
	// A letter caught mid-way by a restart is not sent twice: the user decides.
	_, _ = s.db.Exec(ctx, `UPDATE scheduled_messages SET status='failed', error='The server restarted while sending. Check Sent and send again if needed.'
		WHERE status='sending'`)
	t := time.NewTicker(15 * time.Second)
	defer t.Stop()
	for {
		s.sendDueScheduled(ctx)
		s.returnDueSnoozed(ctx)
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

func (s *Server) sendDueScheduled(ctx context.Context) {
	rows, err := s.db.Query(ctx, `UPDATE scheduled_messages SET status='sending' WHERE id IN (
			SELECT id FROM scheduled_messages WHERE status='pending' AND send_at <= now() ORDER BY send_at LIMIT 20 FOR UPDATE SKIP LOCKED)
		RETURNING id, user_id, account, ident_id, message_id, subject, recipients, reply_folder, reply_uid`)
	if err != nil {
		return
	}
	type job struct {
		id, user                int64
		ident                   *int64
		account, msgID, subject string
		rcpts                   []string
		replyFolder, replyUID   string
	}
	var jobs []job
	for rows.Next() {
		var j job
		if rows.Scan(&j.id, &j.user, &j.account, &j.ident, &j.msgID, &j.subject, &j.rcpts, &j.replyFolder, &j.replyUID) == nil {
			jobs = append(jobs, j)
		}
	}
	rows.Close()
	for _, j := range jobs {
		status, errText := "sent", ""
		if err := s.sendScheduled(ctx, j.user, j.account, j.ident, j.msgID, j.subject, j.rcpts, j.replyFolder, j.replyUID); err != nil {
			status = "failed"
			if errors.Is(err, errNotFound) {
				status = "cancelled"
			} else {
				errText = sendErrorText(err)
				slog.Warn("scheduled send", "account", j.account, "err", err)
			}
		}
		_, _ = s.db.Exec(ctx, `UPDATE scheduled_messages SET status=$2, error=$3, sent_at=CASE WHEN $2='sent' THEN now() END WHERE id=$1`, j.id, status, errText)
		if status != "cancelled" {
			s.hub.publishUser(j.user, Event{Type: "scheduled", Account: j.account, Info: map[string]string{"status": status, "subject": j.subject, "error": errText}})
		}
	}
}

func (s *Server) sendScheduled(ctx context.Context, user int64, account string, ident *int64, msgID, subject string, rcpts []string, replyFolder, replyUID string) error {
	c, err := s.dialAs(account)
	if err != nil {
		return err
	}
	defer c.Logout()
	if _, err := c.Select(scheduledFolder, false); err != nil {
		return errNotFound
	}
	uid, err := uidByMessageID(c, msgID)
	if err != nil {
		return err
	}
	raw, _, err := fetchRawFlags(c, uid)
	if err != nil {
		return err
	}
	raw = setHeader(raw, "Date", time.Now().Format(time.RFC1123Z))
	if ident != nil {
		ea, err := scanExt(s.db.QueryRow(ctx, `SELECT `+extCols+` FROM external_accounts WHERE id=$1`, *ident))
		if err != nil {
			return fmt.Errorf("the external address used for this letter was removed")
		}
		if err := s.submitExternal(ea, rcpts, raw); err != nil {
			return err
		}
		go s.afterExternalSend(ea, msgID, raw)
	} else if err := s.submit(account, rcpts, raw); err != nil {
		return err
	}
	var login string
	_ = s.db.QueryRow(ctx, `SELECT login FROM users WHERE id=$1`, user).Scan(&login)
	s.recordSent(ctx, &Session{UserID: user, Login: login}, account, msgID, subject, rcpts, len(raw))
	if sent, err := s.folders.byRole(c, account, "sent"); err == nil {
		if err := appendWithin(c, sent, []string{imap.SeenFlag}, time.Now(), raw, 2*time.Minute); err != nil {
			slog.Warn("scheduled: append to Sent", "err", err)
		}
	}
	if _, err := c.Select(scheduledFolder, false); err == nil {
		removeDraftUID(c, uid)
	}
	if replyFolder != "" && replyUID != "" {
		markAnswered(c, replyFolder, replyUID)
	}
	s.hub.countsChanged(account)
	return nil
}

// ---------- snooze ----------

// POST /api/messages/snooze {account, folder, uids, until}
func (s *Server) handleSnooze(w http.ResponseWriter, r *http.Request, sess *Session) {
	var in struct {
		Account string    `json:"account"`
		Folder  string    `json:"folder"`
		UIDs    []uint32  `json:"uids"`
		Until   time.Time `json:"until"`
	}
	if err := readJSON(r, &in); err != nil || len(in.UIDs) == 0 || in.Folder == "" {
		writeErr(w, http.StatusBadRequest, "No messages selected")
		return
	}
	if in.Until.Before(time.Now().Add(time.Minute)) || in.Until.After(time.Now().AddDate(1, 0, 0)) {
		writeErr(w, http.StatusBadRequest, "Choose a time in the future (within a year)")
		return
	}
	if in.Folder == snoozedFolder {
		writeErr(w, http.StatusBadRequest, "The letter is already snoozed")
		return
	}
	s.withAccount(w, r, sess, in.Account, false, func(c *client.Client, acc Account) {
		if _, err := c.Select(in.Folder, false); err != nil {
			writeErr(w, http.StatusNotFound, "Folder not found")
			return
		}
		set := new(imap.SeqSet)
		set.AddNum(in.UIDs...)
		ids := fetchMessageIDs(c, set)
		if len(ids) == 0 {
			writeErr(w, http.StatusBadRequest, "These letters cannot be snoozed (no Message-ID)")
			return
		}
		if err := ensureFolder(c, snoozedFolder); err != nil {
			serverErr(w, "snooze folder", err)
			return
		}
		if _, err := c.Select(in.Folder, false); err != nil {
			serverErr(w, "snooze select", err)
			return
		}
		for _, id := range ids {
			if _, err := s.db.Exec(r.Context(), `INSERT INTO snoozed_messages(user_id, account, message_id, folder, until) VALUES ($1,$2,$3,$4,$5)
				ON CONFLICT (account, message_id) DO UPDATE SET until=EXCLUDED.until, folder=EXCLUDED.folder, user_id=EXCLUDED.user_id`,
				sess.UserID, acc.Address, id, in.Folder, in.Until); err != nil {
				serverErr(w, "snooze save", err)
				return
			}
		}
		if err := c.UidMove(set, snoozedFolder); err != nil {
			_, _ = s.db.Exec(r.Context(), `DELETE FROM snoozed_messages WHERE account=$1 AND message_id = ANY($2)`, acc.Address, ids)
			serverErr(w, "snooze move", err)
			return
		}
		s.hub.countsChanged(acc.Address)
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "count": len(ids)})
	})
}

// POST /api/messages/unsnooze {account, messageId}: back now.
func (s *Server) handleUnsnooze(w http.ResponseWriter, r *http.Request, sess *Session) {
	var in struct {
		Account   string `json:"account"`
		MessageID string `json:"messageId"`
	}
	if err := readJSON(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "Invalid request")
		return
	}
	if _, ok := sess.account(in.Account); !ok {
		writeErr(w, http.StatusForbidden, "No access")
		return
	}
	tag, err := s.db.Exec(r.Context(), `UPDATE snoozed_messages SET until=now() WHERE account=$1 AND message_id=$2`, in.Account, in.MessageID)
	if err != nil || tag.RowsAffected() == 0 {
		writeErr(w, http.StatusNotFound, "The letter is not snoozed")
		return
	}
	s.returnDueSnoozed(r.Context())
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) returnDueSnoozed(ctx context.Context) {
	rows, err := s.db.Query(ctx, `SELECT id, user_id, account, message_id, folder FROM snoozed_messages WHERE until <= now() ORDER BY until LIMIT 50`)
	if err != nil {
		return
	}
	type item struct {
		id, user               int64
		account, msgID, folder string
	}
	var items []item
	for rows.Next() {
		var it item
		if rows.Scan(&it.id, &it.user, &it.account, &it.msgID, &it.folder) == nil {
			items = append(items, it)
		}
	}
	rows.Close()
	for _, it := range items {
		if err := s.unsnooze(it.account, it.msgID, it.folder); err != nil && !errors.Is(err, errNotFound) {
			slog.Warn("snooze return", "account", it.account, "err", err)
			continue // tried again in 15 seconds
		}
		_, _ = s.db.Exec(ctx, `DELETE FROM snoozed_messages WHERE id=$1`, it.id)
	}
}

// unsnooze puts the letter back on top of its folder, unread: a new copy (arrival time now)
// replaces the one in Snoozed.
func (s *Server) unsnooze(account, msgID, folder string) error {
	c, err := s.dialAs(account)
	if err != nil {
		return err
	}
	defer c.Logout()
	if _, err := c.Select(snoozedFolder, false); err != nil {
		return errNotFound
	}
	uid, err := uidByMessageID(c, msgID)
	if err != nil {
		return err
	}
	raw, flags, err := fetchRawFlags(c, uid)
	if err != nil {
		return err
	}
	if folderExists(c, folder) != nil {
		folder = "INBOX"
	}
	keep := []string{}
	for _, f := range flags {
		if f != imap.SeenFlag && f != imap.RecentFlag && f != imap.DeletedFlag {
			keep = append(keep, f)
		}
	}
	if err := appendWithin(c, folder, keep, time.Now(), raw, 2*time.Minute); err != nil {
		return err
	}
	if _, err := c.Select(snoozedFolder, false); err == nil {
		removeDraftUID(c, uid)
	}
	s.hub.countsChanged(account)
	return nil
}
