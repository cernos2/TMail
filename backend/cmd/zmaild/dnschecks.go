package main

import (
	"context"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"golang.org/x/net/dns/dnsmessage"
)

// More checks of a domain on the "Domains" page: the signing key on disk against DNS, blocklists,
// the certificate, delivery protection (MTA-STS, TLS-RPT, DANE) and automatic setup of mail
// programs. Optional checks are recommendations: a missing one is not an error.

func (s *Server) moreDomainChecks(ctx context.Context, domain, selector, pub string) []dnsCheck {
	res := s.resolver()
	host := s.cfg.Hostname
	var out []dnsCheck
	add := func(c dnsCheck) { out = append(out, c) }

	// The private key rspamd signs with must match the public key published in DNS.
	if selector != "" {
		k := dnsCheck{Key: "dkimfile", Group: "security", Title: "DKIM: the server's signing key", Type: "—", Name: "—", Expected: "—"}
		filePub, err := dkimFilePublic(filepath.Join(s.cfg.DKIMDir, domain+"."+selector+".key"))
		switch {
		case err != nil:
			k.Status, k.Hint = "missing", "The server has no private key for this domain: letters are sent without a DKIM signature. Create the key again."
		case filePub != pub:
			k.Status, k.Hint = "wrong", "The server signs with a different key than the one in the domain settings. Create the key again and update DNS."
		default:
			k.Status = "ok"
			k.Found = "SHA-256 " + shortHash(filePub)
		}
		add(k)
	}

	// Blocklists: the server's IP and the domain itself.
	if s.cfg.PublicIP != "" {
		if ip := net.ParseIP(s.cfg.PublicIP).To4(); ip != nil {
			rev := fmt.Sprintf("%d.%d.%d.%d", ip[3], ip[2], ip[1], ip[0])
			bl := dnsCheck{Key: "blocklist", Group: "security", Title: "Blocklists: the server's IP", Type: "DNSBL", Name: s.cfg.PublicIP}
			var listed []string
			for _, z := range []struct{ zone, name string }{{"zen.spamhaus.org", "Spamhaus ZEN"}, {"b.barracudacentral.org", "Barracuda"}, {"bl.spamcop.net", "SpamCop"}} {
				c, cancel := context.WithTimeout(ctx, 4*time.Second)
				ips, err := res.LookupHost(c, rev+"."+z.zone)
				cancel()
				if err == nil && len(ips) > 0 && strings.HasPrefix(ips[0], "127.") && !strings.HasPrefix(ips[0], "127.255.255.") {
					why := ips[0]
					if txt, err := res.LookupTXT(ctx, rev+"."+z.zone); err == nil && len(txt) > 0 {
						why = txt[0] // "Listed by SBL, see https://check.spamhaus.org/…"
					}
					listed = append(listed, z.name+": "+why)
				}
			}
			if len(listed) > 0 {
				bl.Status, bl.Found = "wrong", strings.Join(listed, ", ")
				bl.Hint = "The server's IP is on a blocklist: many servers reject or bin its mail. Find the reason on the list's website (usually a hacked mailbox sending spam) and request removal; meanwhile you can send through an external SMTP server (Mail server → Relay)."
			} else {
				bl.Status, bl.Found = "ok", "Spamhaus, Barracuda, SpamCop"
			}
			add(bl)
		}
	}
	dbl := dnsCheck{Key: "dbl", Group: "security", Title: "Blocklists: the domain", Type: "DNSBL", Name: domain}
	{
		c, cancel := context.WithTimeout(ctx, 4*time.Second)
		ips, err := res.LookupHost(c, domain+".dbl.spamhaus.org")
		cancel()
		if err == nil && len(ips) > 0 && strings.HasPrefix(ips[0], "127.0.1.") {
			dbl.Status, dbl.Found = "wrong", "Spamhaus DBL ("+ips[0]+")"
			dbl.Hint = "The domain is on the Spamhaus domain blocklist: links to it and its letters are treated as spam. Check spamhaus.org for the reason."
		} else {
			dbl.Status, dbl.Found = "ok", "Spamhaus DBL"
		}
	}
	add(dbl)

	// The TLS certificate of the mail server.
	cert := dnsCheck{Key: "cert", Group: "security", Title: "Certificate of the mail server", Type: "TLS", Name: host}
	if c, err := readCert(filepath.Join(s.cfg.TLSDir, "fullchain.pem")); err != nil {
		cert.Status, cert.Hint = "missing", "No certificate: mail programs will warn or refuse to connect."
	} else {
		left := time.Until(c.NotAfter)
		cert.Found = fmt.Sprintf("%s · %s", c.Issuer.CommonName, c.NotAfter.UTC().Format("2006-01-02"))
		switch {
		case c.VerifyHostname(host) != nil:
			cert.Status, cert.Hint = "wrong", "The certificate is not for this server name."
		case left < 0:
			cert.Status, cert.Hint = "wrong", "The certificate has expired. The certs service renews it automatically: check that port 80 is open."
		case left < 14*24*time.Hour:
			cert.Status, cert.Hint = "wrong", "The certificate expires soon and was not renewed. Check that port 80 is open for Let's Encrypt."
		default:
			cert.Status = "ok"
		}
	}
	add(cert)

	// MTA-STS: other servers deliver to us only over verified TLS.
	sts := dnsCheck{Key: "mtasts", Group: "security", Optional: true, Title: "MTA-STS: delivery only over encrypted connections", Type: "TXT", Name: "_mta-sts",
		Expected: "v=STSv1; id=" + s.mtaSTSID(ctx)}
	if txts, err := res.LookupTXT(ctx, "_mta-sts."+domain); err == nil {
		for _, t := range txts {
			if strings.HasPrefix(strings.ToLower(strings.TrimSpace(t)), "v=stsv1") {
				sts.Found = t
			}
		}
	}
	if sts.Found == "" {
		sts.Status, sts.Hint = "missing", "Recommended: without it, mail to you can be read or redirected by intercepting the connection. Add the TXT record and the mta-sts name below."
	} else {
		sts.Status = "ok"
	}
	add(sts)
	stsHost := dnsCheck{Key: "mtastshost", Group: "security", Optional: true, Title: "MTA-STS: policy address", Type: "CNAME", Name: "mta-sts", Expected: host + "."}
	if s.pointsHere(ctx, "mta-sts."+domain) {
		stsHost.Found = "→ " + host
		if pol, err := fetchPolicy(ctx, "https://mta-sts."+domain+"/.well-known/mta-sts.txt"); err != nil {
			stsHost.Status, stsHost.Hint = "wrong", "The name points here, but its certificate is not ready yet: it is issued automatically within about 10 minutes."
		} else if !strings.Contains(pol, "mx: "+host) {
			stsHost.Status, stsHost.Hint = "wrong", "The policy does not name this mail server."
		} else {
			stsHost.Status = "ok"
			if line := firstLineWith(pol, "mode:"); line != "" {
				stsHost.Found += " · " + line
			}
		}
	} else {
		stsHost.Status, stsHost.Hint = "missing", "Recommended together with the MTA-STS record: the name must point to this server."
	}
	add(stsHost)
	rpt := dnsCheck{Key: "tlsrpt", Group: "security", Optional: true, Title: "TLS-RPT: reports on delivery problems", Type: "TXT", Name: "_smtp._tls",
		Expected: "v=TLSRPTv1; rua=mailto:postmaster@" + domain}
	if txts, err := res.LookupTXT(ctx, "_smtp._tls."+domain); err == nil {
		for _, t := range txts {
			if strings.HasPrefix(strings.ToLower(strings.TrimSpace(t)), "v=tlsrptv1") {
				rpt.Found = t
			}
		}
	}
	if rpt.Found == "" {
		rpt.Status, rpt.Hint = "missing", "Recommended: other servers will report failed encrypted deliveries to postmaster."
	} else {
		rpt.Status = "ok"
	}
	add(rpt)

	// DANE: TLSA of the mail server (works only when the server's zone is signed with DNSSEC).
	tlsa := dnsCheck{Key: "tlsa", Group: "security", Optional: true, Title: "DANE: the server's key in DNS", Type: "TLSA", Name: "_25._tcp." + host + "."}
	if c, err := readCert(filepath.Join(s.cfg.TLSDir, "fullchain.pem")); err == nil {
		sum := sha256.Sum256(c.RawSubjectPublicKeyInfo)
		tlsa.Expected = "3 1 1 " + hex.EncodeToString(sum[:])
	}
	recs, secure, err := s.lookupTLSA(ctx, "_25._tcp."+host)
	switch {
	case err != nil || len(recs) == 0:
		tlsa.Status, tlsa.Hint = "missing", "Recommended if the DNS zone of the server name is signed with DNSSEC: servers that check DANE will then deliver only over verified TLS. The key stays the same across certificate renewals."
	case !secure:
		tlsa.Found = strings.Join(recs, ", ")
		tlsa.Status, tlsa.Hint = "wrong", "The record exists, but the zone is not signed with DNSSEC: other servers ignore it. Turn on DNSSEC at the DNS provider."
	case !slices.Contains(recs, tlsa.Expected):
		tlsa.Found = strings.Join(recs, ", ")
		tlsa.Status, tlsa.Hint = "wrong", "The record does not match the server's key: servers checking DANE will not deliver mail. Replace it with the expected one now."
	default:
		tlsa.Found, tlsa.Status = tlsa.Expected, "ok"
	}
	add(tlsa)

	// Automatic setup of mail programs.
	for _, a := range []struct{ key, sub, title string }{
		{"autoconfig", "autoconfig", "Autoconfig: Thunderbird and other programs set themselves up"},
		{"autodiscover", "autodiscover", "Autodiscover: Outlook sets itself up"},
	} {
		c := dnsCheck{Key: a.key, Group: "setup", Optional: true, Title: a.title, Type: "CNAME", Name: a.sub, Expected: host + "."}
		if s.pointsHere(ctx, a.sub+"."+domain) {
			c.Status, c.Found = "ok", "→ "+host
		} else {
			c.Status, c.Hint = "missing", "Recommended: a mail program finds the server settings by the e-mail address alone."
		}
		add(c)
	}
	for _, sr := range []struct{ key, service, title string }{
		{"srvautodiscover", "_autodiscover._tcp", "SRV: Outlook settings"},
		{"srvimaps", "_imaps._tcp", "SRV: incoming mail (IMAP)"},
		{"srvsubmissions", "_submissions._tcp", "SRV: outgoing mail (SMTP)"},
		{"srvcaldavs", "_caldavs._tcp", "SRV: calendars (CalDAV)"},
		{"srvcarddavs", "_carddavs._tcp", "SRV: contacts (CardDAV)"},
	} {
		port := map[string]int{"_autodiscover._tcp": 443, "_imaps._tcp": 993, "_submissions._tcp": 465, "_caldavs._tcp": 443, "_carddavs._tcp": 443}[sr.service]
		c := dnsCheck{Key: sr.key, Group: "setup", Optional: true, Title: sr.title, Type: "SRV", Name: sr.service,
			Expected: fmt.Sprintf("0 1 %d %s.", port, host)}
		if _, addrs, err := res.LookupSRV(ctx, "", "", sr.service+"."+domain); err == nil && len(addrs) > 0 {
			c.Found = fmt.Sprintf("%d %d %d %s", addrs[0].Priority, addrs[0].Weight, addrs[0].Port, addrs[0].Target)
			if strings.EqualFold(strings.TrimSuffix(addrs[0].Target, "."), host) && int(addrs[0].Port) == port {
				c.Status = "ok"
			} else {
				c.Status, c.Hint = "wrong", "The record points to another server or port."
			}
		} else {
			c.Status, c.Hint = "missing", "Recommended: phones and mail programs find the server by the domain."
		}
		add(c)
	}
	return out
}

