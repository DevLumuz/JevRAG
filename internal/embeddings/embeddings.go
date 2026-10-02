// Package embeddings provides an interface for text embedding and a real
// implementation using Gemini's embedding model, plus a fake for tests.
package embeddings

import (
	"context"
	"fmt"
	"os"

	"google.golang.org/genai"
)

// Client generates vector embeddings from text.
type Client interface {
	Embed(ctx context.Context, text string) ([]float64, error)
	EmbedBatch(ctx context.Context, texts []string) ([][]float64, error)
}

// --- Gemini implementation ---

const defaultModel = "gemini-embedding-001"

// GeminiClient calls the Gemini Embedding API.
type GeminiClient struct {
	client *genai.Client
	model  string
}

// NewGeminiClient creates an embeddings client using the GEMINI_API_KEY env var.
func NewGeminiClient(ctx context.Context) (*GeminiClient, error) {
	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("embeddings: GEMINI_API_KEY not set")
	}

	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  apiKey,
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		return nil, fmt.Errorf("embeddings: creating genai client: %w", err)
	}

	return &GeminiClient{client: client, model: defaultModel}, nil
}

// Embed returns the embedding for a single text.
func (g *GeminiClient) Embed(ctx context.Context, text string) ([]float64, error) {
	result, err := g.client.Models.EmbedContent(ctx, g.model, genai.Text(text), nil)
	if err != nil {
		return nil, fmt.Errorf("embeddings: embed: %w", err)
	}
	if result == nil || len(result.Embeddings) == 0 {
		return nil, fmt.Errorf("embeddings: empty result")
	}
	return float32sToFloat64s(result.Embeddings[0].Values), nil
}

// EmbedBatch returns embeddings for multiple texts in one call.
func (g *GeminiClient) EmbedBatch(ctx context.Context, texts []string) ([][]float64, error) {
	parts := make([]genai.Part, len(texts))
	for i, t := range texts {
		parts[i] = genai.Text(t)
	}

	result, err := g.client.Models.EmbedContent(ctx, g.model, parts[0], &genai.EmbedContentConfig{})
	if err != nil {
		return nil, fmt.Errorf("embeddings: batch embed: %w", err)
	}

	// The API may not support true batching in a single call. Fall back to sequential.
	if result == nil || len(result.Embeddings) < len(texts) {
		vecs := make([][]float64, len(texts))
		for i, t := range texts {
			v, err := g.Embed(ctx, t)
			if err != nil {
				return nil, fmt.Errorf("embeddings: batch item %d: %w", i, err)
			}
			vecs[i] = v
		}
		return vecs, nil
	}

	vecs := make([][]float64, len(result.Embeddings))
	for i, emb := range result.Embeddings {
		vecs[i] = float32sToFloat64s(emb.Values)
	}
	return vecs, nil
}

func float32sToFloat64s(f32s []float32) []float64 {
	f64s := make([]float64, len(f32s))
	for i, v := range f32s {
		f64s[i] = float64(v)
	}
	return f64s
}

// --- Fake for tests ---

// FakeClient returns pre-configured embeddings for known texts.
// Unknown texts get a zero vector of the specified dimension.
type FakeClient struct {
	Vectors   map[string][]float64
	Dimension int
}

// Embed returns the pre-configured vector, or zeros.
func (f *FakeClient) Embed(_ context.Context, text string) ([]float64, error) {
	if v, ok := f.Vectors[text]; ok {
		return v, nil
	}
	return make([]float64, f.Dimension), nil
}

// EmbedBatch returns embeddings for each text sequentially.
func (f *FakeClient) EmbedBatch(ctx context.Context, texts []string) ([][]float64, error) {
	vecs := make([][]float64, len(texts))
	for i, t := range texts {
		v, err := f.Embed(ctx, t)
		if err != nil {
			return nil, err
		}
		vecs[i] = v
	}
	return vecs, nil
}
