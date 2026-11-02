package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Server-wide mail settings chosen by an administrator: the MTA-STS policy published for our
// domains, sending through an external SMTP server (relay) and per-mailbox sending limits.
// Kept in app_state; the Postfix part is written to the folder Postfix watches (relay.go).

type relayConf struct {
	Enabled  bool   `json:"enabled"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Security string `json:"security"` // ssl | starttls
	Username string `json:"username"`
	PassEnc  []byte `json:"passEnc,omitempty"`
}

type mailSettings struct {
	MTASTSMode string    `json:"mtaStsMode"` // testing | enforce | none
	Relay      relayConf `json:"relay"`
	LimitHour  int       `json:"limitHour"` // letters per hour from one mailbox, 0 = no limit
	LimitDay   int       `json:"limitDay"`  // recipients per day from one mailbox, 0 = no limit
}

func defaultMailSettings() mailSettings {
	return mailSettings{MTASTSMode: "testing", LimitHour: 200, LimitDay: 2000, Relay: relayConf{Port: 587, Security: "starttls"}}
}

var mailSettingsCache struct {
	sync.Mutex
	v  *mailSettings
	at time.Time
}

func (s *Server) mailSettings(ctx context.Context) mailSettings {
	mailSettingsCache.Lock()
	defer mailSettingsCache.Unlock()
	if mailSettingsCache.v != nil && time.Since(mailSettingsCache.at) < 30*time.Second {
		return *mailSettingsCache.v
	}
	m := defaultMailSettings()
	var raw []byte
	if s.db.QueryRow(ctx, `SELECT value FROM app_state WHERE key='mail_settings'`).Scan(&raw) == nil {
		_ = json.Unmarshal(raw, &m)
	}
	mailSettingsCache.v, mailSettingsCache.at = &m, time.Now()
	return m
}

func (s *Server) saveMailSettings(ctx context.Context, m mailSettings) error {
	raw, _ := json.Marshal(m)
	_, err := s.db.Exec(ctx, `INSERT INTO app_state(key, value) VALUES ('mail_settings', $1::text::jsonb)
		ON CONFLICT (key) DO UPDATE SET value=EXCLUDED.value, updated_at=now()`, string(raw))
	mailSettingsCache.Lock()
	mailSettingsCache.v = nil
	mailSettingsCache.Unlock()
	return err
}

// GET /.well-known/mta-sts.txt on mta-sts.<domain>: tells other servers to deliver to us only
// over verified TLS (RFC 8461).
func (s *Server) handleMTASTSPolicy(w http.ResponseWriter, r *http.Request) {
	host := strings.ToLower(strings.Split(r.Host, ":")[0])
	domain, ok := strings.CutPrefix(host, "mta-sts.")
	if !ok || !s.ourDomain(r.Context(), domain) {
		http.NotFound(w, r)
		return
	}
	mode := s.mailSettings(r.Context()).MTASTSMode
	age := 604800
	if mode != "enforce" {
		age = 86400
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "max-age=3600")
	_, _ = fmt.Fprintf(w, "version: STSv1\r\nmode: %s\r\nmx: %s\r\nmax_age: %d\r\n", mode, s.cfg.Hostname, age)
}

// mtaSTSID: the "id" of the _mta-sts TXT record; it must change when the policy changes.
func (s *Server) mtaSTSID(ctx context.Context) string {
	var at time.Time
	_ = s.db.QueryRow(ctx, `SELECT updated_at FROM app_state WHERE key='mail_settings'`).Scan(&at)
	if at.IsZero() {
		at = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	}
	return at.UTC().Format("20060102T150405")
}

// ---------- admin API ----------

type mailSettingsView struct {
	MTASTSMode string `json:"mtaStsMode"`
	MTASTSID   string `json:"mtaStsId"`
	Relay      struct {
		Enabled  bool   `json:"enabled"`
		Host     string `json:"host"`
		Port     int    `json:"port"`
		Security string `json:"security"`
		Username string `json:"username"`
		HasPass  bool   `json:"hasPassword"`
		Status   string `json:"status"` // what Postfix last applied
	} `json:"relay"`
	LimitHour int `json:"limitHour"`
	LimitDay  int `json:"limitDay"`
}

func (s *Server) mailSettingsView(ctx context.Context) mailSettingsView {
	m := s.mailSettings(ctx)
	var v mailSettingsView
	v.MTASTSMode, v.MTASTSID, v.LimitHour, v.LimitDay = m.MTASTSMode, s.mtaSTSID(ctx), m.LimitHour, m.LimitDay
	v.Relay.Enabled, v.Relay.Host, v.Relay.Port, v.Relay.Security, v.Relay.Username = m.Relay.Enabled, m.Relay.Host, m.Relay.Port, m.Relay.Security, m.Relay.Username
	v.Relay.HasPass = len(m.Relay.PassEnc) > 0
	v.Relay.Status = relayStatus()
	return v
}

func (s *Server) adminMailSettings(w http.ResponseWriter, r *http.Request, sess *Session) {
	writeJSON(w, http.StatusOK, s.mailSettingsView(r.Context()))
}

func (s *Server) adminMailSettingsSave(w http.ResponseWriter, r *http.Request, sess *Session) {
	var in struct {
		MTASTSMode *string `json:"mtaStsMode"`
		LimitHour  *int    `json:"limitHour"`
		LimitDay   *int    `json:"limitDay"`
		Relay      *struct {
			Enabled  bool    `json:"enabled"`
			Host     string  `json:"host"`
			Port     int     `json:"port"`
			Security string  `json:"security"`
			Username string  `json:"username"`
			Password *string `json:"password"` // nil = keep
		} `json:"relay"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 64<<10)).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "Invalid request")
		return
	}
	ctx := r.Context()
	m := s.mailSettings(ctx)
	if in.MTASTSMode != nil {
		switch *in.MTASTSMode {
		case "testing", "enforce", "none":
			m.MTASTSMode = *in.MTASTSMode
		default:
			writeErr(w, http.StatusBadRequest, "Unknown MTA-STS mode")
			return
		}
	}
	if in.LimitHour != nil {
		if *in.LimitHour < 0 || *in.LimitHour > 100000 {
			writeErr(w, http.StatusBadRequest, "Invalid limit")
			return
		}
		m.LimitHour = *in.LimitHour
	}
	if in.LimitDay != nil {
		if *in.LimitDay < 0 || *in.LimitDay > 1000000 {
			writeErr(w, http.StatusBadRequest, "Invalid limit")
			return
		}
		m.LimitDay = *in.LimitDay
	}
	relayChanged := false
	if rl := in.Relay; rl != nil {
		host := strings.ToLower(strings.TrimSpace(rl.Host))
		if rl.Enabled && (!domainRe.MatchString(host) || rl.Port < 1 || rl.Port > 65535) {
			writeErr(w, http.StatusBadRequest, "Specify the SMTP server name and port")
			return
		}
		if rl.Security != "ssl" && rl.Security != "starttls" {
			rl.Security = "starttls"
		}
		m.Relay.Enabled, m.Relay.Host, m.Relay.Port, m.Relay.Security = rl.Enabled, host, rl.Port, rl.Security
		m.Relay.Username = strings.TrimSpace(rl.Username)
		if rl.Password != nil {
			if *rl.Password == "" {
				m.Relay.PassEnc = nil
			} else {
				enc, err := s.seal(*rl.Password)
				if err != nil {
					serverErr(w, "relay seal", err)
					return
				}
				m.Relay.PassEnc = enc
			}
		}
		if m.Relay.Enabled && (m.Relay.Username == "") != (len(m.Relay.PassEnc) == 0) {
			writeErr(w, http.StatusBadRequest, "Enter both the login and the password of the SMTP server, or neither")
			return
		}
		relayChanged = true
	}
	if err := s.saveMailSettings(ctx, m); err != nil {
		serverErr(w, "mail settings save", err)
		return
	}
	if relayChanged {
		if err := s.writeRelayConfig(m.Relay); err != nil {
			serverErr(w, "relay config", err)
			return
		}
	}
	s.auditAdmin(r, sess, "mail.settings", "", map[string]any{"mtaSts": m.MTASTSMode, "relay": m.Relay.Enabled, "relayHost": m.Relay.Host,
		"limitHour": m.LimitHour, "limitDay": m.LimitDay})
	writeJSON(w, http.StatusOK, s.mailSettingsView(ctx))
}
