package main

import (
	"bytes"
	"context"
	"mime"
	"strings"
	"testing"

	"github.com/ProtonMail/gopenpgp/v3/crypto"
)

func testMessage() []byte {
	return []byte("From: a@example.com\r\nTo: b@example.org\r\nSubject: Hello\r\nMIME-Version: 1.0\r\n" +
		"Content-Type: multipart/alternative; boundary=\"x\"\r\n\r\n--x\r\nContent-Type: text/plain; charset=utf-8\r\nContent-Transfer-Encoding: quoted-printable\r\n\r\n" +
		"Secret text =D0=BF=D1=80=D0=B8=D0=B2=D0=B5=D1=82\r\n--x\r\nContent-Type: text/html; charset=utf-8\r\nContent-Transfer-Encoding: quoted-printable\r\n\r\n<p>Secret text</p>\r\n--x--\r\n")
}

func TestPGPSignAndVerify(t *testing.T) {
	own, err := pgp.KeyGeneration().AddUserId("A", "a@example.com").New().GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	out, err := wrapPGP(testMessage(), own, nil, true, false)
	if err != nil {
		t.Fatal(err)
	}
	h, body, ok := splitHeaderBody(out)
	if !ok || h.Get("Subject") != "Hello" {
		t.Fatalf("outer headers lost:\n%s", out)
	}
	mt, params, _ := mime.ParseMediaType(h.Get("Content-Type"))
	if mt != "multipart/signed" {
		t.Fatalf("not signed: %s", mt)
	}
	parts := splitMultipart(body, params["boundary"])
	if len(parts) != 2 {
		t.Fatalf("parts: %d", len(parts))
	}
	_, sig, _ := splitHeaderBody(parts[1])
	kr, _ := crypto.NewKeyRing(own)
	v, _ := pgp.Verify().VerificationKeys(kr).New()
	res, err := v.VerifyDetached(parts[0], sig, crypto.Auto)
	if err != nil || res.SignatureError() != nil {
		t.Fatalf("signature does not verify: %v %v", err, res.SignatureError())
	}
	// A changed letter fails.
	res, _ = v.VerifyDetached(bytes.Replace(parts[0], []byte("Secret"), []byte("Public"), 1), sig, crypto.Auto)
	if res == nil || res.SignatureError() == nil {
		t.Fatal("tampered letter verified")
	}
}

func TestPGPEncryptAndRead(t *testing.T) {
	own, _ := pgp.KeyGeneration().AddUserId("A", "a@example.com").New().GenerateKey()
	other, _ := pgp.KeyGeneration().AddUserId("B", "b@example.org").New().GenerateKey()
	otherPub, _ := other.ToPublic()
	rk, _ := crypto.NewKeyRing(otherPub)
	out, err := wrapPGP(testMessage(), own, rk, true, true)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(out, []byte("Secret")) {
		t.Fatal("plain text visible in the encrypted letter")
	}
	h, body, _ := splitHeaderBody(out)
	_, params, _ := mime.ParseMediaType(h.Get("Content-Type"))
	parts := splitMultipart(body, params["boundary"])
	_, armored, _ := splitHeaderBody(parts[1])
	s := &Server{}
	for _, k := range []*crypto.Key{own, other} { // the recipient and the sender's own copy
		inner, st, err := s.decryptFor(context.Background(), k, "", armored)
		if err != nil {
			t.Fatal(err)
		}
		html, text, _ := parseInner(inner)
		if !strings.Contains(text, "Secret text привет") || !strings.Contains(html, "Secret text") {
			t.Fatalf("decrypted body: %q %q", text, html)
		}
		if st == nil || !st.Signed {
			t.Fatal("inner signature not seen")
		}
	}
}
