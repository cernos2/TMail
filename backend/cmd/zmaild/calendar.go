package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/teambition/rrule-go"
)

// Calendars, events and the monthly planner. Every user has a personal calendar and may create
// more, share them with colleagues (read or edit) and publish a read-only subscription link.
// Phones and desktop programs sync over CalDAV (dav.go). Reminders are pushed to the author's open
// tabs over the event stream; a repeating event reminds before every occurrence.

type CalEvent struct {
	ID            int64      `json:"id"`
	CalendarID    int64      `json:"calendarId"`
	Title         string     `json:"title"`
	Notes         string     `json:"notes"`
	Start         time.Time  `json:"start"`
	End           *time.Time `json:"end,omitempty"`
	AllDay        bool       `json:"allDay"`
	Color         string     `json:"color"`
	IsTask        bool       `json:"isTask"`
	Done          bool       `json:"done"`
	RemindMinutes *int       `json:"remindMinutes,omitempty"`
	MailRef       string     `json:"mailRef,omitempty"`
	RRule         string     `json:"rrule,omitempty"`
	Occurrence    *time.Time `json:"occurrence,omitempty"`  // this repetition of a repeating event
	SeriesStart   *time.Time `json:"seriesStart,omitempty"` // a repetition: start and end of the first one
	SeriesEnd     *time.Time `json:"seriesEnd,omitempty"`
	CanEdit       bool       `json:"canEdit"`
}

var calColors = map[string]bool{"blue": true, "green": true, "yellow": true, "red": true, "purple": true, "gray": true, "teal": true, "orange": true}

const calCols = `e.id, e.calendar_id, e.title, e.notes, e.starts_at, e.ends_at, e.all_day, e.color, e.is_task, e.done, e.remind_minutes, e.mail_ref, e.rrule`

func scanEvent(row pgx.Row, extra ...any) (CalEvent, error) {
	var e CalEvent
	err := row.Scan(append([]any{&e.ID, &e.CalendarID, &e.Title, &e.Notes, &e.Start, &e.End, &e.AllDay, &e.Color, &e.IsTask, &e.Done,
		&e.RemindMinutes, &e.MailRef, &e.RRule}, extra...)...)
	return e, err
}

// userTZ: the user's time zone (all-day events and repetitions are counted in it).
func (s *Server) userTZ(ctx context.Context, user int64) *time.Location {
	_, tz := s.userLocale(ctx, user)
	if tz == "" {
		tz = serverLocale().Timezone
	}
	return loadTZ(tz)
}

// ensureCalendar returns the user's personal calendar, creating it for a new user.
func (s *Server) ensureCalendar(ctx context.Context, user int64) (int64, error) {
	var id int64
	err := s.db.QueryRow(ctx, `SELECT id FROM calendars WHERE owner_id=$1 ORDER BY id LIMIT 1`, user).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		err = s.db.QueryRow(ctx, `INSERT INTO calendars(owner_id) VALUES ($1) RETURNING id`, user).Scan(&id)
	}
	return id, err
}

// calAccess: "write" for the owner and colleagues allowed to edit, "read" for the others it is
// shared with, "" otherwise.
func (s *Server) calAccess(ctx context.Context, user, cal int64) string {
	var access string
	_ = s.db.QueryRow(ctx, `SELECT CASE WHEN c.owner_id=$2 OR sh.can_write THEN 'write' WHEN sh.user_id IS NOT NULL THEN 'read' ELSE '' END
		FROM calendars c LEFT JOIN calendar_shares sh ON sh.calendar_id=c.id AND sh.user_id=$2 WHERE c.id=$1`, cal, user).Scan(&access)
	return access
}

func newUID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	h := hex.EncodeToString(b)
	return h[:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:]
}

// remindAt: the start of the occurrence the next reminder is about.
func remindAt(e CalEvent, uid, raw string, tz *time.Location) *time.Time {
	if e.RemindMinutes == nil {
		return nil
	}
	if e.RRule == "" {
		return &e.Start
	}
	if t, ok := nextOccurrence(e, uid, raw, tz, time.Now().Add(-time.Minute)); ok {
		return &t
	}
	return nil
}

