package main

import (
	"mime"
	"sort"
	"strconv"
	"strings"

	"github.com/emersion/go-imap"
	"github.com/emersion/go-message/charset"
)

// partFilename returns an attachment's file name. Dovecot hands over parameters as they appear in
// the message, so RFC 2231 forms (filename*=utf-8”%D0%94…, filename*0*=…) used by Outlook,
// Thunderbird and most mail libraries for non-Latin names have to be decoded here; go-imap only
// handles plain and RFC 2047 (=?utf-8?b?…?=) values.
func partFilename(bs *imap.BodyStructure) string {
	if n := paramValue(bs.DispositionParams, "filename"); n != "" {
		return n
	}
	return paramValue(bs.Params, "name")
}

func paramValue(params map[string]string, key string) string {
	if v, ok := params[key]; ok && v != "" {
		return decodeWords(v)
	}
	var keys []string
	for k := range params {
		if strings.HasPrefix(strings.ToLower(k), key+"*") {
			keys = append(keys, k)
		}
	}
	if len(keys) == 0 {
		return ""
	}
	// Let the standard library do RFC 2231 decoding and continuation joining.
	sort.Slice(keys, func(i, j int) bool { return sectionOf(keys[i]) < sectionOf(keys[j]) })
	var b strings.Builder
	b.WriteString("x")
	for _, k := range keys {
		v := params[k]
		b.WriteString("; " + strings.ToLower(k) + "=")
		if strings.HasSuffix(k, "*") {
			b.WriteString(v) // extended value: charset''percent-encoded, no quoting
		} else {
			b.WriteString(strconv.Quote(v))
		}
	}
	if _, p, err := mime.ParseMediaType(b.String()); err == nil && p[key] != "" {
		return p[key]
	}
	// Unsupported charset (e.g. windows-1251''…): decode the percent-escapes ourselves.
	if v := params[key+"*"]; v != "" {
		if i := strings.Index(v, "''"); i >= 0 {
			cs, rest := v[:i], v[i+2:]
			if raw, err := percentDecode(rest); err == nil {
				return textForPreviewName(raw, cs)
			}
		}
	}
	return ""
}

func sectionOf(k string) int {
	k = strings.TrimSuffix(k, "*")
	if i := strings.LastIndex(k, "*"); i >= 0 {
		if n, err := strconv.Atoi(k[i+1:]); err == nil {
			return n
		}
	}
	return -1
}

func decodeWords(v string) string {
	if out, err := (&mime.WordDecoder{CharsetReader: charset.Reader}).DecodeHeader(v); err == nil {
		return out
	}
	return v
}

func percentDecode(s string) ([]byte, error) {
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		if s[i] == '%' && i+2 < len(s) {
			n, err := strconv.ParseUint(s[i+1:i+3], 16, 8)
			if err != nil {
				return nil, err
			}
			out = append(out, byte(n))
			i += 2
			continue
		}
		out = append(out, s[i])
	}
	return out, nil
}

func textForPreviewName(raw []byte, cs string) string {
	return strings.TrimSpace(textForPreview(raw, cs))
}
