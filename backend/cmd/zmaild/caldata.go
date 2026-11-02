package main

import (
	"bytes"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/emersion/go-ical"
)

// iCalendar ⇄ TMAIL events: phones and desktop programs (CalDAV), subscription links (ICS) and
// repeating events. An event a phone sent keeps its original iCalendar text, so what TMAIL does not
// show (attendees, locations, attachments…) survives an edit in the webmail.

const icsProdID = "-//TMAIL//Calendar//EN"

func loadTZ(name string) *time.Location {
	if loc, err := time.LoadLocation(name); err == nil && name != "" {
		return loc
	}
	return time.UTC
}

// icsTime reads DATE / DATE-TIME values. Unknown time zone names (Outlook's "Russian Standard
// Time") fall back to the user's zone instead of failing the whole event.
func icsTime(p *ical.Prop, tz *time.Location) (t time.Time, allDay bool, err error) {
	if p == nil {
		return time.Time{}, false, fmt.Errorf("no date")
	}
	allDay = p.ValueType() == ical.ValueDate || (p.ValueType() == ical.ValueDefault && len(p.Value) == 8)
	t, err = p.DateTime(tz)
	if err != nil && p.Params.Get(ical.PropTimezoneID) != "" {
		q := *p
		q.Params = ical.Params{}
		for k, v := range p.Params {
			if k != ical.PropTimezoneID {
				q.Params[k] = v
			}
		}
		t, err = q.DateTime(tz)
	}
	return t, allDay, err
}

// masterOf: the main VEVENT/VTODO (not an edited single occurrence).
func masterOf(cal *ical.Calendar) *ical.Component {
	var first *ical.Component
	for _, c := range cal.Children {
		if c.Name != ical.CompEvent && c.Name != ical.CompToDo {
			continue
		}
		if first == nil {
			first = c
		}
		if c.Props.Get(ical.PropRecurrenceID) == nil {
			return c
		}
	}
	return first
}

func decodeICS(raw string) (*ical.Calendar, error) {
	return ical.NewDecoder(strings.NewReader(raw)).Decode()
}

func encodeICS(cal *ical.Calendar) (string, error) {
	var b bytes.Buffer
	if err := ical.NewEncoder(&b).Encode(cal); err != nil {
		return "", err
	}
	return b.String(), nil
}

// parseICS turns an object from a phone into event fields.
func parseICS(raw string, tz *time.Location) (e CalEvent, uid string, err error) {
	cal, err := decodeICS(raw)
	if err != nil {
		return e, "", err
	}
	m := masterOf(cal)
	if m == nil {
		return e, "", fmt.Errorf("no event or task in the calendar object")
	}
	uid, _ = m.Props.Text(ical.PropUID)
	e.Title, _ = m.Props.Text(ical.PropSummary)
	e.Title = strings.TrimSpace(e.Title)
	if e.Title == "" {
		e.Title = "—"
	}
	e.Notes, _ = m.Props.Text(ical.PropDescription)
	e.Color = "blue"
	if c, _ := m.Props.Text("COLOR"); calColors[strings.ToLower(c)] {
		e.Color = strings.ToLower(c)
	}
	if rr := m.Props.Get(ical.PropRecurrenceRule); rr != nil {
		e.RRule = rr.Value
	}
	if m.Name == ical.CompToDo {
		e.IsTask = true
		st, _ := m.Props.Text(ical.PropStatus)
		e.Done = strings.EqualFold(st, "COMPLETED") || m.Props.Get(ical.PropCompleted) != nil
		p := m.Props.Get(ical.PropDue)
		if p == nil {
			p = m.Props.Get(ical.PropDateTimeStart)
		}
		if p != nil {
			e.Start, e.AllDay, err = icsTime(p, tz)
			if err != nil {
				return e, uid, err
			}
		} else {
			e.Start, e.AllDay = time.Now().In(tz).Truncate(24*time.Hour), true
		}
	} else {
		e.Start, e.AllDay, err = icsTime(m.Props.Get(ical.PropDateTimeStart), tz)
		if err != nil {
			return e, uid, err
		}
		var end time.Time
		if p := m.Props.Get(ical.PropDateTimeEnd); p != nil {
			end, _, _ = icsTime(p, tz)
		} else if p := m.Props.Get(ical.PropDuration); p != nil {
			if d, err := p.Duration(); err == nil {
				end = e.Start.Add(d)
			}
		}
		if e.AllDay && !end.IsZero() {
			end = end.AddDate(0, 0, -1) // iCalendar's end date is exclusive, TMAIL's is the last day
		}
		if !end.IsZero() && end.After(e.Start) {
			e.End = &end
		}
	}
	for _, a := range m.Children {
		if a.Name != ical.CompAlarm {
			continue
		}
		p := a.Props.Get(ical.PropTrigger)
		if p == nil {
			continue
		}
		var mins int
		if d, err := p.Duration(); err == nil && p.ValueType() != ical.ValueDateTime {
			mins = int(-d.Minutes())
		} else if t, err := p.DateTime(tz); err == nil {
			mins = int(e.Start.Sub(t).Minutes())
		}
		if mins < 0 {
			mins = 0
		}
		if mins <= 60*24*14 {
			e.RemindMinutes = &mins
		}
		break
	}
	return e, uid, nil
}

