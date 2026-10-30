package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/emersion/go-imap"
)

// Deleting a mailbox together with all its letters. The mailbox is switched off at once (no mail is
// accepted, nobody can open it, the collector stops) and its letters are erased after a grace
// period; until then an administrator can restore it. "Erase now" skips the wait.

const mailboxGrace = 7 * 24 * time.Hour

// POST /api/admin/mailboxes/{id}/delete {confirm: "<address>", now: bool}
func (s *Server) adminDeleteMailbox(w http.ResponseWriter, r *http.Request, sess *Session) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	var in struct {
		Confirm string `json:"confirm"`
		Now     bool   `json:"now"`
	}
	if err := readJSON(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "Invalid request")
		return
	}
	var addr string
	var deleted *time.Time
	if err := s.db.QueryRow(r.Context(), `SELECT address, deleted_at FROM mailboxes WHERE id=$1`, id).Scan(&addr, &deleted); err != nil {
		writeErr(w, http.StatusNotFound, "Mailbox not found")
		return
	}
	// The address typed by hand: nothing is erased by a stray click.
	if !strings.EqualFold(strings.TrimSpace(in.Confirm), addr) {
		writeErr(w, http.StatusBadRequest, fmt.Sprintf("To confirm, type the full mailbox address: %s", addr))
		return
	}
	if deleted == nil {
		if _, err := s.db.Exec(r.Context(), `UPDATE mailboxes SET active=false, deleted_at=now(), purge_after=now()+make_interval(secs => $2), deleted_by=$3, updated_at=now() WHERE id=$1`,
			id, mailboxGrace.Seconds(), sess.Login); err != nil {
			serverErr(w, "mailbox delete", err)
			return
		}
		s.auditAdmin(r, sess, "mailbox.delete", addr, map[string]any{"now": in.Now})
		s.hub.countsChanged(addr)
	}
	if in.Now {
		if err := s.purgeMailbox(r.Context(), id, addr); err != nil {
			log.Printf("purge %s: %v", addr, err)
			writeErr(w, http.StatusBadGateway, fmt.Sprintf("The mailbox is disabled, but its messages could not be erased now. This will be done automatically: %v", err))
			return
		}
		s.auditAdmin(r, sess, "mailbox.purge", addr, nil)
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "purged": in.Now})
}

// POST /api/admin/mailboxes/{id}/restore — undo a deletion during the grace period.
func (s *Server) adminRestoreMailbox(w http.ResponseWriter, r *http.Request, sess *Session) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	var addr string
	if err := s.db.QueryRow(r.Context(), `UPDATE mailboxes SET active=true, deleted_at=NULL, purge_after=NULL, deleted_by='', updated_at=now()
		WHERE id=$1 AND deleted_at IS NOT NULL RETURNING address`, id).Scan(&addr); err != nil {
		writeErr(w, http.StatusNotFound, "Mailbox not found or already erased")
		return
	}
	s.auditAdmin(r, sess, "mailbox.restore", addr, nil)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// purgeMailbox erases every letter and folder of the mailbox, then everything TMAIL kept about it.
