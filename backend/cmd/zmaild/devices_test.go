package main

import (
	"strings"
	"testing"
	"time"

	"github.com/emersion/go-vcard"
)

func TestSetHeaderReplacesDate(t *testing.T) {
	raw := []byte("From: a@b.c\r\nDate: Mon, 1 Jan 2026 10:00:00 +0000\r\nSubject: x\r\n folded\r\n\r\nbody\r\n")
	out := string(setHeader(raw, "Date", "Tue, 2 Jan 2026 11:00:00 +0000"))
	if !strings.Contains(out, "Date: Tue, 2 Jan 2026 11:00:00 +0000\r\n") || strings.Contains(out, "Mon, 1 Jan") {
		t.Fatalf("date not replaced: %q", out)
	}
	if !strings.HasSuffix(out, "\r\n\r\nbody\r\n") || !strings.Contains(out, "Subject: x\r\n folded\r\n") {
		t.Fatalf("message damaged: %q", out)
	}
	out = string(setHeader([]byte("From: a@b.c\r\n\r\nbody"), "Date", "X"))
	if !strings.Contains(out, "Date: X\r\n\r\nbody") {
		t.Fatalf("date not added: %q", out)
	}
}

func TestICSRoundTrip(t *testing.T) {
	tz, _ := time.LoadLocation("Europe/Moscow")
	start := time.Date(2026, 11, 2, 10, 0, 0, 0, tz)
	end := start.Add(90 * time.Minute)
	rem := 15
	e := CalEvent{Title: "Встреча; с ООО «Ромашка»", Notes: "строка1\nстрока2", Start: start, End: &end, Color: "green", RemindMinutes: &rem, RRule: "FREQ=WEEKLY;COUNT=4"}
	raw := buildICS(&e, "uid-1", tz, "")
	got, uid, err := parseICS(raw, tz)
	if err != nil || uid != "uid-1" {
		t.Fatalf("parse: %v %q\n%s", err, uid, raw)
	}
	if got.Title != e.Title || got.Notes != e.Notes || !got.Start.Equal(start) || got.End == nil || !got.End.Equal(end) || got.RRule != e.RRule ||
		got.RemindMinutes == nil || *got.RemindMinutes != 15 || got.Color != "green" {
		t.Fatalf("round trip differs: %+v\n%s", got, raw)
	}
	occ := expandEvent(got, uid, raw, tz, start.AddDate(0, 0, -1), start.AddDate(0, 2, 0))
	if len(occ) != 4 || !occ[1].Start.Equal(start.AddDate(0, 0, 7)) || occ[1].End == nil || !occ[1].End.Equal(end.AddDate(0, 0, 7)) {
		t.Fatalf("expected 4 weekly occurrences, got %d: %+v", len(occ), occ)
	}
	// Deleting one occurrence.
	raw2 := addExDate(&got, uid, tz, raw, occ[1].Start)
	if n := len(expandEvent(got, uid, raw2, tz, start.AddDate(0, 0, -1), start.AddDate(0, 2, 0))); n != 3 {
		t.Fatalf("after EXDATE: %d occurrences\n%s", n, raw2)
	}
	if next, ok := nextOccurrence(got, uid, raw2, tz, start); !ok || !next.Equal(start.AddDate(0, 0, 14)) {
		t.Fatalf("next occurrence: %v %v", next, ok)
	}
}

