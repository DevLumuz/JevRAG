package llm_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"jev/internal/llm"
)

func server(t *testing.T, statuses []int, body string) (*httptest.Server, *atomic.Int32, *map[string]any) {
	t.Helper()
	var calls atomic.Int32
	var lastReq map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := int(calls.Add(1))
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &lastReq)
		w.Header().Set("Content-Type", "application/json")
		if n <= len(statuses) {
			w.WriteHeader(statuses[n-1])
			_, _ = io.WriteString(w, `{"error":{"code":429,"message":"slow down","status":"RESOURCE_EXHAUSTED"}}`)
			return
		}
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(srv.Close)
	return srv, &calls, &lastReq
}

const okBody = `{"candidates":[{"content":{"parts":[{"text":"{\"relevance\":\"high\"}"}],"role":"model"}}],
 "usageMetadata":{"promptTokenCount":120,"candidatesTokenCount":9,"thoughtsTokenCount":3}}`

func TestGenerateJSON(t *testing.T) {
	srv, calls, req := server(t, nil, okBody)
	t.Setenv("GEMINI_API_KEY", "k")
	g, err := llm.NewGemini(context.Background(), llm.Options{Model: "gemini-3.8-flash", BaseURL: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	text, in, out, err := g.GenerateJSON(context.Background(), "prompt", map[string]any{"type": "object"})
	if err != nil {
		t.Fatal(err)
	}
	if text != `{"relevance":"high"}` || in != 120 || out != 12 || calls.Load() != 1 {
		t.Errorf("text=%q in=%d out=%d calls=%d", text, in, out, calls.Load())
	}
	gc, _ := json.Marshal((*req)["generationConfig"])
	for _, want := range []string{`"responseMimeType":"application/json"`, `"temperature":0`, `"thinkingLevel":"MINIMAL"`} {
		if !strings.Contains(string(gc), want) {
			t.Errorf("generationConfig %s lacks %s", gc, want)
		}
	}
}

func TestGenerateJSON_RetriesRateLimit(t *testing.T) {
	if testing.Short() {
		t.Skip("waits for backoff")
	}
	srv, calls, _ := server(t, []int{429}, okBody)
	t.Setenv("GEMINI_API_KEY", "k")
	g, _ := llm.NewGemini(context.Background(), llm.Options{Model: "gemini-2.5-flash", BaseURL: srv.URL})
	if _, _, _, err := g.GenerateJSON(context.Background(), "p", nil); err != nil {
		t.Fatalf("expected success after retry: %v", err)
	}
	if calls.Load() != 2 {
		t.Errorf("calls = %d, want 2", calls.Load())
	}
}
