package main

import (
	"encoding/json"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Counterparty cards for companies of any country. The column inn holds the main tax or
// registration ID of a company in its own country (INN in Russia, EIN in the US, a VAT or company
// number elsewhere), bik the bank code, account the account number or IBAN; ids holds the other
// identifiers. The Russian rules (INN checksum, 10/12 digits) apply only to Russian companies, or
// when the country is not known, the server works in Russia (or has no country set) and the ID
// looks like an INN.

// serverCountry: the country this mail service works in (COUNTRY), "" when not set.
func serverCountry() string { return serverLocale().Country }

// countryNames: what a model or a person may write instead of the ISO code.
var countryNames = map[string]string{"RUSSIA": "RU", "RUSSIAN FEDERATION": "RU", "RF": "RU", "РОССИЯ": "RU", "РФ": "RU",
	"РОССИЙСКАЯ ФЕДЕРАЦИЯ": "RU", "UK": "GB", "UNITED KINGDOM": "GB", "GREAT BRITAIN": "GB", "ENGLAND": "GB", "USA": "US",
	"UNITED STATES": "US", "UNITED STATES OF AMERICA": "US", "GERMANY": "DE", "FRANCE": "FR", "ITALY": "IT", "SPAIN": "ES",
	"POLAND": "PL", "NETHERLANDS": "NL", "CHINA": "CN", "INDIA": "IN", "BRAZIL": "BR", "TURKEY": "TR", "TÜRKIYE": "TR",
	"KAZAKHSTAN": "KZ", "КАЗАХСТАН": "KZ", "BELARUS": "BY", "БЕЛАРУСЬ": "BY", "UKRAINE": "UA", "УКРАИНА": "UA"}

// normCountry: an ISO 3166-1 alpha-2 code in upper case, or "" when it is not one.
func normCountry(c string) string {
	c = strings.ToUpper(strings.TrimSpace(c))
	if v, ok := countryNames[c]; ok {
		return v
	}
	if len(c) == 2 && c[0] >= 'A' && c[0] <= 'Z' && c[1] >= 'A' && c[1] <= 'Z' {
		return c
	}
	return ""
}

// normTaxID brings an identifier (tax ID, bank code, account number, IBAN…) to the form it is
// stored and compared in: Latin letters and digits only, upper case. "de 123.456-789" → "DE123456789";
// a label in another alphabet next to the number is dropped as well.
func normTaxID(s string) string {
	var b strings.Builder
	for _, r := range strings.ToUpper(s) {
		if r >= '0' && r <= '9' || r >= 'A' && r <= 'Z' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// hasLatin: the value has Latin letters (a VAT number, an IBAN…), so it is not a Russian INN.
func hasLatin(s string) bool {
	for _, r := range s {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' {
			return true
		}
	}
	return false
}

// innRules: whether the main ID of a company of country cc (normalised, may be "") is read as a
// Russian INN.
func innRules(raw, cc string) bool {
	if cc == "RU" {
		return true
	}
	if cc != "" {
		return false
	}
	sc := serverCountry()
	return (sc == "" || sc == "RU") && !hasLatin(raw)
}

// cleanTaxID returns the main ID of a company as it is stored, and its country: a valid Russian
// INN gives "RU" when the country was not known. A Russian INN with a wrong checksum is dropped
// (a misread number); other countries' IDs are only normalised.
func cleanTaxID(raw, country string) (id, cc string) {
	cc = normCountry(country)
	if strings.TrimSpace(raw) == "" {
		return "", cc
	}
	if innRules(raw, cc) {
		if d := reDigits.ReplaceAllString(raw, ""); validINN(d) {
			return d, "RU"
		}
		return "", cc
	}
	return normTaxID(raw), cc
}

// cleanIDs: the other identifiers of a company, {"label": "value"}, from the model's answer or a
// form. Values equal to the main ID are skipped; labels and values are kept as written.
func cleanIDs(in map[string]string, main string) map[string]string {
	out := map[string]string{}
	for k, v := range in {
		k, v = strings.TrimSpace(k), strings.TrimSpace(v)
		if k == "" || v == "" || utf8.RuneCountInString(k) > 40 || utf8.RuneCountInString(v) > 80 || len(out) >= 20 {
			continue
		}
		if main != "" && normTaxID(v) == main {
			continue
		}
		out[k] = v
	}
	return out
}

// parseIDs reads other_ids from the model's answer leniently: an object of strings or numbers.
func parseIDs(raw json.RawMessage) map[string]string {
	var m map[string]any
	if len(raw) == 0 || json.Unmarshal(raw, &m) != nil {
		return nil
	}
	out := map[string]string{}
	for k, v := range m {
		switch x := v.(type) {
		case string:
			out[k] = x
		case float64:
			out[k] = strconv.FormatFloat(x, 'f', -1, 64)
		}
	}
	return out
}

// idsJSON: the identifiers as JSON text for a jsonb column ("{}" when none).
func idsJSON(m map[string]string) string {
	if len(m) == 0 {
		return "{}"
	}
	b, err := json.Marshal(m)
	if err != nil {
		return "{}"
	}
	return string(b)
}
