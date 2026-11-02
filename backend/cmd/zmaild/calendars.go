package main

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Calendars of a user: own ones (the personal one and any created later) and colleagues' ones
// shared with them. The owner decides who sees a calendar and who may edit it, and may publish a
// secret read-only link (ICS) to subscribe from Google Calendar, Apple Calendar or Outlook.

type calendarInfo struct {
	ID        int64         `json:"id"`
	Name      string        `json:"name"` // "" = personal calendar, named by the page in the user's language
	Color     string        `json:"color"`
	Mine      bool          `json:"mine"`
	CanWrite  bool          `json:"canWrite"`
	Hidden    bool          `json:"hidden"`
	Owner     string        `json:"owner"` // owner's name (or login) for colleagues' calendars
	ICSURL    string        `json:"icsUrl,omitempty"`
	Shares    []calendarAcl `json:"shares,omitempty"`
	DAVPath   string        `json:"davPath"`
	CreatedAt time.Time     `json:"-"`
}

type calendarAcl struct {
	UserID   int64  `json:"userId"`
	Login    string `json:"login"`
	Name     string `json:"name"`
	CanWrite bool   `json:"canWrite"`
}

func (s *Server) icsURL(token string) string {
	return "https://" + s.cfg.Hostname + "/ics/" + token + ".ics"
}

func (s *Server) handleCalendars(w http.ResponseWriter, r *http.Request, sess *Session) {
	ctx := r.Context()
	if _, err := s.ensureCalendar(ctx, sess.UserID); err != nil {
		serverErr(w, "calendar ensure", err)
		return
	}
	rows, err := s.db.Query(ctx, `SELECT c.id, c.name, c.color, c.owner_id = $1, c.owner_id = $1 OR COALESCE(sh.can_write, false),
			CASE WHEN c.owner_id = $1 THEN c.hidden ELSE sh.hidden END, COALESCE(NULLIF(u.display_name, ''), u.login), COALESCE(c.ics_token, ''),
			COALESCE((SELECT json_agg(json_build_object('userId', x.user_id, 'login', xu.login, 'name', xu.display_name, 'canWrite', x.can_write) ORDER BY xu.login)
				FROM calendar_shares x JOIN users xu ON xu.id = x.user_id WHERE x.calendar_id = c.id AND c.owner_id = $1), '[]')
		FROM calendars c JOIN users u ON u.id = c.owner_id
		LEFT JOIN calendar_shares sh ON sh.calendar_id = c.id AND sh.user_id = $1
		WHERE c.owner_id = $1 OR sh.user_id IS NOT NULL
		ORDER BY c.owner_id <> $1, c.id`, sess.UserID)
	if err != nil {
		serverErr(w, "calendars", err)
		return
	}
	defer rows.Close()
	out := []calendarInfo{}
	for rows.Next() {
		var c calendarInfo
		var token string
		if err := rows.Scan(&c.ID, &c.Name, &c.Color, &c.Mine, &c.CanWrite, &c.Hidden, &c.Owner, &token, &c.Shares); err != nil {
			serverErr(w, "calendars scan", err)
			return
		}
		if token != "" && c.Mine {
			c.ICSURL = s.icsURL(token)
		}
		c.DAVPath = "/dav/calendars/" + sess.Login + "/" + strconv.FormatInt(c.ID, 10) + "/"
		out = append(out, c)
	}
	writeJSON(w, http.StatusOK, map[string]any{"calendars": out, "dav": "https://" + s.cfg.Hostname + "/dav/", "login": sess.Login})
}

// GET /api/calendars/people: colleagues a calendar can be shared with.
func (s *Server) handleCalendarPeople(w http.ResponseWriter, r *http.Request, sess *Session) {
	rows, err := s.db.Query(r.Context(), `SELECT id, login, display_name FROM users WHERE active AND id <> $1 ORDER BY lower(coalesce(nullif(display_name, ''), login))`, sess.UserID)
	if err != nil {
		serverErr(w, "calendar people", err)
		return
	}
	defer rows.Close()
	out := []calendarAcl{}
	for rows.Next() {
		var p calendarAcl
		if rows.Scan(&p.UserID, &p.Login, &p.Name) == nil {
			out = append(out, p)
		}
	}
	writeJSON(w, http.StatusOK, out)
}

