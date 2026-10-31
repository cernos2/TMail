package main

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"testing"

	"github.com/emersion/go-imap"
)

// Binary attachments must survive decoding byte for byte.
func TestDecodeTransferBinary(t *testing.T) {
	data := make([]byte, 70000)
	_, _ = rand.Read(data)
	enc := base64.StdEncoding.EncodeToString(data)
	var wrapped bytes.Buffer
	for i := 0; i < len(enc); i += 76 {
		end := min(i+76, len(enc))
		wrapped.WriteString(enc[i:end] + "\r\n")
	}
	if got := decodeTransfer(wrapped.Bytes(), "base64"); !bytes.Equal(got, data) {
		t.Fatalf("binary data changed: got %d bytes, want %d", len(got), len(data))
	}
}

func TestPartFilename(t *testing.T) {
	cases := []struct {
		params map[string]string
		want   string
	}{
		{map[string]string{"filename": "plain.pdf"}, "plain.pdf"},
		{map[string]string{"filename": "=?utf-8?b?0JTQvtCz0L7QstC+0YAuZG9jeA==?="}, "Договор.docx"},
		{map[string]string{"filename*": "utf-8''%D0%94%D0%BE%D0%B3%D0%BE%D0%B2%D0%BE%D1%80.docx"}, "Договор.docx"},
		{map[string]string{"filename*0*": "utf-8''%D0%94%D0%BE%D0%B3", "filename*1*": "%D0%BE%D0%B2%D0%BE%D1%80", "filename*2": ".docx"}, "Договор.docx"},
		{map[string]string{"filename*": "windows-1251''%C4%EE%E3%EE%E2%EE%F0.doc"}, "Договор.doc"},
	}
	for _, c := range cases {
		if got := partFilename(&imap.BodyStructure{DispositionParams: c.params}); got != c.want {
			t.Errorf("%v: got %q, want %q", c.params, got, c.want)
		}
	}
}

func TestTextForPreviewCharset(t *testing.T) {
	cp1251 := []byte{0xD1, 0xEA, 0xEB, 0xE0, 0xE4} // "Склад"
	for _, cs := range []string{"", "us-ascii", "utf-8"} {
		if got := textForPreview(cp1251, cs); got != "Склад" {
			t.Errorf("charset %q: got %q", cs, got)
		}
	}
	if got := textForPreview([]byte("Склад"), "us-ascii"); got != "Склад" {
		t.Errorf("utf-8 bytes declared ascii: got %q", got)
	}
}

func TestTextForPreviewWrongDeclared(t *testing.T) {
	if got := textForPreview([]byte("Склад"), "windows-1251"); got != "Склад" {
		t.Errorf("utf-8 declared cp1251: got %q", got)
	}
}

func TestBaseSubject(t *testing.T) {
	for in, want := range map[string]string{"Re: RE: Fwd: Заказ  Сервис": "заказ сервис", "Отв: Счёт": "счёт", "Re[2]: Test": "test", "Заказ": "заказ", "RE: ***SPAM*** Re: Запрос ООО": "запрос ооо"} {
		if got := baseSubject(in); got != want {
			t.Errorf("%q: got %q, want %q", in, got, want)
		}
	}
}

func TestValidINN(t *testing.T) {
	for inn, want := range map[string]bool{"7707083893": true, "7707083894": false, "9701184850": true, "711700935600": true, "711700935601": false, "123": false} {
		if got := validINN(inn); got != want {
			t.Errorf("%s: got %v", inn, got)
		}
	}
	if p := normPhone("+7 (495) 123-45-68 доб. 12"); p != "+7(495) 123-45-68 доб. 12" {
		t.Errorf("phone ext: %q", p)
	}
	if p := normPhone("8 (495) 640-45-98"); p != "+7(495) 640-45-98" {
		t.Errorf("phone: %q", p)
	}
}

func TestNormPattern(t *testing.T) {
	for in, want := range map[string]string{"Иван <Ivan@Firma.RU>": "ivan@firma.ru", "@spam.ru": "@spam.ru", "spam.ru": "@spam.ru", "bad value": ""} {
		if got, _, _ := normPattern(in); got != want {
			t.Errorf("%q: got %q", in, got)
		}
	}
}

func TestDecodedSize(t *testing.T) {
	raw := make([]byte, 1000000)
	enc := base64.StdEncoding.EncodeToString(raw)
	lines := (len(enc) + 75) / 76
	size := uint32(len(enc) + lines*2)
	got := decodedSize(&imap.BodyStructure{Encoding: "base64", Size: size})
	if d := int(got) - len(raw); d < -100 || d > 100 {
		t.Errorf("decoded size %d for %d bytes", got, len(raw))
	}
}