func shortHash(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:6])
}

func firstLineWith(text, prefix string) string {
	for _, l := range strings.Split(text, "\n") {
		if l = strings.TrimSpace(l); strings.HasPrefix(l, prefix) {
			return l
		}
	}
	return ""
}

// dkimFilePublic: the public key (base64 DER, as in DNS) of a DKIM private key file.
func dkimFilePublic(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	blk, _ := pem.Decode(b)
	if blk == nil {
		return "", fmt.Errorf("not a PEM file")
	}
	var key *rsa.PrivateKey
	if k, err := x509.ParsePKCS1PrivateKey(blk.Bytes); err == nil {
		key = k
	} else if k, err := x509.ParsePKCS8PrivateKey(blk.Bytes); err == nil {
		rk, ok := k.(*rsa.PrivateKey)
		if !ok {
			return "", fmt.Errorf("not an RSA key")
		}
		key = rk
	} else {
		return "", err
	}
	der, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(der), nil
}

func readCert(path string) (*x509.Certificate, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	blk, _ := pem.Decode(b)
	if blk == nil {
		return nil, fmt.Errorf("no certificate")
	}
	return x509.ParseCertificate(blk.Bytes)
}

// pointsHere: the name resolves to this server's address.
func (s *Server) pointsHere(ctx context.Context, name string) bool {
	res := s.resolver()
	c, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	ips, err := res.LookupHost(c, name)
	if err != nil {
		return false
	}
	ours, _ := res.LookupHost(c, s.cfg.Hostname)
	if s.cfg.PublicIP != "" {
		ours = append(ours, s.cfg.PublicIP)
	}
	for _, ip := range ips {
		if slices.Contains(ours, ip) {
			return true
		}
	}
	return false
}

