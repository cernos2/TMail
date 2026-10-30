package main

import "testing"

func TestAddressQuery(t *testing.T) {
	for q, want := range map[string]bool{"marketplace@seller.ozon.ru": true, "@ozon.ru": true, "ozon.ru": true, "почта.рф": true,
		"счет": false, "ozon": false, "счет 123.45": false, "Иванов Иван": false, "1.5": false} {
		if reAddressQuery.MatchString(q) != want {
			t.Errorf("%q: want %v", q, want)
		}
	}
}
