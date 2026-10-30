package main

import "testing"

func TestCleanTaxID(t *testing.T) {
	t.Setenv("COUNTRY", "")
	cases := []struct{ raw, country, id, cc string }{
		{"7707083893", "", "7707083893", "RU"},         // a valid INN on a server without a country: Russian
		{"ИНН 7707 083 893", "ru", "7707083893", "RU"}, // label and spaces dropped
		{"7707083894", "RU", "", "RU"},                 // wrong checksum: a misread number
		{"7707083894", "", "", ""},
		{"7707083893/770701001", "", "", ""}, // INN and KPP stuck together
		{"DE 123.456-789", "", "DE123456789", ""},
		{"de123456789", "Germany", "DE123456789", "DE"},
		{"12-3456789", "US", "123456789", "US"},      // EIN: no INN checksum for other countries
		{"771234567890", "KZ", "771234567890", "KZ"}, // a BIN looks like an INN, but it is Kazakh
		{"", "UK", "", "GB"},
	}
	for _, c := range cases {
		if id, cc := cleanTaxID(c.raw, c.country); id != c.id || cc != c.cc {
			t.Errorf("cleanTaxID(%q, %q) = %q, %q; want %q, %q", c.raw, c.country, id, cc, c.id, c.cc)
		}
	}
	// A server in another country reads an unknown company's ID without the Russian rules.
	t.Setenv("COUNTRY", "us")
	if id, cc := cleanTaxID("12-3456789", ""); id != "123456789" || cc != "" {
		t.Errorf("US server: %q %q", id, cc)
	}
	ids := cleanIDs(map[string]string{"VAT": " DE123456789 ", "Tax ID": "12-3456789", "": "x", "Empty": " "}, "123456789")
	if len(ids) != 1 || ids["VAT"] != "DE123456789" {
		t.Errorf("cleanIDs: %v", ids)
	}
	if m := parseIDs([]byte(`{"Company number": 1234567, "VAT": "GB123"}`)); m["Company number"] != "1234567" || m["VAT"] != "GB123" {
		t.Errorf("parseIDs: %v", m)
	}
	if m := parseIDs([]byte(`["x"]`)); m != nil {
		t.Errorf("parseIDs of a list: %v", m)
	}
}
