package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5"
)

// ---------- quota usage via the doveadm HTTP API ----------

type usage struct{ bytes, messages int64 }

func (s *Server) quotaUsage(ctx context.Context, addrs []string) map[string]usage {
	out := map[string]usage{}
	if len(addrs) == 0 || s.cfg.DoveadmKey == "" {
		return out
	}
	cmds := make([]any, len(addrs))
	for i, a := range addrs {
		cmds[i] = []any{"quotaGet", map[string]string{"user": a}, strconv.Itoa(i)}
	}
	body, _ := json.Marshal(cmds)
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, s.cfg.DoveadmURL, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "X-Dovecot-API "+base64.StdEncoding.EncodeToString([]byte(s.cfg.DoveadmKey)))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return out
	}
	defer resp.Body.Close()
	var res [][]json.RawMessage
	if json.NewDecoder(resp.Body).Decode(&res) != nil {
		return out
	}
	for _, item := range res {
		if len(item) < 3 {
			continue
		}
		var kind, tag string
		_ = json.Unmarshal(item[0], &kind)
		_ = json.Unmarshal(item[2], &tag)
		i, err := strconv.Atoi(tag)
		if kind != "doveadmResponse" || err != nil || i >= len(addrs) {
			continue
		}
		var rows []map[string]string
		_ = json.Unmarshal(item[1], &rows)
		var u usage
		for _, r := range rows {
			v, _ := strconv.ParseInt(r["value"], 10, 64)
			switch r["type"] {
			case "STORAGE":
				u.bytes = v * 1024 // doveadm reports kilobytes
			case "MESSAGE":
				u.messages = v
			}
		}
		out[addrs[i]] = u
	}
	return out
}

// ---------- DNS checks ----------

type dnsCheck struct {
	Key      string `json:"key"`
	Title    string `json:"title"`
	Status   string `json:"status"` // ok | missing | wrong
	Type     string `json:"type"`
	Name     string `json:"name"`
	Expected string `json:"expected"`
	Found    string `json:"found"`
	Hint     string `json:"hint"`
}

// resolver asks our own validating Unbound, not the provider's resolver.
func (s *Server) resolver() *net.Resolver {
	return &net.Resolver{PreferGo: true, Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
		var d net.Dialer
		return d.DialContext(ctx, network, s.cfg.DNSAddr)
	}}
}

