package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap/client"
)

// "Mail client" mode: an external mailbox (someone@gmail.com) is its own mailbox in TMAIL,
// mail is sent through its own SMTP, and read / flagged / deleted / archived are kept in sync in
// both directions with the external server.

// ensureExternalMailbox creates the TMAIL mailbox for an external address (and its domain, marked
// external so Postfix never treats it as ours) and gives the user access to it.
func (s *Server) ensureExternalMailbox(ctx context.Context, address, name string, userID int64) error {
	at := strings.LastIndex(address, "@")
	if at < 1 {
		return errors.New("invalid address")
	}
	local, domain := address[:at], address[at+1:]
	var domainID int64
	var external bool
	err := s.db.QueryRow(ctx, `SELECT id, external FROM domains WHERE name=$1`, domain).Scan(&domainID, &external)
	if err == nil && !external {
		return fmt.Errorf("%s is a domain of this server: such a mailbox is created in \"Mailboxes\", not connected", domain)
	}
	if err != nil {
		if err := s.db.QueryRow(ctx, `INSERT INTO domains(name, external) VALUES ($1, true) RETURNING id`, domain).Scan(&domainID); err != nil {
			return err
		}
	}
	// Nobody logs in to it directly: the password hash is unusable; TMAIL opens it as master user.
	junk := make([]byte, 16)
	_, _ = rand.Read(junk)
	var mailboxID int64
	if err := s.db.QueryRow(ctx, `INSERT INTO mailboxes(domain_id, local_part, address, display_name, password_hash, quota_bytes)
		VALUES ($1,$2,$3,$4,$5, 53687091200)
		ON CONFLICT (address) DO UPDATE SET active=true, display_name=CASE WHEN mailboxes.display_name='' THEN EXCLUDED.display_name ELSE mailboxes.display_name END
		RETURNING id`, domainID, local, address, name, "!disabled-"+hex.EncodeToString(junk)).Scan(&mailboxID); err != nil {
		return err
	}
	if userID > 0 {
		_, _ = s.db.Exec(ctx, `INSERT INTO user_mailboxes(user_id, mailbox_id, can_send) VALUES ($1,$2,true) ON CONFLICT DO NOTHING`, userID, mailboxID)
	}
	return nil
}

// externalFor returns the external account that a TMAIL mailbox mirrors (client mode), if any.
func (s *Server) externalFor(ctx context.Context, mailbox string) (extAccount, bool) {
	a, err := scanExt(s.db.QueryRow(ctx, `SELECT `+extCols+` FROM external_accounts WHERE mailbox=$1 AND lower(address)=lower(mailbox) LIMIT 1`, mailbox))
	return a, err == nil
}

// remoteFolders lists the external server's folders once per connection.
type remoteFolders struct {
	boxes []*imap.MailboxInfo
}

func listRemote(rc *client.Client) (*remoteFolders, error) {
	ch := make(chan *imap.MailboxInfo, 64)
	done := make(chan error, 1)
	go func() { done <- rc.List("", "*", ch) }()
	rf := &remoteFolders{}
	for mb := range ch {
		rf.boxes = append(rf.boxes, mb)
	}
	return rf, <-done
}

// byRole finds the remote folder for a role ("trash", "junk", "archive", "all", "inbox").
func (rf *remoteFolders) byRole(role string) string {
	if role == "inbox" {
		return "INBOX"
	}
	for _, mb := range rf.boxes {
		if role == "all" && hasAttr(mb.Attributes, imap.AllAttr) || role != "all" && roleOf(mb) == role {
			return mb.Name
		}
	}
	for _, mb := range rf.boxes {
		if guessRole(mb) == role {
			return mb.Name
		}
	}
	return ""
}

