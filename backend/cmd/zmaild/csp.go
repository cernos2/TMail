package main

import (
	"crypto/sha256"
	"encoding/base64"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"
)

// Content-Security-Policy for the SPA pages. Nuxt puts two inline scripts into every page
// (import map and runtime config); they are allowed by hash, so no other script can run even if
// something slips past the sanitiser. The hashes are taken from the files themselves because the
// web build is redeployed without restarting the server.

var reInlineScript = regexp.MustCompile(`(?s)<script(?:\s[^>]*)?>(.*?)</script>`)

type cspEntry struct {
	mod    time.Time
	policy string
}

var cspCache sync.Map // path → cspEntry

func pageCSP(file string) string {
	st, err := os.Stat(file)
	if err != nil {
		return ""
	}
	if e, ok := cspCache.Load(file); ok && e.(cspEntry).mod.Equal(st.ModTime()) {
		return e.(cspEntry).policy
	}
	b, err := os.ReadFile(file)
	if err != nil {
		return ""
	}
	var hashes []string
	for _, m := range reInlineScript.FindAllSubmatch(b, -1) {
		tag := string(m[0][:strings.Index(string(m[0]), ">")])
		if len(m[1]) == 0 || strings.Contains(tag, " src=") || strings.Contains(tag, `type="application/json"`) {
			continue
		}
		sum := sha256.Sum256(m[1])
		hashes = append(hashes, "'sha256-"+base64.StdEncoding.EncodeToString(sum[:])+"'")
	}
	policy := strings.Join([]string{
		"default-src 'self'",
		"script-src 'self' " + strings.Join(hashes, " "),
		"style-src 'self' 'unsafe-inline'",
		// Letters are shown in srcdoc frames, which inherit this policy: remote pictures are
		// limited further by the frame's own policy.
		"img-src 'self' data: blob: https: http:",
		"font-src 'self' data: https:",
		"connect-src 'self' blob: data:",
		"worker-src 'self' blob:",
		"frame-src 'self' blob: data:",
		"media-src 'self' blob:",
		"object-src 'none'",
		"base-uri 'self'",
		"form-action 'self'",
		"frame-ancestors 'self'",
	}, "; ")
	cspCache.Store(file, cspEntry{st.ModTime(), policy})
	return policy
}
