package main

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFilePageStatuses(t *testing.T) {
	r := httptest.NewRequest("GET", "/f/t", nil)
	r.Header.Set("Accept-Language", "en-US,en;q=0.9")
	for status, want := range map[string]string{"clean": "Checked by antivirus", "pending": "being checked by the antivirus", "infected": "Download is blocked"} {
		var b strings.Builder
		if err := filePageTpl.Execute(&b, pageLocale(r, map[string]any{"Name": "f.zip", "Token": "t", "Status": status, "VirusNote": " (X)"})); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(b.String(), want) || (status != "clean") == strings.Contains(b.String(), "/download\">Download") {
			t.Errorf("%s: wrong page", status)
		}
	}
	var b strings.Builder
	if err := filePageTpl.Execute(&b, pageLocale(r, map[string]any{"Gone": true, "Status": ""})); err != nil {
		t.Fatal(err)
	}
}

// The visitor's browser language decides; an unknown one falls back to the server's or English.
func TestFilePageLanguage(t *testing.T) {
	r := httptest.NewRequest("GET", "/f/t", nil)
	r.Header.Set("Accept-Language", "ru-RU,ru;q=0.9,en;q=0.8")
	var b strings.Builder
	if err := filePageTpl.Execute(&b, pageLocale(r, map[string]any{"Name": "f.zip", "Token": "t", "Status": "clean"})); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(b.String(), `lang="ru"`) || !strings.Contains(b.String(), "Скачать") {
		t.Error("Russian browser: page not in Russian")
	}
	if got := pickPageLang("xx-YY"); got != "en" && pageTexts[got] == nil {
		t.Errorf("unknown language: got %q", got)
	}
}
