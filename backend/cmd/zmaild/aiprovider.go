package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Which AI the assistant talks to. Any OpenAI-compatible API works: a local model behind vLLM or
// Ollama (Loggi), OpenAI itself, OpenRouter (hundreds of models with one key), or another gateway.
// The administrator picks it in "AI assistant"; until then the .env values from the installer are used
// (AI_API_URL / AI_API_KEY / AI_MODEL, or the older LOGGI_API_URL / LOGGI_API_KEY).
// Speech recognition (dictation) may live elsewhere: OpenRouter has none, so it has its own address.

type aiProvider struct {
	Kind     string `json:"kind"` // loggi | openai | openrouter | custom
	Name     string `json:"name"` // the assistant's name in the interface
	URL      string `json:"url"`
	Key      string `json:"-"`
	Model    string `json:"model"` // default model of the prompts
	ASRURL   string `json:"asrUrl"`
	ASRKey   string `json:"-"`
	ASRModel string `json:"asrModel"`
}

// Ready-made settings for the administrator's choice.
var aiPresets = map[string]aiProvider{
	"openai":     {Kind: "openai", URL: "https://api.openai.com", Model: "gpt-4o-mini", ASRModel: "whisper-1"},
	"openrouter": {Kind: "openrouter", URL: "https://openrouter.ai/api", Model: "openai/gpt-4o-mini"},
}

var (
	aiProvMu  sync.RWMutex
	aiProv    aiProvider
	aiProvSet bool // a row in ai_provider overrides .env
)

// aiBase: the address without a trailing "/" or "/v1" — the paths below add "/v1/…".
func aiBase(u string) string {
	u = strings.TrimRight(strings.TrimSpace(u), "/")
	return strings.TrimSuffix(u, "/v1")
}

func (s *Server) loadAIProvider(ctx context.Context) {
	p := aiProvider{Kind: "custom", Name: "Loggi",
		URL: env("AI_API_URL", s.cfg.LoggiURL), Key: env("AI_API_KEY", s.cfg.LoggiKey), Model: env("AI_MODEL", "loggi")}
	if strings.Contains(p.URL, "openrouter.ai") {
		p.Kind = "openrouter"
	} else if strings.Contains(p.URL, "api.openai.com") {
		p.Kind = "openai"
	}
	var keyEnc, asrEnc []byte
	err := s.db.QueryRow(ctx, `SELECT kind, name, url, key_enc, model, asr_url, asr_key_enc, asr_model FROM ai_provider WHERE id=1`).
		Scan(&p.Kind, &p.Name, &p.URL, &keyEnc, &p.Model, &p.ASRURL, &asrEnc, &p.ASRModel)
	set := err == nil
	if set {
		p.Key, p.ASRKey = "", ""
		if len(keyEnc) > 0 {
			p.Key, _ = s.unseal(keyEnc)
		}
		if len(asrEnc) > 0 {
			p.ASRKey, _ = s.unseal(asrEnc)
		}
	}
	p.URL = aiBase(p.URL)
	p.ASRURL = aiBase(p.ASRURL)
	aiProvMu.Lock()
	aiProv, aiProvSet = p, set
	aiProvMu.Unlock()
}

func (s *Server) ai() aiProvider {
	aiProvMu.RLock()
	defer aiProvMu.RUnlock()
	return aiProv
}

// asr: where dictation goes — its own address, or the chat API's (except OpenRouter, which has none).
func (p aiProvider) asr() (base, key, model string) {
	if p.ASRURL != "" {
		key = p.ASRKey
		if key == "" && p.ASRURL == p.URL {
			key = p.Key
		}
		return p.ASRURL, key, p.ASRModel
	}
	if p.Kind == "openrouter" {
		return "", "", ""
	}
	return p.URL, p.Key, p.ASRModel
}

// headers adds the key and, for OpenRouter, the site's name (shown in its statistics).
func (s *Server) aiHeaders(req *http.Request, key string) {
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	if s.ai().Kind == "openrouter" {
		req.Header.Set("HTTP-Referer", "https://"+s.cfg.Hostname)
		req.Header.Set("X-Title", "TMAIL")
	}
}

