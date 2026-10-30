// preview — converts office documents to PDF with LibreOffice for the webmail preview, and renders
// first-page thumbnails (PNG) of PDFs and office documents with poppler.
// It runs without any network (Docker network_mode: none) and listens on a unix socket shared with
// the backend. One conversion at a time keeps memory use predictable on a small server.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"
)

const (
	socketPath = "/run/preview/preview.sock"
	maxInput   = 25 << 20
	timeout    = 90 * time.Second
)

var extRe = regexp.MustCompile(`^\.[a-z0-9]{1,5}$`)
var sem = make(chan struct{}, 1)

func convert(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	ext := strings.ToLower(filepath.Ext(r.URL.Query().Get("name")))
	if !extRe.MatchString(ext) {
		http.Error(w, "bad file name", http.StatusBadRequest)
		return
	}
	select {
	case sem <- struct{}{}:
		defer func() { <-sem }()
	case <-r.Context().Done():
		return
	}
	dir, err := os.MkdirTemp("", "conv-")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer os.RemoveAll(dir)
	in := filepath.Join(dir, "document"+ext)
	f, err := os.Create(in)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	n, err := io.Copy(f, io.LimitReader(r.Body, maxInput+1))
	f.Close()
	if err != nil || n > maxInput {
		http.Error(w, "file too large", http.StatusRequestEntityTooLarge)
		return
	}
	if r.URL.Path == "/html" {
		html(w, r, dir, in, ext)
		return
	}
	if r.URL.Path == "/ocr" {
		ocr(w, r, dir, in, ext)
		return
	}
	if r.URL.Path == "/text" {
		text(w, r, dir, in, ext)
		return
	}
	if r.URL.Path == "/thumb" {
		thumb(w, r, dir, in, ext)
		return
	}
	if r.URL.Query().Get("to") == "xlsx" {
		// Old spreadsheets (.xls, .ods) → .xlsx, which TMAIL renders as a grid itself.
		out := filepath.Join(dir, "document.xlsx")
		if !unoconvert(r.Context(), in, out, "xlsx", "") {
			ctx, cancel := context.WithTimeout(r.Context(), timeout)
			defer cancel()
			cmd := command(ctx, "soffice", "--headless", "--norestore", "--nologo", "--nodefault", "--nolockcheck",
				"-env:UserInstallation=file://"+filepath.Join(dir, "profile"), "--convert-to", "xlsx", "--outdir", dir, in)
			cmd.Env = append(os.Environ(), "HOME="+dir)
			_ = cmd.Run()
		}
		b, err := os.ReadFile(out)
		if err != nil || len(b) == 0 {
			http.Error(w, "conversion failed", http.StatusUnprocessableEntity)
			return
		}
		w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
		_, _ = w.Write(b)
		return
	}
	pdf, err := toPDF(r.Context(), dir, in, ext)
	if err != nil {
		http.Error(w, "conversion failed", http.StatusUnprocessableEntity)
		return
	}
	log.Printf("converted %s (%d KB → %d KB)", ext, n>>10, len(pdf)>>10)
	w.Header().Set("Content-Type", "application/pdf")
	_, _ = w.Write(pdf)
}

