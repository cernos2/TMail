package main

import (
	"encoding/json"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Black / white lists of senders. Rspamd reads them as maps from TMAIL (loopback only):
// blocked senders go to Junk on arrival, trusted ones never do.

type senderRule struct {
	ID        int64     `json:"id"`
	Kind      string    `json:"kind"`
	Pattern   string    `json:"pattern"`
	Name      string    `json:"name"`
	Note      string    `json:"note"`
	CreatedBy string    `json:"createdBy"`
	CreatedAt time.Time `json:"createdAt"`
}

// normPattern accepts "Name <a@b.ru>", "a@b.ru", "@b.ru" or "b.ru".
func normPattern(s string) (pattern, name string, ok bool) {
	s = strings.TrimSpace(s)
	if i := strings.Index(s, "<"); i >= 0 && strings.HasSuffix(s, ">") {
		name = strings.Trim(strings.TrimSpace(s[:i]), `"`)
		s = s[i+1 : len(s)-1]
	}
	s = strings.ToLower(strings.TrimSpace(s))
	switch {
	case emailRe.MatchString(s):
		return s, name, true
	case strings.HasPrefix(s, "@") && strings.Contains(s[1:], ".") && !strings.ContainsAny(s[1:], "@ "):
		return s, name, true
	case strings.Contains(s, ".") && !strings.ContainsAny(s, "@ /"):
		return "@" + s, name, true
	}
	return "", "", false
}

func (s *Server) handleSenderRules(w http.ResponseWriter, r *http.Request, sess *Session) {
	rows, err := s.db.Query(r.Context(), `SELECT id, kind, pattern, name, note, created_by, created_at FROM sender_rules ORDER BY created_at DESC`)
	if err != nil {
		serverErr(w, "sender rules", err)
		return
	}
	defer rows.Close()
	out := []senderRule{}
	for rows.Next() {
		var x senderRule
		if rows.Scan(&x.ID, &x.Kind, &x.Pattern, &x.Name, &x.Note, &x.CreatedBy, &x.CreatedAt) == nil {
			out = append(out, x)
		}
	}
	writeJSON(w, http.StatusOK, out)
}

// POST /api/sender-rules {kind, entries: ["a@b.ru", "@c.ru", …], note} — add (or move between lists).
func (s *Server) handleSenderRuleAdd(w http.ResponseWriter, r *http.Request, sess *Session) {
	var in struct {
		Kind    string   `json:"kind"`
		Entries []string `json:"entries"`
		Note    string   `json:"note"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 256<<10)).Decode(&in); err != nil || (in.Kind != "block" && in.Kind != "allow") {
		writeErr(w, http.StatusBadRequest, "Invalid data")
		return
	}
	added, bad := 0, []string{}
	for _, raw := range in.Entries {
		for _, part := range strings.FieldsFunc(raw, func(r rune) bool { return r == '\n' || r == ',' || r == ';' }) {
			if strings.TrimSpace(part) == "" {
				continue
			}
			p, name, ok := normPattern(part)
			// The lists are company-wide: a regular user may list one exact address from a letter,
			// but not whole domains, and may not overturn a rule that already exists.
			if !ok || (!sess.IsAdmin && strings.HasPrefix(p, "@")) {
				bad = append(bad, strings.TrimSpace(part))
				continue
			}
			q := `INSERT INTO sender_rules(kind, pattern, name, note, created_by) VALUES ($1,$2,$3,$4,$5)
				ON CONFLICT (pattern) DO UPDATE SET kind=EXCLUDED.kind, name=COALESCE(NULLIF(EXCLUDED.name,''), sender_rules.name), note=EXCLUDED.note, created_by=EXCLUDED.created_by, created_at=now()`
			if !sess.IsAdmin {
				q = `INSERT INTO sender_rules(kind, pattern, name, note, created_by) VALUES ($1,$2,$3,$4,$5) ON CONFLICT (pattern) DO NOTHING`
			}
			if tag, err := s.db.Exec(r.Context(), q, in.Kind, p, name, strings.TrimSpace(in.Note), sess.Login); err == nil && tag.RowsAffected() > 0 {
				added++
			}
		}
	}
	if added > 0 {
		s.audit(r.Context(), sess.UserID, sess.Login, clientIP(r), "senders."+in.Kind, strconv.Itoa(added), nil)
	}
	writeJSON(w, http.StatusOK, map[string]any{"added": added, "invalid": bad})
}

func (s *Server) handleSenderRuleDelete(w http.ResponseWriter, r *http.Request, sess *Session) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	var p string
	_ = s.db.QueryRow(r.Context(), `DELETE FROM sender_rules WHERE id=$1 RETURNING pattern`, id).Scan(&p)
	s.audit(r.Context(), sess.UserID, sess.Login, clientIP(r), "senders.remove", p, nil)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// GET /internal/rspamd/{kind}-{addr|domain}.map — plain list for Rspamd, loopback only.
func (s *Server) serveRspamdMap(w http.ResponseWriter, r *http.Request) {
	host, _, _ := net.SplitHostPort(r.RemoteAddr)
	if ip := net.ParseIP(host); ip == nil || !ip.IsLoopback() {
		http.NotFound(w, r)
		return
	}
	name := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/internal/rspamd/"), ".map")
	kind, part, ok := strings.Cut(name, "-")
	if !ok || (kind != "block" && kind != "allow") || (part != "addr" && part != "domain") {
		http.NotFound(w, r)
		return
	}
	cond := `pattern NOT LIKE '@%'`
	if part == "domain" {
		cond = `pattern LIKE '@%'`
	}
	rows, err := s.db.Query(r.Context(), `SELECT pattern FROM sender_rules WHERE kind=$1 AND `+cond+` ORDER BY pattern`, kind)
	if err != nil {
		http.Error(w, "db", http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	var b strings.Builder
	b.WriteString("# TMAIL " + name + "\n")
	for rows.Next() {
		var p string
		if rows.Scan(&p) == nil {
			b.WriteString(strings.TrimPrefix(p, "@") + "\n")
		}
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write([]byte(b.String()))
}
