package main

import (
	"bytes"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/minio/minio-go/v7"
	_ "golang.org/x/image/webp"
)

// Pictures in signatures (a logo, a banner) are not attached to every letter: they live in the
// file storage and the signature links to them at https://files.<domain>/i/<token>.<ext>, so the
// recipient's mail program shows them like any picture from the web. Kept for good: old letters
// still point to them.

const sigImageMax = 2 << 20

var sigImageTypes = map[string]string{"png": "image/png", "jpeg": "image/jpeg", "gif": "image/gif", "webp": "image/webp"}

// POST /api/signature-image with the picture as the body; answers {"url": …}.
func (s *Server) handleSignatureImage(w http.ResponseWriter, r *http.Request, sess *Session) {
	c, err := s.s3()
	if err != nil {
		writeErr(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, sigImageMax+1))
	if err != nil || len(data) > sigImageMax {
		writeErr(w, http.StatusRequestEntityTooLarge, "A signature image must be smaller than 2 MB")
		return
	}
	// Only a real picture: the type comes from its content, not from the browser.
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	ct, ok := sigImageTypes[format]
	if err != nil || !ok || cfg.Width > 4000 || cfg.Height > 4000 {
		writeErr(w, http.StatusBadRequest, "This is not a PNG, JPEG, GIF or WebP image")
		return
	}
	ext := format
	if ext == "jpeg" {
		ext = "jpg"
	}
	name := newToken() + "." + ext
	if _, err := c.PutObject(r.Context(), s3Bucket(), "sig/"+name, bytes.NewReader(data), int64(len(data)), minio.PutObjectOptions{ContentType: ct}); err != nil {
		log.Printf("signature image: %v", err)
		writeErr(w, http.StatusBadGateway, "Could not save the image to storage")
		return
	}
	s.audit(r.Context(), sess.UserID, sess.Login, clientIP(r), "signature.image", name, map[string]any{"size": len(data)})
	writeJSON(w, http.StatusOK, map[string]string{"url": "https://" + s.filesHost() + "/i/" + name})
}

// GET /i/{name}: a signature picture, for everyone (recipients load it from their mail program).
func (s *Server) sigImage(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	dot := strings.LastIndexByte(name, '.')
	if dot < 10 || strings.ContainsAny(name, "/\\") {
		http.NotFound(w, r)
		return
	}
	ext := name[dot+1:]
	if ext == "jpg" {
		ext = "jpeg"
	}
	ct, ok := sigImageTypes[ext]
	c, err := s.s3()
	if !ok || err != nil {
		http.NotFound(w, r)
		return
	}
	obj, err := c.GetObject(r.Context(), s3Bucket(), "sig/"+name, minio.GetObjectOptions{})
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer obj.Close()
	st, err := obj.Stat()
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	http.ServeContent(w, r, "", st.LastModified, obj)
}
