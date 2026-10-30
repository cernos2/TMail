// zmaild — backend of TMAIL webmail: auth, IMAP/SMTP access on behalf of users,
// live updates (IMAP IDLE → Server-Sent Events) and the static Nuxt frontend.
package main

import (
	"context"
	"crypto/tls"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/emersion/go-imap"
	"github.com/emersion/go-message/charset"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Config struct {
	Hostname   string // public host name, also the TLS name of Dovecot/Postfix
	DBURL      string
	MasterUser string // Dovecot master user
	MasterPass string
	IMAPAddr   string
	SMTPAddr   string
	TLSDir     string // fullchain.pem + privkey.pem
	ACMEDir    string // HTTP-01 webroot shared with the certs service
	WebDir     string // built Nuxt app
	HTTPAddr   string
	HTTPSAddr  string

	PublicIP     string
	DoveadmURL   string
	DoveadmKey   string
	RspamdURL    string
	DNSAddr      string
	DKIMDir      string
	DKIMSelector string
	LoggiURL     string
	LoggiKey     string
	SpoolDir     string
}

type Server struct {
	cfg      Config
	db       *pgxpool.Pool
	hub      *Hub
	crm      crmJob
	limiter  *rateLimiter
	limLogin *rateLimiter // failed logins per account, whatever IPs they come from
	folders  *folderCache
	contacts *contactSyncer
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})))
	imap.CharsetReader = charset.Reader

	cfg := Config{
		Hostname:   env("MAIL_HOSTNAME", "localhost"),
		DBURL:      os.Getenv("ZMAIL_DB_URL"),
		MasterUser: env("DOVECOT_MASTER_USER", "zmail-app"),
		MasterPass: os.Getenv("DOVECOT_MASTER_PASSWORD"),
		IMAPAddr:   env("IMAP_ADDR", "127.0.0.1:993"),
		SMTPAddr:   env("SMTP_ADDR", "127.0.0.1:465"),
		TLSDir:     env("TLS_DIR", "/tls"),
		ACMEDir:    env("ACME_DIR", "/acme"),
		WebDir:     env("WEB_DIR", "/srv/web"),
		HTTPAddr:   env("HTTP_ADDR", ":80"),
		HTTPSAddr:  env("HTTPS_ADDR", ":443"),

		PublicIP:     os.Getenv("PUBLIC_IP"),
		DoveadmURL:   env("DOVEADM_URL", "http://127.0.0.1:8080/doveadm/v1"),
		DoveadmKey:   os.Getenv("DOVEADM_API_KEY"),
		RspamdURL:    env("RSPAMD_URL", "http://127.0.0.1:11334"),
		DNSAddr:      env("DNS_ADDR", "127.0.0.1:8053"),
		DKIMDir:      env("DKIM_DIR", "/dkim"),
		DKIMSelector: env("DKIM_SELECTOR", "z2026"),
		LoggiURL:     env("LOGGI_API_URL", ""),
		LoggiKey:     env("LOGGI_API_KEY", ""),
		SpoolDir:     env("POSTFIX_SPOOL", "/postfix-spool"),
	}
	if cfg.DBURL == "" || cfg.MasterPass == "" {
		slog.Error("ZMAIL_DB_URL and DOVECOT_MASTER_PASSWORD are required")
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	db, err := pgxpool.New(ctx, cfg.DBURL)
	if err != nil {
		slog.Error("database", "err", err)
		os.Exit(1)
	}
	defer db.Close()

	indexDB = db
	s := &Server{cfg: cfg, db: db, limiter: newRateLimiter(10, 10*time.Minute), limLogin: newRateLimiter(20, 15*time.Minute), folders: newFolderCache(), contacts: newContactSyncer()}
	s.hub = newHub(s)
	s.loadAIProvider(ctx)
	go s.cleanupSessions(ctx)
	go s.runReminders(ctx)
	go s.runSorter(ctx)
	go s.runCollector(ctx)
	go s.runBigFileCleanup(ctx)
	go s.runQuarantineCleanup(ctx)
	go s.runBigFileScanner(ctx)
	go s.runMailboxPurge(ctx)
	go s.runFolderExpiry(ctx)
	go s.runSearchIndex(ctx)
	go spellCheck([]string{"проверка", "check"}) // load the dictionaries (a Russian and an English word) before the first letter
	go conns.reap()
	go func() { s.crmEnsureOwn(ctx); s.crmDedupe(ctx); s.crmResume(ctx) }()

	certs := &certLoader{dir: cfg.TLSDir}
	https := &http.Server{
		Addr:              cfg.HTTPSAddr,
		Handler:           s.securityHeaders(s.routes()),
		TLSConfig:         &tls.Config{GetCertificate: certs.Get, MinVersion: tls.VersionTLS12},
		ReadHeaderTimeout: 15 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	plain := &http.Server{Addr: cfg.HTTPAddr, Handler: s.httpRedirect(), ReadHeaderTimeout: 10 * time.Second}

	errc := make(chan error, 2)
	go func() { errc <- https.ListenAndServeTLS("", "") }()
	go func() { errc <- plain.ListenAndServe() }()
	slog.Info("zmaild started", "https", cfg.HTTPSAddr, "http", cfg.HTTPAddr, "host", cfg.Hostname)

	select {
	case <-ctx.Done():
	case err := <-errc:
		if !errors.Is(err, http.ErrServerClosed) {
			slog.Error("server", "err", err)
		}
	}
	shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = https.Shutdown(shutdown)
	_ = plain.Shutdown(shutdown)
}

// httpRedirect serves ACME HTTP-01 challenges and redirects everything else to HTTPS.
func (s *Server) httpRedirect() http.Handler {
	acme := http.StripPrefix("/.well-known/acme-challenge/", http.FileServer(http.Dir(filepath.Join(s.cfg.ACMEDir, ".well-known", "acme-challenge"))))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/.well-known/acme-challenge/") {
			acme.ServeHTTP(w, r)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/internal/rspamd/") {
			s.serveRspamdMap(w, r)
			return
		}
		http.Redirect(w, r, "https://"+s.cfg.Hostname+r.URL.RequestURI(), http.StatusMovedPermanently)
	})
}