func (s *Server) purgeMailbox(ctx context.Context, id int64, addr string) error {
	// The mail server only opens active mailboxes: switched on for the few seconds of erasing.
	// Nothing new can arrive meanwhile that would survive: the row is removed right after.
	if _, err := s.db.Exec(ctx, `UPDATE mailboxes SET active=true WHERE id=$1 AND deleted_at IS NOT NULL`, id); err != nil {
		return err
	}
	err := s.wipeMailboxIMAP(addr)
	if err != nil {
		_, _ = s.db.Exec(ctx, `UPDATE mailboxes SET active=false WHERE id=$1`, id)
		return err
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var domainID int64
	var external bool
	_ = tx.QueryRow(ctx, `SELECT m.domain_id, d.external FROM mailboxes m JOIN domains d ON d.id=m.domain_id WHERE m.id=$1`, id).Scan(&domainID, &external)
	for _, q := range []string{
		`DELETE FROM external_accounts WHERE mailbox=$1`, // its seen/messages/folders go by cascade
		`DELETE FROM folder_rules WHERE account=$1`,
		`DELETE FROM sort_state WHERE account=$1`,
		`DELETE FROM sort_log WHERE account=$1`,
		`DELETE FROM crm_files WHERE account=$1`,
		`DELETE FROM crm_letters WHERE account=$1`,
		`DELETE FROM crm_processed WHERE account=$1`,
		`DELETE FROM crm_state WHERE account=$1`,
		`DELETE FROM autoreplies WHERE mailbox=$1`,
		`DELETE FROM reply_templates WHERE mailbox=$1`,
		`DELETE FROM av_quarantine WHERE mailbox=$1`,
		// AI tokens limited to this mailbox only are revoked (an empty list would mean "all").
		`UPDATE mcp_tokens SET revoked_at=now() WHERE revoked_at IS NULL AND mailboxes = ARRAY[$1]::text[]`,
		`UPDATE mcp_tokens SET mailboxes=array_remove(mailboxes, $1) WHERE $1 = ANY(mailboxes)`,
	} {
		if _, err := tx.Exec(ctx, q, addr); err != nil {
			return err
		}
	}
	// user_mailboxes, mailbox_settings, contacts, sent_messages… go with the row (ON DELETE CASCADE).
	if _, err := tx.Exec(ctx, `DELETE FROM mailboxes WHERE id=$1`, id); err != nil {
		return err
	}
	if external { // the domain of a mirrored Gmail… address exists only for this mailbox
		_, _ = tx.Exec(ctx, `DELETE FROM domains WHERE id=$1 AND external AND NOT EXISTS (SELECT 1 FROM mailboxes WHERE domain_id=$1)`, domainID)
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	log.Printf("mailbox %s erased", addr)
	return nil
}

// wipeMailboxIMAP deletes all letters of every folder and every folder except the Inbox.
func (s *Server) wipeMailboxIMAP(addr string) error {
	c, err := s.dialAs(addr)
	if err != nil {
		return err
	}
	defer c.Logout()
	folders, err := listFolders(c, false)
	if err != nil {
		return err
	}
	for _, f := range folders {
		mb, err := c.Select(f.Name, false)
		if err != nil {
			continue
		}
		if mb.Messages > 0 {
			all := new(imap.SeqSet)
			all.AddRange(1, 0)
			if err := c.Store(all, imap.FormatFlagsOp(imap.AddFlags, true), []interface{}{imap.DeletedFlag}, nil); err != nil {
				return err
			}
			if err := c.Expunge(nil); err != nil { // the whole folder goes, so plain EXPUNGE is right here
				return err
			}
		}
	}
	_, _ = c.Select("INBOX", true) // a folder cannot be deleted while it is selected
	for _, f := range folders {
		if !strings.EqualFold(f.Name, "INBOX") {
			_ = c.Delete(f.Name)
		}
	}
	return nil
}

// runMailboxPurge erases mailboxes whose grace period is over.
func (s *Server) runMailboxPurge(ctx context.Context) {
	t := time.NewTicker(time.Hour)
	defer t.Stop()
	for {
		rows, err := s.db.Query(ctx, `SELECT id, address FROM mailboxes WHERE deleted_at IS NOT NULL AND purge_after <= now()`)
		if err == nil {
			type mb struct {
				id   int64
				addr string
			}
			var list []mb
			for rows.Next() {
				var m mb
				if rows.Scan(&m.id, &m.addr) == nil {
					list = append(list, m)
				}
			}
			rows.Close()
			for _, m := range list {
				if err := s.purgeMailbox(ctx, m.id, m.addr); err != nil {
					log.Printf("purge %s: %v", m.addr, err)
				} else {
					s.audit(ctx, 0, "system", "", "mailbox.purge", m.addr, nil)
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
