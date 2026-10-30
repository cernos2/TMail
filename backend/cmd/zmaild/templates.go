package main

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
)

// Reply templates ("Template replies"): shared company templates (admins edit them) and personal ones.

type replyTemplate struct {
	ID      int64  `json:"id"`
	Title   string `json:"title"`
	Body    string `json:"body"`
	Mailbox string `json:"mailbox"`
	Shared  bool   `json:"shared"`
	Mine    bool   `json:"mine"`
}

func (s *Server) handleTemplates(w http.ResponseWriter, r *http.Request, sess *Session) {
	rows, err := s.db.Query(r.Context(), `SELECT id, title, body, mailbox, owner_id IS NULL, COALESCE(owner_id = $1, false)
		FROM reply_templates WHERE owner_id IS NULL OR owner_id = $1 ORDER BY owner_id NULLS FIRST, sort, title`, sess.UserID)
	if err != nil {
		serverErr(w, "templates", err)
		return
	}
	defer rows.Close()
	out := []replyTemplate{}
	for rows.Next() {
		var t replyTemplate
		if rows.Scan(&t.ID, &t.Title, &t.Body, &t.Mailbox, &t.Shared, &t.Mine) == nil {
			out = append(out, t)
		}
	}
	writeJSON(w, http.StatusOK, out)
}

func readTemplate(w http.ResponseWriter, r *http.Request, sess *Session) (replyTemplate, bool) {
	var t replyTemplate
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&t); err != nil {
		writeErr(w, http.StatusBadRequest, "Invalid data")
		return t, false
	}
	t.Title, t.Body = strings.TrimSpace(t.Title), strings.TrimSpace(t.Body)
	if t.Title == "" || t.Body == "" {
		writeErr(w, http.StatusBadRequest, "A template title and text are required")
		return t, false
	}
	if t.Shared && !sess.IsAdmin {
		writeErr(w, http.StatusForbidden, "Shared templates are changed by administrators")
		return t, false
	}
	return t, true
}

func (s *Server) handleTemplateCreate(w http.ResponseWriter, r *http.Request, sess *Session) {
	t, ok := readTemplate(w, r, sess)
	if !ok {
		return
	}
	var owner any = sess.UserID
	if t.Shared {
		owner = nil
	}
	if err := s.db.QueryRow(r.Context(), `INSERT INTO reply_templates(owner_id, title, body, mailbox, sort)
		VALUES ($1,$2,$3,$4,(SELECT COALESCE(max(sort),0)+10 FROM reply_templates)) RETURNING id`, owner, t.Title, t.Body, t.Mailbox).Scan(&t.ID); err != nil {
		serverErr(w, "template create", err)
		return
	}
	writeJSON(w, http.StatusOK, t)
}

// Personal templates can be changed by their owner, shared ones by admins.
const tplAllowed = `(owner_id = $2 OR (owner_id IS NULL AND $3))`

func (s *Server) handleTemplateUpdate(w http.ResponseWriter, r *http.Request, sess *Session) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	t, ok := readTemplate(w, r, sess)
	if !ok {
		return
	}
	tag, err := s.db.Exec(r.Context(), `UPDATE reply_templates SET title=$4, body=$5, mailbox=$6, updated_at=now() WHERE id=$1 AND `+tplAllowed,
		id, sess.UserID, sess.IsAdmin, t.Title, t.Body, t.Mailbox)
	if err != nil || tag.RowsAffected() == 0 {
		writeErr(w, http.StatusForbidden, "This template cannot be changed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleTemplateDelete(w http.ResponseWriter, r *http.Request, sess *Session) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	tag, err := s.db.Exec(r.Context(), `DELETE FROM reply_templates WHERE id=$1 AND `+tplAllowed, id, sess.UserID, sess.IsAdmin)
	if err != nil || tag.RowsAffected() == 0 {
		writeErr(w, http.StatusForbidden, "This template cannot be deleted")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
