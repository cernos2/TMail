package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"html"
	"image"
	_ "image/gif"
	"image/jpeg"
	_ "image/png"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
)

// Avatars, in order: a photo uploaded by our user (for their personal mailbox), the sender's BIMI
// brand logo (only when the message passed DMARC), Gravatar. External images are fetched and
// cached by the server so the reader's browser never contacts third parties.

const (
	avatarSize      = 160
	avatarCacheTTL  = 7 * 24 * time.Hour
	avatarMissTTL   = 24 * time.Hour
	avatarMaxRemote = 512 << 10
	avatarMaxUpload = 5 << 20
)

// External fetches go only to public addresses: BIMI URLs come from the sender's DNS and must
// not be able to reach our internal services.
var remoteClient = &http.Client{
	Timeout: 6 * time.Second,
	Transport: &http.Transport{
		DialContext:           (&net.Dialer{Timeout: 4 * time.Second, Control: denyPrivate}).DialContext,
		TLSHandshakeTimeout:   4 * time.Second,
		ResponseHeaderTimeout: 5 * time.Second,
		MaxIdleConns:          10,
	},
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 3 || req.URL.Scheme != "https" {
			return errors.New("redirect not allowed")
		}
		return nil
	},
}

func denyPrivate(_, address string, _ syscall.RawConn) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return err
	}
	ip := net.ParseIP(host)
	if ip == nil || ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsUnspecified() || ip.IsMulticast() || ip.Equal(net.ParseIP("100.100.100.200")) {
		return fmt.Errorf("address %s not allowed", host)
	}
	if ip4 := ip.To4(); ip4 != nil && ip4[0] == 100 && ip4[1]&0xc0 == 64 { // 100.64.0.0/10 carrier-grade NAT
		return fmt.Errorf("address %s not allowed", host)
	}
	return nil
}

func (s *Server) handleAvatar(w http.ResponseWriter, r *http.Request, sess *Session) {
	email := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("email")))
	if !emailRe.MatchString(email) {
		writeErr(w, http.StatusBadRequest, "Invalid address")
		return
	}
	allowBIMI := r.URL.Query().Get("bimi") == "1"
	ctx := r.Context()

	// 1. Our own user's photo, for their personal mailbox.
	var mime string
	var data []byte
	err := s.db.QueryRow(ctx, `
		SELECT ua.mime, ua.data FROM user_avatars ua
		JOIN user_mailboxes um ON um.user_id = ua.user_id AND um.personal
		JOIN mailboxes m ON m.id = um.mailbox_id
		WHERE m.address = $1 LIMIT 1`, email).Scan(&mime, &data)
	if err == nil {
		serveAvatar(w, mime, data, time.Hour)
		return
	}
	// 2. Brand logo (BIMI) — only for authenticated mail.
	if allowBIMI {
		domain := email[strings.LastIndex(email, "@")+1:]
		if mime, data = s.cachedAvatar(ctx, "bimi:"+domain, func() (string, []byte) { return s.fetchBIMI(ctx, domain) }); len(data) > 0 {
			serveAvatar(w, mime, data, 24*time.Hour)
			return
		}
	}
	// 3. Gravatar.
	if mime, data = s.cachedAvatar(ctx, "gravatar:"+email, func() (string, []byte) { return fetchGravatar(ctx, email) }); len(data) > 0 {
		serveAvatar(w, mime, data, 24*time.Hour)
		return
	}
	// 4. The icon of the company's website (most companies publish no BIMI) — for authenticated mail
	// from a company domain only, so a forged letter cannot borrow someone's logo.
	if domain := siteDomain(email); allowBIMI && domain != "" {
		if mime, data = s.cachedAvatar(ctx, "site:"+domain, func() (string, []byte) { return fetchSiteIcon(ctx, domain) }); len(data) > 0 {
			serveAvatar(w, mime, data, 7*24*time.Hour)
			return
		}
	}
	// Browsers may remember the miss; the page then shows initials.
	w.Header().Set("Cache-Control", "private, max-age=21600")
	w.WriteHeader(http.StatusNotFound)
}

func serveAvatar(w http.ResponseWriter, mime string, data []byte, maxAge time.Duration) {
	w.Header().Set("Content-Type", mime)
	w.Header().Set("Cache-Control", "private, max-age="+strconv.Itoa(int(maxAge.Seconds())))
	// SVG logos are external content: never let them run anything if opened directly.
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; sandbox")
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	_, _ = w.Write(data)
}

// cachedAvatar returns a cached image (possibly a cached miss) or fetches and stores it.
func (s *Server) cachedAvatar(ctx context.Context, key string, fetch func() (string, []byte)) (string, []byte) {
	var mime string
	var data []byte
	var at time.Time
	if err := s.db.QueryRow(ctx, `SELECT mime, data, fetched_at FROM avatar_cache WHERE key=$1`, key).Scan(&mime, &data, &at); err == nil {
		ttl := avatarCacheTTL
		if len(data) == 0 {
			ttl = avatarMissTTL
		}
		if time.Since(at) < ttl {
			return mime, data
		}
	}
	mime, data = fetch()
	if data == nil {
		data = []byte{} // "no picture" is cached too (the column is NOT NULL)
	}
	_, _ = s.db.Exec(ctx, `INSERT INTO avatar_cache(key, mime, data, fetched_at) VALUES ($1,$2,$3,now())
		ON CONFLICT (key) DO UPDATE SET mime=EXCLUDED.mime, data=EXCLUDED.data, fetched_at=now()`, key, mime, data)
	return mime, data
}

