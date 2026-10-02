package jev_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"jev/internal/jev"
)

func TestHTTPClient_SystemOne(t *testing.T) {
	var gotPath, gotAuth string
	var gotBody map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &gotBody)

		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{
			"model": "jev-2026-09-15",
			"answers": {
				"relevance": {"type": "choice", "choice": "high", "confidence": 0.9,
					"probabilities": {"irrelevant": 0.05, "weak": 0.05, "high": 0.8, "direct": 0.1}},
				"sufficient": {"type": "noul", "noul": 0.72},
				"urgency": {"type": "score", "score": 1.7, "confidence": 0.8,
					"legend": {"0": "low", "1": "mid", "2": "high"},
					"probabilities": {"0": 0.1, "1": 0.1, "2": 0.8}}
			},
			"usage": {"input_tokens": 120, "output_tokens": 12}
		}`)
	}))
	defer srv.Close()

	c, err := jev.NewHTTPClient(jev.Options{BaseURL: srv.URL, APIKey: "k-123"})
	if err != nil {
		t.Fatalf("NewHTTPClient() error: %v", err)
	}

	resp, err := c.SystemOne(context.Background(), map[string]string{"query": "q"}, map[string]jev.Question{
		"relevance":  jev.Choice("How relevant?", map[string]string{"irrelevant": "", "weak": "", "high": "", "direct": ""}),
		"sufficient": jev.Noul("Is the evidence sufficient?"),
		"urgency":    jev.Score("How urgent?", []string{"low", "mid", "high"}),
	})
	if err != nil {
		t.Fatalf("SystemOne() error: %v", err)
	}

	// Request shape.
	if gotPath != "/v1/systemone" {
		t.Errorf("path = %q, want /v1/systemone", gotPath)
	}
	if gotAuth != "Bearer k-123" {
		t.Errorf("Authorization = %q", gotAuth)
	}
	if gotBody["model"] != "jev-latest" {
		t.Errorf("model = %v, want jev-latest", gotBody["model"])
	}
	qs := gotBody["questions"].(map[string]any)
	rel := qs["relevance"].(map[string]any)
	if rel["type"] != "choice" || rel["instructions"] != "How relevant?" {
		t.Errorf("relevance question = %v", rel)
	}
	if _, ok := qs["sufficient"].(map[string]any)["criteria"]; ok {
		t.Errorf("noul without criteria must omit the field")
	}

	// Response decoding.
	if resp.Model != "jev-2026-09-15" {
		t.Errorf("Model = %q", resp.Model)
	}
	if a := resp.Answers["relevance"]; a.Choice != "high" || a.Confidence != 0.9 || a.Probabilities["direct"] != 0.1 {
		t.Errorf("relevance answer = %+v", a)
	}
	if a := resp.Answers["sufficient"]; a.Noul != 0.72 {
		t.Errorf("sufficient answer = %+v", a)
	}
	if a := resp.Answers["urgency"]; a.Score != 1.7 || a.Probabilities["2"] != 0.8 {
		t.Errorf("urgency answer = %+v", a)
	}
	if resp.Usage.InputTokens != 120 || resp.Usage.OutputTokens != 12 {
		t.Errorf("Usage = %+v", resp.Usage)
	}
}

func TestHTTPClient_ErrorIncludesBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"detail":[{"loc":["body","state"],"msg":"Field required"}]}`)
	}))
	defer srv.Close()

	c, _ := jev.NewHTTPClient(jev.Options{BaseURL: srv.URL, APIKey: "k", Backoff: time.Millisecond})
	_, err := c.SystemOne(context.Background(), "s", map[string]jev.Question{"x": jev.Noul("?")})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "400") || !strings.Contains(err.Error(), "Field required") {
		t.Errorf("error = %v, want status and body", err)
	}
}

func TestHTTPClient_RetriesOverload(t *testing.T) {
	for _, status := range []int{http.StatusTooManyRequests, 529} {
		calls := 0
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls++
			b, _ := io.ReadAll(r.Body)
			if !strings.Contains(string(b), `"questions"`) {
				t.Errorf("attempt %d sent an empty body", calls)
			}
			if calls < 3 {
				w.WriteHeader(status)
				return
			}
			_, _ = io.WriteString(w, `{"model":"m","answers":{"x":{"type":"noul","noul":0.3}},"usage":{"input_tokens":5}}`)
		}))

		c, _ := jev.NewHTTPClient(jev.Options{BaseURL: srv.URL, APIKey: "k", Backoff: time.Millisecond})
		resp, err := c.SystemOne(context.Background(), "s", map[string]jev.Question{"x": jev.Noul("?")})
		srv.Close()
		if err != nil {
			t.Fatalf("status %d: SystemOne() error after retries: %v", status, err)
		}
		if calls != 3 || resp.Answers["x"].Noul != 0.3 {
			t.Errorf("status %d: calls = %d, answer = %+v", status, calls, resp.Answers["x"])
		}
	}
}

func TestNewHTTPClient_RequiresKey(t *testing.T) {
	t.Setenv("TYPESAFE_API_KEY", "")
	if _, err := jev.NewHTTPClient(jev.Options{}); err == nil {
		t.Fatal("expected error without API key")
	}
}

func TestNewHTTPClient_ReadsEnv(t *testing.T) {
	t.Setenv("TYPESAFE_API_KEY", "from-env")
	if _, err := jev.NewHTTPClient(jev.Options{}); err != nil {
		t.Fatalf("NewHTTPClient() error: %v", err)
	}
}

func TestFakeClient(t *testing.T) {
	fake := &jev.FakeClient{
		Respond: func(state any, qs map[string]jev.Question) (*jev.Response, error) {
			return &jev.Response{Answers: map[string]jev.Answer{"q": {Type: "noul", Noul: 0.4}}}, nil
		},
	}
	resp, err := fake.SystemOne(context.Background(), "s", map[string]jev.Question{"q": jev.Noul("?")})
	if err != nil {
		t.Fatalf("SystemOne() error: %v", err)
	}
	if resp.Answers["q"].Noul != 0.4 {
		t.Errorf("Noul = %v, want 0.4", resp.Answers["q"].Noul)
	}
	if len(fake.Calls) != 1 {
		t.Errorf("Calls = %d, want 1", len(fake.Calls))
	}
}
