package main

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// Pages the server renders itself for people outside TMAIL (the download page of a file sent as a
// link) speak the visitor's language: the first language of the browser that has a translation,
// else the server's default language, else English. The texts are the "filepage.*" keys of the
// interface translations, exported by web/scripts/export-server-i18n.mjs.

//go:embed pagelocale.json
var pageTextsJSON []byte

var pageTexts = func() map[string]map[string]string {
	m := map[string]map[string]string{}
	_ = json.Unmarshal(pageTextsJSON, &m)
	return m
}()

var rtlLangs = map[string]bool{"ar": true, "he": true, "fa": true, "ur": true, "ps": true, "sd": true, "ckb": true}

// pickPageLang matches an Accept-Language header against the available translations.
func pickPageLang(accept string) string {
	for _, part := range strings.Split(accept, ",") {
		tag := strings.TrimSpace(strings.SplitN(part, ";", 2)[0])
		if tag == "" || tag == "*" {
			continue
		}
		if _, ok := pageTexts[tag]; ok {
			return tag
		}
		base := strings.ToLower(strings.SplitN(tag, "-", 2)[0])
		if base == "zh" {
			if strings.Contains(strings.ToLower(tag), "tw") || strings.Contains(strings.ToLower(tag), "hant") || strings.Contains(strings.ToLower(tag), "hk") {
				base = "zh-TW"
			} else {
				base = "zh-CN"
			}
		} else if base == "pt" {
			base = "pt-BR"
			if strings.EqualFold(tag, "pt-PT") {
				base = "pt-PT"
			}
		} else if base == "no" || base == "nn" {
			base = "nb"
		}
		if _, ok := pageTexts[base]; ok {
			return base
		}
	}
	if d := serverLocale().Lang; pageTexts[d] != nil {
		return d
	}
	return "en"
}

// pageLocale adds Lang, Dir, Org and the translation function T (key, then name/value pairs for
// the {placeholders}) to a page template's data.
func pageLocale(r *http.Request, data map[string]any) map[string]any {
	lang := pickPageLang(r.Header.Get("Accept-Language"))
	dict, en := pageTexts[lang], pageTexts["en"]
	data["Lang"] = lang
	data["Dir"] = "ltr"
	if rtlLangs[lang] {
		data["Dir"] = "rtl"
	}
	data["Org"] = env("ORG_NAME", "TMAIL")
	data["T"] = func(key string, kv ...any) string {
		text, ok := dict[key]
		if !ok {
			text = en[key]
		}
		for i := 0; i+1 < len(kv); i += 2 {
			v := ""
			if kv[i+1] != nil {
				v = fmt.Sprint(kv[i+1])
			}
			text = strings.ReplaceAll(text, "{"+fmt.Sprint(kv[i])+"}", v)
		}
		return text
	}
	return data
}

// parenthesized: " (name)" after a sentence's subject, or nothing.
func parenthesized(s string) string {
	if s == "" {
		return ""
	}
	return " (" + s + ")"
}