func guessRole(mb *imap.MailboxInfo) string {
	n := strings.ToLower(mb.Name)
	if mb.Delimiter != "" {
		n = strings.TrimPrefix(n, "inbox"+mb.Delimiter)
	}
	// English and Russian folder names: detection data, not interface text.
	switch n {
	case "sent", "sent items", "sent messages", "отправленные":
		return "sent"
	case "drafts", "черновики":
		return "drafts"
	case "trash", "deleted", "deleted items", "deleted messages", "корзина", "удаленные", "удалённые":
		return "trash"
	case "spam", "junk", "junk e-mail", "спам":
		return "junk"
	case "archive", "архив":
		return "archive"
	}
	return ""
}

// ---------- TMAIL → external ----------

type extLoc struct {
	accountID int64
	folder    string
	uid       uint32
	messageID string
}

// pushExternal repeats an action done in TMAIL on the external server (runs in the background).
func (s *Server) pushExternal(mailbox string, ids []string, action, target string) {
	ctx := context.Background()
	a, ok := s.externalFor(ctx, mailbox)
	if !ok || !a.Enabled || a.Protocol != "imap" {
		return
	}
	var twoWay bool
	if s.db.QueryRow(ctx, `SELECT two_way FROM external_accounts WHERE id=$1`, a.ID).Scan(&twoWay) != nil || !twoWay {
		return
	}
	rows, err := s.db.Query(ctx, `SELECT remote_folder, remote_uid, message_id FROM external_messages WHERE account_id=$1 AND message_id = ANY($2)`, a.ID, ids)
	if err != nil {
		return
	}
	var locs []extLoc
	for rows.Next() {
		var l extLoc
		var uid int64
		if rows.Scan(&l.folder, &uid, &l.messageID) == nil {
			l.uid, l.accountID = uint32(uid), a.ID
			locs = append(locs, l)
		}
	}
	rows.Close()
	if len(locs) == 0 {
		return
	}
	pass, err := s.unseal(a.passEnc)
	if err != nil {
		return
	}
	rc, err := remoteIMAP(a, pass)
	if err != nil {
		log.Printf("two-way %s: %v", a.Address, err)
		return
	}
	defer rc.Logout()
	rf, _ := listRemote(rc)
	byFolder := map[string][]extLoc{}
	for _, l := range locs {
		byFolder[l.folder] = append(byFolder[l.folder], l)
	}
	for rfolder, ls := range byFolder {
		if _, err := rc.Select(rfolder, false); err != nil {
			continue
		}
		set := new(imap.SeqSet)
		for _, l := range ls {
			set.AddNum(l.uid)
		}
		store := func(op imap.FlagsOp, flag string) error {
			return rc.UidStore(set, imap.FormatFlagsOp(op, true), []interface{}{flag}, nil)
		}
		var err error
		moved := false
		switch action {
		case "seen", "unseen", "flag", "unflag":
			col, flag, val := "seen", imap.SeenFlag, true
			var op imap.FlagsOp = imap.AddFlags
			if action == "unseen" {
				op, val = imap.RemoveFlags, false
			}
			if action == "flag" || action == "unflag" {
				col, flag = "flagged", imap.FlaggedFlag
				if action == "unflag" {
					op, val = imap.RemoveFlags, false
				}
			}
			if err = store(op, flag); err == nil {
				for _, l := range ls {
					_, _ = s.db.Exec(ctx, `UPDATE external_messages SET `+col+`=$4 WHERE account_id=$1 AND remote_folder=$2 AND remote_uid=$3`, a.ID, rfolder, int64(l.uid), val)
				}
			}
		case "delete":
			trash := rf.byRole("trash")
			if trash == "" || trash == rfolder {
				if err = store(imap.AddFlags, imap.DeletedFlag); err == nil {
					err = uidExpunge(rc, set)
				}
			} else {
				err = rc.UidMove(set, trash)
			}
			moved = true
		case "move":
			dest := ""
			switch {
			case target == "archive":
				// Gmail archives by taking the letter out of the Inbox ("All mail" keeps it).
				if dest = rf.byRole("archive"); dest == "" {
					dest = rf.byRole("all")
				}
			case target == "junk" || target == "trash" || target == "inbox":
				dest = rf.byRole(target)
			case strings.HasPrefix(target, "folder:"):
				want := strings.TrimPrefix(target, "folder:")
				for _, mb := range rf.boxes {
					if localName(mb) == want {
						dest = mb.Name
					}
				}
			}
			if dest == "" || dest == rfolder {
				continue
			}
			if dest == rf.byRole("all") && strings.EqualFold(rfolder, "INBOX") {
				// Gmail: removing the Inbox label = archive; MOVE to All mail does exactly that.
				err = rc.UidMove(set, dest)
			} else {
				err = rc.UidMove(set, dest)
			}
			moved = true
		}
		if err != nil {
			log.Printf("two-way %s %s %s: %v", a.Address, action, rfolder, err)
			continue
		}
		if moved {
			// The letter now has another UID there; it is found again by Message-ID on the next pass.
			for _, l := range ls {
				_, _ = s.db.Exec(ctx, `DELETE FROM external_messages WHERE account_id=$1 AND remote_folder=$2 AND remote_uid=$3`, a.ID, rfolder, int64(l.uid))
			}
		}
	}
}

