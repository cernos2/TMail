package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"html/template"
	"image"
	"image/jpeg"
	"io"
	"log"
	"mime"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"golang.org/x/image/draw"
)

// Large files are not attached but uploaded to S3 storage (Garage, a separate container) and sent
// as a link card in the letter body. TMAIL serves the links itself at https://files.<domain>/f/<token>:
// a page with a preview and a download button; the file is kept for bigFileDays days.

const (
	bigFileMax = 2 << 30 // 2 GB per file
	// bigFileUserQuota: what one user may keep in links at a time.
	bigFileUserQuota = 20 << 30
	bigFileDays      = 30
	thumbSrcMax      = 40 << 20
)

var (
	s3Once   sync.Once
	s3Client *minio.Client
	s3Err    error
)

func (s *Server) s3() (*minio.Client, error) {
	s3Once.Do(func() {
		ak, sk := env("S3_ACCESS_KEY", ""), env("S3_SECRET_KEY", "")
		if ak == "" || sk == "" {
			s3Err = fmt.Errorf("file storage is not configured")
			return
		}
		s3Client, s3Err = minio.New(env("S3_ENDPOINT", "127.0.0.1:3900"), &minio.Options{
			Creds: credentials.NewStaticV4(ak, sk, ""), Secure: false, Region: "garage", BucketLookup: minio.BucketLookupPath,
		})
	})
	return s3Client, s3Err
}

func s3Bucket() string { return env("S3_BUCKET", "tmail-files") }

func (s *Server) filesHost() string {
	if h := env("FILES_HOSTNAME", ""); h != "" {
		return h
	}
	return s.cfg.Hostname
}