func (s *Server) checkDomainDNS(ctx context.Context, domain string) []dnsCheck {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	res := s.resolver()
	host := s.cfg.Hostname
	var selector, pub string
	_ = s.db.QueryRow(ctx, `SELECT k.selector, k.public_key FROM dkim_keys k JOIN domains d ON d.id=k.domain_id
		WHERE d.name=$1 AND k.active ORDER BY k.created_at DESC LIMIT 1`, domain).Scan(&selector, &pub)

	var out []dnsCheck
	add := func(c dnsCheck) { out = append(out, c) }

	// MX
	mx := dnsCheck{Key: "mx", Title: "MX: where mail is delivered", Type: "MX", Name: "@", Expected: "10 " + host + "."}
	if recs, err := res.LookupMX(ctx, domain); err == nil && len(recs) > 0 {
		var found []string
		ok := false
		for _, r := range recs {
			found = append(found, fmt.Sprintf("%d %s", r.Pref, r.Host))
			if strings.EqualFold(strings.TrimSuffix(r.Host, "."), host) {
				ok = true
			}
		}
		mx.Found = strings.Join(found, ", ")
		mx.Status = map[bool]string{true: "ok", false: "wrong"}[ok]
		if !ok {
			mx.Hint = "MX points to another server: mail for this domain will not arrive here."
		}
	} else {
		mx.Status, mx.Hint = "missing", "Without an MX record, mail for this domain is not delivered."
	}
	add(mx)

	// SPF
	spf := dnsCheck{Key: "spf", Title: "SPF: who may send mail", Type: "TXT", Name: "@", Expected: "v=spf1 mx ~all"}
	if txts, err := res.LookupTXT(ctx, domain); err == nil {
		for _, t := range txts {
			if strings.HasPrefix(strings.ToLower(t), "v=spf1") {
				spf.Found = t
			}
		}
	}
	switch {
	case spf.Found == "":
		spf.Status, spf.Hint = "missing", "Without SPF, messages end up in spam more often."
	case strings.Contains(spf.Found, " mx") || strings.Contains(spf.Found, "a:"+host) || (s.cfg.PublicIP != "" && strings.Contains(spf.Found, "ip4:"+s.cfg.PublicIP)):
		spf.Status = "ok"
	default:
		spf.Status, spf.Hint = "wrong", "SPF does not allow sending from this server. Add \"mx\" to the record."
	}
	add(spf)

	// DKIM
	dk := dnsCheck{Key: "dkim", Title: "DKIM: message signature", Type: "TXT", Name: selector + "._domainkey", Expected: "v=DKIM1; k=rsa; p=" + pub}
	if selector == "" {
		dk.Status, dk.Hint, dk.Name, dk.Expected = "missing", "The domain has no DKIM key.", "—", "—"
	} else if txts, err := res.LookupTXT(ctx, selector+"._domainkey."+domain); err == nil && len(txts) > 0 {
		dk.Found = strings.Join(txts, "")
		norm := strings.ReplaceAll(strings.ReplaceAll(dk.Found, " ", ""), "\"", "")
		if strings.Contains(norm, "p="+pub) {
			dk.Status = "ok"
		} else {
			dk.Status, dk.Hint = "wrong", "The key in DNS does not match the server's key: signatures will fail verification."
		}
	} else {
		dk.Status, dk.Hint = "missing", "Without DKIM, recipients cannot verify that a message really comes from you."
	}
	add(dk)

	// DMARC
	dm := dnsCheck{Key: "dmarc", Title: "DMARC: policy for forged mail", Type: "TXT", Name: "_dmarc",
		Expected: "v=DMARC1; p=none; rua=mailto:postmaster@" + domain + "; adkim=s; aspf=r"}
	if txts, err := res.LookupTXT(ctx, "_dmarc."+domain); err == nil {
		for _, t := range txts {
			if strings.HasPrefix(strings.ToLower(t), "v=dmarc1") {
				dm.Found = t
			}
		}
	}
	if dm.Found == "" {
		dm.Status, dm.Hint = "missing", "Gmail and other large providers trust domains without DMARC less."
	} else {
		dm.Status = "ok"
	}
	add(dm)

	// A of the mail host and PTR of the server IP
	a := dnsCheck{Key: "a", Title: "A: mail server address", Type: "A", Name: host, Expected: s.cfg.PublicIP}
	if ips, err := res.LookupHost(ctx, host); err == nil {
		a.Found = strings.Join(ips, ", ")
		a.Status = "ok"
		if s.cfg.PublicIP != "" && !contains(ips, s.cfg.PublicIP) {
			a.Status, a.Hint = "wrong", "The mail server name points to another IP."
		}
	} else {
		a.Status, a.Hint = "missing", "The mail server name does not resolve."
	}
	add(a)

	if s.cfg.PublicIP != "" {
		ptr := dnsCheck{Key: "ptr", Title: "PTR: reverse DNS of the IP", Type: "PTR", Name: s.cfg.PublicIP, Expected: host + ".",
			Hint: ""}
		if names, err := res.LookupAddr(ctx, s.cfg.PublicIP); err == nil && len(names) > 0 {
			ptr.Found = strings.Join(names, ", ")
			ptr.Status = "wrong"
			for _, n := range names {
				if strings.EqualFold(strings.TrimSuffix(n, "."), host) {
					ptr.Status = "ok"
				}
			}
			if ptr.Status != "ok" {
				ptr.Hint = "PTR is changed at the server's hosting provider (not the domain registrar). A mismatch with the server name hurts deliverability."
			}
		} else {
			ptr.Status, ptr.Hint = "missing", "Ask the server's hosting provider to set up PTR."
		}
		add(ptr)
	}
	return out
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

func (s *Server) adminDomainDNS(w http.ResponseWriter, r *http.Request, sess *Session) {
	id, ok := pathID(r)
	var name string
	if !ok || s.db.QueryRow(r.Context(), `SELECT name FROM domains WHERE id=$1`, id).Scan(&name) != nil {
		writeErr(w, http.StatusNotFound, "Domain not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"domain": name, "checks": s.checkDomainDNS(r.Context(), name), "checkedAt": time.Now()})
}

// ---------- server status ----------

type svcCheck struct {
	Key    string `json:"key"`
	Title  string `json:"title"`
	OK     bool   `json:"ok"`
	Detail string `json:"detail"`
}

func (s *Server) serviceChecks(ctx context.Context) []svcCheck {
	type probe struct {
		key, title string
		fn         func() (string, error)
	}
	banner := func(addr string, useTLS bool) func() (string, error) {
		return func() (string, error) {
			d := net.Dialer{Timeout: 4 * time.Second}
			var c net.Conn
			var err error
			if useTLS {
				c, err = tls.DialWithDialer(&d, "tcp", addr, &tls.Config{ServerName: s.cfg.Hostname})
			} else {
				c, err = d.Dial("tcp", addr)
			}
			if err != nil {
				return "", err
			}
			defer c.Close()
			_ = c.SetDeadline(time.Now().Add(4 * time.Second))
			line, err := bufio.NewReader(c).ReadString('\n')
			return strings.TrimSpace(line), err
		}
	}
	probes := []probe{
		{"smtp", "Incoming mail (SMTP, port 25)", banner("127.0.0.1:25", false)},
		{"submission", "Sending (SMTP, port 465)", banner("127.0.0.1:465", true)},
		{"imap", "IMAP (port 993)", banner("127.0.0.1:993", true)},
		{"db", "Database", func() (string, error) { return "responding", s.db.Ping(ctx) }},
		{"rspamd", "Rspamd antispam", func() (string, error) {
			st, err := s.rspamdStat(ctx)
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("messages scanned: %d", st.Scanned), nil
		}},
		{"clamav", "ClamAV antivirus", func() (string, error) {
			c, cancel := context.WithTimeout(ctx, 4*time.Second)
			defer cancel()
			v, err := clamVersion(c)
			if parts := strings.SplitN(v, "/", 3); err == nil && len(parts) == 3 {
				return fmt.Sprintf("databases %s of %s", parts[1], strings.TrimSpace(parts[2])), nil
			}
			return v, err
		}},
		{"dns", "DNS resolver", func() (string, error) {
			c, cancel := context.WithTimeout(ctx, 4*time.Second)
			defer cancel()
			_, err := s.resolver().LookupHost(c, "gmail.com")
			return "responding", err
		}},
	}
	out := make([]svcCheck, len(probes))
	var wg sync.WaitGroup
	for i, p := range probes {
		wg.Add(1)
		go func(i int, p probe) {
			defer wg.Done()
			detail, err := p.fn()
			out[i] = svcCheck{Key: p.key, Title: p.title, OK: err == nil, Detail: detail}
			if err != nil {
				out[i].Detail = "not responding"
			}
		}(i, p)
	}
	wg.Wait()
	return out
}

type rspamdStat struct {
	Scanned int64            `json:"scanned"`
	Learned int64            `json:"learned"`
	Actions map[string]int64 `json:"actions"`
}

func (s *Server) rspamdStat(ctx context.Context) (rspamdStat, error) {
	var st rspamdStat
	ctx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, s.cfg.RspamdURL+"/stat", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return st, err
	}
	defer resp.Body.Close()
	err = json.NewDecoder(resp.Body).Decode(&st)
	return st, err
}

func certInfo(dir string) (notAfter time.Time, issuer string, err error) {
	b, err := os.ReadFile(filepath.Join(dir, "fullchain.pem"))
	if err != nil {
		return
	}
	block, _ := pem.Decode(b)
	if block == nil {
		return notAfter, "", fmt.Errorf("bad pem")
	}
	c, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return
	}
	return c.NotAfter, c.Issuer.CommonName + orgOf(c), nil
}

func orgOf(c *x509.Certificate) string {
	if len(c.Issuer.Organization) > 0 {
		return " (" + c.Issuer.Organization[0] + ")"
	}
	return ""
}

func meminfo() map[string]int64 {
	out := map[string]int64{}
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return out
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) >= 2 {
			v, _ := strconv.ParseInt(fields[1], 10, 64)
			out[strings.TrimSuffix(fields[0], ":")] = v * 1024
		}
	}
	return out
}