// localName is the TMAIL folder name a custom remote folder is mirrored to.
func localName(mb *imap.MailboxInfo) string {
	name := mb.Name
	if mb.Delimiter != "" {
		name = strings.TrimPrefix(name, "INBOX"+mb.Delimiter)
		name = strings.ReplaceAll(name, mb.Delimiter, " - ")
	}
	name = strings.TrimPrefix(name, "[Gmail] - ")
	return strings.Map(func(r rune) rune {
		if strings.ContainsRune("/\\*%\"", r) {
			return '-'
		}
		return r
	}, name)
}

func fetchMessageIDs(c *client.Client, set *imap.SeqSet) []string {
	ch := make(chan *imap.Message, 16)
	done := make(chan error, 1)
	go func() { done <- c.UidFetch(set, []imap.FetchItem{imap.FetchEnvelope}, ch) }()
	var ids []string
	for m := range ch {
		if m.Envelope != nil && m.Envelope.MessageId != "" {
			ids = append(ids, m.Envelope.MessageId)
		}
	}
	<-done
	return ids
}

// ---------- external → TMAIL ----------

// pullRemoteState applies changes made on the external server (read on the phone, flagged,
// deleted, archived) to the TMAIL copies of letters fetched from one remote folder.
func (s *Server) pullRemoteState(ctx context.Context, a extAccount, rc, local *client.Client, rf *remoteFolders, remote string) {
	rows, err := s.db.Query(ctx, `SELECT remote_uid, message_id, local_folder, seen, flagged FROM external_messages
		WHERE account_id=$1 AND remote_folder=$2 ORDER BY remote_uid DESC LIMIT 2000`, a.ID, remote)
	if err != nil {
		return
	}
	type row struct {
		uid           uint32
		mid, folder   string
		seen, flagged bool
	}
	known := map[uint32]row{}
	set := new(imap.SeqSet)
	for rows.Next() {
		var r row
		var uid int64
		if rows.Scan(&uid, &r.mid, &r.folder, &r.seen, &r.flagged) == nil {
			r.uid = uint32(uid)
			known[r.uid] = r
			set.AddNum(r.uid)
		}
	}
	rows.Close()
	if len(known) == 0 {
		return
	}
	ch := make(chan *imap.Message, 64)
	done := make(chan error, 1)
	go func() { done <- rc.UidFetch(set, []imap.FetchItem{imap.FetchUid, imap.FetchFlags}, ch) }()
	present := map[uint32]bool{}
	type change struct {
		r             row
		seen, flagged bool
	}
	var changes []change
	for m := range ch {
		present[m.Uid] = true
		r, ok := known[m.Uid]
		if !ok {
			continue
		}
		seen, flagged := false, false
		for _, f := range m.Flags {
			seen = seen || f == imap.SeenFlag
			flagged = flagged || f == imap.FlaggedFlag
		}
		if seen != r.seen || flagged != r.flagged {
			changes = append(changes, change{r, seen, flagged})
		}
	}
	if err := <-done; err != nil {
		return
	}
	for _, ch := range changes {
		if uid, ok := s.findLocal(local, ch.r.folder, ch.r.mid); ok {
			set := new(imap.SeqSet)
			set.AddNum(uid)
			apply := func(on bool, flag string) {
				var op imap.FlagsOp = imap.RemoveFlags
				if on {
					op = imap.AddFlags
				}
				_ = local.UidStore(set, imap.FormatFlagsOp(op, true), []interface{}{flag}, nil)
			}
			if ch.seen != ch.r.seen {
				apply(ch.seen, imap.SeenFlag)
			}
			if ch.flagged != ch.r.flagged {
				apply(ch.flagged, imap.FlaggedFlag)
			}
		}
		_, _ = s.db.Exec(ctx, `UPDATE external_messages SET seen=$4, flagged=$5 WHERE account_id=$1 AND remote_folder=$2 AND remote_uid=$3`,
			a.ID, remote, int64(ch.r.uid), ch.seen, ch.flagged)
	}
	// Gone from the remote folder: deleted there (now in its Trash) or archived.
	trash := rf.byRole("trash")
	for uid, r := range known {
		if present[uid] {
			continue
		}
		inTrash := false
		if trash != "" && trash != remote {
			if _, err := rc.Select(trash, true); err == nil {
				crit := imap.NewSearchCriteria()
				crit.Header.Add("Message-Id", r.mid)
				if u, err := rc.UidSearch(crit); err == nil && len(u) > 0 {
					inTrash = true
				}
			}
			_, _ = rc.Select(remote, true)
		}
		role := "archive"
		if inTrash {
			role = "trash"
		}
		if strings.EqualFold(remote, "INBOX") || inTrash {
			if luid, ok := s.findLocal(local, r.folder, r.mid); ok {
				if dest, err := s.folders.byRole(local, a.Mailbox, role); err == nil && dest != r.folder {
					set := new(imap.SeqSet)
					set.AddNum(luid)
					_ = local.UidMove(set, dest)
				}
			}
		}
		_, _ = s.db.Exec(ctx, `DELETE FROM external_messages WHERE account_id=$1 AND remote_folder=$2 AND remote_uid=$3`, a.ID, remote, int64(uid))
	}
}

