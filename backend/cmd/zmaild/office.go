package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Office documents are previewed as PDF. Conversion runs in the isolated "preview" container
// (LibreOffice, no network) reached over a unix socket; results are cached on disk by content hash,
// so opening the same file again — or the same file in another message — is instant.

// Spreadsheets are shown as an HTML grid, other documents as PDF pages.
var sheetExts = map[string]bool{".xls": true, ".xlsx": true, ".xlsm": true, ".ods": true}

var officeExts = map[string]bool{".doc": true, ".docx": true, ".docm": true, ".rtf": true, ".odt": true,
	".xls": true, ".xlsx": true, ".xlsm": true, ".ods": true, ".ppt": true, ".pptx": true, ".odp": true, ".pps": true, ".ppsx": true}

const (
	officeMaxInput = 25 << 20
	officeCacheTTL = 14 * 24 * time.Hour
)

var (
	officeClient = &http.Client{
		Timeout: 100 * time.Second,
		Transport: &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "unix", env("PREVIEW_SOCKET", "/run/preview/preview.sock"))
		}},
	}
	officeMu      sync.Mutex
	officePending = map[string]*officeJob{}
	officeDir     = env("PREVIEW_CACHE_DIR", filepath.Join(os.TempDir(), "tmail-office")) // a volume, so it survives updates
	officeSweep   time.Time
)

type officeJob struct {
	done chan struct{}
	pdf  []byte
	err  error
}

func (s *Server) officeToPDF(ctx context.Context, name string, data []byte) ([]byte, error) {
	return s.officeRender(ctx, "convert", name, data)
}

// sheetHTML converts a spreadsheet into sanitized HTML tables.
func (s *Server) sheetHTML(ctx context.Context, name string, data []byte) ([]byte, error) {
	if out, err := s.sheetHTMLFast(ctx, name, data); err == nil && len(out) > 0 {
		return out, nil
	} else if err != nil {
		log.Printf("sheet fast path %q: %v — using LibreOffice", name, err)
	}
	raw, err := s.officeRender(ctx, "html", name, data)
	if err != nil {
		return nil, err
	}
	return []byte(sheetPolicy.Sanitize(string(raw))), nil
}

// officeThumb renders the first page of a PDF or office document as a PNG.
func (s *Server) officeThumb(ctx context.Context, name string, data []byte) ([]byte, error) {
	return s.officeRender(ctx, "thumb", name, data)
}

func (s *Server) officeRender(ctx context.Context, kind, name string, data []byte) ([]byte, error) {
	return s.officeRenderQ(ctx, kind, "", name, data)
}

// officeText extracts plain text from a PDF or office document.
func (s *Server) officeText(ctx context.Context, name string, data []byte) (string, error) {
	b, err := s.officeRenderQ(ctx, "text", "", name, data)
	return string(b), err
}

// officePage renders page n of a PDF or document as a PNG of the given width.
func (s *Server) officePage(ctx context.Context, name string, data []byte, n, width int) ([]byte, error) {
	return s.officeRenderQ(ctx, "thumb", fmt.Sprintf("&page=%d&width=%d", n, width), name, data)
}

func (s *Server) officeRenderQ(ctx context.Context, kind, extra, name string, data []byte) ([]byte, error) {
	if len(data) > officeMaxInput {
		return nil, errors.New("document too large")
	}
	sum := sha256.Sum256(data)
	key := hex.EncodeToString(sum[:]) + "." + kind + strings.NewReplacer("&", ".", "=", "").Replace(extra)
	file := filepath.Join(officeDir, key)
	if pdf, err := os.ReadFile(file); err == nil {
		return pdf, nil
	}
	// Several people opening the same attachment share one conversion.
	officeMu.Lock()
	job, running := officePending[key]
	if !running {
		job = &officeJob{done: make(chan struct{})}
		officePending[key] = job
	}
	officeMu.Unlock()
	if !running {
		go func() {
			job.pdf, job.err = convertOffice(kind, extra, name, data)
			if job.err == nil {
				_ = os.MkdirAll(officeDir, 0o700)
				_ = os.WriteFile(file, job.pdf, 0o600)
				sweepOfficeCache()
			}
			officeMu.Lock()
			delete(officePending, key)
			officeMu.Unlock()
			close(job.done)
		}()
	}
	select {
	case <-job.done:
		return job.pdf, job.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func convertOffice(kind, extra, name string, data []byte) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://preview/"+kind+"?name="+url.QueryEscape(filepath.Base(name))+extra, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	resp, err := officeClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 100<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("converter: %d %s", resp.StatusCode, bytes.TrimSpace(body))
	}
	if kind == "convert" && extra == "" && !bytes.HasPrefix(body, []byte("%PDF")) || kind == "thumb" && !bytes.HasPrefix(body, []byte("\x89PNG")) || (kind == "html" || kind == "ocr") && len(body) == 0 {
		return nil, errors.New("converter returned unexpected data")
	}
	return body, nil
}

// sweepOfficeCache removes old conversions, at most once an hour.
func sweepOfficeCache() {
	officeMu.Lock()
	if time.Since(officeSweep) < time.Hour {
		officeMu.Unlock()
		return
	}
	officeSweep = time.Now()
	officeMu.Unlock()
	entries, _ := os.ReadDir(officeDir)
	for _, e := range entries {
		if info, err := e.Info(); err == nil && time.Since(info.ModTime()) > officeCacheTTL {
			_ = os.Remove(filepath.Join(officeDir, e.Name()))
		}
	}
}
