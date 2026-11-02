package main

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/emersion/go-vcard"
)

// CalDAV and CardDAV: calendars and counterparties on phones (iPhone, Android with DAVx⁵),
// macOS, Thunderbird and Outlook (with a CalDAV add-on). Only what those programs use is here:
// PROPFIND, REPORT (multiget, query), GET, PUT, DELETE, OPTIONS. Every collection has a "ctag"
// that changes with its content, so a phone re-reads only the calendars that changed.
//
//	/dav/principals/<login>/           the user
//	/dav/calendars/<login>/<id>/<file> calendars (own and shared with the user) and their events
//	/dav/contacts/<login>/crm/<file>   counterparties

const (
	nsDAV   = "DAV:"
	nsCal   = "urn:ietf:params:xml:ns:caldav"
	nsCard  = "urn:ietf:params:xml:ns:carddav"
	nsCS    = "http://calendarserver.org/ns/"
	nsApple = "http://apple.com/ns/ical/"
)

var davPrefixes = map[string]string{nsDAV: "d", nsCal: "c", nsCard: "card", nsCS: "cs", nsApple: "a"}

const davMaxBody = 2 << 20

type davUser struct {
	id     int64
	login  string
	name   string
	admin  bool
	emails []string
	tz     *time.Location
	lang   string
}

func (u *davUser) principal() string { return "/dav/principals/" + url.PathEscape(u.login) + "/" }
func (u *davUser) calHome() string   { return "/dav/calendars/" + url.PathEscape(u.login) + "/" }
func (u *davUser) cardHome() string  { return "/dav/contacts/" + url.PathEscape(u.login) + "/" }
func (u *davUser) book() string      { return u.cardHome() + "crm/" }

// ---------- request XML ----------

type xnode struct {
	name  xml.Name
	attrs []xml.Attr
	kids  []*xnode
	text  string
}

func parseXMLTree(r io.Reader) (*xnode, error) {
	dec := xml.NewDecoder(io.LimitReader(r, davMaxBody))
	var stack []*xnode
	var root *xnode
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			n := &xnode{name: t.Name, attrs: t.Attr}
			if len(stack) > 0 {
				p := stack[len(stack)-1]
				p.kids = append(p.kids, n)
			} else if root == nil {
				root = n
			}
			stack = append(stack, n)
		case xml.EndElement:
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
		case xml.CharData:
			if len(stack) > 0 {
				stack[len(stack)-1].text += string(t)
			}
		}
	}
	return root, nil
}

func (n *xnode) child(space, local string) *xnode {
	if n == nil {
		return nil
	}
	for _, k := range n.kids {
		if k.name.Space == space && k.name.Local == local {
			return k
		}
	}
	return nil
}

// all finds every descendant with the name.
func (n *xnode) all(space, local string) []*xnode {
	if n == nil {
		return nil
	}
	var out []*xnode
	for _, k := range n.kids {
		if k.name.Space == space && k.name.Local == local {
			out = append(out, k)
		}
		out = append(out, k.all(space, local)...)
	}
	return out
}

func (n *xnode) attr(local string) string {
	for _, a := range n.attrs {
		if a.Name.Local == local {
			return a.Value
		}
	}
	return ""
}

// wantedProps: the property names a PROPFIND/REPORT asks for; nil = all (allprop or no body).
func wantedProps(root *xnode) []xml.Name {
	p := root.child(nsDAV, "prop")
	if p == nil {
		return nil
	}
	out := []xml.Name{}
	for _, k := range p.kids {
		out = append(out, k.name)
	}
	return out
}

// ---------- response XML ----------

type davProps map[xml.Name]string // property → inner XML

type davResp struct {
	href   string
	props  davProps
	status int // a resource that is not there (multiget of a deleted event)
}

func xesc(s string) string {
	var b strings.Builder
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}

func davElem(n xml.Name, inner string) string {
	if p, ok := davPrefixes[n.Space]; ok {
		if inner == "" {
			return "<" + p + ":" + n.Local + "/>"
		}
		return "<" + p + ":" + n.Local + ">" + inner + "</" + p + ":" + n.Local + ">"
	}
	attr := ` xmlns:x="` + xesc(n.Space) + `"`
	if inner == "" {
		return "<x:" + n.Local + attr + "/>"
	}
	return "<x:" + n.Local + attr + ">" + inner + "</x:" + n.Local + ">"
}

func href(p string) string { return "<d:href>" + xesc(p) + "</d:href>" }