// buildICS writes the event into its iCalendar object: the stored one (from a phone) is updated in
// place, otherwise a new one is made.
func buildICS(e *CalEvent, uid string, tz *time.Location, raw string) string {
	var cal *ical.Calendar
	if raw != "" {
		cal, _ = decodeICS(raw)
	}
	if cal == nil {
		cal = ical.NewCalendar()
		cal.Props.SetText(ical.PropVersion, "2.0")
		cal.Props.SetText(ical.PropProductID, icsProdID)
	}
	kind := ical.CompEvent
	if e.IsTask {
		kind = ical.CompToDo
	}
	m := masterOf(cal)
	oldStart := ""
	if m != nil {
		if p := m.Props.Get(ical.PropDateTimeStart); p != nil {
			oldStart = p.Value
		}
		if p := m.Props.Get(ical.PropDue); p != nil && m.Name == ical.CompToDo {
			oldStart = p.Value
		}
	}
	if m == nil || m.Name != kind {
		m = ical.NewComponent(kind)
		kids := []*ical.Component{m}
		for _, c := range cal.Children {
			if c.Name == ical.CompTimezone {
				kids = append(kids, c)
			}
		}
		cal.Children = kids
	}
	now := time.Now().UTC()
	setDT := func(name string, t time.Time, date bool) {
		p := ical.NewProp(name)
		if date {
			p.SetDate(t.In(tz))
		} else {
			p.SetDateTime(t.UTC())
		}
		m.Props.Set(p)
	}
	m.Props.SetText(ical.PropUID, uid)
	setDT(ical.PropDateTimeStamp, now, false)
	setDT(ical.PropLastModified, now, false)
	m.Props.SetText(ical.PropSummary, e.Title)
	if e.Notes != "" {
		m.Props.SetText(ical.PropDescription, e.Notes)
	} else {
		m.Props.Del(ical.PropDescription)
	}
	m.Props.SetText("COLOR", e.Color)
	m.Props.Del(ical.PropDuration)
	if e.IsTask {
		m.Props.Del(ical.PropDateTimeStart)
		setDT(ical.PropDue, e.Start, e.AllDay)
		if e.Done {
			m.Props.SetText(ical.PropStatus, "COMPLETED")
			if m.Props.Get(ical.PropCompleted) == nil {
				setDT(ical.PropCompleted, now, false)
			}
		} else {
			m.Props.SetText(ical.PropStatus, "NEEDS-ACTION")
			m.Props.Del(ical.PropCompleted)
		}
	} else {
		setDT(ical.PropDateTimeStart, e.Start, e.AllDay)
		switch {
		case e.AllDay:
			last := e.Start
			if e.End != nil && e.End.After(e.Start) {
				last = *e.End
			}
			setDT(ical.PropDateTimeEnd, last.In(tz).AddDate(0, 0, 1), true)
		case e.End != nil:
			setDT(ical.PropDateTimeEnd, *e.End, false)
		default:
			setDT(ical.PropDateTimeEnd, e.Start.Add(time.Hour), false)
		}
	}
	newStart := ""
	if p := m.Props.Get(ical.PropDateTimeStart); p != nil {
		newStart = p.Value
	} else if p := m.Props.Get(ical.PropDue); p != nil {
		newStart = p.Value
	}
	if e.RRule != "" {
		m.Props.Set(&ical.Prop{Name: ical.PropRecurrenceRule, Params: ical.Params{}, Value: e.RRule})
	} else {
		m.Props.Del(ical.PropRecurrenceRule)
		m.Props.Del(ical.PropExceptionDates)
		m.Props.Del(ical.PropRecurrenceDates)
	}
	// Single edited occurrences no longer match when the series moved or stopped repeating.
	if e.RRule == "" || (oldStart != "" && oldStart != newStart) {
		kids := cal.Children[:0]
		for _, c := range cal.Children {
			if c == m || c.Props.Get(ical.PropRecurrenceID) == nil {
				kids = append(kids, c)
			}
		}
		cal.Children = kids
	}
	alarms := m.Children[:0]
	for _, c := range m.Children {
		if c.Name != ical.CompAlarm {
			alarms = append(alarms, c)
		}
	}
	m.Children = alarms
	if e.RemindMinutes != nil {
		a := ical.NewComponent(ical.CompAlarm)
		a.Props.SetText(ical.PropAction, "DISPLAY")
		a.Props.SetText(ical.PropDescription, e.Title)
		tr := ical.NewProp(ical.PropTrigger)
		tr.SetDuration(-time.Duration(*e.RemindMinutes) * time.Minute)
		a.Props.Set(tr)
		m.Children = append(m.Children, a)
	}
	out, err := encodeICS(cal)
	if err != nil {
		return ""
	}
	return out
}

