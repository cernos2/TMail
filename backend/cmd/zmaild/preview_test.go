package main

import (
	"strings"
	"testing"
)

func TestPreviewText(t *testing.T) {
	for _, c := range []struct {
		in   string
		html bool
		want string
	}{
		{`<html><head><style>*{box-sizing:border-box}body{margin:0;padding:0}a[x-apple-data-detectors]{color:inherit`, true, ""},
		{"body { margin: 0; padding: 0; background: #f1f5f7; -webkit-text-size-adjust: 100%; } Остатки на складе", false, "Остатки на складе"},
		{`<div style="display:none!important;font-size:1px;color:#333333;line-height:1px;max-height:0`, true, ""},
		{`<!-- 96 --><p>Добрый день! Посчитала налоги</p>`, true, "Добрый день! Посчитала налоги"},
		{"Добрый день! Цена {по запросу} и скидка", false, "Добрый день! Цена по запросу и скидка"},
		{"24 октября ⠀⠀⠀⠀ ͏‌͏͏‌ ͏‌ на улицах", false, "24 октября на улицах"},
		{"/* Force Outlook to provide a link. */ Ваш заказ", false, "Ваш заказ"},
		{`@media all and (max-width: 480px){*[class].ib_t{min-width:100%}} Ваш заказ отправлен`, false, "Ваш заказ отправлен"},
	} {
		if got := strings.TrimSuffix(previewText(c.in, c.html), "…"); got != c.want {
			t.Errorf("%q\n got %q\nwant %q", c.in, got, c.want)
		}
	}
}
