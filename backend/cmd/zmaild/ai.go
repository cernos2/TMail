package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// AI assistant on top of Loggi (OpenAI-compatible API, local Qwen model). All prompts live in the
// ai_prompts table and are edited by administrators; the code only fills in the placeholders.

type aiPrompt struct {
	Key         string  `json:"key"`
	Title       string  `json:"title"`
	Description string  `json:"description"`
	System      string  `json:"system"`
	Template    string  `json:"template"`
	Model       string  `json:"model"`
	Temperature float32 `json:"temperature"`
	MaxTokens   int     `json:"maxTokens"`
	Enabled     bool    `json:"enabled"`
	UpdatedAt   string  `json:"updatedAt,omitempty"`
	UpdatedBy   string  `json:"updatedBy,omitempty"`
}

type aiInput struct {
	Key          string `json:"key"`
	From         string `json:"from"`
	To           string `json:"to"`
	Subject      string `json:"subject"`
	Letter       string `json:"letter"`
	Thread       string `json:"thread"`
	Draft        string `json:"draft"`
	Instructions string `json:"instructions"`
	Mailbox      string `json:"mailbox"`
	Templates    string `json:"templates"`
	Attachments  string `json:"attachments"`
	Folders      string `json:"folders"`
}

const (
	aiMaxLetter = 16000 // characters of the letter sent to the model
	aiTimeout   = 150 * time.Second
)

var (
	aiClient = &http.Client{Timeout: aiTimeout}
	aiSlots  = make(chan struct{}, 3) // concurrent requests to the model from this server
	reThink  = regexp.MustCompile(`(?s)<think>.*?</think>`)
)

// A local model may need no key; a public API always does.
func (s *Server) aiConfigured() bool {
	p := s.ai()
	return p.URL != "" && p.Model != "" && (p.Key != "" || p.Kind == "custom" || p.Kind == "loggi")
}

func (s *Server) loadPrompt(ctx context.Context, key string) (aiPrompt, error) {
	var p aiPrompt
	err := s.db.QueryRow(ctx, `SELECT key, title, description, system, template, model, temperature, max_tokens, enabled FROM ai_prompts WHERE key=$1`, key).
		Scan(&p.Key, &p.Title, &p.Description, &p.System, &p.Template, &p.Model, &p.Temperature, &p.MaxTokens, &p.Enabled)
	return p, err
}

func clip(s string, n int) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) > n {
		return string(r[:n]) + "\n[…text truncated…]"
	}
	return string(r)
}

func fillTemplate(t string, in aiInput, user string) string {
	or := func(v, def string) string {
		if strings.TrimSpace(v) == "" {
			return def
		}
		return v
	}
	return strings.NewReplacer(
		"{from}", or(in.From, "—"), "{to}", or(in.To, "—"), "{subject}", or(in.Subject, "(no subject)"),
		"{letter}", or(clip(in.Letter, aiMaxLetter), "—"), "{thread}", or(clip(in.Thread, 24000), "none"), "{draft}", or(clip(in.Draft, 8000), "none"),
		"{instructions}", or(clip(in.Instructions, 2000), "none"), "{user}", or(user, "—"), "{mailbox}", or(in.Mailbox, "—"), "{templates}", or(in.Templates, "none"), "{attachments}", or(in.Attachments, "none"), "{folders}", or(in.Folders, "none"),
	).Replace(t)
}

type aiImage struct {
	Mime string
	Data []byte
}

// complete sends one chat request to Loggi and returns the answer text.
func (s *Server) complete(ctx context.Context, p aiPrompt, user string) (string, error) {
	return s.completeImages(ctx, p, user, nil)
}

