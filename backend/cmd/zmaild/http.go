package main

import (
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/auth/login", s.handleLogin)
	mux.HandleFunc("POST /api/auth/logout", s.auth(s.handleLogout))
	mux.HandleFunc("GET /api/me", s.auth(s.handleMe))
	mux.HandleFunc("GET /api/locale", s.handleLocaleDefaults)
	mux.HandleFunc("GET /api/folders", s.auth(s.handleFolders))
	mux.HandleFunc("GET /api/folders/rules", s.auth(s.handleFolderRules))
	mux.HandleFunc("POST /api/folders", s.auth(s.handleFolderSave))
	mux.HandleFunc("PUT /api/folders", s.auth(s.handleFolderSave))
	mux.HandleFunc("POST /api/folders/sender", s.auth(s.handleFolderSender))
	mux.HandleFunc("DELETE /api/folders", s.auth(s.handleFolderDelete))
	mux.HandleFunc("GET /api/crm/parties", s.auth(s.handleCrmList))
	mux.HandleFunc("GET /api/crm/parties/{id}", s.auth(s.handleCrmGet))
	mux.HandleFunc("POST /api/crm/parties", s.auth(s.handleCrmSave))
	mux.HandleFunc("PUT /api/crm/parties/{id}", s.auth(s.handleCrmSave))
	mux.HandleFunc("DELETE /api/crm/parties/{id}", s.admin(s.handleCrmDelete))
	mux.HandleFunc("POST /api/crm/parties/{id}/self", s.admin(s.handleCrmSelf))
	mux.HandleFunc("POST /api/crm/parties/{id}/merge", s.auth(s.handleCrmMerge))
	mux.HandleFunc("POST /api/crm/parties/{id}/addresses", s.auth(s.handleCrmAddress))
	mux.HandleFunc("DELETE /api/crm/addresses/{id}", s.auth(s.handleCrmAddress))
	mux.HandleFunc("POST /api/crm/suggestions/{id}", s.auth(s.handleCrmSuggestion))
	mux.HandleFunc("GET /api/crm/lookup", s.auth(s.handleCrmLookup))
	mux.HandleFunc("POST /api/crm/extract", s.auth(s.handleCrmExtract))
	mux.HandleFunc("GET /api/autoreply", s.auth(s.handleAutoReplyGet))
	mux.HandleFunc("PUT /api/autoreply", s.auth(s.handleAutoReplySave))
	mux.HandleFunc("POST /api/autoreply/draft", s.auth(s.handleAutoReplyDraft))
	mux.HandleFunc("GET /api/identities", s.auth(s.handleIdentities))
	mux.HandleFunc("POST /api/sender-rules", s.auth(s.handleSenderRuleAdd))
	mux.HandleFunc("GET /api/quota", s.auth(s.handleQuota))
	mux.HandleFunc("GET /api/labels", s.auth(s.handleLabels))
	mux.HandleFunc("POST /api/labels", s.auth(s.handleLabelSave))
	mux.HandleFunc("PUT /api/labels/{id}", s.auth(s.handleLabelSave))
	mux.HandleFunc("DELETE /api/labels/{id}", s.auth(s.handleLabelDelete))
	mux.HandleFunc("GET /api/templates", s.auth(s.handleTemplates))
	mux.HandleFunc("POST /api/templates", s.auth(s.handleTemplateCreate))
	mux.HandleFunc("PUT /api/templates/{id}", s.auth(s.handleTemplateUpdate))
	mux.HandleFunc("DELETE /api/templates/{id}", s.auth(s.handleTemplateDelete))
	mux.HandleFunc("GET /api/messages", s.auth(s.handleMessages))
	mux.HandleFunc("GET /api/message", s.auth(s.handleMessage))
	mux.HandleFunc("GET /api/thread", s.auth(s.handleThread))
	mux.HandleFunc("GET /api/ai/actions", s.auth(s.handleAIActions))
	mux.HandleFunc("POST /api/ai/run", s.auth(s.handleAIRun))
	mux.HandleFunc("POST /api/ai/transcribe", s.auth(s.handleAITranscribe))
	mux.HandleFunc("GET /api/calendar/events", s.auth(s.handleCalEvents))
	mux.HandleFunc("POST /api/calendar/events", s.auth(s.handleCalCreate))
	mux.HandleFunc("PUT /api/calendar/events/{id}", s.auth(s.handleCalUpdate))
	mux.HandleFunc("DELETE /api/calendar/events/{id}", s.auth(s.handleCalDelete))
	mux.HandleFunc("POST /api/calendar/events/{id}/reminder", s.auth(s.handleReminderAction))
	mux.HandleFunc("GET /api/calendar/plan", s.auth(s.handlePlanGet))
	mux.HandleFunc("PUT /api/calendar/plan", s.auth(s.handlePlanPut))
	mux.HandleFunc("GET /api/attachment", s.auth(s.handleAttachment))
	mux.HandleFunc("GET /api/cid", s.auth(s.handleCID))
	mux.HandleFunc("GET /api/attachments.zip", s.auth(s.handleZip))
	mux.HandleFunc("POST /api/messages/action", s.auth(s.handleAction))
	mux.HandleFunc("POST /api/send", s.auth(s.handleSend))
	mux.HandleFunc("POST /api/drafts", s.auth(s.handleSaveDraft))
	mux.HandleFunc("POST /api/drafts/delete", s.auth(s.handleDeleteDraft))
	mux.HandleFunc("GET /api/events", s.auth(s.handleEvents))
	mux.HandleFunc("GET /api/settings", s.auth(s.handleGetSettings))
	mux.HandleFunc("PUT /api/settings", s.auth(s.handlePutSettings))
	mux.HandleFunc("PUT /api/mailbox-settings", s.auth(s.handleMailboxSettings))
	mux.HandleFunc("GET /api/contacts", s.auth(s.handleContacts))
	mux.HandleFunc("GET /api/avatar", s.auth(s.handleAvatar))
	mux.HandleFunc("GET /api/user-photo", s.auth(s.handleUserPhoto))
	mux.HandleFunc("POST /api/me/avatar", s.auth(s.handleUploadAvatar))
	mux.HandleFunc("DELETE /api/me/avatar", s.auth(s.handleDeleteAvatar))
	mux.HandleFunc("POST /api/me/images", s.auth(s.handleImagePrefs))
	mux.HandleFunc("PUT /api/me/locale", s.auth(s.handleSetLocale))
	mux.HandleFunc("POST /api/me/background", s.auth(s.handleSetBackground))
	mux.HandleFunc("GET /api/mcp/tokens", s.auth(s.handleTokens))
	mux.HandleFunc("POST /api/mcp/tokens", s.auth(s.handleTokenCreate))
	mux.HandleFunc("DELETE /api/mcp/tokens/{id}", s.auth(s.handleTokenRevoke))
	mux.HandleFunc("/mcp", s.handleMCP) // AI agents, own Bearer tokens (mcp.go)
	mux.HandleFunc("POST /api/me/background/upload", s.auth(s.handleUploadBackground))
	mux.HandleFunc("GET /api/me/background/image", s.auth(s.handleBackgroundImage))
	mux.HandleFunc("POST /api/bigfiles", s.auth(s.handleBigUpload))
	mux.HandleFunc("DELETE /api/bigfiles/{token}", s.auth(s.handleBigDelete))
	mux.HandleFunc("POST /api/signature-image", s.auth(s.handleSignatureImage))
	mux.HandleFunc("POST /api/spell", s.auth(s.handleSpell))
	mux.HandleFunc("/api/spell/dict", s.auth(s.handleSpellDict))
	s.adminRoutes(mux)
	s.filesRoutes(mux)
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		writeErr(w, http.StatusNotFound, "No such API method")
	})
	mux.Handle("/", s.static())
	// files.<domain> serves only the file links; anything else goes to the webmail.
	files := env("FILES_HOSTNAME", "")
	return gzipJSON(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if files != "" && strings.EqualFold(strings.Split(r.Host, ":")[0], files) &&
			!strings.HasPrefix(r.URL.Path, "/f/") && !strings.HasPrefix(r.URL.Path, "/t/") && !strings.HasPrefix(r.URL.Path, "/i/") && !strings.HasPrefix(r.URL.Path, "/favicon") {
			http.Redirect(w, r, "https://"+s.cfg.Hostname+"/", http.StatusFound)
			return
		}
		// Uploaded files never open on the webmail's own origin.
		if files != "" && !strings.EqualFold(strings.Split(r.Host, ":")[0], files) &&
			(strings.HasPrefix(r.URL.Path, "/f/") || strings.HasPrefix(r.URL.Path, "/t/") || strings.HasPrefix(r.URL.Path, "/i/")) {
			http.Redirect(w, r, "https://"+files+r.URL.RequestURI(), http.StatusFound)
			return
		}
		mux.ServeHTTP(w, r)
	}))
}

