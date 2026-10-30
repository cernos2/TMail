package main

import (
	"strings"
	"testing"
)

func TestStripOwnURLs(t *testing.T) {
	t.Setenv("MAIL_HOSTNAME", "mail.example.com")
	cases := map[string]bool{ // input → must survive
		`<img src="/api/avatar?email=x@evil.ru&bimi=1">`:               false,
		`<img src="https://mail.example.com/api/message?uid=1">`:       false,
		`<img src="//MAIL.example.com:443/api/x">`:                     false,
		`<div style="background:url('/api/attachment?uid=2&as=ocr')">`: false,
		`<img src="https://cdn.other.ru/logo.png">`:                    true,
		`<a href="https://site.ru/page">x</a>`:                         true,
	}
	for in, keep := range cases {
		out := stripOwnURLs(in)
		if (out == in) != keep {
			t.Errorf("%s → %s (keep=%v)", in, out, keep)
		}
	}
	out, _ := sanitizeHTML(`<img src="cid:a1">`, func(string) string { return "/api/attachment?uid=5&part=2" })
	if !strings.Contains(out, "/api/attachment?uid=5") {
		t.Errorf("cid image lost: %s", out)
	}
}