// eventObject: the stored iCalendar text, or one made from the fields (events made in the webmail
// before phones could sync).
func eventObject(e *CalEvent, uid string, tz *time.Location, raw string) string {
	if raw != "" {
		return raw
	}
	return buildICS(e, uid, tz, "")
}

// expandEvent returns the occurrences of an event within [from, to): one for a single event, every
// repetition (edited ones as edited, deleted ones left out) for a repeating one.
func expandEvent(e CalEvent, uid, raw string, tz *time.Location, from, to time.Time) []CalEvent {
	dur := time.Duration(0)
	if e.End != nil {
		dur = e.End.Sub(e.Start)
	}
	if e.RRule == "" {
		if e.Start.Before(to) && !e.Start.Add(dur).Before(from) {
			return []CalEvent{e}
		}
		return nil
	}
	cal, err := decodeICS(eventObject(&e, uid, tz, raw))
	if err != nil {
		return []CalEvent{e}
	}
	m := masterOf(cal)
	if m == nil {
		return []CalEvent{e}
	}
	set, err := m.RecurrenceSet(tz)
	if err != nil || set == nil {
		return []CalEvent{e}
	}
	overrides := map[int64]*ical.Component{}
	for _, c := range cal.Children {
		if p := c.Props.Get(ical.PropRecurrenceID); p != nil && c != m {
			if t, _, err := icsTime(p, tz); err == nil {
				overrides[t.Unix()] = c
			}
		}
	}
	// The start of the series in the zone the times were written in: TMAIL keeps all-day events at
	// local midnight, iCalendar dates are read in the user's zone too.
	starts := set.Between(from.Add(-dur), to, true)
	out := []CalEvent{}
	for i, t := range starts {
		if i >= 500 {
			break
		}
		x := e
		occ := t.UTC()
		x.Occurrence = &occ
		x.SeriesStart, x.SeriesEnd = &e.Start, e.End
		x.Start = t
		if e.End != nil {
			end := t.Add(dur)
			x.End = &end
		}
		if o := overrides[t.Unix()]; o != nil {
			if st, _, err := icsTime(o.Props.Get(ical.PropDateTimeStart), tz); err == nil {
				x.Start = st
				if p := o.Props.Get(ical.PropDateTimeEnd); p != nil {
					if en, _, err := icsTime(p, tz); err == nil && en.After(st) {
						if x.AllDay {
							en = en.AddDate(0, 0, -1)
						}
						x.End = &en
					}
				}
			}
			if s, _ := o.Props.Text(ical.PropSummary); s != "" {
				x.Title = s
			}
			if st, _ := o.Props.Text(ical.PropStatus); strings.EqualFold(st, "CANCELLED") {
				continue
			}
		}
		out = append(out, x)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Start.Before(out[j].Start) })
	return out
}

