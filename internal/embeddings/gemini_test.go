package embeddings_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"jev/internal/embeddings"
)

// fakeGemini serves batchEmbedContents. respond receives the number of texts
// requested and returns the JSON body and status to send back.
func fakeGemini(t *testing.T, respond func(n int) (int, string)) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if !strings.HasSuffix(r.URL.Path, ":batchEmbedContents") {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		var req struct {
			Requests []json.RawMessage `json:"requests"`
		}
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &req)
		status, body := respond(len(req.Requests))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(srv.Close)
	return srv, &calls
}

// vectorsJSON builds a batchEmbedContents response with n vectors of dim d.
func vectorsJSON(n, d int) string {
	var sb strings.Builder
	sb.WriteString(`{"embeddings":[`)
	for i := 0; i < n; i++ {
		if i > 0 {
			sb.WriteString(",")
		}
		sb.WriteString(`{"values":[`)
		for j := 0; j < d; j++ {
			if j > 0 {
				sb.WriteString(",")
			}
			fmt.Fprintf(&sb, "%g", float64(i+1)/float64(j+2))
		}
		sb.WriteString("]}")
	}
	sb.WriteString("]}")
	return sb.String()
}

func newClient(t *testing.T, url string, dims int) *embeddings.GeminiClient {
	t.Helper()
	t.Setenv("GEMINI_API_KEY", "test-key")
	c, err := embeddings.NewGeminiClient(context.Background(), embeddings.GeminiOptions{
		TaskType: embeddings.TaskRetrievalDocument, Dimensions: dims, BaseURL: url,
	})
	if err != nil {
		t.Fatalf("NewGeminiClient() error: %v", err)
	}
	return c
}

func TestGemini_EmbedBatch_OK(t *testing.T) {
	srv, calls := fakeGemini(t, func(n int) (int, string) { return 200, vectorsJSON(n, 4) })
	c := newClient(t, srv.URL, 4)

	vecs, err := c.EmbedBatch(context.Background(), []string{"a", "b", "c"})
	if err != nil {
		t.Fatalf("EmbedBatch() error: %v", err)
	}
	if len(vecs) != 3 || len(vecs[0]) != 4 {
		t.Fatalf("shape = %dx%d, want 3x4", len(vecs), len(vecs[0]))
	}
	if calls.Load() != 1 {
		t.Errorf("calls = %d, want 1", calls.Load())
	}
}

func TestGemini_EmbedBatch_RejectsBadResponses(t *testing.T) {
	tests := []struct {
		name string
		body func(n int) string
	}{
		{"one vector short", func(n int) string { return vectorsJSON(n-1, 4) }},
		{"one vector extra", func(n int) string { return vectorsJSON(n+1, 4) }},
		{"wrong dims", func(n int) string { return vectorsJSON(n, 3) }},
		{"zero vector", func(n int) string { return `{"embeddings":[{"values":[0,0,0,0]},{"values":[1,1,1,1]}]}` }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv, calls := fakeGemini(t, func(n int) (int, string) { return 200, tt.body(n) })
			c := newClient(t, srv.URL, 4)

			_, err := c.EmbedBatch(context.Background(), []string{"a", "b"})
			if !errors.Is(err, embeddings.ErrInvalidResponse) {
				t.Errorf("err = %v, want ErrInvalidResponse", err)
			}
			if embeddings.IsRetryable(err) {
				t.Error("an invalid response must not be retryable")
			}
			// No per-item fallback: a bad batch is never re-billed text by text.
			if calls.Load() != 1 {
				t.Errorf("calls = %d, want exactly 1", calls.Load())
			}
		})
	}
}

func TestGemini_EmbedBatch_RejectsEmptyText(t *testing.T) {
	srv, calls := fakeGemini(t, func(n int) (int, string) { return 200, vectorsJSON(n, 4) })
	c := newClient(t, srv.URL, 4)
	if _, err := c.EmbedBatch(context.Background(), []string{"a", "  "}); err == nil {
		t.Error("expected error for blank text")
	}
	if calls.Load() != 0 {
		t.Errorf("calls = %d, want 0 (rejected before sending)", calls.Load())
	}
}

func TestIsRetryable(t *testing.T) {
	tests := []struct {
		status int
		want   bool
	}{
		{429, true}, {500, true}, {503, true}, {504, true},
		{400, false}, {401, false}, {403, false}, {404, false},
	}
	for _, tt := range tests {
		srv, _ := fakeGemini(t, func(int) (int, string) {
			return tt.status, fmt.Sprintf(`{"error":{"code":%d,"message":"x","status":"X"}}`, tt.status)
		})
		c := newClient(t, srv.URL, 4)
		_, err := c.EmbedBatch(context.Background(), []string{"a"})
		if err == nil {
			t.Fatalf("status %d: expected error", tt.status)
		}
		if got := embeddings.IsRetryable(err); got != tt.want {
			t.Errorf("status %d: IsRetryable = %v, want %v (err %v)", tt.status, got, tt.want, err)
		}
	}
	if embeddings.IsRetryable(context.Canceled) {
		t.Error("context.Canceled must not be retryable")
	}
}
