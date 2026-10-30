package main

import (
	"reflect"
	"testing"
)

func TestSplitLabels(t *testing.T) {
	got := splitLabels(`Входящие,"Клиенты, VIP",info@example.com,Непрочитанные`)
	want := []string{"Входящие", "Клиенты, VIP", "info@example.com", "Непрочитанные"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("%q", got)
	}
	if !reMboxFrom.MatchString("From 1700000000000000000@xxx Mon Jan 02 15:04:05 +0000 2023\n") {
		t.Error("mbox From line")
	}
	if reMboxFrom.MatchString("From the director: hello\n") {
		t.Error("body line taken for a separator")
	}
}