// completeImages also shows the model pictures (scanned documents, PDF pages): Qwen is multimodal.
func (s *Server) completeImages(ctx context.Context, p aiPrompt, user string, images []aiImage) (string, error) {
	if !s.aiConfigured() {
		return "", errors.New("AI is not connected: the administrator has not set up an API in the \"AI assistant\" section")
	}
	select {
	case aiSlots <- struct{}{}:
		defer func() { <-aiSlots }()
	case <-ctx.Done():
		return "", ctx.Err()
	}
	msgs := []map[string]any{}
	if strings.TrimSpace(p.System) != "" {
		msgs = append(msgs, map[string]any{"role": "system", "content": p.System})
	}
	if len(images) == 0 {
		msgs = append(msgs, map[string]any{"role": "user", "content": user})
	} else {
		parts := []map[string]any{{"type": "text", "text": user}}
		for _, im := range images {
			parts = append(parts, map[string]any{"type": "image_url", "image_url": map[string]string{"url": "data:" + im.Mime + ";base64," + base64.StdEncoding.EncodeToString(im.Data)}})
		}
		msgs = append(msgs, map[string]any{"role": "user", "content": parts})
	}
	prov := s.ai()
	model := p.Model
	if model == "" {
		model = prov.Model
	}
	req := map[string]any{
		"model": model, "messages": msgs, "temperature": p.Temperature, "max_tokens": p.MaxTokens,
		// Streaming: Loggi's gateway cuts non-streamed answers that take longer than ~10 s.
		"stream": true,
	}
	switch prov.Kind {
	case "openai": // newer OpenAI models take only this name of the limit
		delete(req, "max_tokens")
		req["max_completion_tokens"] = p.MaxTokens
	case "openrouter":
	default: // vLLM (Qwen): answer at once, without a hidden "thinking" part
		req["chat_template_kwargs"] = map[string]any{"enable_thinking": false}
	}
	body, err := json.Marshal(req)
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(ctx, aiTimeout)
	defer cancel()
	// Loggi briefly takes its model backend out of rotation after an error: one retry covers that.
	var text string
	for attempt := 0; attempt < 2; attempt++ {
		var retry bool
		var noTemp bool
		text, retry, noTemp, err = s.streamChat(ctx, body)
		if noTemp { // a reasoning model that accepts no temperature: ask once more without it
			delete(req, "temperature")
			body, _ = json.Marshal(req)
			text, retry, _, err = s.streamChat(ctx, body)
		}
		if err == nil || !retry || attempt == 1 {
			break
		}
		select {
		case <-time.After(5 * time.Second):
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
	if err != nil {
		return "", err
	}
	text = strings.TrimSpace(reThink.ReplaceAllString(text, ""))
	if text == "" {
		return "", errors.New("The model returned an empty answer. Please try again")
	}
	return text, nil
}

// streamChat sends a streamed chat request and collects the answer. retry is true when the model
// backend was temporarily unavailable; noTemp when the model refused the temperature parameter.
func (s *Server) streamChat(ctx context.Context, body []byte) (text string, retry, noTemp bool, err error) {
	prov := s.ai()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, prov.URL+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", false, false, err
	}
	s.aiHeaders(req, prov.Key)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	resp, err := aiClient.Do(req)
	if err != nil {
		log.Printf("ai: %v", err) // details (address of the AI server) stay in the log
		return "", false, false, fmt.Errorf("%s is unavailable", prov.Name)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
		log.Printf("ai: HTTP %d: %s", resp.StatusCode, truncate(string(raw), 500))
		var e struct {
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		_ = json.Unmarshal(raw, &e)
		msg := strings.ToLower(e.Error.Message)
		if resp.StatusCode == http.StatusBadRequest && strings.Contains(msg, "temperature") {
			return "", false, true, errors.New("temperature")
		}
		switch resp.StatusCode {
		case http.StatusUnauthorized, http.StatusForbidden:
			return "", false, false, fmt.Errorf("%s: the API key was rejected. Check it in \"AI assistant\"", prov.Name)
		case http.StatusPaymentRequired:
			return "", false, false, fmt.Errorf("%s: the API account has run out of funds", prov.Name)
		case http.StatusNotFound:
			return "", false, false, fmt.Errorf("%s: model not found. Choose another one in \"AI assistant\"", prov.Name)
		case http.StatusTooManyRequests:
			return "", true, false, fmt.Errorf("%s: too many requests, try again in a minute", prov.Name)
		}
		return "", resp.StatusCode == http.StatusServiceUnavailable, false, fmt.Errorf("%s returned error %d", prov.Name, resp.StatusCode)
	}
	var out strings.Builder
	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 64<<10), 4<<20)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		data, ok := strings.CutPrefix(line, "data:")
		if !ok {
			continue
		}
		data = strings.TrimSpace(data)
		if data == "[DONE]" {
			break
		}
		var chunk struct {
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
			} `json:"choices"`
			Error *struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if json.Unmarshal([]byte(data), &chunk) != nil {
			continue
		}
		if chunk.Error != nil {
			return "", true, false, fmt.Errorf("%s: %s", prov.Name, chunk.Error.Message)
		}
		for _, c := range chunk.Choices {
			out.WriteString(c.Delta.Content)
		}
		if out.Len() > 1<<20 {
			break
		}
	}
	if err := sc.Err(); err != nil {
		return "", false, false, fmt.Errorf("%s cut off the answer: %w", prov.Name, err)
	}
	return out.String(), false, false, nil
}

