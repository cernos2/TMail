package main

import (
	"context"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// Interface language and time zone. The server has defaults chosen at installation (the country
// the mail service works in); each user may have their own, set by an administrator or by the user.

var reLang = regexp.MustCompile(`^[a-z]{2,3}(-[A-Za-z0-9]{2,4})?$`)

type localeDefaults struct {
	Lang     string `json:"lang"`
	Timezone string `json:"timezone"`
	Country  string `json:"country"`
	Org      string `json:"org"` // the organisation running this mail service (ORG_NAME)
}

func serverLocale() localeDefaults {
	return localeDefaults{
		Lang:     env("DEFAULT_LANG", "en"),
		Timezone: env("DEFAULT_TIMEZONE", "UTC"),
		Country:  strings.ToUpper(env("COUNTRY", "")),
		Org:      env("ORG_NAME", "TMAIL"),
	}
}

// checkLocale validates a language code and a time zone name ("" keeps the server default).
func checkLocale(lang, tz string) string {
	if lang != "" && !reLang.MatchString(lang) {
		return "Unknown language"
	}
	if tz != "" {
		if _, err := time.LoadLocation(tz); err != nil {
			return "Unknown time zone"
		}
	}
	return ""
}

func (s *Server) userLocale(ctx context.Context, user int64) (lang, tz string) {
	_ = s.db.QueryRow(ctx, `SELECT lang, timezone FROM users WHERE id=$1`, user).Scan(&lang, &tz)
	return
}

// PUT /api/me/locale {"lang", "timezone"}: the user's own choice.
func (s *Server) handleSetLocale(w http.ResponseWriter, r *http.Request, sess *Session) {
	var in struct {
		Lang     string `json:"lang"`
		Timezone string `json:"timezone"`
	}
	if err := readJSON(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "Invalid request")
		return
	}
	if msg := checkLocale(in.Lang, in.Timezone); msg != "" {
		writeErr(w, http.StatusBadRequest, msg)
		return
	}
	if _, err := s.db.Exec(r.Context(), `UPDATE users SET lang=$2, timezone=$3, updated_at=now() WHERE id=$1`, sess.UserID, in.Lang, in.Timezone); err != nil {
		serverErr(w, "set locale", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// GET /api/locale: the server's language, time zone and country, for the sign-in page (no session).
func (s *Server) handleLocaleDefaults(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, serverLocale())
}