type calendarInput struct {
	Name   *string        `json:"name"`
	Color  *string        `json:"color"`
	Hidden *bool          `json:"hidden"`
	Shares *[]calendarAcl `json:"shares"`
	ICS    *bool          `json:"ics"` // true: publish (or keep) the link, false: remove it
	Rotate bool           `json:"rotate"`
}

func (s *Server) handleCalendarCreate(w http.ResponseWriter, r *http.Request, sess *Session) {
	var in calendarInput
	if err := readJSON(r, &in); err != nil || in.Name == nil || strings.TrimSpace(*in.Name) == "" {
		writeErr(w, http.StatusBadRequest, "Name the calendar")
		return
	}
	color := "blue"
	if in.Color != nil && calColors[*in.Color] {
		color = *in.Color
	}
	var n int
	_ = s.db.QueryRow(r.Context(), `SELECT count(*) FROM calendars WHERE owner_id=$1`, sess.UserID).Scan(&n)
	if n >= 30 {
		writeErr(w, http.StatusBadRequest, "Too many calendars")
		return
	}
	var id int64
	if err := s.db.QueryRow(r.Context(), `INSERT INTO calendars(owner_id, name, color) VALUES ($1,$2,$3) RETURNING id`,
		sess.UserID, truncate(strings.TrimSpace(*in.Name), 80), color).Scan(&id); err != nil {
		serverErr(w, "calendar create", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]int64{"id": id})
}