// queueSize counts messages waiting in Postfix's active and deferred queues.
func queueSize(spool string) (active, deferred int) {
	count := func(dir string) int {
		n := 0
		_ = filepath.WalkDir(filepath.Join(spool, dir), func(_ string, d fs.DirEntry, err error) error {
			if err == nil && !d.IsDir() {
				n++
			}
			return nil
		})
		return n
	}
	return count("active"), count("deferred")
}

func (s *Server) serverInfo(ctx context.Context) map[string]any {
	info := map[string]any{"hostname": s.cfg.Hostname, "publicIp": s.cfg.PublicIP, "goVersion": runtime.Version()}
	if na, issuer, err := certInfo(s.cfg.TLSDir); err == nil {
		info["certificate"] = map[string]any{"notAfter": na, "issuer": issuer, "daysLeft": int(time.Until(na).Hours() / 24)}
	}
	m := meminfo()
	info["memory"] = map[string]int64{"total": m["MemTotal"], "available": m["MemAvailable"], "swapTotal": m["SwapTotal"], "swapFree": m["SwapFree"]}
	var st syscall.Statfs_t
	if syscall.Statfs(s.cfg.WebDir, &st) == nil {
		info["disk"] = map[string]int64{"total": int64(st.Blocks) * st.Bsize, "free": int64(st.Bavail) * st.Bsize}
	}
	if b, err := os.ReadFile("/proc/loadavg"); err == nil {
		info["load"] = strings.Join(strings.Fields(string(b))[:3], " ")
	}
	info["cpu"] = cpuUsage()
	info["antivirus"] = s.avSummary(ctx)
	if b, err := os.ReadFile("/proc/uptime"); err == nil {
		if f, err := strconv.ParseFloat(strings.Fields(string(b))[0], 64); err == nil {
			info["uptimeSeconds"] = int64(f)
		}
	}
	a, d := queueSize(s.cfg.SpoolDir)
	info["queue"] = map[string]int{"active": a, "deferred": d}
	if st, err := s.rspamdStat(ctx); err == nil {
		info["spam"] = st
	}
	info["clients"] = map[string]any{
		"imap": map[string]any{"host": s.cfg.Hostname, "port": 993, "security": "SSL/TLS"},
		"pop3": map[string]any{"host": s.cfg.Hostname, "port": 995, "security": "SSL/TLS"},
		"smtp": map[string]any{"host": s.cfg.Hostname, "port": 465, "security": "SSL/TLS", "alt": "587 STARTTLS"},
	}
	return info
}