// html converts a spreadsheet to HTML tables (one per sheet) so the browser can show a real grid
// whose cells can be selected and copied.
func html(w http.ResponseWriter, r *http.Request, dir, in, ext string) {
	// Not through unoserver: the HTML (StarCalc) export crashes the shared LibreOffice. TMAIL
	// prepares this view in the background when the letter is opened, so nobody waits for it.
	ctx, cancel := context.WithTimeout(r.Context(), timeout)
	defer cancel()
	cmd := command(ctx, "soffice", "--headless", "--norestore", "--nologo", "--nodefault", "--nolockcheck",
		"-env:UserInstallation=file://"+filepath.Join(dir, "profile"), "--convert-to", "html:HTML (StarCalc):UTF8", "--outdir", dir, in)
	cmd.Env = append(os.Environ(), "HOME="+dir)
	if out, err := cmd.CombinedOutput(); err != nil {
		log.Printf("html %s failed: %v %s", ext, err, strings.TrimSpace(string(out)))
		http.Error(w, "conversion failed", http.StatusUnprocessableEntity)
		return
	}
	b, err := os.ReadFile(filepath.Join(dir, "document.html"))
	if err != nil {
		http.Error(w, "no output", http.StatusUnprocessableEntity)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(b)
}

// text extracts plain text (PDF: first pages with pdftotext; documents: LibreOffice) for the AI.
var sheetExt = map[string]bool{".xls": true, ".xlsx": true, ".xlsm": true, ".xlsb": true, ".ods": true, ".csv": true}

func text(w http.ResponseWriter, r *http.Request, dir, in, ext string) {
	ctx, cancel := context.WithTimeout(r.Context(), timeout)
	defer cancel()
	var out []byte
	var err error
	if ext == ".pdf" {
		out, err = command(ctx, "pdftotext", "-layout", "-l", "5", "-enc", "UTF-8", in, "-").Output()
	} else {
		// Spreadsheets have no plain-text export: they give CSV (UTF-8, ";"-separated).
		to, res := "txt:Text (encoded):UTF8", "document.txt"
		if sheetExt[ext] {
			to = "csv:Text - txt - csv (StarCalc):59,34,76,1,,0,false,true,false,false,false,-1"
		}
		cmd := command(ctx, "soffice", "--headless", "--norestore", "--nologo", "--nodefault", "--nolockcheck",
			"-env:UserInstallation=file://"+filepath.Join(dir, "profile"), "--convert-to", to, "--outdir", dir, in)
		cmd.Env = append(os.Environ(), "HOME="+dir)
		if b, e := cmd.CombinedOutput(); e != nil {
			err = fmt.Errorf("%v %s", e, strings.TrimSpace(string(b)))
		} else if !sheetExt[ext] {
			out, err = os.ReadFile(filepath.Join(dir, res))
		} else { // every sheet: document-<sheet>.csv
			files, _ := filepath.Glob(filepath.Join(dir, "document*.csv"))
			sort.Strings(files)
			for _, f := range files {
				b, e := os.ReadFile(f)
				if e != nil || f == in {
					continue
				}
				sheet := strings.TrimSuffix(strings.TrimPrefix(filepath.Base(f), "document-"), ".csv")
				out = append(out, "== "+sheet+" ==\n"...)
				for _, line := range strings.Split(string(b), "\n") { // empty cells say nothing
					if line = strings.TrimRight(line, ";\r "); line != "" {
						out = append(out, line+"\n"...)
					}
				}
			}
			if len(files) == 0 {
				err = errors.New("no sheets exported")
			}
		}
	}
	if err != nil {
		log.Printf("text %s failed: %v", ext, err)
		http.Error(w, "extract failed", http.StatusUnprocessableEntity)
		return
	}
	if len(out) > 200<<10 {
		out = out[:200<<10]
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write(out)
}

// thumb renders a page as PNG: the first page about 320 px wide by default, or ?page=N&width=W
// (used to show scanned documents to the AI).
func thumb(w http.ResponseWriter, r *http.Request, dir, in, ext string) {
	src := in
	if ext != ".pdf" {
		if _, err := toPDF(r.Context(), dir, in, ext); err != nil {
			http.Error(w, "conversion failed", http.StatusUnprocessableEntity)
			return
		}
		src = filepath.Join(dir, "document.pdf")
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	out := filepath.Join(dir, "thumb")
	page, width := "1", "320"
	if n, err := strconv.Atoi(r.URL.Query().Get("page")); err == nil && n >= 1 && n <= 20 {
		page = strconv.Itoa(n)
	}
	if n, err := strconv.Atoi(r.URL.Query().Get("width")); err == nil && n >= 100 && n <= 2000 {
		width = strconv.Itoa(n)
	}
	cmd := command(ctx, "pdftoppm", "-png", "-f", page, "-l", page, "-singlefile", "-scale-to-x", width, "-scale-to-y", "-1", src, out)
	if b, err := cmd.CombinedOutput(); err != nil {
		log.Printf("thumb %s failed: %v %s", ext, err, strings.TrimSpace(string(b)))
		http.Error(w, "render failed", http.StatusUnprocessableEntity)
		return
	}
	png, err := os.ReadFile(out + ".png")
	if err != nil {
		http.Error(w, "no output", http.StatusUnprocessableEntity)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	_, _ = w.Write(png)
}

var unoPID atomic.Int64 // process group of the running unoserver (its LibreOffice included)

var unoStarted atomic.Int64

func restartUnoserver(why string, crashed bool) {
	if !crashed && time.Since(time.Unix(unoStarted.Load(), 0)) < 45*time.Second {
		return // still starting: failures now are expected, not a broken LibreOffice
	}
	if pid := unoPID.Swap(0); pid > 0 {
		log.Printf("restarting LibreOffice: %s", why)
		_ = syscall.Kill(-int(pid), syscall.SIGKILL)
	}
}

// unoserver: a LibreOffice that stays running; restarted if it stops.
func runUnoserver() {
	for {
		cmd := exec.Command("unoserver", "--interface", "127.0.0.1", "--port", "2003", "--uno-interface", "127.0.0.1", "--uno-port", "2002")
		cmd.Env = append(os.Environ(), "HOME=/home/preview")
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		var buf bytes.Buffer
		cmd.Stdout, cmd.Stderr = &buf, &buf
		start := time.Now()
		err := cmd.Start()
		if err == nil {
			unoPID.Store(int64(cmd.Process.Pid))
			unoStarted.Store(time.Now().Unix())
			err = cmd.Wait()
			unoPID.Store(0)
		}
		out := buf.Bytes()
		log.Printf("unoserver stopped after %s: %v %s", time.Since(start).Round(time.Second), err, strings.TrimSpace(string(out)[max(0, len(out)-300):]))
		time.Sleep(3 * time.Second)
	}
}

// unoconvert converts with the running LibreOffice; false when it is not available (then the
// caller starts LibreOffice itself, which is slower but always works).
func unoconvert(parent context.Context, in, out, format, filter string) bool {
	// Old binary Excel (.xls): the running LibreOffice fails to open many of them and crashes on
	// the next document after that; a LibreOffice of its own opens them fine (a few seconds more).
	if strings.EqualFold(filepath.Ext(in), ".xls") {
		return false
	}
	ctx, cancel := context.WithTimeout(parent, 60*time.Second)
	defer cancel()
	args := []string{"--host", "127.0.0.1", "--port", "2003", "--convert-to", format}
	if filter != "" {
		args = append(args, "--filter", filter)
	}
	start := time.Now()
	b, err := command(ctx, "unoconvert", append(args, in, out)...).CombinedOutput()
	if err != nil {
		log.Printf("unoconvert %s → %s: %v %s", filepath.Ext(in), format, err, strings.TrimSpace(string(b)))
		// A document LibreOffice could not open leaves it broken: the next conversion kills it
		// ("bridge disposed") and every one after that fails. Start it afresh right away.
		if bytes.Contains(b, []byte("Disposed")) { // LibreOffice crashed: dead for all until restarted
			restartUnoserver("LibreOffice crashed", true)
		} else if parent.Err() == nil { // not just the reader who closed the document
			restartUnoserver("a conversion failed", false)
		}
		return false
	}
	log.Printf("unoconvert %s → %s in %s", filepath.Ext(in), format, time.Since(start).Round(time.Millisecond))
	return true
}

func toPDF(parent context.Context, dir, in, ext string) ([]byte, error) {
	if ext == ".pdf" {
		return os.ReadFile(in)
	}
	if out := filepath.Join(dir, "document.pdf"); unoconvert(parent, in, out, "pdf", "") {
		if b, err := os.ReadFile(out); err == nil && len(b) > 0 {
			return b, nil
		}
	}
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	cmd := command(ctx, "soffice", "--headless", "--norestore", "--nologo", "--nodefault", "--nolockcheck",
		"-env:UserInstallation=file://"+filepath.Join(dir, "profile"), "--convert-to", "pdf", "--outdir", dir, in)
	cmd.Env = append(os.Environ(), "HOME="+dir)
	start := time.Now()
	if out, err := cmd.CombinedOutput(); err != nil {
		log.Printf("convert %s failed after %s: %v %s", ext, time.Since(start), err, strings.TrimSpace(string(out)))
		return nil, err
	}
	return os.ReadFile(filepath.Join(dir, "document.pdf"))
}

func main() {
	_ = os.Remove(socketPath)
	l, err := net.Listen("unix", socketPath)
	if err != nil {
		log.Fatal(err)
	}
	_ = os.Chmod(socketPath, 0o666)
	http.HandleFunc("/convert", convert)
	http.HandleFunc("/thumb", convert)
	http.HandleFunc("/html", convert)
	http.HandleFunc("/text", convert)
	http.HandleFunc("/ocr", convert)
	http.HandleFunc("/health", health)
	go runUnoserver()
	go watchdog()
	log.Printf("preview listening on %s", socketPath)
	log.Fatal(http.Serve(l, nil))
}

// ---------- OCR (text on pictures and scans) ----------

type ocrWord struct {
	T    string `json:"t"`
	X    int    `json:"x"`
	Y    int    `json:"y"`
	W    int    `json:"w"`
	H    int    `json:"h"`
	Line int    `json:"l"`
}

type ocrPage struct {
	W     int       `json:"w"`
	H     int       `json:"h"`
	Words []ocrWord `json:"words"`
}

const ocrMaxPages = 5

// ocr recognises Russian and English text with Tesseract and returns words with their boxes, so
// the browser can lay invisible selectable text over the picture. PDFs: the first pages, rendered.
func ocr(w http.ResponseWriter, r *http.Request, dir, in, ext string) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Minute)
	defer cancel()
	var images []string
	if ext == ".pdf" {
		out := filepath.Join(dir, "pg")
		if b, err := command(ctx, "pdftoppm", "-r", "200", "-png", "-f", "1", "-l", strconv.Itoa(ocrMaxPages), in, out).CombinedOutput(); err != nil {
			log.Printf("ocr render: %v %s", err, b)
			http.Error(w, "render failed", http.StatusUnprocessableEntity)
			return
		}
		files, _ := filepath.Glob(out + "-*.png")
		sort.Strings(files)
		images = files
	} else {
		images = []string{in}
	}
	res := struct {
		Pages []ocrPage `json:"pages"`
		Text  string    `json:"text"`
	}{Pages: []ocrPage{}}
	var all strings.Builder
	for _, img := range images {
		f, err := os.Open(img)
		if err != nil {
			continue
		}
		cfg, _, err := image.DecodeConfig(f)
		f.Close()
		if err != nil {
			log.Printf("ocr: unknown image %s: %v", ext, err)
			continue
		}
		out, err := command(ctx, "tesseract", img, "stdout", "-l", "rus+eng", "--psm", "3", "tsv").Output()
		if err != nil {
			log.Printf("ocr: tesseract: %v", err)
			continue
		}
		pg := ocrPage{W: cfg.Width, H: cfg.Height, Words: []ocrWord{}}
		lineIdx, lastKey := -1, ""
		for i, row := range strings.Split(string(out), "\n") {
			c := strings.Split(row, "\t")
			if i == 0 || len(c) < 12 || c[0] != "5" || strings.TrimSpace(c[11]) == "" {
				continue
			}
			if conf, _ := strconv.ParseFloat(c[10], 64); conf < 20 {
				continue
			}
			key := c[2] + "." + c[3] + "." + c[4]
			if key != lastKey {
				lineIdx++
				lastKey = key
				if all.Len() > 0 {
					all.WriteString("\n")
				}
			} else {
				all.WriteString(" ")
			}
			n := func(k int) int { v, _ := strconv.Atoi(c[k]); return v }
			pg.Words = append(pg.Words, ocrWord{T: c[11], X: n(6), Y: n(7), W: n(8), H: n(9), Line: lineIdx})
			all.WriteString(c[11])
		}
		all.WriteString("\n")
		res.Pages = append(res.Pages, pg)
	}
	res.Text = strings.TrimSpace(all.String())
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(res)
}

// command runs a program that is killed with all its children when ctx ends: "soffice" is a
// wrapper script, and killing only it left LibreOffice running — and Wait waiting for its output
// for ever, so every later preview (one at a time) hung behind it.
func command(ctx context.Context, name string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	cmd.WaitDelay = 5 * time.Second
	return cmd
}

// watchdog keeps the service usable without anyone noticing a failure first:
//   - LibreOffice inside unoserver can die while unoserver itself keeps running ("bridge already
//     disposed"): a test conversion every 2 minutes, and the whole group restarted when it fails;
//   - processes that pile up (killed conversions not reaped, "Cannot fork"): the service exits
//     and Docker starts it afresh.
func watchdog() {
	time.Sleep(90 * time.Second) // LibreOffice starts slowly
	dir, _ := os.MkdirTemp("", "selftest-")
	in := filepath.Join(dir, "t.txt")
	_ = os.WriteFile(in, []byte("TMAIL self-test\n"), 0o644)
	fails := 0
	for {
		if n := countProcs(); n > 300 {
			log.Printf("watchdog: %d processes — restarting the service", n)
			os.Exit(1)
		}
		out := filepath.Join(dir, "t.pdf")
		_ = os.Remove(out)
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		ok := false
		select {
		case sem <- struct{}{}: // between real conversions, not in their way
			ok = unoconvert(ctx, in, out, "pdf", "")
			<-sem
		case <-ctx.Done():
			ok = true // busy with real work for 45 s: that is not a failure
		}
		cancel()
		if ok {
			fails = 0
			selfTestOK.Store(time.Now().Unix())
		} else if fails++; fails >= 2 {
			restartUnoserver("the self-test does not convert", true)
			fails = 0
		}
		time.Sleep(2 * time.Minute)
	}
}

var selfTestOK atomic.Int64 // when LibreOffice last converted the test file

// health: "ok" while the service converts; 503 when the self-test has not passed for 10 minutes
// (the server's watchdog restarts the service then).
func health(w http.ResponseWriter, _ *http.Request) {
	last := selfTestOK.Load()
	if last == 0 || time.Since(time.Unix(last, 0)) < 10*time.Minute {
		_, _ = io.WriteString(w, "ok")
		return
	}
	http.Error(w, fmt.Sprintf("self-test failing since %s, %d processes", time.Unix(last, 0).Format(time.RFC3339), countProcs()), http.StatusServiceUnavailable)
}

// countProcs: processes in this container (zombies included).
func countProcs() int {
	n := 0
	entries, _ := os.ReadDir("/proc")
	for _, e := range entries {
		if e.IsDir() && e.Name()[0] >= '0' && e.Name()[0] <= '9' {
			n++
		}
	}
	return n
}
