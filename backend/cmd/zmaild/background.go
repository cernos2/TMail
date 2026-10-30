package main

import (
	"bytes"
	"context"
	"image"
	"image/jpeg"
	"net/http"
	"regexp"
	"strconv"
	"time"

	"golang.org/x/image/draw"
)

// Background picture of the webmail: a ready-made one (drawn by the web app) or the user's own,
// re-encoded here (no EXIF, at most 2560 px wide) and kept in the database.

const (
	bgMaxUpload = 15 << 20
	bgMaxWidth  = 2560
)

var bgUploads = make(chan struct{}, 1)

var reBgPreset = regexp.MustCompile(`^[a-z0-9-]{1,40}$`)

func (s *Server) userBackground(ctx context.Context, userID int64) (string, int64) {
	var bg string
	var ver time.Time
	_ = s.db.QueryRow(ctx, `SELECT u.background, COALESCE(b.updated_at, 'epoch') FROM users u LEFT JOIN user_backgrounds b ON b.user_id = u.id WHERE u.id=$1`, userID).Scan(&bg, &ver)
	return bg, ver.Unix()
}

// POST /api/me/background {background: "" | preset id | "custom"}
func (s *Server) handleSetBackground(w http.ResponseWriter, r *http.Request, sess *Session) {
	var in struct{ Background string }
	if err := readJSON(r, &in); err != nil || (in.Background != "" && !reBgPreset.MatchString(in.Background)) {
		writeErr(w, http.StatusBadRequest, "Unknown background")
		return
	}
	if in.Background == "custom" {
		var n int
		_ = s.db.QueryRow(r.Context(), `SELECT count(*) FROM user_backgrounds WHERE user_id=$1`, sess.UserID).Scan(&n)
		if n == 0 {
			writeErr(w, http.StatusBadRequest, "Upload your image first")
			return
		}
	}
	_, _ = s.db.Exec(r.Context(), `UPDATE users SET background=$2 WHERE id=$1`, sess.UserID, in.Background)
	bg, ver := s.userBackground(r.Context(), sess.UserID)
	writeJSON(w, http.StatusOK, map[string]any{"background": bg, "backgroundVersion": ver})
}

// POST /api/me/background/upload (multipart "photo")
func (s *Server) handleUploadBackground(w http.ResponseWriter, r *http.Request, sess *Session) {
	r.Body = http.MaxBytesReader(w, r.Body, bgMaxUpload+(64<<10))
	if err := r.ParseMultipartForm(bgMaxUpload); err != nil {
		writeErr(w, http.StatusRequestEntityTooLarge, "The image is larger than 15 MB. Choose a smaller file")
		return
	}
	fh := r.MultipartForm.File["photo"]
	if len(fh) == 0 {
		writeErr(w, http.StatusBadRequest, "No file selected")
		return
	}
	bgUploads <- struct{}{} // one picture is decoded at a time (each may take ~250 MB)
	defer func() { <-bgUploads }()
	f, err := fh[0].Open()
	if err != nil {
		writeErr(w, http.StatusBadRequest, "Could not read the file")
		return
	}
	defer f.Close()
	src, err := decodeImage(f)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "This is not an image: JPEG, PNG, WebP or GIF will do")
		return
	}
	b := src.Bounds()
	if b.Dy() > 4000 || b.Dx() > 8000 {
		writeErr(w, http.StatusBadRequest, "The image is too big: at most 8000×4000 pixels")
		return
	}
	if b.Dx() < 800 || b.Dy() < 450 {
		writeErr(w, http.StatusBadRequest, "The image is too small for a background: at least 800×450 pixels are needed")
		return
	}
	dw, dh := b.Dx(), b.Dy()
	if dw > bgMaxWidth {
		dw, dh = bgMaxWidth, b.Dy()*bgMaxWidth/b.Dx()
	}
	dst := image.NewRGBA(image.Rect(0, 0, dw, dh))
	draw.CatmullRom.Scale(dst, dst.Bounds(), src, b, draw.Src, nil)
	var out bytes.Buffer
	if err := jpeg.Encode(&out, dst, &jpeg.Options{Quality: 84}); err != nil {
		serverErr(w, "encode background", err)
		return
	}
	if _, err := s.db.Exec(r.Context(), `INSERT INTO user_backgrounds(user_id, data) VALUES ($1,$2)
		ON CONFLICT (user_id) DO UPDATE SET data=EXCLUDED.data, updated_at=now()`, sess.UserID, out.Bytes()); err != nil {
		serverErr(w, "save background", err)
		return
	}
	_, _ = s.db.Exec(r.Context(), `UPDATE users SET background='custom' WHERE id=$1`, sess.UserID)
	bg, ver := s.userBackground(r.Context(), sess.UserID)
	writeJSON(w, http.StatusOK, map[string]any{"background": bg, "backgroundVersion": ver})
}

// GET /api/me/background/image?v=<version>
func (s *Server) handleBackgroundImage(w http.ResponseWriter, r *http.Request, sess *Session) {
	var data []byte
	if err := s.db.QueryRow(r.Context(), `SELECT data FROM user_backgrounds WHERE user_id=$1`, sess.UserID).Scan(&data); err != nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.Header().Set("Cache-Control", "private, max-age=31536000, immutable") // the URL carries the version
	w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox")
	_, _ = w.Write(data)
}