// GET /api/ai/actions — what the assistant can do for this user (enabled prompts).
func (s *Server) handleAIActions(w http.ResponseWriter, r *http.Request, sess *Session) {
	out := []map[string]string{}
	if s.aiConfigured() {
		rows, err := s.db.Query(r.Context(), `SELECT key, title, description FROM ai_prompts WHERE enabled ORDER BY sort, key`)
		if err == nil {
			for rows.Next() {
				var k, t, d string
				if rows.Scan(&k, &t, &d) == nil {
					out = append(out, map[string]string{"key": k, "title": t, "description": d})
				}
			}
			rows.Close()
		}
	}
	writeJSON(w, http.StatusOK, out)
}

// POST /api/ai/run
func (s *Server) handleAIRun(w http.ResponseWriter, r *http.Request, sess *Session) {
	var in aiInput
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 512<<10)).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "Invalid data")
		return
	}
	p, err := s.loadPrompt(r.Context(), in.Key)
	if err != nil || !p.Enabled {
		writeErr(w, http.StatusNotFound, "This AI action has been disabled by the administrator")
		return
	}
	start := time.Now()
	text, err := s.complete(r.Context(), p, fillTemplate(p.Template, in, sess.Name))
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	s.audit(r.Context(), sess.UserID, sess.Login, clientIP(r), "ai.run", in.Key, map[string]any{"ms": time.Since(start).Milliseconds()})
	writeJSON(w, http.StatusOK, map[string]any{"text": text, "ms": time.Since(start).Milliseconds()})
}

// POST /api/ai/transcribe — speech to text (Loggi ASR) for dictating a reply.
func (s *Server) handleAITranscribe(w http.ResponseWriter, r *http.Request, sess *Session) {
	asrBase, asrKey, asrModel := s.ai().asr()
	if asrBase == "" {
		writeErr(w, http.StatusServiceUnavailable, "Speech recognition is not connected: the administrator has not set up its API in the \"AI assistant\" section")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 25<<20)
	if err := r.ParseMultipartForm(25 << 20); err != nil {
		writeErr(w, http.StatusRequestEntityTooLarge, "The recording is too long")
		return
	}
	fh := r.MultipartForm.File["audio"]
	if len(fh) == 0 {
		writeErr(w, http.StatusBadRequest, "No recording")
		return
	}
	f, err := fh[0].Open()
	if err != nil {
		writeErr(w, http.StatusBadRequest, "Could not read the recording")
		return
	}
	defer f.Close()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	name := fh[0].Filename
	if name == "" {
		name = "voice.webm"
	}
	part, _ := mw.CreateFormFile("file", name)
	if _, err := io.Copy(part, f); err != nil {
		writeErr(w, http.StatusBadRequest, "Could not read the recording")
		return
	}
	if asrModel != "" {
		_ = mw.WriteField("model", asrModel)
	}
	_ = mw.Close()
	ctx, cancel := context.WithTimeout(r.Context(), 120*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, asrBase+"/v1/audio/transcriptions", &body)
	s.aiHeaders(req, asrKey)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	start := time.Now()
	resp, err := aiClient.Do(req)
	if err != nil {
		log.Printf("loggi asr: %v", err)
		writeErr(w, http.StatusBadGateway, "Speech recognition is unavailable")
		return
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		log.Printf("loggi asr: HTTP %d: %s", resp.StatusCode, truncate(string(raw), 500))
		writeErr(w, http.StatusBadGateway, fmt.Sprintf("Speech recognition returned error %d", resp.StatusCode))
		return
	}
	var out struct {
		Text string `json:"text"`
	}
	if json.Unmarshal(raw, &out) != nil {
		out.Text = string(raw)
	}
	s.audit(r.Context(), sess.UserID, sess.Login, clientIP(r), "ai.voice", "", map[string]any{"ms": time.Since(start).Milliseconds()})
	writeJSON(w, http.StatusOK, map[string]string{"text": strings.TrimSpace(out.Text)})
}