func fetchPolicy(ctx context.Context, url string) (string, error) {
	c, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(c, http.MethodGet, url, nil)
	cl := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12}}, CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse // RFC 8461: no redirects
	}}
	resp, err := cl.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	return string(b), err
}

// lookupTLSA asks our validating resolver for TLSA records; secure = the answer is DNSSEC-validated.
func (s *Server) lookupTLSA(ctx context.Context, name string) (recs []string, secure bool, err error) {
	n, err := dnsmessage.NewName(strings.TrimSuffix(name, ".") + ".")
	if err != nil {
		return nil, false, err
	}
	const typeTLSA = dnsmessage.Type(52)
	b := dnsmessage.NewBuilder(nil, dnsmessage.Header{ID: uint16(time.Now().UnixNano()), RecursionDesired: true})
	b.EnableCompression()
	_ = b.StartQuestions()
	_ = b.Question(dnsmessage.Question{Name: n, Type: typeTLSA, Class: dnsmessage.ClassINET})
	_ = b.StartAdditionals()
	var opt dnsmessage.ResourceHeader
	_ = opt.SetEDNS0(1232, dnsmessage.RCodeSuccess, true) // DO: ask for DNSSEC validation
	_ = b.OPTResource(opt, dnsmessage.OPTResource{})
	q, err := b.Finish()
	if err != nil {
		return nil, false, err
	}
	var d net.Dialer
	c, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	conn, err := d.DialContext(c, "udp", s.cfg.DNSAddr)
	if err != nil {
		return nil, false, err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	if _, err := conn.Write(q); err != nil {
		return nil, false, err
	}
	buf := make([]byte, 4096)
	k, err := conn.Read(buf)
	if err != nil {
		return nil, false, err
	}
	var p dnsmessage.Parser
	h, err := p.Start(buf[:k])
	if err != nil {
		return nil, false, err
	}
	_ = p.SkipAllQuestions()
	for {
		rh, err := p.AnswerHeader()
		if err != nil {
			break
		}
		if rh.Type != typeTLSA {
			_ = p.SkipAnswer()
			continue
		}
		r, err := p.UnknownResource()
		if err != nil || len(r.Data) < 4 {
			continue
		}
		recs = append(recs, fmt.Sprintf("%d %d %d %s", r.Data[0], r.Data[1], r.Data[2], hex.EncodeToString(r.Data[3:])))
	}
	return recs, h.AuthenticData, nil
}
