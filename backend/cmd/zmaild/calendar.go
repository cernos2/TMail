package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// Personal calendar and planner. Reminders are pushed to the user's open tabs over the event
// stream; a reminder nobody received is retried until an hour after the event started.

type CalEvent struct {
	ID            int64      `json:"id"`
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
}

var calColors = map[string]bool{"blue": true, "green": true, "yellow": true, "red": true, "purple": true, "gray": true, "teal": true, "orange": true}

const calCols = `id, title, notes, starts_at, ends_at, all_day, color, is_task, done, remind_minutes, mail_ref`

func scanEvent(row pgx.Row) (CalEvent, error) {
	var e CalEvent
	err := row.Scan(&e.ID, &e.Title, &e.Notes, &e.Start, &e.End, &e.AllDay, &e.Color, &e.IsTask, &e.Done, &e.RemindMinutes, &e.MailRef)
	return e, err
}

func (s *Server) handleCalEvents(w http.ResponseWriter, r *http.Request, sess *Session) {
	from, err1 := time.Parse(time.RFC3339, r.URL.Query().Get("from"))
	to, err2 := time.Parse(time.RFC3339, r.URL.Query().Get("to"))
	if err1 != nil || err2 != nil || to.Before(from) || to.Sub(from) > 400*24*time.Hour {
		writeErr(w, http.StatusBadRequest, "Invalid period")
		return
	}
	rows, err := s.db.Query(r.Context(), `SELECT `+calCols+` FROM cal_events
		WHERE user_id=$1 AND starts_at < $3 AND COALESCE(ends_at, starts_at) >= $2 ORDER BY starts_at, id LIMIT 2000`, sess.UserID, from, to)
	if err != nil {
		serverErr(w, "calendar list", err)
		return
	}
	defer rows.Close()
	out := []CalEvent{}
	for rows.Next() {
		e, err := scanEvent(rows)
		if err != nil {
			serverErr(w, "calendar scan", err)
			return
		}
		out = append(out, e)
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
	return e, nil
}

func (s *Server) handleCalCreate(w http.ResponseWriter, r *http.Request, sess *Session) {
	e, err := readEvent(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	out, err := scanEvent(s.db.QueryRow(r.Context(), `INSERT INTO cal_events
		(user_id, title, notes, starts_at, ends_at, all_day, color, is_task, done, remind_minutes, mail_ref)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) RETURNING `+calCols,
		sess.UserID, e.Title, e.Notes, e.Start, e.End, e.AllDay, e.Color, e.IsTask, e.Done, e.RemindMinutes, e.MailRef))
	if err != nil {
		serverErr(w, "calendar create", err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleCalUpdate(w http.ResponseWriter, r *http.Request, sess *Session) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	e, err := readEvent(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	// A changed time or reminder re-arms the reminder.
	out, err := scanEvent(s.db.QueryRow(r.Context(), `UPDATE cal_events SET title=$3, notes=$4, starts_at=$5, ends_at=$6,
		all_day=$7, color=$8, is_task=$9, done=$10,
		reminded_at = CASE WHEN starts_at IS DISTINCT FROM $5 OR remind_minutes IS DISTINCT FROM $11 THEN NULL ELSE reminded_at END,
		ack_at = CASE WHEN starts_at IS DISTINCT FROM $5 OR remind_minutes IS DISTINCT FROM $11 THEN NULL ELSE ack_at END,
		snooze_until = CASE WHEN starts_at IS DISTINCT FROM $5 OR remind_minutes IS DISTINCT FROM $11 THEN NULL ELSE snooze_until END,
		remind_minutes=$11, updated_at=now()
		WHERE id=$1 AND user_id=$2 RETURNING `+calCols,
		id, sess.UserID, e.Title, e.Notes, e.Start, e.End, e.AllDay, e.Color, e.IsTask, e.Done, e.RemindMinutes))
	if errors.Is(err, pgx.ErrNoRows) {
		writeErr(w, http.StatusNotFound, "Event not found")
		return
	}
	if err != nil {
		serverErr(w, "calendar update", err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleCalDelete(w http.ResponseWriter, r *http.Request, sess *Session) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if _, err := s.db.Exec(r.Context(), `DELETE FROM cal_events WHERE id=$1 AND user_id=$2`, id, sess.UserID); err != nil {
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
	// Not confirmed yet: shown again every 5 minutes (unless snoozed), for up to a day after the start.
	rows, err := s.db.Query(ctx, `SELECT user_id, `+calCols+` FROM cal_events
		WHERE remind_minutes IS NOT NULL AND NOT done AND ack_at IS NULL
		  AND starts_at - make_interval(mins => remind_minutes) <= now() AND starts_at > now() - interval '24 hours'
		  AND (reminded_at IS NULL OR reminded_at <= now() - interval '5 minutes' + interval '10 seconds')
		  AND (snooze_until IS NULL OR snooze_until <= now())
		ORDER BY starts_at LIMIT 200`)
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
		if err := rows.Scan(&d.user, &d.ev.ID, &d.ev.Title, &d.ev.Notes, &d.ev.Start, &d.ev.End, &d.ev.AllDay, &d.ev.Color,
			&d.ev.IsTask, &d.ev.Done, &d.ev.RemindMinutes, &d.ev.MailRef); err == nil {
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
	switch in.Action {
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