func (s *Server) adminServer(w http.ResponseWriter, r *http.Request, sess *Session) {
	info := s.serverInfo(r.Context())
	info["services"] = s.serviceChecks(r.Context())
	writeJSON(w, http.StatusOK, info)
}

// ---------- overview ----------

func (s *Server) adminOverview(w http.ResponseWriter, r *http.Request, sess *Session) {
	ctx := r.Context()
	var nUsers, nMailboxes, nAliases, nDomains, nSent, nFailed int
	_ = s.db.QueryRow(ctx, `SELECT (SELECT count(*) FROM users WHERE active), (SELECT count(*) FROM mailboxes WHERE active),
		(SELECT count(*) FROM aliases WHERE active), (SELECT count(*) FROM domains WHERE active AND NOT external),
		(SELECT count(*) FROM sent_messages WHERE sent_at > now() - interval '24 hours'),
		(SELECT count(*) FROM audit_log WHERE action='login.failed' AND at > now() - interval '24 hours')`).
		Scan(&nUsers, &nMailboxes, &nAliases, &nDomains, &nSent, &nFailed)
	counts := map[string]int{"users": nUsers, "mailboxes": nMailboxes, "aliases": nAliases, "domains": nDomains,
		"sent24h": nSent, "failedLogins24h": nFailed}

	type domainHealth struct {
		ID     int64      `json:"id"`
		Name   string     `json:"name"`
		Checks []dnsCheck `json:"checks"`
	}
	var domains []domainHealth
	rows, err := s.db.Query(ctx, `SELECT id, name FROM domains WHERE active AND NOT external ORDER BY name`)
	if err == nil {
		for rows.Next() {
			var d domainHealth
			if rows.Scan(&d.ID, &d.Name) == nil {
				domains = append(domains, d)
			}
		}
		rows.Close()
	}
	var wg sync.WaitGroup
	for i := range domains {
		wg.Add(1)
		go func(i int) { defer wg.Done(); domains[i].Checks = s.checkDomainDNS(ctx, domains[i].Name) }(i)
	}
	var services []svcCheck
	wg.Add(1)
	go func() { defer wg.Done(); services = s.serviceChecks(ctx) }()
	wg.Wait()

	info := s.serverInfo(ctx)
	writeJSON(w, http.StatusOK, map[string]any{
		"counts": counts, "domains": domains, "services": services, "server": info,
		"recent": s.auditRows(ctx, 10, 0, ""),
	})
}