func getRemote(ctx context.Context, url string) (string, []byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", nil, err
	}
	req.Header.Set("User-Agent", "TMAIL avatar fetcher")
	resp, err := remoteClient.Do(req)
	if err != nil {
		return "", nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", nil, fmt.Errorf("status %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, avatarMaxRemote+1))
	if err != nil {
		return "", nil, err
	}
	if len(data) > avatarMaxRemote {
		return "", nil, errors.New("image too large")
	}
	return strings.ToLower(strings.TrimSpace(strings.Split(resp.Header.Get("Content-Type"), ";")[0])), data, nil
}

func fetchGravatar(ctx context.Context, email string) (string, []byte) {
	h := sha256.Sum256([]byte(email))
	mime, data, err := getRemote(ctx, "https://gravatar.com/avatar/"+hex.EncodeToString(h[:])+"?s=160&d=404")
	if err != nil || !strings.HasPrefix(mime, "image/") || mime == "image/svg+xml" {
		return "", nil
	}
	return mime, data
}

var (
	bimiLogoRe = regexp.MustCompile(`(?i)(?:^|;)\s*l\s*=\s*(https://[^;\s]+)`)
	svgDanger  = regexp.MustCompile(`(?i)<script|<foreignobject|\son[a-z]+\s*=|javascript:|<iframe|<embed|<object|xlink:href\s*=\s*["']?(?:https?:|data:)`)
)

// fetchBIMI reads default._bimi.<domain> (falling back to the parent domain) and downloads the SVG logo.
func (s *Server) fetchBIMI(ctx context.Context, domain string) (string, []byte) {
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	res := s.resolver()
	candidates := []string{domain}
	if parts := strings.Split(domain, "."); len(parts) > 2 {
		candidates = append(candidates, strings.Join(parts[len(parts)-2:], "."))
	}
	for _, d := range candidates {
		txts, err := res.LookupTXT(ctx, "default._bimi."+d)
		if err != nil {
			continue
		}
		for _, t := range txts {
			if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(t)), "v=bimi1") {
				continue
			}
			m := bimiLogoRe.FindStringSubmatch(t)
			if m == nil {
				return "", nil // record without a logo (opt-out)
			}
			_, data, err := getRemote(ctx, m[1])
			if err != nil || !bytes.Contains(bytes.ToLower(data[:min(len(data), 2048)]), []byte("<svg")) || svgDanger.Match(data) {
				return "", nil
			}
			return "image/svg+xml", data
		}
	}
	return "", nil
}

// ---------- own photo ----------

func (s *Server) handleUploadAvatar(w http.ResponseWriter, r *http.Request, sess *Session) {
	r.Body = http.MaxBytesReader(w, r.Body, avatarMaxUpload+(64<<10))
	if err := r.ParseMultipartForm(avatarMaxUpload); err != nil {
		writeErr(w, http.StatusRequestEntityTooLarge, "The photo is larger than 5 MB. Choose a smaller file")
		return
	}
	fh := r.MultipartForm.File["photo"]
	if len(fh) == 0 {
		writeErr(w, http.StatusBadRequest, "No file selected")
		return
	}
	f, err := fh[0].Open()
	if err != nil {
		writeErr(w, http.StatusBadRequest, "Could not read the file")
		return
	}
	defer f.Close()
	src, err := decodeImage(f)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "This is not a photo: JPEG, PNG, WebP or GIF will do")
		return
	}
	// Square crop from the centre, resize, re-encode: strips EXIF (including GPS position).
	b := src.Bounds()
	side := min(b.Dx(), b.Dy())
	crop := image.Rect(b.Min.X+(b.Dx()-side)/2, b.Min.Y+(b.Dy()-side)/2, 0, 0)
	crop.Max = crop.Min.Add(image.Pt(side, side))
	dst := image.NewRGBA(image.Rect(0, 0, avatarSize, avatarSize))
	draw.CatmullRom.Scale(dst, dst.Bounds(), src, crop, draw.Src, nil)
	var out bytes.Buffer
	if err := jpeg.Encode(&out, dst, &jpeg.Options{Quality: 86}); err != nil {
		serverErr(w, "encode avatar", err)
		return
	}
	if _, err := s.db.Exec(r.Context(), `INSERT INTO user_avatars(user_id, mime, data) VALUES ($1,'image/jpeg',$2)
		ON CONFLICT (user_id) DO UPDATE SET mime=EXCLUDED.mime, data=EXCLUDED.data, updated_at=now()`, sess.UserID, out.Bytes()); err != nil {
		serverErr(w, "save avatar", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "version": time.Now().Unix()})
}