func (s *Server) handleCalEvents(w http.ResponseWriter, r *http.Request, sess *Session) {
	from, err1 := time.Parse(time.RFC3339, r.URL.Query().Get("from"))
	to, err2 := time.Parse(time.RFC3339, r.URL.Query().Get("to"))
	if err1 != nil || err2 != nil || to.Before(from) || to.Sub(from) > 400*24*time.Hour {
		writeErr(w, http.StatusBadRequest, "Invalid period")
		return
	}
	ctx := r.Context()
	if _, err := s.ensureCalendar(ctx, sess.UserID); err != nil {
		serverErr(w, "calendar ensure", err)
		return
	}
	// Repeating events are read whole (they started before the period) and expanded here.
	rows, err := s.db.Query(ctx, `SELECT `+calCols+`, e.uid, e.ical, (c.owner_id=$1 OR COALESCE(sh.can_write, false))
		FROM cal_events e JOIN calendars c ON c.id = e.calendar_id
		LEFT JOIN calendar_shares sh ON sh.calendar_id = c.id AND sh.user_id = $1
		WHERE ((c.owner_id=$1 AND NOT c.hidden) OR (sh.user_id IS NOT NULL AND NOT sh.hidden))
		  AND e.starts_at < $3 AND (e.rrule <> '' OR COALESCE(e.ends_at, e.starts_at) >= $2)
		ORDER BY e.starts_at, e.id LIMIT 3000`, sess.UserID, from, to)
	if err != nil {
		serverErr(w, "calendar list", err)
		return
	}
	defer rows.Close()
	tz := s.userTZ(ctx, sess.UserID)
	out := []CalEvent{}
	for rows.Next() {
		var uid, raw string
		var canEdit bool
		e, err := scanEvent(rows, &uid, &raw, &canEdit)
		if err != nil {
			serverErr(w, "calendar scan", err)
			return
		}
		e.CanEdit = canEdit
		out = append(out, expandEvent(e, uid, raw, tz, from, to)...)
		if len(out) > 5000 {
			break
		}
	}
	writeJSON(w, http.StatusOK, out)
}

func readEvent(r *http.Request) (CalEvent, error) {
	var e CalEvent
	if err := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 64<<10)).Decode(&e); err != nil {
		return e, errors.New("Invalid data")
	}
	e.Title = strings.TrimSpace(e.Title)
	if e.Title == "" {
		return e, errors.New("Enter a title")
	}
	if len([]rune(e.Title)) > 300 || len(e.Notes) > 20000 {
		return e, errors.New("The text is too long")
	}
	if e.Start.IsZero() {
		return e, errors.New("Specify a date")
	}
	if e.End != nil && e.End.Before(e.Start) {
		return e, errors.New("The end is before the start")
	}
	if !calColors[e.Color] {
		e.Color = "blue"
	}
	if e.RemindMinutes != nil && (*e.RemindMinutes < 0 || *e.RemindMinutes > 60*24*14) {
		return e, errors.New("Invalid reminder")
	}
	e.RRule = strings.TrimPrefix(strings.TrimSpace(e.RRule), "RRULE:")
	if e.RRule != "" {
		if _, err := rrule.StrToROption(e.RRule); err != nil || len(e.RRule) > 500 {
			return e, errors.New("Invalid repeat rule")
		}
	}
	return e, nil
}

