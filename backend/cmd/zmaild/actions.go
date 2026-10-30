package main

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap/client"
	"github.com/jackc/pgx/v5"
)

func (s *Server) handleAction(w http.ResponseWriter, r *http.Request, sess *Session) {
	var in struct {
		Account string   `json:"account"`
		Folder  string   `json:"folder"`
		UIDs    []uint32 `json:"uids"`
		Action  string   `json:"action"` // seen|unseen|flag|unflag|move|delete
		Target  string   `json:"target"` // role for move: inbox|archive|junk|trash
	}
	if err := readJSON(r, &in); err != nil || len(in.UIDs) == 0 || in.Folder == "" {
		writeErr(w, http.StatusBadRequest, "No messages selected")
		return
	}
	s.withAccount(w, r, sess, in.Account, false, func(c *client.Client, acc Account) {
		if _, err := c.Select(in.Folder, false); err != nil {
			writeErr(w, http.StatusNotFound, "Folder not found")
			return
		}
		set := new(imap.SeqSet)
		set.AddNum(in.UIDs...)
		// Mirrored external mailbox (Gmail…): note Message-IDs now, repeat the action there afterwards.
		var extIDs []string
		if _, ok := s.externalFor(r.Context(), acc.Address); ok {
			extIDs = fetchMessageIDs(c, set)
		}
		flag := func(op imap.FlagsOp, f string) error {
			return c.UidStore(set, imap.FormatFlagsOp(op, true), []interface{}{f}, nil)
		}
		var err error
		switch in.Action {
		case "seen":
			err = flag(imap.AddFlags, imap.SeenFlag)
		case "unseen":
			err = flag(imap.RemoveFlags, imap.SeenFlag)
		case "flag":
			err = flag(imap.AddFlags, imap.FlaggedFlag)
		case "unflag":
			err = flag(imap.RemoveFlags, imap.FlaggedFlag)
		case "label", "unlabel":
			id, perr := strconv.ParseInt(in.Target, 10, 64)
			if perr != nil || id <= 0 {
				writeErr(w, http.StatusBadRequest, "No label specified")
				return
			}
			var op imap.FlagsOp = imap.AddFlags
			if in.Action == "unlabel" {
				op = imap.RemoveFlags
			}
			err = flag(op, labelKeyword(id))
		case "move", "delete":
			target := in.Target
			if in.Action == "delete" {
				target = "trash"
			}
			var dest string
			var ferr error
			if name, ok := strings.CutPrefix(target, "folder:"); ok && in.Action == "move" {
				dest, ferr = name, folderExists(c, name)
			} else {
				dest, ferr = s.folders.byRole(c, acc.Address, target)
			}
			if ferr != nil {
				writeErr(w, http.StatusBadRequest, "No such folder")
				return
			}
			if in.Action == "delete" && dest == in.Folder {
				// Already in Trash: delete permanently.
				if err = flag(imap.AddFlags, imap.DeletedFlag); err == nil {
					err = uidExpunge(c, set)
				}
			} else if dest != in.Folder {
				err = c.UidMove(set, dest)
			}
		default:
			writeErr(w, http.StatusBadRequest, "Unknown action")
			return
		}
		if err != nil {
			serverErr(w, "action "+in.Action, err)
			return
		}
		if len(extIDs) > 0 {
			go s.pushExternal(acc.Address, extIDs, in.Action, in.Target)
		}
		s.hub.countsChanged(acc.Address)
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	})
}

// ---------- settings ----------

func (s *Server) handleGetSettings(w http.ResponseWriter, r *http.Request, sess *Session) {
	var data []byte
	err := s.db.QueryRow(r.Context(), `SELECT data FROM user_settings WHERE user_id=$1`, sess.UserID).Scan(&data)
	if err == pgx.ErrNoRows {
		data = []byte("{}")
	} else if err != nil {
		serverErr(w, "settings", err)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(data)
}

func (s *Server) handlePutSettings(w http.ResponseWriter, r *http.Request, sess *Session) {
	var in map[string]any
	if err := readJSON(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "Invalid settings")
		return
	}
	if err := s.saveSettings(r.Context(), sess.UserID, in); err != nil {
		serverErr(w, "save settings", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) saveSettings(ctx context.Context, userID int64, data map[string]any) error {
	_, err := s.db.Exec(ctx, `INSERT INTO user_settings(user_id, data) VALUES ($1, $2)
		ON CONFLICT (user_id) DO UPDATE SET data = user_settings.data || EXCLUDED.data, updated_at = now()`, userID, data)
	return err
}