func (s *Server) handleDeleteAvatar(w http.ResponseWriter, r *http.Request, sess *Session) {
	_, _ = s.db.Exec(r.Context(), `DELETE FROM user_avatars WHERE user_id=$1`, sess.UserID)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// handleUserPhoto serves a user's own photo by login (sidebar, admin list).
func (s *Server) handleUserPhoto(w http.ResponseWriter, r *http.Request, sess *Session) {
	var mime string
	var data []byte
	err := s.db.QueryRow(r.Context(), `SELECT ua.mime, ua.data FROM user_avatars ua JOIN users u ON u.id = ua.user_id WHERE u.login = $1`,
		strings.ToLower(r.URL.Query().Get("login"))).Scan(&mime, &data)
	if err != nil {
		w.Header().Set("Cache-Control", "private, max-age=300")
		w.WriteHeader(http.StatusNotFound)
		return
	}
	serveAvatar(w, mime, data, time.Hour)
}

// siteDomain: the company's site for an address — news.ozon.ru → ozon.ru; "" for free mail
// services (gmail.com, yandex.ru…), where the site is not the sender's.
func siteDomain(email string) string {
	d := email[strings.LastIndex(email, "@")+1:]
	if publicDomains[d] {
		return ""
	}
	parts := strings.Split(d, ".")
	n := 2
	if len(parts) > 2 && regexp.MustCompile(`^(com|net|org|co|msk|spb|edu|gov)$`).MatchString(parts[len(parts)-2]) {
		n = 3 // firma.com.ru
	}
	if len(parts) > n {
		parts = parts[len(parts)-n:]
	}
	b := strings.Join(parts, ".")
	if publicDomains[b] {
		return ""
	}
	return b
}

var (
	reIconLink = regexp.MustCompile(`(?is)<link\b[^>]*>`)
	reAttr     = regexp.MustCompile(`(?is)\b(rel|href|sizes|type)\s*=\s*("[^"]*"|'[^']*'|[^\s>]+)`)
)

// fetchSiteIcon takes the best icon the site's home page announces (apple-touch-icon first, then
// the largest icon), falling back to /favicon.ico.
func fetchSiteIcon(ctx context.Context, domain string) (string, []byte) {
	var page []byte
	base := ""
	for _, u := range []string{"https://" + domain + "/", "https://www." + domain + "/"} {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			continue
		}
		req.Header.Set("User-Agent", "Mozilla/5.0 (TMAIL avatar fetcher)")
		resp, err := remoteClient.Do(req)
		if err != nil {
			continue
		}
		page, _ = io.ReadAll(io.LimitReader(resp.Body, 400<<10)) // the <head> is at the top
		resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			base = resp.Request.URL.String()
			break
		}
	}
	type cand struct {
		href  string
		score int
	}
	var cands []cand
	for _, tag := range reIconLink.FindAll(page, 200) {
		attrs := map[string]string{}
		for _, m := range reAttr.FindAllSubmatch(tag, -1) {
			attrs[strings.ToLower(string(m[1]))] = strings.Trim(string(m[2]), `"'`)
		}
		rel := strings.ToLower(attrs["rel"])
		if !strings.Contains(rel, "icon") || attrs["href"] == "" || strings.Contains(rel, "mask") {
			continue
		}
		score := 10
		if strings.Contains(rel, "apple-touch-icon") {
			score = 300
		}
		if sz := attrs["sizes"]; sz != "" {
			var wd int
			fmt.Sscanf(strings.ToLower(sz), "%dx", &wd)
			if wd > score {
				score = wd
			}
		}
		if strings.Contains(strings.ToLower(attrs["href"]), ".svg") || attrs["type"] == "image/svg+xml" {
			score = max(score, 200)
		}
		cands = append(cands, cand{attrs["href"], score})
	}
	sort.SliceStable(cands, func(i, j int) bool { return cands[i].score > cands[j].score })
	urls := []string{}
	if b, err := url.Parse(base); err == nil && base != "" {
		for _, c := range cands {
			if u, err := b.Parse(html.UnescapeString(c.href)); err == nil && u.Scheme == "https" {
				urls = append(urls, u.String())
			}
		}
	}
	urls = append(urls, "https://"+domain+"/favicon.ico")
	for i, u := range urls {
		if i >= 3 {
			break
		}
		mime, data, err := getRemote(ctx, u)
		if err != nil || len(data) < 100 {
			continue
		}
		if strings.HasSuffix(strings.ToLower(u), ".ico") && !strings.HasPrefix(mime, "image/") {
			mime = "image/x-icon"
		}
		if strings.HasPrefix(mime, "image/") {
			return mime, data
		}
	}
	// Sites behind anti-bot walls or with Russian TLS certificates (Ozon, Sber, T-Bank…): the icon
	// Yandex keeps for the site. An unknown site gives a 70-byte blank.
	if mime, data, err := getRemote(ctx, "https://favicon.yandex.net/favicon/v2/"+url.PathEscape(domain)+"?size=120"); err == nil && len(data) > 200 && strings.HasPrefix(mime, "image/") {
		return mime, data
	}
	return "", nil
}
