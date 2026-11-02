package main

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"log/slog"

	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"
)

// Automatic setup of mail programs: Thunderbird reads autoconfig.<domain>/mail/config-v1.1.xml,
// Outlook autodiscover.<domain>/autodiscover/autodiscover.xml (or the SRV record
// _autodiscover._tcp), iPhones and Macs install a profile from the settings page. The same
// service names (autoconfig., autodiscover., mta-sts.) get onto the TLS certificate as soon as
// their DNS points here.

var reAutoEmail = regexp.MustCompile(`^[^@\s]{1,128}@([a-z0-9.-]{1,253})$`)

// ourDomain: the domain is served by this server (otherwise the request is not answered: nobody
// learns which domains live here by guessing).
func (s *Server) ourDomain(ctx context.Context, domain string) bool {
	var ok bool
	_ = s.db.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM domains WHERE name=$1 AND active)`, strings.ToLower(domain)).Scan(&ok)
	return ok
}

// autoDomain: the mail domain of an autoconfig request — from the address, or from the host name
// (autoconfig.example.com → example.com).
func (s *Server) autoDomain(r *http.Request, email string) string {
	if m := reAutoEmail.FindStringSubmatch(strings.ToLower(strings.TrimSpace(email))); m != nil {
		return m[1]
	}
	host := strings.ToLower(strings.Split(r.Host, ":")[0])
	for _, p := range []string{"autoconfig.", "autodiscover."} {
		if d, ok := strings.CutPrefix(host, p); ok {
			return d
		}
	}
	return host
}

func (s *Server) handleAutoconfig(w http.ResponseWriter, r *http.Request) {
	domain := s.autoDomain(r, r.URL.Query().Get("emailaddress"))
	if !s.ourDomain(r.Context(), domain) {
		http.NotFound(w, r)
		return
	}
	h := xesc(s.cfg.Hostname)
	org := xesc(serverLocale().Org)
	server := func(kind, typ string, port int, socket string) string {
		return fmt.Sprintf(`<%s type="%s"><hostname>%s</hostname><port>%d</port><socketType>%s</socketType>`+
			`<authentication>password-cleartext</authentication><username>%%EMAILADDRESS%%</username></%s>`, kind, typ, h, port, socket, kind)
	}
	body := `<?xml version="1.0" encoding="UTF-8"?>
<clientConfig version="1.1">
<emailProvider id="` + xesc(domain) + `">
<domain>` + xesc(domain) + `</domain>
<displayName>` + org + ` (` + xesc(domain) + `)</displayName>
<displayShortName>` + org + `</displayShortName>
` + server("incomingServer", "imap", 993, "SSL") + `
` + server("incomingServer", "imap", 143, "STARTTLS") + `
` + server("incomingServer", "pop3", 995, "SSL") + `
` + server("outgoingServer", "smtp", 465, "SSL") + `
` + server("outgoingServer", "smtp", 587, "STARTTLS") + `
<documentation url="https://` + h + `/"><descr lang="en">Webmail</descr></documentation>
</emailProvider>
<addressBook type="carddav"><username>%EMAILADDRESS%</username><authentication>http-basic</authentication><serverURL>https://` + h + `/dav/</serverURL></addressBook>
<calendar type="caldav"><username>%EMAILADDRESS%</username><authentication>http-basic</authentication><serverURL>https://` + h + `/dav/</serverURL></calendar>
</clientConfig>
`
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	_, _ = io.WriteString(w, body)
}

// Outlook: POST <Autodiscover><Request><EMailAddress>…
func (s *Server) handleAutodiscover(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email string `xml:"Request>EMailAddress"`
	}
	if r.Method == http.MethodPost {
		_ = xml.NewDecoder(io.LimitReader(r.Body, 64<<10)).Decode(&req)
	}
	if req.Email == "" {
		req.Email = r.URL.Query().Get("email")
	}
	email := strings.ToLower(strings.TrimSpace(req.Email))
	domain := s.autoDomain(r, email)
	if !s.ourDomain(r.Context(), domain) {
		http.NotFound(w, r)
		return
	}
	h := xesc(s.cfg.Hostname)
	login := xesc(email)
	body := `<?xml version="1.0" encoding="utf-8"?>