// GET /api/admin/ai/provider and PUT the same: the connection settings (keys are never sent back).
func (s *Server) adminAIProvider(w http.ResponseWriter, r *http.Request, sess *Session) {
	if r.Method == http.MethodGet {
		p := s.ai()
		_, _, asrModel := p.asr()
		asrBase, asrKey, _ := p.asr()
		writeJSON(w, http.StatusOK, map[string]any{"provider": p, "hasKey": p.Key != "", "hasAsrKey": p.ASRKey != "",
			"fromEnv": !aiProvSet, "presets": aiPresets, "asrActive": asrBase != "" && (asrKey != "" || p.Kind == "loggi" || p.Kind == "custom"), "asrModelInUse": asrModel})
		return
	}
	var in struct {
		aiProvider
		Key         *string `json:"key"`    // nil: keep the stored one
		ASRKey      *string `json:"asrKey"` // nil: keep
		ResetModels bool    `json:"resetModels"`
	}
	if err := readJSON(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "Invalid data")
		return
	}
	p := in.aiProvider
	p.URL, p.ASRURL = aiBase(p.URL), aiBase(p.ASRURL)
	p.Name = strings.TrimSpace(p.Name)
	if p.Name == "" {
		p.Name = "Loggi"
	}
	for _, u := range []string{p.URL, p.ASRURL} {
		if u == "" {
			continue
		}
		if pu, err := url.Parse(u); err != nil || (pu.Scheme != "http" && pu.Scheme != "https") || pu.Host == "" {
			writeErr(w, http.StatusBadRequest, "The API URL must start with https:// or http://")
			return
		}
	}
	if p.URL == "" || strings.TrimSpace(p.Model) == "" {
		writeErr(w, http.StatusBadRequest, "Specify the API URL and the model")
		return
	}
	cur := s.ai()
	key, asrKey := cur.Key, cur.ASRKey
	if in.Key != nil {
		key = strings.TrimSpace(*in.Key)
	}
	if in.ASRKey != nil {
		asrKey = strings.TrimSpace(*in.ASRKey)
	}
	seal := func(v string) []byte {
		if v == "" {
			return nil
		}
		b, _ := s.seal(v)
		return b
	}
	ctx := r.Context()
	_, err := s.db.Exec(ctx, `INSERT INTO ai_provider (id, kind, name, url, key_enc, model, asr_url, asr_key_enc, asr_model, updated_at, updated_by)
		VALUES (1,$1,$2,$3,$4,$5,$6,$7,$8,now(),$9)
		ON CONFLICT (id) DO UPDATE SET kind=$1, name=$2, url=$3, key_enc=$4, model=$5, asr_url=$6, asr_key_enc=$7, asr_model=$8, updated_at=now(), updated_by=$9`,
		p.Kind, p.Name, p.URL, seal(key), strings.TrimSpace(p.Model), p.ASRURL, seal(asrKey), strings.TrimSpace(p.ASRModel), sess.Login)
	if err != nil {
		serverErr(w, "ai provider", err)
		return
	}
	// Another API knows other model names: the prompts follow the default model.
	if in.ResetModels {
		_, _ = s.db.Exec(ctx, `UPDATE ai_prompts SET model=''`)
	}
	s.loadAIProvider(ctx)
	s.audit(ctx, sess.UserID, sess.Login, clientIP(r), "ai.provider", p.Kind, map[string]any{"url": p.URL, "model": p.Model})
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// aiModels lists the models of the API (OpenRouter: several hundred, sorted by name).
func (s *Server) aiModels(ctx context.Context) ([]string, int64, error) {
	p := s.ai()
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, p.URL+"/v1/models", nil)
	s.aiHeaders(req, p.Key)
	start := time.Now()
	resp, err := aiClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, time.Since(start).Milliseconds(), &aiHTTPError{resp.StatusCode}
	}
	var m struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&m)
	ids := []string{}
	for _, d := range m.Data {
		ids = append(ids, d.ID)
	}
	return ids, time.Since(start).Milliseconds(), nil
}

type aiHTTPError struct{ code int }

func (e *aiHTTPError) Error() string {
	if e.code == http.StatusUnauthorized || e.code == http.StatusForbidden {
		return "API key rejected"
	}
	return fmt.Sprintf("API returned an error: %s", http.StatusText(e.code))
}