func writeMultistatus(w http.ResponseWriter, resps []davResp, want []xml.Name) {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="utf-8"?>` + "\n")
	b.WriteString(`<d:multistatus xmlns:d="DAV:" xmlns:c="` + nsCal + `" xmlns:card="` + nsCard + `" xmlns:cs="` + nsCS + `" xmlns:a="` + nsApple + `">`)
	for _, r := range resps {
		b.WriteString("<d:response>" + href(r.href))
		if r.status != 0 {
			fmt.Fprintf(&b, "<d:status>HTTP/1.1 %d %s</d:status></d:response>", r.status, http.StatusText(r.status))
			continue
		}
		var ok, missing strings.Builder
		if want == nil {
			for n, v := range r.props {
				ok.WriteString(davElem(n, v))
			}
		} else {
			for _, n := range want {
				if v, found := r.props[n]; found {
					ok.WriteString(davElem(n, v))
				} else {
					missing.WriteString(davElem(n, ""))
				}
			}
		}
		if ok.Len() > 0 {
			b.WriteString("<d:propstat><d:prop>" + ok.String() + "</d:prop><d:status>HTTP/1.1 200 OK</d:status></d:propstat>")
		}
		if missing.Len() > 0 {
			b.WriteString("<d:propstat><d:prop>" + missing.String() + "</d:prop><d:status>HTTP/1.1 404 Not Found</d:status></d:propstat>")
		}
		b.WriteString("</d:response>")
	}
	b.WriteString("</d:multistatus>")
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.WriteHeader(http.StatusMultiStatus)
	_, _ = io.WriteString(w, b.String())
}

func qn(space, local string) xml.Name { return xml.Name{Space: space, Local: local} }

// ---------- entry point ----------

func davError(w http.ResponseWriter, status int, msg string) {
	http.Error(w, msg, status)
}

func (s *Server) handleDAV(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/.well-known/") {
		http.Redirect(w, r, "/dav/", http.StatusMovedPermanently)
		return
	}
	w.Header().Set("DAV", "1, 3, calendar-access, addressbook")
	if r.Method == http.MethodOptions {
		w.Header().Set("Allow", "OPTIONS, GET, HEAD, PUT, DELETE, PROPFIND, PROPPATCH, REPORT")
		w.WriteHeader(http.StatusOK)
		return
	}
	login, pass, ok := r.BasicAuth()
	if !ok {
		w.Header().Set("WWW-Authenticate", `Basic realm="TMAIL", charset="UTF-8"`)
		davError(w, http.StatusUnauthorized, "Sign in with your TMAIL login and an app password")
		return
	}
	ctx := r.Context()
	uid, err := s.davLogin(ctx, login, pass, clientIP(r))
	if err != nil {
		slog.Warn("dav login failed", "login", login, "ip", clientIP(r))
		w.Header().Set("WWW-Authenticate", `Basic realm="TMAIL", charset="UTF-8"`)
		davError(w, http.StatusUnauthorized, "Wrong username or password")
		return
	}
	u := &davUser{id: uid}
	if err := s.db.QueryRow(ctx, `SELECT login, display_name, is_admin FROM users WHERE id=$1`, uid).Scan(&u.login, &u.name, &u.admin); err != nil {
		davError(w, http.StatusUnauthorized, "Account not found")
		return
	}
	if accs, err := s.accountsFor(ctx, uid); err == nil {
		for _, a := range accs {
			u.emails = append(u.emails, a.Address)
		}
	}
	u.lang, _ = s.userLocale(ctx, uid)
	if u.lang == "" {
		u.lang = serverLocale().Lang
	}
	u.tz = s.userTZ(ctx, uid)
	if _, err := s.ensureCalendar(ctx, uid); err != nil {
		serverErr(w, "dav calendar", err)
		return
	}

	p := path.Clean(r.URL.Path)
	seg := strings.Split(strings.TrimPrefix(strings.TrimPrefix(p, "/dav"), "/"), "/")
	if len(seg) == 1 && seg[0] == "" {
		seg = nil
	}
	for i := range seg {
		seg[i], _ = url.PathUnescape(seg[i])
	}
	// Every path below the kind names a user: only one's own.
	if len(seg) >= 2 && !strings.EqualFold(seg[1], u.login) {
		davError(w, http.StatusForbidden, "Not your calendar")
		return
	}
	switch r.Method {
	case "PROPFIND":
		s.davPropfind(w, r, u, seg)
	case "REPORT":
		s.davReport(w, r, u, seg)
	case "PROPPATCH":
		s.davProppatch(w, r, u, seg)
	case http.MethodGet, http.MethodHead:
		s.davGet(w, r, u, seg)
	case http.MethodPut:
		s.davPut(w, r, u, seg)
	case http.MethodDelete:
		s.davDelete(w, r, u, seg)
	case "MKCOL", "MKCALENDAR", "MOVE", "COPY":
		davError(w, http.StatusForbidden, "Create calendars in the TMAIL webmail")
	default:
		davError(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}

func davDepth(r *http.Request) int {
	if r.Header.Get("Depth") == "0" {
		return 0
	}
	return 1
}

// ---------- calendars ----------

type davCal struct {
	id       int64
	name     string
	color    string
	ctag     int64
	canWrite bool
	owner    string
}

var personalCalName = map[string]string{"ru": "Личный", "uk": "Особистий", "be": "Асабісты", "kk": "Жеке", "de": "Persönlich", "fr": "Personnel",
	"es": "Personal", "it": "Personale", "pt": "Pessoal", "pl": "Osobisty", "tr": "Kişisel", "zh": "个人", "ja": "個人", "ko": "개인", "ar": "شخصي"}

var calHex = map[string]string{"blue": "#3B82F6", "green": "#22C55E", "yellow": "#EAB308", "red": "#EF4444", "purple": "#8B5CF6",
	"teal": "#14B8A6", "orange": "#F97316", "gray": "#6B7280"}

func (s *Server) davCalendars(ctx context.Context, u *davUser, only int64) ([]davCal, error) {
	rows, err := s.db.Query(ctx, `SELECT c.id, c.name, c.color, c.ctag, c.owner_id=$1 OR COALESCE(sh.can_write, false),
			CASE WHEN c.owner_id=$1 THEN '' ELSE COALESCE(NULLIF(ou.display_name, ''), ou.login) END
		FROM calendars c JOIN users ou ON ou.id = c.owner_id
		LEFT JOIN calendar_shares sh ON sh.calendar_id = c.id AND sh.user_id = $1
		WHERE (c.owner_id=$1 OR sh.user_id IS NOT NULL) AND ($2 = 0 OR c.id = $2)
		ORDER BY c.owner_id <> $1, c.id`, u.id, only)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []davCal
	for rows.Next() {
		var c davCal
		if err := rows.Scan(&c.id, &c.name, &c.color, &c.ctag, &c.canWrite, &c.owner); err != nil {
			return nil, err
		}
		if c.name == "" {
			c.name = personalCalName[strings.SplitN(u.lang, "-", 2)[0]]
			if c.name == "" {
				c.name = "Personal"
			}
		}
		if c.owner != "" {
			c.name += " — " + c.owner
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func privileges(write bool) string {
	p := "<d:privilege><d:read/></d:privilege><d:privilege><d:read-current-user-privilege-set/></d:privilege>"
	if write {
		p += "<d:privilege><d:write/></d:privilege><d:privilege><d:write-content/></d:privilege><d:privilege><d:write-properties/></d:privilege>" +
			"<d:privilege><d:bind/></d:privilege><d:privilege><d:unbind/></d:privilege>"
	}
	return p
}

func (u *davUser) commonProps() davProps {
	return davProps{
		qn(nsDAV, "current-user-principal"): href(u.principal()),
		qn(nsDAV, "principal-URL"):          href(u.principal()),
		qn(nsDAV, "owner"):                  href(u.principal()),
	}
}

func (u *davUser) principalProps() davProps {
	p := u.commonProps()
	p[qn(nsDAV, "resourcetype")] = "<d:principal/><d:collection/>"
	p[qn(nsDAV, "displayname")] = xesc(firstNonEmpty(u.name, u.login))
	p[qn(nsCal, "calendar-home-set")] = href(u.calHome())
	p[qn(nsCard, "addressbook-home-set")] = href(u.cardHome())
	addrs := href(u.principal())
	for _, e := range u.emails {
		addrs += href("mailto:" + e)
	}
	p[qn(nsCal, "calendar-user-address-set")] = addrs
	p[qn(nsDAV, "principal-collection-set")] = href("/dav/principals/")
	p[qn(nsDAV, "current-user-privilege-set")] = privileges(false)
	p[qn(nsDAV, "supported-report-set")] = ""
	return p
}

func firstNonEmpty(xs ...string) string {
	for _, x := range xs {
		if x != "" {
			return x
		}
	}
	return ""
}

func (u *davUser) homeProps(name string) davProps {
	p := u.commonProps()
	p[qn(nsDAV, "resourcetype")] = "<d:collection/>"
	p[qn(nsDAV, "displayname")] = xesc(name)
	p[qn(nsDAV, "current-user-privilege-set")] = privileges(false)
	return p
}

func (u *davUser) calendarProps(c davCal) davProps {
	p := u.commonProps()
	p[qn(nsDAV, "resourcetype")] = "<d:collection/><c:calendar/>"
	p[qn(nsDAV, "displayname")] = xesc(c.name)
	p[qn(nsApple, "calendar-color")] = calHex[c.color] + "FF"
	p[qn(nsApple, "calendar-order")] = strconv.FormatInt(c.id, 10)
	p[qn(nsCS, "getctag")] = strconv.FormatInt(c.ctag, 10)
	p[qn(nsDAV, "getetag")] = `"` + strconv.FormatInt(c.ctag, 10) + `"`
	p[qn(nsCal, "supported-calendar-component-set")] = `<c:comp name="VEVENT"/><c:comp name="VTODO"/>`
	p[qn(nsCal, "supported-calendar-data")] = `<c:calendar-data content-type="text/calendar" version="2.0"/>`
	p[qn(nsCal, "max-resource-size")] = strconv.Itoa(davMaxBody)
	p[qn(nsDAV, "current-user-privilege-set")] = privileges(c.canWrite)
	p[qn(nsDAV, "supported-report-set")] = "<d:supported-report><d:report><c:calendar-multiget/></d:report></d:supported-report>" +
		"<d:supported-report><d:report><c:calendar-query/></d:report></d:supported-report>"
	p[qn(nsCal, "schedule-calendar-transp")] = "<c:opaque/>"
	return p
}

type davEvent struct {
	e       CalEvent
	uid     string
	href    string
	raw     string
	updated time.Time
}

func (d davEvent) etag() string {
	return fmt.Sprintf("%x-%x", d.e.ID, d.updated.UnixNano())
}

func (s *Server) davEvents(ctx context.Context, calID int64, hrefs []string) ([]davEvent, error) {
	q := `SELECT ` + calCols + `, e.uid, e.href, e.ical, e.updated_at FROM cal_events e WHERE e.calendar_id=$1`
	args := []any{calID}
	if hrefs != nil {
		q += ` AND e.href = ANY($2)`
		args = append(args, hrefs)
	}
	rows, err := s.db.Query(ctx, q+` ORDER BY e.id`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []davEvent
	for rows.Next() {
		var d davEvent
		e, err := scanEvent(rows, &d.uid, &d.href, &d.raw, &d.updated)
		if err != nil {
			return nil, err
		}
		d.e = e
		out = append(out, d)
	}
	return out, rows.Err()
}

func (u *davUser) eventProps(calPath string, d davEvent, withData bool) davResp {
	comp := "vevent"
	if d.e.IsTask {
		comp = "vtodo"
	}
	p := davProps{
		qn(nsDAV, "getetag"):                `"` + d.etag() + `"`,
		qn(nsDAV, "getcontenttype"):         "text/calendar; charset=utf-8; component=" + comp,
		qn(nsDAV, "resourcetype"):           "",
		qn(nsDAV, "getlastmodified"):        d.updated.UTC().Format(http.TimeFormat),
		qn(nsDAV, "displayname"):            xesc(d.e.Title),
		qn(nsDAV, "current-user-principal"): href(u.principal()),
	}
	if withData {
		p[qn(nsCal, "calendar-data")] = xesc(eventObject(&d.e, d.uid, u.tz, d.raw))
	}
	return davResp{href: calPath + url.PathEscape(d.href), props: p}
}

// ---------- contacts ----------

func (s *Server) bookCtag(ctx context.Context) int64 {
	var c int64
	_ = s.db.QueryRow(ctx, `SELECT ctag FROM dav_ctags WHERE name='crm'`).Scan(&c)
	return c
}

func (u *davUser) bookProps(ctag int64) davProps {
	p := u.commonProps()
	p[qn(nsDAV, "resourcetype")] = "<d:collection/><card:addressbook/>"
	p[qn(nsDAV, "displayname")] = xesc(bookName(u.lang))
	p[qn(nsCS, "getctag")] = strconv.FormatInt(ctag, 10)
	p[qn(nsDAV, "getetag")] = `"` + strconv.FormatInt(ctag, 10) + `"`
	p[qn(nsCard, "supported-address-data")] = `<card:address-data-type content-type="text/vcard" version="3.0"/>`
	p[qn(nsCard, "max-resource-size")] = strconv.Itoa(davMaxBody)
	p[qn(nsDAV, "current-user-privilege-set")] = privileges(true)
	p[qn(nsDAV, "supported-report-set")] = "<d:supported-report><d:report><card:addressbook-multiget/></d:report></d:supported-report>" +
		"<d:supported-report><d:report><card:addressbook-query/></d:report></d:supported-report>"
	return p
}

var bookNames = map[string]string{"ru": "Контрагенты", "uk": "Контрагенти", "be": "Контрагенты", "kk": "Контрагенттер", "de": "Geschäftspartner",
	"fr": "Contacts professionnels", "es": "Contrapartes", "it": "Controparti", "pt": "Contrapartes", "pl": "Kontrahenci", "tr": "Cari hesaplar"}

func bookName(lang string) string {
	if n := bookNames[strings.SplitN(lang, "-", 2)[0]]; n != "" {
		return n
	}
	return "Counterparties"
}

func (u *davUser) cardProps(p vcParty, withData bool) davResp {
	data := encodeCard(partyCard(p))
	props := davProps{
		qn(nsDAV, "getetag"):        `"` + cardETag(data) + `"`,
		qn(nsDAV, "getcontenttype"): "text/vcard; charset=utf-8",
		qn(nsDAV, "resourcetype"):   "",
		qn(nsDAV, "displayname"):    xesc(p.Name),
	}
	if withData {
		props[qn(nsCard, "address-data")] = xesc(data)
	}
	return davResp{href: u.book() + url.PathEscape(p.Href), props: props}
}

func (s *Server) davCards(ctx context.Context, hrefs []string) ([]vcParty, error) {
	if hrefs == nil {
		return s.vcParties(ctx, "ORDER BY p.id")
	}
	return s.vcParties(ctx, "WHERE COALESCE(p.dav_href, p.id::text || '.vcf') = ANY($1) ORDER BY p.id", hrefs)
}

// partyByHref: the counterparty a phone's file name points to (0 = none).
func (s *Server) partyByHref(ctx context.Context, name string) int64 {
	var id int64
	err := s.db.QueryRow(ctx, `SELECT id FROM crm_parties WHERE dav_href=$1`, name).Scan(&id)
	if err == nil {
		return id
	}
	if n, err := strconv.ParseInt(strings.TrimSuffix(name, ".vcf"), 10, 64); err == nil && strings.HasSuffix(name, ".vcf") {
		if s.db.QueryRow(ctx, `SELECT id FROM crm_parties WHERE id=$1 AND dav_href IS NULL`, n).Scan(&id) == nil {
			return id
		}
	}
	return 0
}

// ---------- PROPFIND ----------

func (s *Server) davPropfind(w http.ResponseWriter, r *http.Request, u *davUser, seg []string) {
	var want []xml.Name
	if body, _ := io.ReadAll(io.LimitReader(r.Body, davMaxBody)); len(strings.TrimSpace(string(body))) > 0 {
		root, err := parseXMLTree(strings.NewReader(string(body)))
		if err != nil || root == nil {
			davError(w, http.StatusBadRequest, "Invalid XML")
			return
		}
		if root.child(nsDAV, "allprop") == nil {
			want = wantedProps(root)
		}
	}
	ctx := r.Context()
	depth := davDepth(r)
	var out []davResp
	switch {
	case len(seg) == 0: // /dav/
		p := u.commonProps()
		p[qn(nsDAV, "resourcetype")] = "<d:collection/>"
		p[qn(nsCal, "calendar-home-set")] = href(u.calHome())
		p[qn(nsCard, "addressbook-home-set")] = href(u.cardHome())
		out = append(out, davResp{href: "/dav/", props: p})
		if depth == 1 {
			out = append(out, davResp{href: u.principal(), props: u.principalProps()})
		}
	case seg[0] == "principals" && len(seg) <= 2:
		if len(seg) == 1 {
			out = append(out, davResp{href: "/dav/principals/", props: u.homeProps("principals")})
			if depth == 1 {
				out = append(out, davResp{href: u.principal(), props: u.principalProps()})
			}
		} else {
			out = append(out, davResp{href: u.principal(), props: u.principalProps()})
		}
	case seg[0] == "calendars" && len(seg) == 2:
		out = append(out, davResp{href: u.calHome(), props: u.homeProps(firstNonEmpty(u.name, u.login))})
		if depth == 1 {
			cals, err := s.davCalendars(ctx, u, 0)
			if err != nil {
				serverErr(w, "dav calendars", err)
				return
			}
			for _, c := range cals {
				out = append(out, davResp{href: u.calHome() + strconv.FormatInt(c.id, 10) + "/", props: u.calendarProps(c)})
			}
		}
	case seg[0] == "calendars" && len(seg) == 3:
		cal, ok := s.davCalendar(ctx, u, seg[2])
		if !ok {
			davError(w, http.StatusNotFound, "No such calendar")
			return
		}
		calPath := u.calHome() + seg[2] + "/"
		out = append(out, davResp{href: calPath, props: u.calendarProps(cal)})
		if depth == 1 {
			evs, err := s.davEvents(ctx, cal.id, nil)
			if err != nil {
				serverErr(w, "dav events", err)
				return
			}
			withData := wants(want, qn(nsCal, "calendar-data"))
			for _, d := range evs {
				out = append(out, u.eventProps(calPath, d, withData))
			}
		}
	case seg[0] == "calendars" && len(seg) == 4:
		cal, ok := s.davCalendar(ctx, u, seg[2])
		if !ok {
			davError(w, http.StatusNotFound, "No such calendar")
			return
		}
		evs, err := s.davEvents(ctx, cal.id, []string{seg[3]})
		if err != nil || len(evs) == 0 {
			davError(w, http.StatusNotFound, "No such event")
			return
		}
		out = append(out, u.eventProps(u.calHome()+seg[2]+"/", evs[0], wants(want, qn(nsCal, "calendar-data"))))
	case seg[0] == "contacts" && len(seg) == 2:
		out = append(out, davResp{href: u.cardHome(), props: u.homeProps(firstNonEmpty(u.name, u.login))})
		if depth == 1 {
			out = append(out, davResp{href: u.book(), props: u.bookProps(s.bookCtag(ctx))})
		}
	case seg[0] == "contacts" && len(seg) == 3 && seg[2] == "crm":
		out = append(out, davResp{href: u.book(), props: u.bookProps(s.bookCtag(ctx))})
		if depth == 1 {
			cards, err := s.davCards(ctx, nil)
			if err != nil {
				serverErr(w, "dav cards", err)
				return
			}
			withData := wants(want, qn(nsCard, "address-data"))
			for _, c := range cards {
				out = append(out, u.cardProps(c, withData))
			}
		}
	case seg[0] == "contacts" && len(seg) == 4 && seg[2] == "crm":
		cards, err := s.davCards(ctx, []string{seg[3]})
		if err != nil || len(cards) == 0 {
			davError(w, http.StatusNotFound, "No such contact")
			return
		}
		out = append(out, u.cardProps(cards[0], wants(want, qn(nsCard, "address-data"))))
	default:
		davError(w, http.StatusNotFound, "Not found")
		return
	}
	writeMultistatus(w, out, want)
}

func wants(want []xml.Name, n xml.Name) bool {
	for _, x := range want {
		if x == n {
			return true
		}
	}
	return false
}

func (s *Server) davCalendar(ctx context.Context, u *davUser, idStr string) (davCal, bool) {
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		return davCal{}, false
	}
	cals, err := s.davCalendars(ctx, u, id)
	if err != nil || len(cals) == 0 {
		return davCal{}, false
	}
	return cals[0], true
}

// ---------- REPORT ----------

func (s *Server) davReport(w http.ResponseWriter, r *http.Request, u *davUser, seg []string) {
	root, err := parseXMLTree(r.Body)
	if err != nil || root == nil {
		davError(w, http.StatusBadRequest, "Invalid XML")
		return
	}
	ctx := r.Context()
	want := wantedProps(root)
	hrefNames := func(base string) []string {
		names := []string{}
		for _, h := range root.all(nsDAV, "href") {
			p := strings.TrimSpace(h.text)
			if pu, err := url.Parse(p); err == nil {
				p = pu.Path
			}
			if name, ok := strings.CutPrefix(p, base); ok && name != "" && !strings.Contains(name, "/") {
				if n, err := url.PathUnescape(name); err == nil {
					names = append(names, n)
				}
			}
		}
		return names
	}
	switch {
	case seg != nil && seg[0] == "calendars" && len(seg) == 3:
		cal, ok := s.davCalendar(ctx, u, seg[2])
		if !ok {
			davError(w, http.StatusNotFound, "No such calendar")
			return
		}
		calPath := u.calHome() + seg[2] + "/"
		withData := wants(want, qn(nsCal, "calendar-data"))
		var out []davResp
		switch root.name {
		case qn(nsCal, "calendar-multiget"):
			names := hrefNames(calPath)
			evs, err := s.davEvents(ctx, cal.id, names)
			if err != nil {
				serverErr(w, "dav multiget", err)
				return
			}
			found := map[string]bool{}
			for _, d := range evs {
				found[d.href] = true
				out = append(out, u.eventProps(calPath, d, withData))
			}
			for _, n := range names {
				if !found[n] {
					out = append(out, davResp{href: calPath + url.PathEscape(n), status: http.StatusNotFound})
				}
			}
		case qn(nsCal, "calendar-query"):
			evs, err := s.davEvents(ctx, cal.id, nil)
			if err != nil {
				serverErr(w, "dav query", err)
				return
			}
			wantTask, wantEvent := true, true
			for _, cf := range root.all(nsCal, "comp-filter") {
				switch strings.ToUpper(cf.attr("name")) {
				case "VEVENT":
					wantTask = false
				case "VTODO":
					wantEvent = false
				}
			}
			var from, to time.Time
			if tr := root.all(nsCal, "time-range"); len(tr) > 0 {
				from, _ = time.Parse("20060102T150405Z", tr[0].attr("start"))
				to, _ = time.Parse("20060102T150405Z", tr[0].attr("end"))
			}
			for _, d := range evs {
				if (d.e.IsTask && !wantTask) || (!d.e.IsTask && !wantEvent) {
					continue
				}
				if d.e.RRule == "" && !d.e.IsTask {
					end := d.e.Start
					if d.e.End != nil {
						end = *d.e.End
					}
					if d.e.AllDay {
						end = end.AddDate(0, 0, 1)
					}
					if (!to.IsZero() && !d.e.Start.Before(to)) || (!from.IsZero() && end.Before(from)) {
						continue
					}
				}
				out = append(out, u.eventProps(calPath, d, withData))
			}
		default:
			davError(w, http.StatusForbidden, "Unsupported report")
			return
		}
		writeMultistatus(w, out, want)
	case seg != nil && seg[0] == "contacts" && len(seg) == 3 && seg[2] == "crm":
		withData := wants(want, qn(nsCard, "address-data"))
		var out []davResp
		switch root.name {
		case qn(nsCard, "addressbook-multiget"):
			names := hrefNames(u.book())
			cards, err := s.davCards(ctx, names)
			if err != nil {
				serverErr(w, "dav card multiget", err)
				return
			}
			found := map[string]bool{}
			for _, c := range cards {
				found[c.Href] = true
				out = append(out, u.cardProps(c, withData))
			}
			for _, n := range names {
				if !found[n] {
					out = append(out, davResp{href: u.book() + url.PathEscape(n), status: http.StatusNotFound})
				}
			}
		case qn(nsCard, "addressbook-query"):
			cards, err := s.davCards(ctx, nil)
			if err != nil {
				serverErr(w, "dav card query", err)
				return
			}
			for _, c := range cards {
				out = append(out, u.cardProps(c, withData))
			}
		default:
			davError(w, http.StatusForbidden, "Unsupported report")
			return
		}
		writeMultistatus(w, out, want)
	default:
		// expand-property, principal-search and others: nothing to report here.
		writeMultistatus(w, nil, nil)
	}
}

// PROPPATCH: phones set colours and order of calendars. The calendar is the same for everyone
// using it, so the change is accepted and not stored (the name and colour are set in TMAIL).
func (s *Server) davProppatch(w http.ResponseWriter, r *http.Request, u *davUser, seg []string) {
	root, err := parseXMLTree(r.Body)
	if err != nil || root == nil {
		davError(w, http.StatusBadRequest, "Invalid XML")
		return
	}
	props := davProps{}
	var names []xml.Name
	for _, set := range append(root.all(nsDAV, "set"), root.all(nsDAV, "remove")...) {
		for _, p := range set.child(nsDAV, "prop").kidsOrNil() {
			props[p.name] = ""
			names = append(names, p.name)
		}
	}
	writeMultistatus(w, []davResp{{href: r.URL.Path, props: props}}, names)
}

func (n *xnode) kidsOrNil() []*xnode {
	if n == nil {
		return nil
	}
	return n.kids
}

// ---------- GET / PUT / DELETE ----------

func (s *Server) davGet(w http.ResponseWriter, r *http.Request, u *davUser, seg []string) {
	ctx := r.Context()
	switch {
	case len(seg) == 4 && seg[0] == "calendars":
		cal, ok := s.davCalendar(ctx, u, seg[2])
		if !ok {
			davError(w, http.StatusNotFound, "No such calendar")
			return
		}
		evs, err := s.davEvents(ctx, cal.id, []string{seg[3]})
		if err != nil || len(evs) == 0 {
			davError(w, http.StatusNotFound, "No such event")
			return
		}
		d := evs[0]
		w.Header().Set("Content-Type", "text/calendar; charset=utf-8")
		w.Header().Set("ETag", `"`+d.etag()+`"`)
		w.Header().Set("Last-Modified", d.updated.UTC().Format(http.TimeFormat))
		if r.Method == http.MethodGet {
			_, _ = io.WriteString(w, eventObject(&d.e, d.uid, u.tz, d.raw))
		}
	case len(seg) == 4 && seg[0] == "contacts" && seg[2] == "crm":
		cards, err := s.davCards(ctx, []string{seg[3]})
		if err != nil || len(cards) == 0 {
			davError(w, http.StatusNotFound, "No such contact")
			return
		}
		data := encodeCard(partyCard(cards[0]))
		w.Header().Set("Content-Type", "text/vcard; charset=utf-8")
		w.Header().Set("ETag", `"`+cardETag(data)+`"`)
		if r.Method == http.MethodGet {
			_, _ = io.WriteString(w, data)
		}
	default:
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = io.WriteString(w, "TMAIL calendars and contacts (CalDAV/CardDAV). Add this address to your phone or mail program.\n")
	}
}

// matchETag applies If-Match / If-None-Match to a resource whose current tag is cur ("" = none).
func matchETag(r *http.Request, cur string) bool {
	if im := r.Header.Get("If-Match"); im != "" && im != "*" && strings.Trim(im, `" `) != cur {
		return false
	}
	if r.Header.Get("If-Match") == "*" && cur == "" {
		return false
	}
	if inm := r.Header.Get("If-None-Match"); inm == "*" && cur != "" {
		return false
	}
	return true
}