func (s *Server) securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "SAMEORIGIN")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Permissions-Policy", "camera=(), geolocation=(), payment=(), usb=(), microphone=(self)")
		// A private mail service: search engines must not index its sign-in page or file links (an
		// indexed login form on a young domain is what phishing filters flag as "dangerous").
		h.Set("X-Robots-Tag", "noindex, nofollow, noarchive")
		if r.URL.Path == "/robots.txt" {
			h.Set("Content-Type", "text/plain; charset=utf-8")
			_, _ = w.Write([]byte("User-agent: *\nDisallow: /\n"))
			return
		}
		next.ServeHTTP(w, r)
	})
}

// certLoader re-reads the certificate when the certs service replaces it.
type certLoader struct {
	dir     string
	mu      sync.Mutex
	cert    *tls.Certificate
	modTime time.Time
	checked time.Time
}

func (c *certLoader) Get(*tls.ClientHelloInfo) (*tls.Certificate, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cert != nil && time.Since(c.checked) < time.Minute {
		return c.cert, nil
	}
	c.checked = time.Now()
	full := filepath.Join(c.dir, "fullchain.pem")
	st, err := os.Stat(full)
	if err != nil {
		if c.cert != nil {
			return c.cert, nil
		}
		return nil, err
	}
	if c.cert != nil && st.ModTime().Equal(c.modTime) {
		return c.cert, nil
	}
	cert, err := tls.LoadX509KeyPair(full, filepath.Join(c.dir, "privkey.pem"))
	if err != nil {
		if c.cert != nil {
			return c.cert, nil
		}
		return nil, err
	}
	c.cert, c.modTime = &cert, st.ModTime()
	slog.Info("TLS certificate loaded", "modified", c.modTime)
	return c.cert, nil
}

func (s *Server) cleanupSessions(ctx context.Context) {
	t := time.NewTicker(time.Hour)
	defer t.Stop()
	for {
		_, _ = s.db.Exec(ctx, `DELETE FROM sessions WHERE expires_at < now()`)
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}