// static serves the generated Nuxt SPA; unknown paths fall back to index.html.
func (s *Server) static() http.Handler {
	fs := http.FileServer(http.Dir(s.cfg.WebDir))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := path.Clean("/" + r.URL.Path)
		full := filepath.Join(s.cfg.WebDir, filepath.FromSlash(p))
		if st, err := os.Stat(full); err != nil || (st.IsDir() && p != "/") {
			index := filepath.Join(s.cfg.WebDir, "index.html")
			w.Header().Set("Cache-Control", "no-cache")
			w.Header().Set("Content-Security-Policy", pageCSP(index))
			http.ServeFile(w, r, index)
			return
		} else if st.IsDir() {
			w.Header().Set("Content-Security-Policy", pageCSP(filepath.Join(full, "index.html")))
		} else if strings.HasSuffix(p, ".html") {
			w.Header().Set("Content-Security-Policy", pageCSP(full))
		}
		if strings.HasPrefix(p, "/_nuxt/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			if servePrecompressed(w, r, full) {
				return
			}
		} else {
			w.Header().Set("Cache-Control", "no-cache")
		}
		fs.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// serverErr logs the cause and shows the user a generic, actionable message.
func serverErr(w http.ResponseWriter, what string, err error) {
	slog.Error(what, "err", err)
	writeErr(w, http.StatusBadGateway, "The mail server did not respond. Try again in a minute.")
}

func readJSON(r *http.Request, v any) error {
	dec := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 1<<20))
	return dec.Decode(v)
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// rateLimiter counts failed logins per IP in a sliding window.
type rateLimiter struct {
	mu     sync.Mutex
	max    int
	window time.Duration
	hits   map[string][]time.Time
}

func newRateLimiter(max int, window time.Duration) *rateLimiter {
	return &rateLimiter{max: max, window: window, hits: map[string][]time.Time{}}
}

func (l *rateLimiter) blocked(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	cut := time.Now().Add(-l.window)
	h := l.hits[key][:0]
	for _, t := range l.hits[key] {
		if t.After(cut) {
			h = append(h, t)
		}
	}
	if len(h) == 0 {
		delete(l.hits, key)
		return false
	}
	l.hits[key] = h
	return len(h) >= l.max
}

func (l *rateLimiter) fail(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.hits[key] = append(l.hits[key], time.Now())
	if len(l.hits) > 50000 { // a botnet: drop what is outside the window
		cut := time.Now().Add(-l.window)
		for k, h := range l.hits {
			if len(h) == 0 || h[len(h)-1].Before(cut) {
				delete(l.hits, k)
			}
		}
	}
}
