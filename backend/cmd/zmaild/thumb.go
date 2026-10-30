package main

import (
	"bytes"
	"context"
	"image"
	"image/jpeg"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"golang.org/x/image/draw"
)

// Attachment thumbnails for the cards under a letter, like Gmail: images are scaled here,
// PDFs and office documents get their first page rendered by the preview container.

const thumbWidth = 320

func (s *Server) serveThumb(w http.ResponseWriter, r *http.Request, mt, name string, data []byte) {
	var out []byte
	ctype := "image/png"
	switch kind := previewKind(mt, name); {
	case kind == "image":
		img, err := decodeImageBytes(data)
		if err != nil {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		b := img.Bounds()
		if b.Dx() > thumbWidth {
			h := b.Dy() * thumbWidth / b.Dx()
			dst := image.NewRGBA(image.Rect(0, 0, thumbWidth, max(h, 1)))
			draw.ApproxBiLinear.Scale(dst, dst.Bounds(), img, b, draw.Src, nil)
			img = dst
		}
		var buf bytes.Buffer
		if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 80}); err != nil {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		out, ctype = buf.Bytes(), "image/jpeg"
	case kind == "pdf" || kind == "office" || kind == "sheet" || strings.EqualFold(mt, "application/pdf"):
		if kind == "pdf" && !strings.HasSuffix(strings.ToLower(name), ".pdf") {
			name += ".pdf"
		}
		// Office documents: make (and cache) the PDF first and draw the thumbnail from it, so opening
		// the preview afterwards is instant. Spreadsheets also get their grid view ready.
		if kind == "office" || kind == "sheet" {
			pdf, err := s.officeToPDF(r.Context(), name, data)
			if err != nil {
				w.Header().Set("Cache-Control", "private, max-age=600")
				w.WriteHeader(http.StatusNotFound)
				return
			}
			if kind == "sheet" {
				go func() {
					ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
					defer cancel()
					_, _ = s.sheetHTML(ctx, name, data)
				}()
			}
			name, data = strings.TrimSuffix(name, filepath.Ext(name))+".pdf", pdf
		}
		png, err := s.officeThumb(r.Context(), name, data)
		if err != nil {
			w.Header().Set("Cache-Control", "private, max-age=600")
			w.WriteHeader(http.StatusNotFound)
			return
		}
		out = png
	default:
		w.WriteHeader(http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", ctype)
	w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox")
	w.Header().Set("Cache-Control", "private, max-age=86400")
	w.Header().Set("Content-Length", strconv.Itoa(len(out)))
	_, _ = w.Write(out)
}