// ---------- audit ----------

type auditRow struct {
	ID      int64          `json:"id"`
	At      time.Time      `json:"at"`
	Actor   string         `json:"actor"`
	Action  string         `json:"action"`
	Target  string         `json:"target"`
	Details map[string]any `json:"details"`
	IP      string         `json:"ip"`
}

func (s *Server) auditRows(ctx context.Context, limit int, before int64, kind string) []auditRow {
	q := `SELECT id, at, actor_login, action, target, details, ip FROM audit_log WHERE ($1 = 0 OR id < $1)`
	switch kind {
	case "login":
		q += ` AND action LIKE 'login.%'`
	case "admin":
		q += ` AND action NOT LIKE 'login.%'`
	}
	rows, err := s.db.Query(ctx, q+` ORDER BY id DESC LIMIT $2`, before, limit)
	if err != nil {
		return []auditRow{}
	}
	list, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (auditRow, error) {
		var a auditRow
		err := row.Scan(&a.ID, &a.At, &a.Actor, &a.Action, &a.Target, &a.Details, &a.IP)
		return a, err
	})
	if err != nil {
		return []auditRow{}
	}
	return list
}

func (s *Server) adminAudit(w http.ResponseWriter, r *http.Request, sess *Session) {
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	before, _ := strconv.ParseInt(q.Get("before"), 10, 64)
	kind := q.Get("kind")
	if kind == "sent" {
		rows, err := s.db.Query(r.Context(), `
			SELECT sm.id, sm.sent_at, sm.user_login, m.address, sm.subject, sm.recipients
			FROM sent_messages sm JOIN mailboxes m ON m.id = sm.mailbox_id
			WHERE ($1 = 0 OR sm.id < $1) ORDER BY sm.id DESC LIMIT $2`, before, limit)
		if err != nil {
			serverErr(w, "sent log", err)
			return
		}
		type sentRow struct {
			ID         int64     `json:"id"`
			At         time.Time `json:"at"`
			Login      string    `json:"login"`
			Mailbox    string    `json:"mailbox"`
			Subject    string    `json:"subject"`
			Recipients []string  `json:"recipients"`
		}
		list, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (sentRow, error) {
			var x sentRow
			err := row.Scan(&x.ID, &x.At, &x.Login, &x.Mailbox, &x.Subject, &x.Recipients)
			return x, err
		})
		if err != nil {
			serverErr(w, "sent log scan", err)
			return
		}
		writeJSON(w, http.StatusOK, list)
		return
	}
	writeJSON(w, http.StatusOK, s.auditRows(r.Context(), limit, before, kind))
}

// GET /api/quota — used and allowed space of the user's mailboxes (sidebar indicator).
var (
	quotaMu   sync.Mutex
	quotaSeen = map[string]struct {
		u  usage
		at time.Time
	}{}
)

