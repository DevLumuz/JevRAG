// Package jev provides a client for TypeSafe's JEV model.
// JEV is a non-autoregressive decision model that returns typed decisions
// (Choice, Score, Noul) instead of free text.
//
// The wire contract mirrors the official API (POST /v1/systemone, see
// https://docs.typesafe.ai/api): one request carries a state and several
// named questions, which JEV answers independently and in parallel.
package jev

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
)

// Question is one named judgment about the request's state.
type Question struct {
	Type         string `json:"type"` // "choice", "score" or "noul"
	Instructions any    `json:"instructions,omitempty"`
	Criteria     any    `json:"criteria,omitempty"`
}

// Choice builds a question that picks one label. Each label maps to a
// description of when it applies; an empty description sends the label alone.
func Choice(instructions string, criteria map[string]string) Question {
	c := make(map[string]any, len(criteria))
	for label, desc := range criteria {
		if desc == "" {
			c[label] = nil
		} else {
			c[label] = desc
		}
	}
	return Question{Type: "choice", Instructions: instructions, Criteria: c}
}

// Score builds a question that rates the state on ordered levels (index 0 first).
func Score(instructions string, levels []string) Question {
	return Question{Type: "score", Instructions: instructions, Criteria: levels}
}

// Noul builds a yes/no question.
func Noul(instructions string) Question {
	return Question{Type: "noul", Instructions: instructions}
}

// Answer is the answer to one question. Which fields are set depends on Type:
// choice → Choice, Confidence, Probabilities; score → Score, Confidence,
// Probabilities (keyed "0", "1"...); noul → Noul.
type Answer struct {
	Type          string             `json:"type"`
	Choice        string             `json:"choice,omitempty"`
	Score         float64            `json:"score,omitempty"`
	Noul          float64            `json:"noul,omitempty"`
	Confidence    float64            `json:"confidence,omitempty"`
	Probabilities map[string]float64 `json:"probabilities,omitempty"`
}

// Usage reports billable tokens. Output tokens are currently free.
type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// Response holds the answers keyed by the question names in the request.
type Response struct {
	Model   string            `json:"model"`
	Answers map[string]Answer `json:"answers"`
	Usage   Usage             `json:"usage"`
}

// Client asks JEV a set of named questions about one state.
type Client interface {
	SystemOne(ctx context.Context, state any, questions map[string]Question) (*Response, error)
}

// --- Real HTTP client ---

const (
	defaultBaseURL = "https://api.typesafe.ai"
	defaultModel   = "jev-latest"
	maxErrorBody   = 200
)

// Options configures the JEV client. Empty fields fall back to the
// TYPESAFE_API_KEY / TYPESAFE_BASE_URL env vars, then to the defaults.
type Options struct {
	APIKey     string
	BaseURL    string
	Model      string
	HTTPClient *http.Client
}

// HTTPClient calls the TypeSafe API.
type HTTPClient struct {
	baseURL    string
	apiKey     string
	model      string
	httpClient *http.Client
}

// NewHTTPClient creates a JEV client.
func NewHTTPClient(opts Options) (*HTTPClient, error) {
	apiKey := firstNonEmpty(opts.APIKey, os.Getenv("TYPESAFE_API_KEY"))
	if apiKey == "" {
		return nil, fmt.Errorf("jev: TYPESAFE_API_KEY not set")
	}
	hc := opts.HTTPClient
	if hc == nil {
		hc = http.DefaultClient
	}
	return &HTTPClient{
		baseURL:    firstNonEmpty(opts.BaseURL, os.Getenv("TYPESAFE_BASE_URL"), defaultBaseURL),
		apiKey:     apiKey,
		model:      firstNonEmpty(opts.Model, defaultModel),
		httpClient: hc,
	}, nil
}

type request struct {
	State     any                 `json:"state"`
	Model     string              `json:"model"`
	Questions map[string]Question `json:"questions"`
}

// SystemOne sends one request to POST /v1/systemone.
func (c *HTTPClient) SystemOne(ctx context.Context, state any, questions map[string]Question) (*Response, error) {
	data, err := json.Marshal(request{State: state, Model: c.model, Questions: questions})
	if err != nil {
		return nil, fmt.Errorf("jev: marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/systemone", bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("jev: request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("jev: http: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBody))
		return nil, fmt.Errorf("jev: HTTP %d: %s", resp.StatusCode, body)
	}

	var out Response
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("jev: decode: %w", err)
	}
	return &out, nil
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// --- Fake for tests ---

// Call records one SystemOne invocation on a FakeClient.
type Call struct {
	State     any
	Questions map[string]Question
}

// FakeClient answers with a caller-supplied function and records every call.
// Safe for concurrent use.
type FakeClient struct {
	Respond func(state any, questions map[string]Question) (*Response, error)

	mu    sync.Mutex
	Calls []Call
}

// SystemOne records the call and delegates to Respond.
func (f *FakeClient) SystemOne(_ context.Context, state any, questions map[string]Question) (*Response, error) {
	f.mu.Lock()
	f.Calls = append(f.Calls, Call{State: state, Questions: questions})
	f.mu.Unlock()
	if f.Respond == nil {
		return nil, fmt.Errorf("jev fake: Respond not set")
	}
	return f.Respond(state, questions)
}
