// Package jev provides a client interface for TypeSafe's JEV model.
// JEV is a non-autoregressive decision model that returns typed decisions
// (Choice, Score, Noul) instead of free text.
package jev

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
)

// Client covers JEV's three primitives.
type Client interface {
	Noul(ctx context.Context, state, question string) (probability float64, err error)
	Score(ctx context.Context, state, question string, levels []string) (score, confidence float64, err error)
	Choice(ctx context.Context, state, question string, options []string) (choice string, confidence float64, err error)
}

// --- Real HTTP client ---

const defaultBaseURL = "https://api.typesafe.ai/v1"

// HTTPClient calls the TypeSafe JEV API.
type HTTPClient struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

// Options configures the JEV client.
type Options struct {
	BaseURL    string
	HTTPClient *http.Client
}

// NewHTTPClient creates a JEV client using the TYPESAGE_API_KEY env var.
func NewHTTPClient(opts Options) (*HTTPClient, error) {
	apiKey := os.Getenv("TYPESAGE_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("jev: TYPESAGE_API_KEY not set")
	}
	base := opts.BaseURL
	if base == "" {
		base = defaultBaseURL
	}
	hc := opts.HTTPClient
	if hc == nil {
		hc = http.DefaultClient
	}
	return &HTTPClient{baseURL: base, apiKey: apiKey, httpClient: hc}, nil
}

type apiRequest struct {
	State    string   `json:"state"`
	Question string   `json:"question"`
	Options  []string `json:"options,omitempty"`
	Levels   []string `json:"levels,omitempty"`
}

type choiceResponse struct {
	Choice     string  `json:"choice"`
	Confidence float64 `json:"confidence"`
}

type scoreResponse struct {
	Score      float64 `json:"score"`
	Confidence float64 `json:"confidence"`
}

type noulResponse struct {
	Probability float64 `json:"probability"`
}

func (c *HTTPClient) post(ctx context.Context, endpoint string, body any, result any) error {
	data, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("jev: marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+endpoint, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("jev: request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("jev: http: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("jev: HTTP %d", resp.StatusCode)
	}

	if err := json.NewDecoder(resp.Body).Decode(result); err != nil {
		return fmt.Errorf("jev: decode: %w", err)
	}
	return nil
}

func (c *HTTPClient) Noul(ctx context.Context, state, question string) (float64, error) {
	var resp noulResponse
	err := c.post(ctx, "/noul", apiRequest{State: state, Question: question}, &resp)
	return resp.Probability, err
}

func (c *HTTPClient) Score(ctx context.Context, state, question string, levels []string) (float64, float64, error) {
	var resp scoreResponse
	err := c.post(ctx, "/score", apiRequest{State: state, Question: question, Levels: levels}, &resp)
	return resp.Score, resp.Confidence, err
}

func (c *HTTPClient) Choice(ctx context.Context, state, question string, options []string) (string, float64, error) {
	var resp choiceResponse
	err := c.post(ctx, "/choice", apiRequest{State: state, Question: question, Options: options}, &resp)
	return resp.Choice, resp.Confidence, err
}

// --- Fake for tests ---

// ChoiceResult holds a pre-configured choice response.
type ChoiceResult struct {
	Choice     string
	Confidence float64
}

// ScoreResult holds a pre-configured score response.
type ScoreResult struct {
	Score      float64
	Confidence float64
}

// FakeClient returns pre-configured responses keyed by question.
type FakeClient struct {
	ChoiceResults map[string]ChoiceResult
	ScoreResults  map[string]ScoreResult
	NoulResults   map[string]float64
}

func (f *FakeClient) Choice(_ context.Context, _, question string, _ []string) (string, float64, error) {
	if r, ok := f.ChoiceResults[question]; ok {
		return r.Choice, r.Confidence, nil
	}
	return "", 0, fmt.Errorf("jev fake: no result for question %q", question)
}

func (f *FakeClient) Score(_ context.Context, _, question string, _ []string) (float64, float64, error) {
	if r, ok := f.ScoreResults[question]; ok {
		return r.Score, r.Confidence, nil
	}
	return 0, 0, fmt.Errorf("jev fake: no result for question %q", question)
}

func (f *FakeClient) Noul(_ context.Context, _, question string) (float64, error) {
	if p, ok := f.NoulResults[question]; ok {
		return p, nil
	}
	return 0, fmt.Errorf("jev fake: no result for question %q", question)
}
