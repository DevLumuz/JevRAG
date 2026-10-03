// Package llm is the Gemini generation client used by the LLM judge (option 4).
package llm

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	"google.golang.org/genai"
)

// Options configures a Gemini generator.
type Options struct {
	Model   string // e.g. "gemini-3.8-flash"
	BaseURL string // overrides the endpoint (tests)
}

// Gemini generates JSON answers with a Gemini model. Thinking is turned to
// its minimum (it is billed as output) and temperature is 0.
type Gemini struct {
	client *genai.Client
	model  string
}

// NewGemini creates a generator using the GEMINI_API_KEY env var.
func NewGemini(ctx context.Context, opts Options) (*Gemini, error) {
	key := os.Getenv("GEMINI_API_KEY")
	if key == "" {
		return nil, errors.New("llm: GEMINI_API_KEY not set")
	}
	if opts.Model == "" {
		return nil, errors.New("llm: model not set")
	}
	timeout := 2 * time.Minute
	c, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey: key, Backend: genai.BackendGeminiAPI,
		HTTPOptions: genai.HTTPOptions{BaseURL: opts.BaseURL, Timeout: &timeout},
	})
	if err != nil {
		return nil, fmt.Errorf("llm: creating client: %w", err)
	}
	return &Gemini{client: c, model: opts.Model}, nil
}

func (g *Gemini) config(schema map[string]any) *genai.GenerateContentConfig {
	zero := float32(0)
	cfg := &genai.GenerateContentConfig{
		Temperature:        &zero,
		ResponseMIMEType:   "application/json",
		ResponseJsonSchema: schema,
	}
	if strings.HasPrefix(g.model, "gemini-2") {
		budget := int32(0)
		cfg.ThinkingConfig = &genai.ThinkingConfig{ThinkingBudget: &budget}
	} else {
		cfg.ThinkingConfig = &genai.ThinkingConfig{ThinkingLevel: genai.ThinkingLevelMinimal}
	}
	return cfg
}

// GenerateJSON returns the model's JSON text and billable tokens (output
// includes thinking tokens). Rate limits, 5xx and network errors are retried
// with backoff; other errors are returned at once.
func (g *Gemini) GenerateJSON(ctx context.Context, prompt string, schema map[string]any) (string, int64, int64, error) {
	delay := 2 * time.Second
	for attempt := 0; ; attempt++ {
		resp, err := g.client.Models.GenerateContent(ctx, g.model, genai.Text(prompt), g.config(schema))
		if err == nil {
			var in, out int64
			if u := resp.UsageMetadata; u != nil {
				in = int64(u.PromptTokenCount)
				out = int64(u.CandidatesTokenCount) + int64(u.ThoughtsTokenCount)
			}
			text := resp.Text()
			if text == "" {
				return "", in, out, errors.New("llm: empty response")
			}
			return text, in, out, nil
		}
		if !retryable(err) || attempt >= 5 {
			return "", 0, 0, err
		}
		select {
		case <-ctx.Done():
			return "", 0, 0, ctx.Err()
		case <-time.After(delay):
		}
		delay *= 2
	}
}

func retryable(err error) bool {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	var ae genai.APIError
	if errors.As(err, &ae) {
		return ae.Code == 429 || ae.Code >= 500
	}
	var aep *genai.APIError
	if errors.As(err, &aep) {
		return aep.Code == 429 || aep.Code >= 500
	}
	var ne net.Error
	return errors.As(err, &ne)
}
