package main

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMCPProtocol(t *testing.T) {
	s := &Server{cfg: Config{Hostname: "mail.example.com"}}
	sess := &Session{Login: "tim"}
	r := httptest.NewRequest("POST", "/mcp", nil)
	res, e := s.mcpDispatch(context.Background(), r, sess, &mcpToken{}, rpcReq{Method: "initialize", Params: json.RawMessage(`{"protocolVersion":"2025-03-26"}`)})
	if e != nil || res.(map[string]any)["protocolVersion"] != "2025-03-26" {
		t.Fatalf("initialize: %v %v", res, e)
	}
	names := func(tok *mcpToken) string {
		res, _ := s.mcpDispatch(context.Background(), r, sess, tok, rpcReq{Method: "tools/list"})
		var n []string
		for _, x := range res.(map[string]any)["tools"].([]obj) {
			n = append(n, x["name"].(string))
		}
		return strings.Join(n, ",")
	}
	if ro := names(&mcpToken{}); strings.Contains(ro, "send_message") || strings.Contains(ro, "modify_messages") || !strings.Contains(ro, "create_draft") {
		t.Errorf("read-only token tools: %s", ro)
	}
	if all := names(&mcpToken{CanSend: true, CanModify: true}); !strings.Contains(all, "send_message") || !strings.Contains(all, "modify_messages") {
		t.Errorf("full token tools: %s", all)
	}
	if _, e := s.mcpDispatch(context.Background(), r, sess, &mcpToken{}, rpcReq{Method: "nope"}); e == nil || e.Code != -32601 {
		t.Errorf("unknown method: %v", e)
	}
	if k, v := folderQuery("Sent"); k != "role" || v != "sent" {
		t.Errorf("folderQuery: %s %s", k, v)
	}
	if k, v := folderQuery("Поставщики"); k != "folder" || v != "Поставщики" {
		t.Errorf("folderQuery custom: %s %s", k, v)
	}
	if got := htmlToText(`<style>p{color:red}</style><p>Добрый&nbsp;день!</p><div>Счёт <b>№5</b></div><br>Спасибо`); got != "Добрый день!\nСчёт №5\n\nСпасибо" && !strings.Contains(got, "Добрый день!\nСчёт №5") {
		t.Errorf("htmlToText: %q", got)
	}
	if tk := newMCPToken(); !strings.HasPrefix(tk, "tmail_") || len(tk) < 30 || tokenHash(tk) == tokenHash(newMCPToken()) {
		t.Errorf("token: %s", tk)
	}
}