func (s *Server) davPut(w http.ResponseWriter, r *http.Request, u *davUser, seg []string) {
	ctx := r.Context()
	body, err := io.ReadAll(io.LimitReader(r.Body, davMaxBody+1))
	if err != nil || len(body) > davMaxBody {
		davError(w, http.StatusRequestEntityTooLarge, "Too large")
		return
	}
	switch {
	case len(seg) == 4 && seg[0] == "calendars":
		cal, ok := s.davCalendar(ctx, u, seg[2])
		if !ok {
			davError(w, http.StatusNotFound, "No such calendar")
			return
		}
		if !cal.canWrite {
			davError(w, http.StatusForbidden, "This calendar is read-only for you")
			return
		}
		name := seg[3]
		raw := string(body)
		e, uid, err := parseICS(raw, u.tz)
		if err != nil {
			davError(w, http.StatusBadRequest, "Invalid iCalendar: "+err.Error())
			return
		}
		if uid == "" {
			uid = newUID()
		}
		evs, err := s.davEvents(ctx, cal.id, []string{name})
		if err != nil {
			serverErr(w, "dav put lookup", err)
			return
		}
		cur := ""
		if len(evs) > 0 {
			cur = evs[0].etag()
		}
		if !matchETag(r, cur) {
			davError(w, http.StatusPreconditionFailed, "The event was changed meanwhile")
			return
		}
		var id int64
		var updated time.Time
		color := cal.color
		if e.Color != "blue" {
			color = e.Color
		}
		if len(evs) == 0 {
			err = s.db.QueryRow(ctx, `INSERT INTO cal_events (user_id, calendar_id, uid, href, title, notes, starts_at, ends_at, all_day, color,
				is_task, done, remind_minutes, rrule, ical, remind_at) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)
				RETURNING id, updated_at`,
				u.id, cal.id, uid, name, truncate(e.Title, 300), e.Notes, e.Start, e.End, e.AllDay, color, e.IsTask, e.Done, e.RemindMinutes,
				e.RRule, raw, remindAt(e, uid, raw, u.tz)).Scan(&id, &updated)
		} else {
			old := evs[0].e
			rearm := !old.Start.Equal(e.Start) || old.RRule != e.RRule || (old.RemindMinutes == nil) != (e.RemindMinutes == nil) ||
				(old.RemindMinutes != nil && e.RemindMinutes != nil && *old.RemindMinutes != *e.RemindMinutes)
			err = s.db.QueryRow(ctx, `UPDATE cal_events SET uid=$2, title=$3, notes=$4, starts_at=$5, ends_at=$6, all_day=$7, is_task=$8, done=$9,
				remind_minutes=$10, rrule=$11, ical=$12,
				remind_at = CASE WHEN $13 THEN $14 ELSE remind_at END,
				reminded_at = CASE WHEN $13 THEN NULL ELSE reminded_at END,
				ack_at = CASE WHEN $13 THEN NULL ELSE ack_at END,
				snooze_until = CASE WHEN $13 THEN NULL ELSE snooze_until END,
				updated_at=now() WHERE id=$1 RETURNING id, updated_at`,
				old.ID, uid, truncate(e.Title, 300), e.Notes, e.Start, e.End, e.AllDay, e.IsTask, e.Done, e.RemindMinutes, e.RRule, raw,
				rearm, remindAt(e, uid, raw, u.tz)).Scan(&id, &updated)
		}
		if err != nil {
			serverErr(w, "dav put event", err)
			return
		}
		w.Header().Set("ETag", `"`+davEvent{e: CalEvent{ID: id}, updated: updated}.etag()+`"`)
		if len(evs) == 0 {
			w.WriteHeader(http.StatusCreated)
		} else {
			w.WriteHeader(http.StatusNoContent)
		}
	case len(seg) == 4 && seg[0] == "contacts" && seg[2] == "crm":
		name := seg[3]
		c, err := vcard.NewDecoder(strings.NewReader(string(body))).Decode()
		if err != nil {
			davError(w, http.StatusBadRequest, "Invalid vCard")
			return
		}
		id := s.partyByHref(ctx, name)
		cur := ""
		if id > 0 {
			if cards, err := s.davCards(ctx, []string{name}); err == nil && len(cards) == 1 {
				cur = cardETag(encodeCard(partyCard(cards[0])))
			}
		}
		if !matchETag(r, cur) {
			davError(w, http.StatusPreconditionFailed, "The contact was changed meanwhile")
			return
		}
		newID, err := s.savePartyCard(ctx, id, name, c)
		if err != nil {
			davError(w, http.StatusBadRequest, err.Error())
			return
		}
		s.audit(ctx, u.id, u.login, clientIP(r), "crm.dav_save", strconv.FormatInt(newID, 10), nil)
		if cards, err := s.vcParties(ctx, "WHERE p.id=$1", newID); err == nil && len(cards) == 1 {
			w.Header().Set("ETag", `"`+cardETag(encodeCard(partyCard(cards[0])))+`"`)
		}
		if id == 0 {
			w.WriteHeader(http.StatusCreated)
		} else {
			w.WriteHeader(http.StatusNoContent)
		}
	default:
		davError(w, http.StatusForbidden, "Cannot write here")
	}
}

