package main

import "testing"

func TestCardDocNames(t *testing.T) {
	for name, want := range map[string]bool{
		"ООО_Ромашка_Карта_партнера (6).pdf":   true,
		"Информационная карта ООО Ромашка.pdf": true,
		"Карта предприятия с 29.06.pdf":        true,
		"КАРТОЧКА  ПРЕДПРИЯТИЯ Ромашка.doc":    true,
		"Сведения о компании.docx":             true,
		"Карта Сбербанк 1234.jpg":              false,
		"Запрос от 02.11..pdf":                 false,
		"Фото.jpg":                             false,
	} {
		if got := reCardDoc.MatchString(name); got != want {
			t.Errorf("reCardDoc(%q) = %v, want %v", name, got, want)
		}
		if want && !reDocName.MatchString(name) {
			t.Errorf("reDocName(%q) = false", name)
		}
	}
}