func (s *Server) handleCalCreate(w http.ResponseWriter, r *http.Request, sess *Session) {
	e, err := readEvent(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	ctx := r.Context()
	if e.CalendarID == 0 {
		if e.CalendarID, err = s.ensureCalendar(ctx, sess.UserID); err != nil {
			serverErr(w, "calendar ensure", err)
			return
		}
	}
	if s.calAccess(ctx, sess.UserID, e.CalendarID) != "write" {
		writeErr(w, http.StatusForbidden, "You cannot add events to this calendar")
		return
	}
	tz := s.userTZ(ctx, sess.UserID)
	uid := newUID()
	raw := buildICS(&e, uid, tz, "")
	out, err := scanEvent(s.db.QueryRow(ctx, `INSERT INTO cal_events AS e
		(user_id, calendar_id, uid, href, title, notes, starts_at, ends_at, all_day, color, is_task, done, remind_minutes, mail_ref, rrule, ical, remind_at)
		VALUES ($1,$2,$3,$3||'.ics',$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16) RETURNING `+calCols,
		sess.UserID, e.CalendarID, uid, e.Title, e.Notes, e.Start, e.End, e.AllDay, e.Color, e.IsTask, e.Done, e.RemindMinutes, e.MailRef,
		e.RRule, raw, remindAt(e, uid, raw, tz)))
	if err != nil {
		serverErr(w, "calendar create", err)
		return
	}
	out.CanEdit = true
	writeJSON(w, http.StatusOK, out)
}

// loadEventForEdit: the event, its iCalendar object and whether the user may change it.
func (s *Server) loadEventForEdit(ctx context.Context, user, id int64) (e CalEvent, uid, raw string, err error) {
	e, err = scanEvent(s.db.QueryRow(ctx, `SELECT `+calCols+`, e.uid, e.ical FROM cal_events e WHERE e.id=$1`, id), &uid, &raw)
	if err != nil {
		return e, "", "", errors.New("Event not found")
	}
	if s.calAccess(ctx, user, e.CalendarID) != "write" {
		return e, "", "", errors.New("You cannot change events in this calendar")
	}
	return e, uid, raw, nil
}

func (s *Server) handleCalUpdate(w http.ResponseWriter, r *http.Request, sess *Session) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	e, err := readEvent(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	ctx := r.Context()
	old, uid, raw, err := s.loadEventForEdit(ctx, sess.UserID, id)
	if err != nil {
		writeErr(w, http.StatusNotFound, err.Error())
		return
	}
	if e.CalendarID == 0 {
		e.CalendarID = old.CalendarID
	}
	if e.CalendarID != old.CalendarID && s.calAccess(ctx, sess.UserID, e.CalendarID) != "write" {
		writeErr(w, http.StatusForbidden, "You cannot add events to this calendar")
		return
	}
	if e.MailRef == "" {
		e.MailRef = old.MailRef
	}
	tz := s.userTZ(ctx, sess.UserID)
	raw = buildICS(&e, uid, tz, raw)
	// A changed time, repetition or reminder re-arms the reminder.
	rearm := !old.Start.Equal(e.Start) || old.RRule != e.RRule || (old.RemindMinutes == nil) != (e.RemindMinutes == nil) ||
		(old.RemindMinutes != nil && e.RemindMinutes != nil && *old.RemindMinutes != *e.RemindMinutes)
	out, err := scanEvent(s.db.QueryRow(ctx, `UPDATE cal_events e SET calendar_id=$2, title=$3, notes=$4, starts_at=$5, ends_at=$6,
		all_day=$7, color=$8, is_task=$9, done=$10, remind_minutes=$11, mail_ref=$12, rrule=$13, ical=$14,
		remind_at = CASE WHEN $15 THEN $16 ELSE remind_at END,
		reminded_at = CASE WHEN $15 THEN NULL ELSE reminded_at END,
		ack_at = CASE WHEN $15 THEN NULL ELSE ack_at END,
		snooze_until = CASE WHEN $15 THEN NULL ELSE snooze_until END,
		updated_at=now()
		WHERE id=$1 RETURNING `+calCols,
		id, e.CalendarID, e.Title, e.Notes, e.Start, e.End, e.AllDay, e.Color, e.IsTask, e.Done, e.RemindMinutes, e.MailRef, e.RRule, raw,
		rearm, remindAt(e, uid, raw, tz)))
	if err != nil {
		serverErr(w, "calendar update", err)
		return
	}
	out.CanEdit = true
	writeJSON(w, http.StatusOK, out)
}

// DELETE /api/calendar/events/{id}[?occurrence=<start>]: the event, or one repetition of it.
func (s *Server) handleCalDelete(w http.ResponseWriter, r *http.Request, sess *Session) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	ctx := r.Context()
	e, uid, raw, err := s.loadEventForEdit(ctx, sess.UserID, id)
	if err != nil {
		writeErr(w, http.StatusNotFound, err.Error())
		return
	}
	if occ, err := time.Parse(time.RFC3339, r.URL.Query().Get("occurrence")); err == nil && e.RRule != "" {
		tz := s.userTZ(ctx, sess.UserID)
		raw = addExDate(&e, uid, tz, raw, occ)
		if _, err := s.db.Exec(ctx, `UPDATE cal_events SET ical=$2, updated_at=now() WHERE id=$1`, id, raw); err != nil {
			serverErr(w, "calendar exdate", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
		return
	}
	if _, err := s.db.Exec(ctx, `DELETE FROM cal_events WHERE id=$1`, id); err != nil {
		serverErr(w, "calendar delete", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// ---------- monthly planner page ----------

type planItem struct {
	Text string `json:"text"`
	Done bool   `json:"done"`
}

type planMonth struct {
	Goals []planItem `json:"goals"`
	Tasks []planItem `json:"tasks"`
	Notes string     `json:"notes"`
}

func monthParam(r *http.Request) (time.Time, bool) {
	t, err := time.Parse("2006-01", r.URL.Query().Get("m"))
	return t, err == nil
}

func (s *Server) handlePlanGet(w http.ResponseWriter, r *http.Request, sess *Session) {
	m, ok := monthParam(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, "Invalid month")
		return
	}
	p := planMonth{Goals: []planItem{}, Tasks: []planItem{}}
	var goals, tasks []byte
	err := s.db.QueryRow(r.Context(), `SELECT goals, tasks, notes FROM cal_months WHERE user_id=$1 AND month=$2`, sess.UserID, m).Scan(&goals, &tasks, &p.Notes)
	if err == nil {
		_ = json.Unmarshal(goals, &p.Goals)
		_ = json.Unmarshal(tasks, &p.Tasks)
	} else if !errors.Is(err, pgx.ErrNoRows) {
		serverErr(w, "planner get", err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (s *Server) handlePlanPut(w http.ResponseWriter, r *http.Request, sess *Session) {
	m, ok := monthParam(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, "Invalid month")
		return
	}
	var p planMonth
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 128<<10)).Decode(&p); err != nil || len(p.Goals) > 50 || len(p.Tasks) > 200 {
		writeErr(w, http.StatusBadRequest, "Invalid data")
		return
	}
	if p.Goals == nil {
		p.Goals = []planItem{}
	}
	if p.Tasks == nil {
		p.Tasks = []planItem{}
	}
	goals, _ := json.Marshal(p.Goals)
	tasks, _ := json.Marshal(p.Tasks)
	if _, err := s.db.Exec(r.Context(), `INSERT INTO cal_months(user_id, month, goals, tasks, notes) VALUES ($1,$2,$3,$4,$5)
		ON CONFLICT (user_id, month) DO UPDATE SET goals=EXCLUDED.goals, tasks=EXCLUDED.tasks, notes=EXCLUDED.notes, updated_at=now()`,
		sess.UserID, m, goals, tasks, p.Notes); err != nil {
		serverErr(w, "planner save", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// ---------- reminders ----------

func (s *Server) runReminders(ctx context.Context) {
	t := time.NewTicker(20 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.sendDueReminders(ctx)
		}
	}
}

func (s *Server) sendDueReminders(ctx context.Context) {
	s.advanceRepeating(ctx)
	// Not confirmed yet: shown again every 5 minutes (unless snoozed), for up to a day after the start.
	rows, err := s.db.Query(ctx, `SELECT e.user_id, COALESCE(e.remind_at, e.starts_at), `+calCols+` FROM cal_events e
		WHERE e.remind_minutes IS NOT NULL AND NOT e.done AND e.ack_at IS NULL
		  AND COALESCE(e.remind_at, e.starts_at) - make_interval(mins => e.remind_minutes) <= now()
		  AND COALESCE(e.remind_at, e.starts_at) > now() - interval '24 hours'
		  AND (e.reminded_at IS NULL OR e.reminded_at <= now() - interval '5 minutes' + interval '10 seconds')
		  AND (e.snooze_until IS NULL OR e.snooze_until <= now())
		ORDER BY 2 LIMIT 200`)
	if err != nil {
		log.Printf("reminders: %v", err)
		return
	}
	type due struct {
		user int64
		ev   CalEvent
	}
	var list []due
	for rows.Next() {
		var d due
		var at time.Time
		var ev CalEvent
		if err := rows.Scan(&d.user, &at, &ev.ID, &ev.CalendarID, &ev.Title, &ev.Notes, &ev.Start, &ev.End, &ev.AllDay, &ev.Color,
			&ev.IsTask, &ev.Done, &ev.RemindMinutes, &ev.MailRef, &ev.RRule); err == nil {
			if ev.RRule != "" { // the reminder is about this occurrence
				if ev.End != nil {
					end := at.Add(ev.End.Sub(ev.Start))
					ev.End = &end
				}
				ev.Start = at
			}
			d.ev = ev
			list = append(list, d)
		}
	}
	rows.Close()
	for _, d := range list {
		ev := d.ev
		if s.hub.publishUser(d.user, Event{Type: "reminder", Event: &ev}) > 0 {
			_, _ = s.db.Exec(ctx, `UPDATE cal_events SET reminded_at=now() WHERE id=$1`, ev.ID)
		}
	}
}

// advanceRepeating moves the reminder of a repeating event to its next occurrence once the
// current one is a day old (nobody confirmed it) or has no date yet.
func (s *Server) advanceRepeating(ctx context.Context) {
	rows, err := s.db.Query(ctx, `SELECT `+calCols+`, e.uid, e.ical, e.user_id FROM cal_events e
		WHERE e.rrule <> '' AND e.remind_minutes IS NOT NULL AND NOT e.done
		  AND (e.remind_at IS NULL OR e.remind_at < now() - interval '24 hours') LIMIT 100`)
	if err != nil {
		return
	}
	type item struct {
		e        CalEvent
		uid, raw string
		user     int64
	}
	var list []item
	for rows.Next() {
		var it item
		if e, err := scanEvent(rows, &it.uid, &it.raw, &it.user); err == nil {
			it.e = e
			list = append(list, it)
		}
	}
	rows.Close()
	for _, it := range list {
		next := remindAt(it.e, it.uid, it.raw, s.userTZ(ctx, it.user))
		if next == nil { // the series is over
			_, _ = s.db.Exec(ctx, `UPDATE cal_events SET remind_minutes=NULL WHERE id=$1`, it.e.ID)
			continue
		}
		_, _ = s.db.Exec(ctx, `UPDATE cal_events SET remind_at=$2, reminded_at=NULL, ack_at=NULL, snooze_until=NULL WHERE id=$1`, it.e.ID, *next)
	}
}

// POST /api/calendar/events/{id}/reminder {action: "done" | "ack" | "snooze", minutes}
// done — the task is done (or the event handled); ack — "got it", stop reminding;
// snooze — remind again in N minutes. Other open tabs close the reminder too.
func (s *Server) handleReminderAction(w http.ResponseWriter, r *http.Request, sess *Session) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	var in struct {
		Action  string `json:"action"`
		Minutes int    `json:"minutes"`
	}
	if err := readJSON(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "Invalid data")
		return
	}
	var q string
	args := []any{id, sess.UserID}
	// A repeating event: "done" and "got it" close this occurrence; the next one will remind again.
	if in.Action == "done" || in.Action == "ack" {
		var rr, uid, raw string
		var at *time.Time
		if s.db.QueryRow(r.Context(), `SELECT rrule, uid, ical, remind_at FROM cal_events WHERE id=$1 AND user_id=$2`, id, sess.UserID).
			Scan(&rr, &uid, &raw, &at) == nil && rr != "" && at != nil {
			e, err := scanEvent(s.db.QueryRow(r.Context(), `SELECT `+calCols+` FROM cal_events e WHERE e.id=$1`, id))
			if err == nil {
				next, ok := nextOccurrence(e, uid, raw, s.userTZ(r.Context(), sess.UserID), *at)
				q = `UPDATE cal_events SET remind_at=$3, reminded_at=NULL, ack_at=NULL, snooze_until=NULL WHERE id=$1 AND user_id=$2`
				if !ok {
					q = `UPDATE cal_events SET ack_at=now() WHERE id=$1 AND user_id=$2 AND $3::timestamptz IS NULL`
					args = append(args, nil)
				} else {
					args = append(args, next)
				}
				in.Action = "next"
			}
		}
	}
	switch in.Action {
	case "next":
	case "done":
		q = `UPDATE cal_events SET done = done OR is_task, ack_at=now(), updated_at=now() WHERE id=$1 AND user_id=$2`
	case "ack":
		q = `UPDATE cal_events SET ack_at=now(), updated_at=now() WHERE id=$1 AND user_id=$2`
	case "snooze":
		if in.Minutes < 1 || in.Minutes > 24*60 {
			in.Minutes = 15
		}
		q = `UPDATE cal_events SET snooze_until=now() + make_interval(mins => $3), updated_at=now() WHERE id=$1 AND user_id=$2`
		args = append(args, in.Minutes)
	default:
		writeErr(w, http.StatusBadRequest, "Unknown action")
		return
	}
	tag, err := s.db.Exec(r.Context(), q, args...)
	if err != nil || tag.RowsAffected() == 0 {
		writeErr(w, http.StatusNotFound, "Event not found")
		return
	}
	s.hub.publishUser(sess.UserID, Event{Type: "reminder_done", Event: &CalEvent{ID: id}})
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
