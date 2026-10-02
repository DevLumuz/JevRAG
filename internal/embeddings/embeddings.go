// Package embeddings provides an interface for text embedding and a real
// implementation using Gemini's embedding model, plus a fake for tests.
package embeddings

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net"
	"os"
	"strings"
	"time"

	"google.golang.org/genai"
)

// Client generates vector embeddings from text.
type Client interface {
	Embed(ctx context.Context, text string) ([]float64, error)
	EmbedBatch(ctx context.Context, texts []string) ([][]float64, error)
}

// --- Gemini implementation ---

const (
	defaultModel   = "gemini-embedding-001"
	requestTimeout = 2 * time.Minute
)

// Task types recommended by Google for retrieval: documents are embedded as
// RETRIEVAL_DOCUMENT and the questions that search them as RETRIEVAL_QUERY
// (or QUESTION_ANSWERING).
const (
	TaskRetrievalDocument = "RETRIEVAL_DOCUMENT"
	TaskRetrievalQuery    = "RETRIEVAL_QUERY"
	TaskQuestionAnswering = "QUESTION_ANSWERING"
)

// ErrInvalidResponse means the API answered 200 but the payload cannot be
// trusted (wrong count, wrong dims, zero or non-finite vectors). It is never
// retried: the call was billed and repeating it would bill again.
var ErrInvalidResponse = errors.New("embeddings: invalid response")

// GeminiOptions configures a GeminiClient.
type GeminiOptions struct {
	TaskType   string // e.g. TaskRetrievalDocument; empty uses the API default
	Dimensions int    // output dimensionality (768, 1536, 3072); 0 uses the API default (3072)
	BaseURL    string // overrides the API endpoint (tests)
}

// GeminiClient calls the Gemini Embedding API.
type GeminiClient struct {
	client *genai.Client
	model  string
	dims   int
	config *genai.EmbedContentConfig
}

// NewGeminiClient creates an embeddings client using the GEMINI_API_KEY env var.
func NewGeminiClient(ctx context.Context, opts GeminiOptions) (*GeminiClient, error) {
	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("embeddings: GEMINI_API_KEY not set")
	}

	timeout := requestTimeout
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:      apiKey,
		Backend:     genai.BackendGeminiAPI,
		HTTPOptions: genai.HTTPOptions{BaseURL: opts.BaseURL, Timeout: &timeout},
	})
	if err != nil {
		return nil, fmt.Errorf("embeddings: creating genai client: %w", err)
	}

	cfg := &genai.EmbedContentConfig{TaskType: opts.TaskType}
	if opts.Dimensions > 0 {
		d := int32(opts.Dimensions)
		cfg.OutputDimensionality = &d
	}
	return &GeminiClient{client: client, model: defaultModel, dims: opts.Dimensions, config: cfg}, nil
}

// Model returns the embedding model name.
func (g *GeminiClient) Model() string { return g.model }

// Embed returns the embedding for a single text.
func (g *GeminiClient) Embed(ctx context.Context, text string) ([]float64, error) {
	vecs, err := g.EmbedBatch(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	return vecs[0], nil
}

// EmbedBatch embeds texts in one batchEmbedContents call (at most 100 texts).
// The response is validated strictly; there is no per-item fallback, so a
// batch is billed at most once per call.
func (g *GeminiClient) EmbedBatch(ctx context.Context, texts []string) ([][]float64, error) {
	contents := make([]*genai.Content, 0, len(texts))
	for i, t := range texts {
		if strings.TrimSpace(t) == "" {
			return nil, fmt.Errorf("embeddings: text %d is blank", i)
		}
		contents = append(contents, genai.Text(t)...)
	}

	result, err := g.client.Models.EmbedContent(ctx, g.model, contents, g.config)
	if err != nil {
		return nil, fmt.Errorf("embeddings: batch embed: %w", err)
	}
	if result == nil || len(result.Embeddings) != len(texts) {
		got := 0
		if result != nil {
			got = len(result.Embeddings)
		}
		return nil, fmt.Errorf("%w: %d embeddings for %d texts", ErrInvalidResponse, got, len(texts))
	}

	vecs := make([][]float64, len(texts))
	for i, emb := range result.Embeddings {
		if emb == nil {
			return nil, fmt.Errorf("%w: embedding %d is null", ErrInvalidResponse, i)
		}
		if g.dims > 0 && len(emb.Values) != g.dims {
			return nil, fmt.Errorf("%w: embedding %d has %d dims, want %d", ErrInvalidResponse, i, len(emb.Values), g.dims)
		}
		v := float32sToFloat64s(emb.Values)
		if err := checkVector(v); err != nil {
			return nil, fmt.Errorf("%w: embedding %d: %v", ErrInvalidResponse, i, err)
		}
		vecs[i] = v
	}
	return vecs, nil
}

func checkVector(v []float64) error {
	if len(v) == 0 {
		return errors.New("empty vector")
	}
	norm := 0.0
	for _, x := range v {
		if math.IsNaN(x) || math.IsInf(x, 0) {
			return errors.New("non-finite value")
		}
		norm += x * x
	}
	if norm == 0 {
		return errors.New("zero vector")
	}
	return nil
}

// IsRetryable reports whether an EmbedBatch error is worth retrying: rate
// limits (429), server errors (5xx) and network failures. Bad requests, auth
// errors, invalid responses and cancellation are not.
func IsRetryable(err error) bool {
	if err == nil || errors.Is(err, ErrInvalidResponse) ||
		errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
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