// ---------- admin ----------

func (s *Server) adminAI(w http.ResponseWriter, r *http.Request, sess *Session) {
	rows, err := s.db.Query(r.Context(), `SELECT key, title, description, system, template, model, temperature, max_tokens, enabled,
		to_char(updated_at, 'YYYY-MM-DD"T"HH24:MI:SSOF'), updated_by FROM ai_prompts ORDER BY sort, key`)
	if err != nil {
		serverErr(w, "ai prompts", err)
		return
	}
	prompts := []aiPrompt{}
	for rows.Next() {
		var p aiPrompt
		if err := rows.Scan(&p.Key, &p.Title, &p.Description, &p.System, &p.Template, &p.Model, &p.Temperature, &p.MaxTokens, &p.Enabled, &p.UpdatedAt, &p.UpdatedBy); err == nil {
			prompts = append(prompts, p)
		}
	}
	rows.Close()
	prov := s.ai()
	status := map[string]any{"configured": s.aiConfigured(), "name": prov.Name, "kind": prov.Kind, "model": prov.Model}
	if u, err := url.Parse(prov.URL); err == nil && prov.URL != "" {
		status["host"] = u.Host
		status["secure"] = u.Scheme == "https"
	}
	if prov.URL != "" {
		ids, ms, err := s.aiModels(r.Context())
		status["ms"] = ms
		if err != nil {
			status["error"] = err.Error()
		} else {
			status["reachable"] = true
			status["models"] = ids
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": status, "prompts": prompts})
}

func (s *Server) adminAIUpdate(w http.ResponseWriter, r *http.Request, sess *Session) {
	var p aiPrompt
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 256<<10)).Decode(&p); err != nil {
		writeErr(w, http.StatusBadRequest, "Invalid data")
		return
	}
	p.Title = strings.TrimSpace(p.Title)
	if p.Title == "" || strings.TrimSpace(p.Template) == "" {
		writeErr(w, http.StatusBadRequest, "A title and a request template are required")
		return
	}
	if p.Temperature < 0 || p.Temperature > 1.5 || p.MaxTokens < 50 || p.MaxTokens > 8000 {
		writeErr(w, http.StatusBadRequest, "Check the model, temperature (0–1.5) and answer length (50–8000)")
		return
	}
	tag, err := s.db.Exec(r.Context(), `UPDATE ai_prompts SET title=$2, description=$3, system=$4, template=$5, model=$6, temperature=$7,
		max_tokens=$8, enabled=$9, updated_at=now(), updated_by=$10 WHERE key=$1`,
		r.PathValue("key"), p.Title, p.Description, p.System, p.Template, p.Model, p.Temperature, p.MaxTokens, p.Enabled, sess.Login)
	if err != nil {
		serverErr(w, "ai prompt save", err)
		return
	}
	if tag.RowsAffected() == 0 {
		writeErr(w, http.StatusNotFound, "Prompt not found")
		return
	}
	s.audit(r.Context(), sess.UserID, sess.Login, clientIP(r), "ai.prompt", r.PathValue("key"), nil)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// POST /api/admin/ai/test — runs an (unsaved) prompt on a sample letter.
func (s *Server) adminAITest(w http.ResponseWriter, r *http.Request, sess *Session) {
	var body struct {
		Prompt aiPrompt `json:"prompt"`
		Input  aiInput  `json:"input"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 512<<10)).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "Invalid data")
		return
	}
	start := time.Now()
	user := fillTemplate(body.Prompt.Template, body.Input, sess.Name)
	text, err := s.complete(r.Context(), body.Prompt, user)
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"text": text, "request": user, "ms": time.Since(start).Milliseconds()})
}