// nextOccurrence: the first start of the event after the given moment (for reminders).
func nextOccurrence(e CalEvent, uid, raw string, tz *time.Location, after time.Time) (time.Time, bool) {
	if e.RRule == "" {
		return e.Start, e.Start.After(after)
	}
	cal, err := decodeICS(eventObject(&e, uid, tz, raw))
	if err != nil {
		return time.Time{}, false
	}
	m := masterOf(cal)
	if m == nil {
		return time.Time{}, false
	}
	set, err := m.RecurrenceSet(tz)
	if err != nil || set == nil {
		return time.Time{}, false
	}
	t := set.After(after, false)
	return t, !t.IsZero()
}

// addExDate removes one occurrence from a repeating event ("delete only this one").
func addExDate(e *CalEvent, uid string, tz *time.Location, raw string, occ time.Time) string {
	cal, err := decodeICS(eventObject(e, uid, tz, raw))
	if err != nil {
		return raw
	}
	m := masterOf(cal)
	if m == nil {
		return raw
	}
	p := ical.NewProp(ical.PropExceptionDates)
	if e.AllDay {
		p.SetDate(occ.In(tz))
	} else {
		p.SetDateTime(occ.UTC())
	}
	m.Props.Add(p)
	kids := cal.Children[:0]
	for _, c := range cal.Children {
		if rp := c.Props.Get(ical.PropRecurrenceID); rp != nil && c != m {
			if t, _, err := icsTime(rp, tz); err == nil && t.Equal(occ) {
				continue
			}
		}
		kids = append(kids, c)
	}
	cal.Children = kids
	out, err := encodeICS(cal)
	if err != nil {
		return raw
	}
	return out
}

// calendarFeed: all events of a calendar as one iCalendar file (subscription link).
func calendarFeed(name string, objects []string) string {
	cal := ical.NewCalendar()
	cal.Props.SetText(ical.PropVersion, "2.0")
	cal.Props.SetText(ical.PropProductID, icsProdID)
	raw := func(name, value string, params ical.Params) {
		cal.Props.Set(&ical.Prop{Name: name, Params: params, Value: value})
	}
	text := strings.NewReplacer(`\`, `\\`, ";", `\;`, ",", `\,`, "\n", `\n`).Replace(name)
	raw("NAME", text, ical.Params{})
	raw("X-WR-CALNAME", text, ical.Params{}) // Google and Outlook read this one
	raw("REFRESH-INTERVAL", "PT15M", ical.Params{"VALUE": {"DURATION"}})
	raw("X-PUBLISHED-TTL", "PT15M", ical.Params{})
	seenTZ := map[string]bool{}
	for _, raw := range objects {
		c, err := decodeICS(raw)
		if err != nil {
			continue
		}
		for _, k := range c.Children {
			if k.Name == ical.CompTimezone {
				id, _ := k.Props.Text(ical.PropTimezoneID)
				if seenTZ[id] {
					continue
				}
				seenTZ[id] = true
			}
			cal.Children = append(cal.Children, k)
		}
	}
	out, _ := encodeICS(cal)
	return out
}
