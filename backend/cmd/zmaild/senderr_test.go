package main

import (
	"errors"
	"strings"
	"testing"
)

func TestSendErrorText(t *testing.T) {
	cases := map[string]string{
		`rcpt anna@example.com: 550 "5.1.1 <anna@example.com>: Recipient address rejected: User unknown in virtual mailbox table"`: "anna@example.com does not exist or the mailbox is disabled",
		`rcpt x@y.ru: 451 4.3.0 try later`:           "temporarily",
		`552 5.3.4 Message size exceeds fixed limit`: "too large",
		`535 5.7.8 Authentication failed`:            "username or password",
	}
	for in, want := range cases {
		if got := sendErrorText(errors.New(in)); !strings.Contains(got, want) {
			t.Errorf("%s → %s", in, got)
		}
	}
}
