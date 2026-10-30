package main

import (
	"strings"
	"testing"
)

func TestAntivirusHelpers(t *testing.T) {
	if v, err := parseClamReply("stream: Win.Test.EICAR_HDB-1 FOUND\x00"); err != nil || v != "Win.Test.EICAR_HDB-1" {
		t.Fatalf("found: %q %v", v, err)
	}
	if v, err := parseClamReply("stream: OK\x00"); err != nil || v != "" {
		t.Fatalf("ok: %q %v", v, err)
	}
	if _, err := parseClamReply("INSTREAM size limit exceeded. ERROR\x00"); err == nil {
		t.Fatal("error reply must fail")
	}
	kinds := map[string]string{
		"Heuristics.OLE2.ContainsMacros": "macro",
		"Heuristics.Encrypted.Zip":       "encrypted",
		"Heuristics.Broken.Executable":   "heuristic",
		"Sanesecurity.Junk.12345":        "spam",
		"Sanesecurity.Phishing.Bank.1":   "spam",
		"Sanesecurity.BadMacro.Doc.1":    "virus",
		"Win.Trojan.Agent-1":             "virus",
		"Porcupine.Malware.1":            "virus",
	}
	for name, want := range kinds {
		if got := avKind(name); got != want {
			t.Errorf("avKind(%s) = %s, want %s", name, got, want)
		}
	}
	if f := avFlags([]string{"File is encrypted"}); len(f) != 1 || f[0] != "encrypted" {
		t.Errorf("rspamd wording: %v", f)
	}
	if f := avFlags([]string{"Heuristics.OLE2.ContainsMacros, File is encrypted: Heuristics.Encrypted.Zip"}); strings.Join(f, ",") != "macro,encrypted" {
		t.Errorf("avFlags: %v", f)
	}
	for name, want := range map[string]string{"Счёт.pdf.exe": "exec", "отчёт.XLSM": "macro", "вход.html": "html", "договор.pdf": "", "архив.zip": ""} {
		if got := attachmentRisk(name); got != want {
			t.Errorf("risk(%s) = %q, want %q", name, got, want)
		}
	}
	raw := "From: a@b.ru\r\nX-Virus: forged\r\n  continued\r\nSubject: x\r\n\r\nX-Virus: in body stays\r\n"
	out := string(setVirusHeader([]byte(raw), "Heuristics.Encrypted.Zip"))
	if !strings.HasPrefix(out, "X-Virus: Heuristics.Encrypted.Zip\r\nFrom: a@b.ru\r\nSubject: x\r\n\r\nX-Virus: in body stays") {
		t.Errorf("setVirusHeader:\n%s", out)
	}
}

func TestHasFiles(t *testing.T) {
	plain := "From: a@b.ru\r\nContent-Type: text/plain; charset=utf-8\r\n\r\nПривет"
	withFile := "Content-Type: multipart/mixed; boundary=x\r\n\r\n--x\r\nContent-Type: application/pdf; name=\"a.pdf\"\r\nContent-Disposition: attachment; filename=\"a.pdf\"\r\n\r\nJVBER\r\n--x--"
	if reHasFiles.MatchString(plain) || !reHasFiles.MatchString(withFile) {
		t.Fatal("reHasFiles")
	}
}