func newToken() string {
	b := make([]byte, 15)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

type bigFileInfo struct {
	Token   string    `json:"token"`
	URL     string    `json:"url"`
	Thumb   string    `json:"thumb,omitempty"`
	Name    string    `json:"name"`
	Size    int64     `json:"size"`
	Mime    string    `json:"mime"`
	Expires time.Time `json:"expires"`
}

// POST /api/bigfiles?name=&size=&mailbox= with the raw file as the body (streamed to S3).
func (s *Server) handleBigUpload(w http.ResponseWriter, r *http.Request, sess *Session) {
	c, err := s.s3()
	if err != nil {
		writeErr(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	q := r.URL.Query()
	name := strings.TrimSpace(filepath.Base(strings.ReplaceAll(q.Get("name"), "\\", "/")))
	size, _ := strconv.ParseInt(q.Get("size"), 10, 64)
	if name == "" || name == "." || size <= 0 {
		writeErr(w, http.StatusBadRequest, "No file")
		return
	}
	if size > bigFileMax {
		writeErr(w, http.StatusRequestEntityTooLarge, "The file is larger than 2 GB and cannot be sent")
		return
	}
	var used int64
	_ = s.db.QueryRow(r.Context(), `SELECT COALESCE(SUM(size),0) FROM big_files WHERE user_id=$1 AND expires_at > now()`, sess.UserID).Scan(&used)
	if used+size > bigFileUserQuota {
		writeErr(w, http.StatusRequestEntityTooLarge, fmt.Sprintf("The limit for files sent as links is %d GB per user per %d days. Old links free up space when they expire.", bigFileUserQuota>>30, bigFileDays))
		return
	}
	mailbox := q.Get("mailbox")
	if _, ok := sess.account(mailbox); !ok {
		mailbox = ""
	}
	ct := r.Header.Get("Content-Type")
	if ct == "" || strings.HasPrefix(ct, "multipart/") {
		ct = mime.TypeByExtension(strings.ToLower(filepath.Ext(name)))
	}
	if ct == "" {
		ct = "application/octet-stream"
	}
	token := newToken()
	key := time.Now().Format("2006/01/") + token
	body := http.MaxBytesReader(w, r.Body, size)
	if _, err := c.PutObject(r.Context(), s3Bucket(), key, body, size, minio.PutObjectOptions{ContentType: ct}); err != nil {
		log.Printf("bigfile upload %q: %v", name, err)
		writeErr(w, http.StatusBadGateway, "Could not save the file to storage")
		return
	}
	// Antivirus (first 100 MB), read back from the local storage. A slow or busy antivirus
	// (it pauses while loading new signatures) must not hold the upload: after the time limit
	// the link is created anyway, as for incoming mail.
	avStatus := "clean"
	if found, err := s.scanStored(r.Context(), c, key, 90*time.Second); err != nil {
		log.Printf("bigfile antivirus %q: %v", name, err)
		avStatus = "pending" // the link works, the download opens once the background check is done
	} else if k := avKind(found); k == "encrypted" || k == "unchecked" || k == "heuristic" {
		_ = c.RemoveObject(context.Background(), s3Bucket(), key, minio.RemoveObjectOptions{})
		writeErr(w, http.StatusUnprocessableEntity, fmt.Sprintf("%s The link was not created.", lockedText(name, k)))
		return
	} else if avKind(found) == "virus" {
		_ = c.RemoveObject(context.Background(), s3Bucket(), key, minio.RemoveObjectOptions{})
		s.audit(r.Context(), sess.UserID, sess.Login, clientIP(r), "av.upload_blocked", name, map[string]any{"virus": found})
		writeErr(w, http.StatusUnprocessableEntity, fmt.Sprintf("A virus was found in %q (%s). The link was not created.", name, found))
		return
	}
	exp := time.Now().AddDate(0, 0, bigFileDays)
	if _, err := s.db.Exec(r.Context(), `INSERT INTO big_files(token, s3_key, name, mime, size, user_id, mailbox, sender_name, expires_at, av_status)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, token, key, name, ct, size, sess.UserID, mailbox, sess.Name, exp, avStatus); err != nil {
		serverErr(w, "bigfile save", err)
		return
	}
	info := bigFileInfo{Token: token, URL: "https://" + s.filesHost() + "/f/" + token, Name: name, Size: size, Mime: ct, Expires: exp}
	// Thumbnail for pictures, PDFs and documents (made now, so it can go into the letter card) —
	// only for a checked file: the picture is public.
	if size <= thumbSrcMax && avStatus == "clean" {
		if s.makeBigThumb(r.Context(), c, key, token, name, ct) {
			info.Thumb = "https://" + s.filesHost() + "/t/" + token + ".jpg"
		}
	}
	s.audit(r.Context(), sess.UserID, sess.Login, clientIP(r), "bigfile.upload", name, map[string]any{"size": size})
	writeJSON(w, http.StatusOK, info)
}

func (s *Server) makeBigThumb(ctx context.Context, c *minio.Client, key, token, name, ct string) bool {
	kind := previewKind(ct, name)
	if kind != "image" && kind != "pdf" && kind != "office" && kind != "sheet" {
		return false
	}
	obj, err := c.GetObject(ctx, s3Bucket(), key, minio.GetObjectOptions{})
	if err != nil {
		return false
	}
	data, err := io.ReadAll(io.LimitReader(obj, thumbSrcMax+1))
	obj.Close()
	if err != nil || len(data) > thumbSrcMax {
		return false
	}
	var img image.Image
	if kind == "image" {
		img, err = decodeImageBytes(data)
	} else {
		n := name
		if kind == "pdf" && !strings.HasSuffix(strings.ToLower(n), ".pdf") {
			n += ".pdf"
		}
		var png []byte
		if png, err = s.officeThumb(ctx, n, data); err == nil {
			img, err = decodeImageBytes(png)
		}
	}
	if err != nil || img == nil {
		return false
	}
	// Square-ish 240×180 preview, cropped from the top (documents) or the centre (photos).
	const tw, th = 240, 180
	b := img.Bounds()
	src := b
	if float64(b.Dx())/float64(b.Dy()) > float64(tw)/float64(th) {
		w := b.Dy() * tw / th
		src = image.Rect(b.Min.X+(b.Dx()-w)/2, b.Min.Y, b.Min.X+(b.Dx()-w)/2+w, b.Max.Y)
	} else {
		h := b.Dx() * th / tw
		top := b.Min.Y
		if kind == "image" {
			top += (b.Dy() - h) / 2
		}
		src = image.Rect(b.Min.X, top, b.Max.X, top+h)
	}
	dst := image.NewRGBA(image.Rect(0, 0, tw, th))
	draw.CatmullRom.Scale(dst, dst.Bounds(), img, src, draw.Src, nil)
	var out bytes.Buffer
	if err := jpeg.Encode(&out, dst, &jpeg.Options{Quality: 82}); err != nil {
		return false
	}
	if _, err := c.PutObject(ctx, s3Bucket(), "thumbs/"+token+".jpg", bytes.NewReader(out.Bytes()), int64(out.Len()), minio.PutObjectOptions{ContentType: "image/jpeg"}); err != nil {
		return false
	}
	_, _ = s.db.Exec(ctx, `UPDATE big_files SET thumb=true WHERE token=$1`, token)
	return true
}

// ---------- public side: files.<domain> ----------

type bigFileRow struct {
	Token, Key, Name, Mime, Sender string
	Size                           int64
	Thumb                          bool
	Downloads                      int
	Created, Expires               time.Time
	AvStatus, AvFound              string // pending | clean | infected
}

func (s *Server) bigFile(ctx context.Context, token string) (bigFileRow, bool) {
	var f bigFileRow
	err := s.db.QueryRow(ctx, `SELECT token, s3_key, name, mime, sender_name, size, thumb, downloads, created_at, expires_at, av_status, av_found
		FROM big_files WHERE token=$1 AND expires_at > now()`, token).
		Scan(&f.Token, &f.Key, &f.Name, &f.Mime, &f.Sender, &f.Size, &f.Thumb, &f.Downloads, &f.Created, &f.Expires, &f.AvStatus, &f.AvFound)
	return f, err == nil
}

// filesRoutes serves the link pages; mounted for the files host (and under the mail host too).
func (s *Server) filesRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /f/{token}", s.filePage)
	mux.HandleFunc("GET /f/{token}/download", s.fileDownload)
	mux.HandleFunc("GET /t/{name}", s.fileThumb)
	mux.HandleFunc("GET /i/{name}", s.sigImage)
}

func (s *Server) fileThumb(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimSuffix(r.PathValue("name"), ".jpg")
	f, ok := s.bigFile(r.Context(), token)
	c, err := s.s3()
	if !ok || !f.Thumb || err != nil || f.AvStatus != "clean" {
		http.NotFound(w, r)
		return
	}
	obj, err := c.GetObject(r.Context(), s3Bucket(), "thumbs/"+token+".jpg", minio.GetObjectOptions{})
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer obj.Close()
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	_, _ = io.Copy(w, obj)
}

func (s *Server) fileDownload(w http.ResponseWriter, r *http.Request) {
	f, ok := s.bigFile(r.Context(), r.PathValue("token"))
	c, err := s.s3()
	if !ok || err != nil {
		s.fileGone(w, r)
		return
	}
	if f.AvStatus != "clean" { // not checked yet, or malware: the page explains
		http.Redirect(w, r, "/f/"+f.Token, http.StatusSeeOther)
		return
	}
	obj, err := c.GetObject(r.Context(), s3Bucket(), f.Key, minio.GetObjectOptions{})
	if err != nil {
		s.fileGone(w, r)
		return
	}
	defer obj.Close()
	if r.URL.Query().Get("inline") != "1" && (r.Header.Get("Range") == "" || strings.HasPrefix(r.Header.Get("Range"), "bytes=0-")) {
		_, _ = s.db.Exec(r.Context(), `UPDATE big_files SET downloads=downloads+1, last_download=now() WHERE token=$1`, f.Token)
	}
	disp := "attachment"
	if r.URL.Query().Get("inline") == "1" && inlineSafe(f.Mime) {
		disp = "inline"
	}
	// The stored type came from the uploader's browser: only pictures and PDF are shown as such.
	if disp == "inline" {
		w.Header().Set("Content-Type", f.Mime)
	} else {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Security-Policy", "sandbox")
	}
	w.Header().Set("Content-Disposition", mime.FormatMediaType(disp, map[string]string{"filename": f.Name}))
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeContent(w, r, "", f.Created, obj) // Range requests: resumable downloads
}

func (s *Server) fileGone(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusNotFound)
	_ = filePageTpl.Execute(w, pageLocale(r, map[string]any{"Gone": true, "Status": ""}))
}

func (s *Server) filePage(w http.ResponseWriter, r *http.Request) {
	f, ok := s.bigFile(r.Context(), r.PathValue("token"))
	if !ok {
		s.fileGone(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; img-src 'self'; style-src 'unsafe-inline'; frame-ancestors 'none'")
	ext := strings.ToUpper(strings.TrimPrefix(filepath.Ext(f.Name), "."))
	_ = filePageTpl.Execute(w, pageLocale(r, map[string]any{
		"Name": f.Name, "Size": humanSize(f.Size), "Ext": ext, "Sender": f.Sender, "Token": f.Token, "Thumb": f.Thumb,
		"Image":  strings.HasPrefix(f.Mime, "image/") && inlineSafe(f.Mime) && f.AvStatus == "clean",
		"Status": f.AvStatus, "VirusNote": parenthesized(f.AvFound), "Locked": avKind(f.AvFound) == "encrypted" || avKind(f.AvFound) == "unchecked" || avKind(f.AvFound) == "heuristic",
		"Expires": f.Expires.Format("2006-01-02"),
	}))
}

func humanSize(n int64) string {
	switch {
	case n >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(n)/(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%d KB", n>>10)
	}
	return fmt.Sprintf("%d B", n)
}

var filePageTpl = template.Must(template.New("f").Parse(`<!doctype html><html lang="{{.Lang}}" dir="{{.Dir}}"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1"><meta name="robots" content="noindex">{{if eq .Status "pending"}}<meta http-equiv="refresh" content="30">{{end}}
<title>{{if .Gone}}{{call .T "filepage.unavailable"}}{{else}}{{.Name}}{{end}} — {{.Org}}</title>
<style>
*{box-sizing:border-box}body{margin:0;min-height:100vh;display:grid;place-items:center;background:#eef2f6;font:15px/1.45 Arial,Helvetica,sans-serif;color:#1d2733;padding:24px}
.card{background:#fff;border-radius:9px;box-shadow:0 10px 40px rgba(20,40,70,.12);width:min(520px,100%);overflow:hidden}
.brand{display:flex;align-items:center;gap:10px;padding:16px 22px;border-bottom:1px solid #e6ebf0;font-weight:700;letter-spacing:.04em;color:#0a5fd7}
.brand i{width:10px;height:10px;border-radius:1.5px;background:linear-gradient(135deg,#0a5fd7,#20c5f5)}
.pv{background:#f4f6f9;display:grid;place-items:center;min-height:180px;border-bottom:1px solid #e6ebf0}
.pv{padding:18px}.pv img{display:block;max-width:100%;max-height:360px;border-radius:3px;box-shadow:0 2px 12px rgba(20,40,70,.18);background:#fff}
.ext{width:84px;height:104px;border-radius:5px;background:#fff;border:1px solid #dfe5ec;display:grid;place-items:center;font-weight:700;color:#0a5fd7;font-size:18px;box-shadow:0 2px 6px rgba(0,0,0,.05)}
.body{padding:20px 22px 24px}h1{font-size:18px;margin:0 0 6px;word-break:break-word}
.meta{color:#5f6b78;font-size:13.5px;margin:0 0 18px}
.btn{display:block;text-align:center;background:#0a5fd7;color:#fff;text-decoration:none;font-weight:700;padding:13px 16px;border-radius:6px}
.btn:hover{background:#084fb4}.av{font-size:13px;margin:12px 0 0;text-align:center}.av.ok{color:#1e8048}
.av.bad,.av.wait{margin:0;padding:12px 14px;border-radius:6px;text-align:left}.av.bad{background:#fdecea;color:#b3261e}.av.wait{background:#fff6e0;color:#7a5200}.foot{color:#8a96a3;font-size:12px;margin-top:14px;text-align:center}
</style></head><body><div class="card"><div class="brand"><i></i>{{.Org}}</div>
{{if .Gone}}<div class="body"><h1>{{call .T "filepage.goneTitle"}}</h1><p class="meta">{{call .T "filepage.goneText"}}</p></div>
{{else}}<div class="pv">{{if .Image}}<img src="/f/{{.Token}}/download?inline=1" alt="">{{else if .Thumb}}<img src="/t/{{.Token}}.jpg" alt="">{{else}}<div class="ext">{{.Ext}}</div>{{end}}</div>
<div class="body"><h1>{{.Name}}</h1><p class="meta">{{.Size}}{{if .Sender}} · {{call .T "filepage.sentBy" "sender" .Sender}}{{end}}<br>{{call .T "filepage.validUntil" "date" .Expires}}</p>
{{if eq .Status "clean"}}<a class="btn" href="/f/{{.Token}}/download">{{call .T "filepage.download"}}</a><p class="av ok">✓ {{call .T "filepage.checked"}}</p>
{{else if .Locked}}<p class="av bad">{{call .T "filepage.locked"}}</p>
{{else if eq .Status "infected"}}<p class="av bad">{{call .T "filepage.infected" "virus" .VirusNote}}</p>
{{else}}<p class="av wait">{{call .T "filepage.checking"}}</p>{{end}}
<p class="foot">{{call .T "filepage.foot" "org" .Org}}</p></div>{{end}}
</div></body></html>`))

// ---------- expiry ----------

func (s *Server) runBigFileCleanup(ctx context.Context) {
	t := time.NewTicker(6 * time.Hour)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			c, err := s.s3()
			if err != nil {
				continue
			}
			rows, err := s.db.Query(ctx, `SELECT id, s3_key, token FROM big_files WHERE expires_at < now() LIMIT 500`)
			if err != nil {
				continue
			}
			type x struct {
				id         int64
				key, token string
			}
			var list []x
			for rows.Next() {
				var v x
				if rows.Scan(&v.id, &v.key, &v.token) == nil {
					list = append(list, v)
				}
			}
			rows.Close()
			for _, v := range list {
				_ = c.RemoveObject(ctx, s3Bucket(), v.key, minio.RemoveObjectOptions{})
				_ = c.RemoveObject(ctx, s3Bucket(), "thumbs/"+v.token+".jpg", minio.RemoveObjectOptions{})
				_, _ = s.db.Exec(ctx, `DELETE FROM big_files WHERE id=$1`, v.id)
			}
		}
	}
}

func (s *Server) scanStored(ctx context.Context, c *minio.Client, key string, limit time.Duration) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, limit)
	defer cancel()
	obj, err := c.GetObject(ctx, s3Bucket(), key, minio.GetObjectOptions{})
	if err != nil {
		return "", err
	}
	defer obj.Close()
	found, err := clamScan(ctx, obj)
	if errors.Is(err, errTooLarge) {
		return "Heuristics.Limits.Exceeded.Size", nil
	}
	return found, err
}

// runBigFileScanner finishes the antivirus check of links whose scan did not complete at upload
// (and of links made before the antivirus existed). Malware is deleted from the storage.
func (s *Server) runBigFileScanner(ctx context.Context) {
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		c, err := s.s3()
		if err != nil {
			continue
		}
		rows, err := s.db.Query(ctx, `SELECT token, s3_key, name, mime, size FROM big_files WHERE av_status='pending' AND expires_at > now()
			AND (av_next_at IS NULL OR av_next_at < now()) ORDER BY created_at LIMIT 10`)
		if err != nil {
			continue
		}
		type item struct {
			token, key, name, mime string
			size                   int64
		}
		var todo []item
		for rows.Next() {
			var x item
			if rows.Scan(&x.token, &x.key, &x.name, &x.mime, &x.size) == nil {
				todo = append(todo, x)
			}
		}
		rows.Close()
		for _, x := range todo {
			found, err := s.scanStored(ctx, c, x.key, 30*time.Minute) // a 2 GB file takes minutes
			if errors.Is(err, errClamDown) {
				break // antivirus busy or down: everything waits for the next minute
			}
			if err != nil {
				// This file only (missing object, scan too slow…): try again later, give up after
				// 6 attempts — then it is "unchecked" and is never handed out.
				log.Printf("bigfile rescan %q: %v", x.name, err)
				_, _ = s.db.Exec(ctx, `UPDATE big_files SET av_tries=av_tries+1, av_next_at=now() + (interval '10 minutes' * (av_tries+1)),
					av_status=CASE WHEN av_tries+1 >= 6 THEN 'infected' ELSE av_status END,
					av_found=CASE WHEN av_tries+1 >= 6 THEN 'Heuristics.Limits.Exceeded.Unscannable' ELSE av_found END WHERE token=$1`, x.token)
				continue
			}
			status := "clean"
			if k := avKind(found); k == "virus" || k == "encrypted" || k == "unchecked" || k == "heuristic" {
				status = "infected" // or impossible to check: never handed out
				_ = c.RemoveObject(ctx, s3Bucket(), x.key, minio.RemoveObjectOptions{})
				_ = c.RemoveObject(ctx, s3Bucket(), "thumbs/"+x.token+".jpg", minio.RemoveObjectOptions{})
				log.Printf("bigfile %q: %s — removed", x.name, found)
			}
			_, _ = s.db.Exec(ctx, `UPDATE big_files SET av_status=$2, av_found=$3 WHERE token=$1`, x.token, status, found)
			if status == "clean" && x.size <= thumbSrcMax { // the public picture appears only now
				s.makeBigThumb(ctx, c, x.key, x.token, x.name, x.mime)
			}
		}
	}
}

// DELETE /api/bigfiles/{token} — a large file taken out of a letter or left in a deleted draft:
// removed at once, if it is the user's own and nobody has downloaded it yet.
func (s *Server) handleBigDelete(w http.ResponseWriter, r *http.Request, sess *Session) {
	var id int64
	var key string
	token := r.PathValue("token")
	if err := s.db.QueryRow(r.Context(), `SELECT id, s3_key FROM big_files WHERE token=$1 AND user_id=$2 AND downloads=0`, token, sess.UserID).Scan(&id, &key); err != nil {
		writeJSON(w, http.StatusOK, map[string]bool{"ok": false}) // someone else's, already sent and downloaded, or gone
		return
	}
	if c, err := s.s3(); err == nil {
		_ = c.RemoveObject(r.Context(), s3Bucket(), key, minio.RemoveObjectOptions{})
		_ = c.RemoveObject(r.Context(), s3Bucket(), "thumbs/"+token+".jpg", minio.RemoveObjectOptions{})
	}
	_, _ = s.db.Exec(r.Context(), `DELETE FROM big_files WHERE id=$1`, id)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