// findLocal finds a letter in a TMAIL folder by Message-ID (selects the folder read-write).
func (s *Server) findLocal(local *client.Client, folder, mid string) (uint32, bool) {
	if _, err := local.Select(folder, false); err != nil {
		return 0, false
	}
	crit := imap.NewSearchCriteria()
	crit.Header.Add("Message-Id", mid)
	uids, err := local.UidSearch(crit)
	if err != nil || len(uids) == 0 {
		return 0, false
	}
	return uids[len(uids)-1], true
}

// afterExternalSend: the source server's own Sent copy must not come back as a duplicate, and
// servers that do not keep sent mail themselves (unlike Gmail) get a copy in their Sent folder.
func (s *Server) afterExternalSend(a extAccount, msgID string, msg []byte) {
	ctx := context.Background()
	for _, k := range []string{msgID, "<" + strings.Trim(msgID, "<>") + ">"} {
		_, _ = s.db.Exec(ctx, `INSERT INTO external_seen(account_id, key) VALUES ($1,$2) ON CONFLICT DO NOTHING`, a.ID, k)
	}
	if a.Protocol != "imap" || strings.Contains(strings.ToLower(a.SMTPHost), "gmail.com") {
		return
	}
	pass, err := s.unseal(a.passEnc)
	if err != nil {
		return
	}
	rc, err := remoteIMAP(a, pass)
	if err != nil {
		return
	}
	defer rc.Logout()
	rf, err := listRemote(rc)
	if err != nil {
		return
	}
	if sent := rf.byRole("sent"); sent != "" {
		if err := appendWithin(rc, sent, []string{imap.SeenFlag}, time.Now(), msg, 2*time.Minute); err != nil {
			log.Printf("external sent copy %s: %v", a.Address, err)
		}
	}
}
