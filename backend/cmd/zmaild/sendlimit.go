package main

import (
	"bufio"
	"context"
	"fmt"
	"log/slog"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Sending limits per mailbox: Postfix asks this policy service (127.0.0.1:10040) at the end of
// every letter sent with a password (mail programs, phones and the webmail alike). Over the limit
// the letter is deferred with an explanation, and administrators see it in the audit log.
// Postfix goes on without asking when the service is down (smtpd_policy_service_default_action).

func (s *Server) runSendPolicy(ctx context.Context) {
	addr := env("POLICY_ADDR", "127.0.0.1:10040")
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		slog.Error("send policy service", "err", err)
		return
	}
	go func() { <-ctx.Done(); ln.Close() }()
	go s.cleanSendLog(ctx)
	for {
		c, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			time.Sleep(time.Second)
			continue
		}
		go s.servePolicy(ctx, c)
	}
}

// servePolicy speaks the Postfix policy protocol: name=value lines, an empty line ends a
// request; one connection carries many requests.
func (s *Server) servePolicy(ctx context.Context, c net.Conn) {
	defer c.Close()
	rd := bufio.NewReader(c)
	req := map[string]string{}
	for {
		_ = c.SetReadDeadline(time.Now().Add(10 * time.Minute))
		line, err := rd.ReadString('\n')
		if err != nil {
			return
		}
		line = strings.TrimRight(line, "\r\n")
		if line != "" {
			if k, v, ok := strings.Cut(line, "="); ok {
				req[k] = v
			}
			continue
		}
		action := s.policyDecision(ctx, req)
		if _, err := fmt.Fprintf(c, "action=%s\n\n", action); err != nil {
			return
		}
		req = map[string]string{}
	}
}

var limitWarned sync.Map // mailbox → time of the last audit entry

func (s *Server) policyDecision(ctx context.Context, req map[string]string) string {
	login := strings.ToLower(req["sasl_username"])
	if login == "" || req["protocol_state"] != "END-OF-MESSAGE" {
		return "DUNNO"
	}
	mailbox, _, _ := strings.Cut(login, "*") // the webmail signs in as "mailbox*master"
	rcpts, _ := strconv.Atoi(req["recipient_count"])
	if rcpts < 1 {
		rcpts = 1
	}
	m := s.mailSettings(ctx)
	hourLimit, dayLimit := m.LimitHour, m.LimitDay
	var own, ownDay *int
	if s.db.QueryRow(ctx, `SELECT limit_hour, limit_day FROM mailboxes WHERE address=$1`, mailbox).Scan(&own, &ownDay) == nil {
		if own != nil {
			hourLimit = *own
		}
		if ownDay != nil {
			dayLimit = *ownDay
		}
	}
	var hour, day int
	_ = s.db.QueryRow(ctx, `SELECT count(*) FILTER (WHERE at > now() - interval '1 hour'), COALESCE(sum(recipients), 0)
		FROM send_log WHERE mailbox=$1 AND NOT blocked AND at > now() - interval '24 hours'`, mailbox).Scan(&hour, &day)
	reason := ""
	switch {
	case hourLimit > 0 && hour+1 > hourLimit:
		reason = fmt.Sprintf("Sending limit reached: %d letters per hour from %s. Try again later or ask your administrator", hourLimit, mailbox)
	case dayLimit > 0 && day+rcpts > dayLimit:
		reason = fmt.Sprintf("Sending limit reached: %d recipients per day from %s. Try again tomorrow or ask your administrator", dayLimit, mailbox)
	}
	_, _ = s.db.Exec(ctx, `INSERT INTO send_log(mailbox, recipients, blocked) VALUES ($1,$2,$3)`, mailbox, rcpts, reason != "")
	if reason == "" {
		return "DUNNO"
	}
	if t, ok := limitWarned.Load(mailbox); !ok || time.Since(t.(time.Time)) > time.Hour {
		limitWarned.Store(mailbox, time.Now())
		slog.Warn("sending limit", "mailbox", mailbox, "hour", hour, "day", day, "ip", req["client_address"])
		s.audit(ctx, 0, "postfix", req["client_address"], "mail.limit_reached", mailbox, map[string]any{"lastHour": hour, "lastDay": day,
			"limitHour": hourLimit, "limitDay": dayLimit})
	}
	return "450 4.7.1 " + reason
}

func (s *Server) cleanSendLog(ctx context.Context) {
	for {
		_, _ = s.db.Exec(ctx, `DELETE FROM send_log WHERE at < now() - interval '2 days'`)
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Hour):
		}
	}
}