func (s *Server) handleCalendarUpdate(w http.ResponseWriter, r *http.Request, sess *Session) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	var in calendarInput
	if err := readJSON(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "Invalid request")
		return
	}
	ctx := r.Context()
	var owner int64
	if err := s.db.QueryRow(ctx, `SELECT owner_id FROM calendars WHERE id=$1`, id).Scan(&owner); err != nil {
		writeErr(w, http.StatusNotFound, "Calendar not found")
		return
	}
	mine := owner == sess.UserID
	if !mine {
		// A colleague may only switch a shared calendar on or off in their own view.
		if in.Hidden == nil || in.Name != nil || in.Color != nil || in.Shares != nil || in.ICS != nil {
			writeErr(w, http.StatusForbidden, "Only the owner can change this calendar")
			return
		}
		if _, err := s.db.Exec(ctx, `UPDATE calendar_shares SET hidden=$3 WHERE calendar_id=$1 AND user_id=$2`, id, sess.UserID, *in.Hidden); err != nil {
			serverErr(w, "calendar hide", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
		return
	}
	if in.Name != nil {
		_, _ = s.db.Exec(ctx, `UPDATE calendars SET name=$2, ctag=ctag+1 WHERE id=$1`, id, truncate(strings.TrimSpace(*in.Name), 80))
	}
	if in.Color != nil && calColors[*in.Color] {
		_, _ = s.db.Exec(ctx, `UPDATE calendars SET color=$2, ctag=ctag+1 WHERE id=$1`, id, *in.Color)
	}
	if in.Hidden != nil {
		_, _ = s.db.Exec(ctx, `UPDATE calendars SET hidden=$2 WHERE id=$1`, id, *in.Hidden)
	}
	if in.Shares != nil {
		tx, err := s.db.Begin(ctx)
		if err != nil {
			serverErr(w, "calendar shares", err)
			return
		}
		defer tx.Rollback(ctx)
		keep := []int64{}
		for _, sh := range *in.Shares {
			if sh.UserID == sess.UserID {
				continue
			}
			if _, err := tx.Exec(ctx, `INSERT INTO calendar_shares(calendar_id, user_id, can_write)
				SELECT $1, id, $3 FROM users WHERE id=$2 AND active
				ON CONFLICT (calendar_id, user_id) DO UPDATE SET can_write=EXCLUDED.can_write`, id, sh.UserID, sh.CanWrite); err != nil {
				serverErr(w, "calendar share", err)
				return
			}
			keep = append(keep, sh.UserID)
		}
		if _, err := tx.Exec(ctx, `DELETE FROM calendar_shares WHERE calendar_id=$1 AND NOT (user_id = ANY($2))`, id, keep); err != nil {
			serverErr(w, "calendar unshare", err)
			return
		}
		if err := tx.Commit(ctx); err != nil {
			serverErr(w, "calendar shares commit", err)
			return
		}
		s.audit(ctx, sess.UserID, sess.Login, clientIP(r), "calendar.share", strconv.FormatInt(id, 10), map[string]any{"users": keep})
	}
	resp := map[string]any{"ok": true}
	if in.ICS != nil {
		if *in.ICS {
			var token string
			_ = s.db.QueryRow(ctx, `SELECT COALESCE(ics_token, '') FROM calendars WHERE id=$1`, id).Scan(&token)
			if token == "" || in.Rotate {
				b := make([]byte, 24)
				_, _ = rand.Read(b)
				token = hex.EncodeToString(b)
				_, _ = s.db.Exec(ctx, `UPDATE calendars SET ics_token=$2 WHERE id=$1`, id, token)
				s.audit(ctx, sess.UserID, sess.Login, clientIP(r), "calendar.ics_publish", strconv.FormatInt(id, 10), nil)
			}
			resp["icsUrl"] = s.icsURL(token)
		} else {
			_, _ = s.db.Exec(ctx, `UPDATE calendars SET ics_token=NULL WHERE id=$1`, id)
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleCalendarDelete(w http.ResponseWriter, r *http.Request, sess *Session) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	ctx := r.Context()
	var first int64
	_ = s.db.QueryRow(ctx, `SELECT id FROM calendars WHERE owner_id=$1 ORDER BY id LIMIT 1`, sess.UserID).Scan(&first)
	if first == id {
		writeErr(w, http.StatusBadRequest, "The personal calendar cannot be deleted")
		return
	}
	tag, err := s.db.Exec(ctx, `DELETE FROM calendars WHERE id=$1 AND owner_id=$2`, id, sess.UserID)
	if err != nil || tag.RowsAffected() == 0 {
		writeErr(w, http.StatusNotFound, "Calendar not found")
		return
	}
	s.audit(ctx, sess.UserID, sess.Login, clientIP(r), "calendar.delete", strconv.FormatInt(id, 10), nil)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// GET /ics/<token>.ics — the published calendar, read-only, no sign-in (the token is the key).
func (s *Server) handleICSFeed(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimSuffix(r.PathValue("file"), ".ics")
	if len(token) != 48 {
		http.NotFound(w, r)
		return
	}
	ctx := r.Context()
	var id, owner int64
	var name string
	if err := s.db.QueryRow(ctx, `SELECT id, owner_id, name FROM calendars WHERE ics_token=$1`, token).Scan(&id, &owner, &name); err != nil {
		http.NotFound(w, r)
		return
	}
	if name == "" {
		name = s.cfg.Hostname
	}
	tz := s.userTZ(ctx, owner)
	rows, err := s.db.Query(ctx, `SELECT `+calCols+`, e.uid, e.ical FROM cal_events e WHERE e.calendar_id=$1
		AND (e.rrule <> '' OR e.starts_at > now() - interval '400 days') ORDER BY e.starts_at LIMIT 5000`, id)
	if err != nil {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
		return
	}
	var objects []string
	for rows.Next() {
		var uid, raw string
		if e, err := scanEvent(rows, &uid, &raw); err == nil {
			objects = append(objects, eventObject(&e, uid, tz, raw))
		}
	}
	rows.Close()
	w.Header().Set("Content-Type", "text/calendar; charset=utf-8")
	w.Header().Set("Cache-Control", "private, max-age=300")
	w.Header().Set("Content-Disposition", `inline; filename="calendar.ics"`)
	_, _ = w.Write([]byte(calendarFeed(name, objects)))
}