func (s *Server) davDelete(w http.ResponseWriter, r *http.Request, u *davUser, seg []string) {
	ctx := r.Context()
	switch {
	case len(seg) == 4 && seg[0] == "calendars":
		cal, ok := s.davCalendar(ctx, u, seg[2])
		if !ok {
			davError(w, http.StatusNotFound, "No such calendar")
			return
		}
		if !cal.canWrite {
			davError(w, http.StatusForbidden, "This calendar is read-only for you")
			return
		}
		tag, err := s.db.Exec(ctx, `DELETE FROM cal_events WHERE calendar_id=$1 AND href=$2`, cal.id, seg[3])
		if err != nil {
			serverErr(w, "dav delete event", err)
			return
		}
		if tag.RowsAffected() == 0 {
			davError(w, http.StatusNotFound, "No such event")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	case len(seg) == 4 && seg[0] == "contacts" && seg[2] == "crm":
		// Counterparties are shared by the whole company: only an administrator removes them.
		id := s.partyByHref(ctx, seg[3])
		if id == 0 {
			davError(w, http.StatusNotFound, "No such contact")
			return
		}
		if !u.admin {
			davError(w, http.StatusForbidden, "Only an administrator can delete counterparties")
			return
		}
		if _, err := s.db.Exec(ctx, `DELETE FROM crm_parties WHERE id=$1`, id); err != nil {
			serverErr(w, "dav delete contact", err)
			return
		}
		s.audit(ctx, u.id, u.login, clientIP(r), "crm.dav_delete", strconv.FormatInt(id, 10), nil)
		w.WriteHeader(http.StatusNoContent)
	default:
		davError(w, http.StatusForbidden, "Cannot delete this")
	}
}