func (s *Server) handleQuota(w http.ResponseWriter, r *http.Request, sess *Session) {
	addrs := userAccounts(sess)
	limits := map[string]int64{}
	if rows, err := s.db.Query(r.Context(), `SELECT address, quota_bytes FROM mailboxes WHERE address = ANY($1)`, addrs); err == nil {
		for rows.Next() {
			var a string
			var q int64
			if rows.Scan(&a, &q) == nil {
				limits[a] = q
			}
		}
		rows.Close()
	}
	// The meter needs no second-by-second accuracy: Dovecot is asked at most every 2 minutes.
	used := map[string]usage{}
	var ask []string
	quotaMu.Lock()
	for _, a := range addrs {
		if q, ok := quotaSeen[a]; ok && time.Since(q.at) < 2*time.Minute {
			used[a] = q.u
		} else {
			ask = append(ask, a)
		}
	}
	quotaMu.Unlock()
	if len(ask) > 0 {
		fresh := s.quotaUsage(r.Context(), ask)
		quotaMu.Lock()
		for a, u := range fresh {
			used[a] = u
			quotaSeen[a] = struct {
				u  usage
				at time.Time
			}{u, time.Now()}
		}
		quotaMu.Unlock()
	}
	out := []map[string]any{}
	for _, a := range addrs {
		u := used[a]
		out = append(out, map[string]any{"address": a, "used": u.bytes, "messages": u.messages, "limit": limits[a]})
	}
	writeJSON(w, http.StatusOK, out)
}

// cpuUsage: share of busy CPU time over a short sample (the whole server, from /proc/stat),
// plus the number of cores. iowait is reported apart: high values mean the disk/swap is the brake.
func cpuUsage() map[string]any {
	read := func() (busy, total, iowait uint64, ok bool) {
		b, err := os.ReadFile("/proc/stat")
		if err != nil {
			return 0, 0, 0, false
		}
		f := strings.Fields(strings.SplitN(string(b), "\n", 2)[0])
		if len(f) < 8 {
			return 0, 0, 0, false
		}
		var v []uint64
		for _, x := range f[1:] {
			n, _ := strconv.ParseUint(x, 10, 64)
			v = append(v, n)
		}
		for i, n := range v {
			if i < 8 { // user nice system idle iowait irq softirq steal
				total += n
			}
		}
		idle := v[3] + v[4]
		return total - idle, total, v[4], true
	}
	b1, t1, w1, ok1 := read()
	time.Sleep(400 * time.Millisecond)
	b2, t2, w2, ok2 := read()
	out := map[string]any{"cores": runtime.NumCPU()}
	if ok1 && ok2 && t2 > t1 {
		out["percent"] = int(float64(b2-b1) / float64(t2-t1) * 100)
		out["iowait"] = int(float64(w2-w1) / float64(t2-t1) * 100)
	}
	return out
}

// avSummary: how the antivirus is doing, for the server page.
func (s *Server) avSummary(ctx context.Context) map[string]any {
	out := map[string]any{"ok": false}
	c, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	if v, err := clamVersion(c); err == nil {
		out["ok"] = true
		if parts := strings.SplitN(v, "/", 3); len(parts) == 3 {
			out["engine"], out["signatures"] = parts[0], parts[1]
			if t, err := time.Parse("Mon Jan _2 15:04:05 2006", strings.TrimSpace(parts[2])); err == nil {
				out["updated"] = t
				out["ageHours"] = int(time.Since(t).Hours())
			}
		}
	}
	var checked, found, quarantine, pending int
	_ = s.db.QueryRow(ctx, `SELECT count(*), count(*) FILTER (WHERE found <> '') FROM av_scans WHERE scanned_at > now() - interval '24 hours'`).Scan(&checked, &found)
	_ = s.db.QueryRow(ctx, `SELECT count(*) FROM av_quarantine WHERE released_at IS NULL`).Scan(&quarantine)
	_ = s.db.QueryRow(ctx, `SELECT count(*) FROM big_files WHERE av_status='pending' AND expires_at > now()`).Scan(&pending)
	rejected := 0
	for _, r := range rspamdRejected(ctx) {
		if time.Since(r.Time) < 24*time.Hour {
			rejected++
		}
	}
	out["checked24h"], out["found24h"], out["quarantine"], out["pendingLinks"], out["rejected24h"] = checked, found, quarantine, pending, rejected
	return out
}
