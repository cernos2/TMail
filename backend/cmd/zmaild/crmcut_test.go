package main

import (
	"strings"
	"testing"
)

func TestQuoteCut(t *testing.T) {
	cases := []string{
		"Добрый день, оплатили.\n\nпт, 10 янв. 2020 г. в 11:03, Иванов Пётр <ivanov.petr@gmail.com>:\n> Прошу оплатить",
		"Добрый день, оплатили.\n\n08.01.2020, 09:56, \"Иванов Пётр\" <ivanov.petr@gmail.com>:\nПрошу документы",
		"Пересылаю документы от поставщика.\n\n---------- Forwarded message ---------\nFrom: Воронов <v@alrosa.ru>\nДата: 8 янв.",
		"Смотрите ниже.\n\n-------- Пересылаемое сообщение --------\n08.01.2020, 12:12, \"Поплюйков\" <p@alrosa.ru>:",
	}
	for _, c := range cases {
		loc := reQuoteCut.FindStringIndex(c)
		if loc == nil || loc[0] > strings.Index(c, "\n\n")+2 { // right after the sender's own text
			t.Errorf("not cut early: %q (loc %v)", c, loc)
		}
	}
	if reQuoteCut.MatchString("Добрый день! Прошу выставить счёт на 7 насосов.\nС уважением, Максим") {
		t.Error("cut a plain letter")
	}
}

func TestFwdSubject(t *testing.T) {
	for s, want := range map[string]bool{"Fwd: документы": true, "FW: Счет": true, "Fwd: FW: Документы": true, "Пересл: акт": true, "Re: оплатить": false, "оплатить": false} {
		if reFwdSubject.MatchString(s) != want {
			t.Errorf("%q", s)
		}
	}
}

func TestCompanyKey(t *testing.T) {
	same := [][2]string{{`ООО "ГЛАВТЕПЛО"`, "ГлавТепло"}, {"ГлавТепло.ком", "ООО «ГлавТепло»"}, {"ИП Ведмеденко Андрей", "ведмеденко андрей"}, {`ОБЩЕСТВО С ОГРАНИЧЕННОЙ ОТВЕТСТВЕННОСТЬЮ "ГЛАВТЕПЛО"`, `ООО "ГЛАВТЕПЛО"`}, {`Интернет-магазин "ГлавТепло.ком"`, `ООО "ГЛАВТЕПЛО"`},
		{"Глав Тепло", "GlavTeplo"}, {"ГлавТепло - интернет-магазин", "ГЛАВТЕПЛО"}, {"ГлавТепло Ком", "главтепло"}, {"Glav-Teplo.ru", "ГлавТепло"},
		{"ООО «ТД Импульс» ОП Восток", "Общество с ограниченной ответственностью \"Торговый Дом Импульс\""}, {`ООО "Сантехплюс" 70-10-11`, "Сантехплюс"},
		{`ООО "СКМ" (Строительная Компания Милан)`, "СКМ"}}
	for _, p := range same {
		if companyKey(p[0]) != companyKey(p[1]) {
			t.Errorf("%q vs %q: %q %q", p[0], p[1], companyKey(p[0]), companyKey(p[1]))
		}
	}
	for _, p := range [][2]string{{"ООО ВТС", "ООО ТЕРЕМ"}, {"ТД Импульс", "Импульс"}, {"Импульс групп", "Импульс"}, {"ООО Ромашка", "ООО Ромашка 2"}} {
		if companyKey(p[0]) == companyKey(p[1]) {
			t.Errorf("different companies matched: %q %q", p[0], p[1])
		}
	}
}

func TestOwnName(t *testing.T) {
	o := ownIdent{names2: map[string]bool{}, keys: []string{companyKey(`ООО "ГЛАВТЕПЛО"`)}}
	for n, want := range map[string]bool{"ГлавТепло - интернет-магазин": true, "ГлавТепло Ком": true, "✪ ГлавТепло - интернет-магазин": true, `ООО "НП Теплокомплект"`: false, "ООО «ТЕРЕМ»": false} {
		if o.isOwnName(n) != want {
			t.Errorf("%q: want %v", n, want)
		}
	}
}