<Autodiscover xmlns="http://schemas.microsoft.com/exchange/autodiscover/responseschema/2006">
<Response xmlns="http://schemas.microsoft.com/exchange/autodiscover/outlook/responseschema/2006a">
<Account><AccountType>email</AccountType><Action>settings</Action>
<Protocol><Type>IMAP</Type><Server>` + h + `</Server><Port>993</Port><DomainRequired>off</DomainRequired><LoginName>` + login + `</LoginName><SPA>off</SPA><SSL>on</SSL><AuthRequired>on</AuthRequired></Protocol>
<Protocol><Type>POP3</Type><Server>` + h + `</Server><Port>995</Port><DomainRequired>off</DomainRequired><LoginName>` + login + `</LoginName><SPA>off</SPA><SSL>on</SSL><AuthRequired>on</AuthRequired></Protocol>
<Protocol><Type>SMTP</Type><Server>` + h + `</Server><Port>465</Port><DomainRequired>off</DomainRequired><LoginName>` + login + `</LoginName><SPA>off</SPA><SSL>on</SSL><Encryption>SSL</Encryption><AuthRequired>on</AuthRequired><UsePOPAuth>on</UsePOPAuth><SMTPLast>off</SMTPLast></Protocol>
</Account>
</Response>
</Autodiscover>
`
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	_, _ = io.WriteString(w, body)
}

// New Outlook asks the JSON service first; it is pointed to the XML one.
func (s *Server) handleAutodiscoverJSON(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"Protocol": "AutodiscoverV1", "Url": "https://" + s.cfg.Hostname + "/autodiscover/autodiscover.xml"})
}

// isAutoconfigPath: answered on plain HTTP too (Thunderbird tries http://autoconfig.<domain>).
func isAutoconfigPath(p string) bool {
	p = strings.ToLower(p)
	return p == "/mail/config-v1.1.xml" || p == "/.well-known/autoconfig/mail/config-v1.1.xml"
}

func isAutodiscoverPath(p string) bool {
	p = strings.ToLower(p)
	return p == "/autodiscover/autodiscover.xml" || strings.HasPrefix(p, "/autodiscover/autodiscover.json")
}

func (s *Server) serveAutoSetup(w http.ResponseWriter, r *http.Request) bool {
	switch {
	case isAutoconfigPath(r.URL.Path):
		s.handleAutoconfig(w, r)
	case strings.HasPrefix(strings.ToLower(r.URL.Path), "/autodiscover/autodiscover.json"):
		s.handleAutodiscoverJSON(w, r)
	case isAutodiscoverPath(r.URL.Path):
		s.handleAutodiscover(w, r)
	case r.URL.Path == "/.well-known/mta-sts.txt":
		s.handleMTASTSPolicy(w, r)
	default:
		return false
	}
	return true
}

// ---------- names on the certificate ----------

// serviceHosts: autoconfig., autodiscover. and mta-sts. names of our domains whose DNS points
// to this server. The certs service adds them to the certificate (a name that does not point here
// would make Let's Encrypt refuse the whole certificate).
func (s *Server) serviceHosts(ctx context.Context) []string {
	rows, err := s.db.Query(ctx, `SELECT name FROM domains WHERE active ORDER BY name`)
	if err != nil {
		return nil
	}
	var domains []string
	for rows.Next() {
		var d string
		if rows.Scan(&d) == nil {
			domains = append(domains, d)
		}
	}
	rows.Close()
	res := s.resolver()
	ours, _ := res.LookupHost(ctx, s.cfg.Hostname)
	if s.cfg.PublicIP != "" && !slices.Contains(ours, s.cfg.PublicIP) {
		ours = append(ours, s.cfg.PublicIP)
	}
	var out []string
	for _, d := range domains {
		for _, sub := range []string{"autoconfig", "autodiscover", "mta-sts"} {
			name := sub + "." + d
			if strings.EqualFold(name, s.cfg.Hostname) {
				continue
			}
			c, cancel := context.WithTimeout(ctx, 5*time.Second)
			ips, err := res.LookupHost(c, name)
			cancel()
			if err != nil {
				continue
			}
			for _, ip := range ips {
				if slices.Contains(ours, ip) {
					out = append(out, name)
					break
				}
			}
		}
	}
	return out
}

func (s *Server) runCertNames(ctx context.Context) {
	file := filepath.Join(env("CERT_NAMES_DIR", "/certnames"), "names")
	if st, err := os.Stat(filepath.Dir(file)); err != nil || !st.IsDir() {
		return // an installation without the shared folder
	}
	for {
		names := s.serviceHosts(ctx)
		data := strings.Join(names, "\n")
		if old, _ := os.ReadFile(file); string(old) != data {
			if err := os.WriteFile(file+".tmp", []byte(data), 0o644); err == nil && os.Rename(file+".tmp", file) == nil {
				slog.Info("certificate names", "names", names)
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(10 * time.Minute):
		}
	}
}
