package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/smtp"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Sending through an external SMTP server ("relay", "smarthost"): for a provider that blocks port
// 25 or an IP address with a poor reputation. The settings go to a folder shared with the Postfix
// container, which applies them and reports back (services/postfix/entrypoint.sh).

func postfixDynDir() string { return env("POSTFIX_DYN_DIR", "/postfix-dyn") }

func (s *Server) writeRelayConfig(rl relayConf) error {
	dir := postfixDynDir()
	if st, err := os.Stat(dir); err != nil || !st.IsDir() {
		return fmt.Errorf("the folder shared with Postfix (%s) is missing: update compose.yml", dir)
	}
	conf, secret := "", ""
	if rl.Enabled {
		wrapper := "no"
		if rl.Security == "ssl" {
			wrapper = "yes"
		}
		conf = fmt.Sprintf("host=%s\nport=%d\nwrapper=%s\n", rl.Host, rl.Port, wrapper)
		if rl.Username != "" && len(rl.PassEnc) > 0 {
			pass, err := s.unseal(rl.PassEnc)
			if err != nil {
				return err
			}
			if strings.ContainsAny(rl.Username+pass, "\n\r") {
				return fmt.Errorf("the login or password contains a line break")
			}
			secret = fmt.Sprintf("[%s]:%d %s:%s\n", rl.Host, rl.Port, rl.Username, pass)
		}
	}
	write := func(name, data string, mode os.FileMode) error {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p+".tmp", []byte(data), mode); err != nil {
			return err
		}
		return os.Rename(p+".tmp", p)
	}
	// The secret first: Postfix reloads when relay.conf changes.
	if err := write("relay.secret", secret, 0o600); err != nil {
		return err
	}
	return write("relay.conf", conf, 0o644)
}

// relayStatus: what Postfix reported after applying the settings ("" = nothing yet).
func relayStatus() string {
	b, _ := os.ReadFile(filepath.Join(postfixDynDir(), "relay.applied"))
	return strings.TrimSpace(string(b))
}

// syncRelayConfig writes the stored relay settings at start (the shared folder may be new).
func (s *Server) syncRelayConfig(ctx context.Context) {
	m := s.mailSettings(ctx)
	if _, err := os.Stat(filepath.Join(postfixDynDir(), "relay.conf")); err == nil && !m.Relay.Enabled {
		return
	}
	if err := s.writeRelayConfig(m.Relay); err != nil {
		slog.Warn("relay config", "err", err)
	}
}

// POST /api/admin/mail-settings/relay-test {host, port, security, username, password}:
// connects and signs in, without sending anything.
func (s *Server) adminRelayTest(w http.ResponseWriter, r *http.Request, sess *Session) {
	var in struct {
		Host     string  `json:"host"`
		Port     int     `json:"port"`
		Security string  `json:"security"`
		Username string  `json:"username"`
		Password *string `json:"password"`
	}
	if err := readJSON(r, &in); err != nil || !domainRe.MatchString(strings.ToLower(strings.TrimSpace(in.Host))) || in.Port < 1 || in.Port > 65535 {
		writeErr(w, http.StatusBadRequest, "Specify the SMTP server name and port")
		return
	}
	pass := ""
	if in.Password != nil {
		pass = *in.Password
	} else if m := s.mailSettings(r.Context()); len(m.Relay.PassEnc) > 0 {
		pass, _ = s.unseal(m.Relay.PassEnc)
	}
	host := strings.ToLower(strings.TrimSpace(in.Host))
	if err := testSMTP(host, in.Port, in.Security == "ssl", strings.TrimSpace(in.Username), pass); err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func testSMTP(host string, port int, wrapper bool, user, pass string) error {
	addr := net.JoinHostPort(host, strconv.Itoa(port))
	d := &net.Dialer{Timeout: 15 * time.Second}
	var conn net.Conn
	var err error
	tc := &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12}
	if wrapper {
		conn, err = tls.DialWithDialer(d, "tcp", addr, tc)
	} else {
		conn, err = d.Dial("tcp", addr)
	}
	if err != nil {
		return fmt.Errorf("Could not connect to %s: %v", addr, err)
	}
	_ = conn.SetDeadline(time.Now().Add(30 * time.Second))
	c, err := smtp.NewClient(conn, host)
	if err != nil {
		conn.Close()
		return fmt.Errorf("The server did not answer as an SMTP server: %v", err)
	}
	defer c.Close()
	if !wrapper {
		if ok, _ := c.Extension("STARTTLS"); !ok {
			return fmt.Errorf("The server does not offer STARTTLS encryption on this port")
		}
		if err := c.StartTLS(tc); err != nil {
			return fmt.Errorf("Encryption (STARTTLS) failed: %v", err)
		}
	}
	if user != "" {
		if err := c.Auth(smtp.PlainAuth("", user, pass, host)); err != nil {
			return fmt.Errorf("The server did not accept the login or password: %v", err)
		}
	}
	_ = c.Quit()
	return nil
}