func TestICSAllDayAndPhoneObject(t *testing.T) {
	tz, _ := time.LoadLocation("Europe/Moscow")
	phone := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//Apple Inc.//iOS 18//EN\r\nBEGIN:VEVENT\r\nUID:ABC\r\nDTSTAMP:20261101T120000Z\r\n" +
		"DTSTART;VALUE=DATE:20261105\r\nDTEND;VALUE=DATE:20261107\r\nSUMMARY:Отпуск\r\nLOCATION:Сочи\r\n" +
		"BEGIN:VALARM\r\nACTION:DISPLAY\r\nTRIGGER:-PT1H\r\nDESCRIPTION:x\r\nEND:VALARM\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
	e, uid, err := parseICS(phone, tz)
	if err != nil || uid != "ABC" || !e.AllDay {
		t.Fatalf("parse: %v %+v", err, e)
	}
	if !e.Start.Equal(time.Date(2026, 11, 5, 0, 0, 0, 0, tz)) || e.End == nil || !e.End.Equal(time.Date(2026, 11, 6, 0, 0, 0, 0, tz)) {
		t.Fatalf("all-day dates: %v %v", e.Start, e.End)
	}
	if e.RemindMinutes == nil || *e.RemindMinutes != 60 {
		t.Fatalf("alarm: %v", e.RemindMinutes)
	}
	// Edited in TMAIL: the location the webmail does not show survives.
	e.Title = "Отпуск!"
	out := buildICS(&e, uid, tz, phone)
	if !strings.Contains(out, "LOCATION:Сочи") || !strings.Contains(out, "SUMMARY:Отпуск!") || !strings.Contains(out, "DTEND;VALUE=DATE:20261107") {
		t.Fatalf("edit lost data:\n%s", out)
	}
	// Unknown Windows time zone falls back to the user's zone.
	win := strings.Replace(phone, "DTSTART;VALUE=DATE:20261105\r\nDTEND;VALUE=DATE:20261107", "DTSTART;TZID=Russian Standard Time:20261105T090000\r\nDTEND;TZID=Russian Standard Time:20261105T100000", 1)
	e2, _, err := parseICS(win, tz)
	if err != nil || e2.AllDay || !e2.Start.Equal(time.Date(2026, 11, 5, 9, 0, 0, 0, tz)) {
		t.Fatalf("windows tz: %v %+v", err, e2)
	}
}

func TestPartyCardRoundTrip(t *testing.T) {
	p := vcParty{ID: 7, Kind: "person", Name: "Иван Петров", Position: "Директор", Phones: []string{"+79991234567"}, Emails: []string{"ivan@firma.ru"},
		Company: "ООО «Ромашка»; филиал", Notes: "a,b\nc", Addresses: []string{"Москва, ул. Ленина, 1"}}
	data := encodeCard(partyCard(p))
	c, err := vcard.NewDecoder(strings.NewReader(data)).Decode()
	if err != nil {
		t.Fatal(err)
	}
	got := cardFields(c)
	if got.Name != p.Name || got.Kind != "person" || got.Position != p.Position || got.Company != "ООО «Ромашка», филиал" || got.Notes != p.Notes ||
		len(got.Phones) != 1 || len(got.Emails) != 1 || got.Emails[0] != "ivan@firma.ru" || len(got.Addresses) != 1 {
		t.Fatalf("card differs: %+v\n%s", got, data)
	}
	co := encodeCard(partyCard(vcParty{ID: 8, Kind: "company", Name: "ООО «Ромашка»"}))
	c2, _ := vcard.NewDecoder(strings.NewReader(co)).Decode()
	if g := cardFields(c2); g.Kind != "company" || g.Name != "ООО «Ромашка»" {
		t.Fatalf("company: %+v\n%s", g, co)
	}
}

func TestAppSecret(t *testing.T) {
	s := newAppSecret()
	if len(s) != 19 || strings.Count(s, "-") != 3 || strings.ContainsAny(s, "l0O1") {
		t.Fatalf("bad secret %q", s)
	}
}

func TestConfigBundleSeal(t *testing.T) {
	sealed, err := sealBundle([]byte("hello"), "correct horse battery")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := openBundle(sealed, "wrong passphrase!"); err == nil {
		t.Fatal("opened with a wrong passphrase")
	}
	if p, err := openBundle(sealed, "correct horse battery"); err != nil || string(p) != "hello" {
		t.Fatalf("open: %v %q", err, p)
	}
}
