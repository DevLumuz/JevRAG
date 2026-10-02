package embeddings_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"jev/internal/embeddings"
)

// countingClient returns deterministic 3-dim vectors and can fail first.
type countingClient struct {
	calls    int
	batches  [][]string
	failWith []error // returned (in order) before succeeding
}

func (c *countingClient) Embed(ctx context.Context, t string) ([]float64, error) {
	v, err := c.EmbedBatch(ctx, []string{t})
	if err != nil {
		return nil, err
	}
	return v[0], nil
}

func (c *countingClient) EmbedBatch(_ context.Context, texts []string) ([][]float64, error) {
	c.calls++
	if len(c.failWith) > 0 {
		err := c.failWith[0]
		c.failWith = c.failWith[1:]
		return nil, err
	}
	c.batches = append(c.batches, texts)
	out := make([][]float64, len(texts))
	for i, t := range texts {
		out[i] = []float64{float64(len(t)), 1, 2}
	}
	return out, nil
}

type tempNetErr struct{}

func (tempNetErr) Error() string   { return "connection reset" }
func (tempNetErr) Timeout() bool   { return false }
func (tempNetErr) Temporary() bool { return true }

func openTestStore(t *testing.T) *embeddings.Store {
	t.Helper()
	s, err := embeddings.OpenStore(t.TempDir(), meta)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func texts(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = fmt.Sprintf("text number %03d", i)
	}
	return out
}

func TestEmbedAll_RequiresConfirm(t *testing.T) {
	c := &countingClient{}
	_, err := embeddings.EmbedAll(context.Background(), c, openTestStore(t), texts(5), embeddings.RunOptions{})
	if !errors.Is(err, embeddings.ErrNotConfirmed) {
		t.Fatalf("err = %v, want ErrNotConfirmed", err)
	}
	if c.calls != 0 {
		t.Errorf("calls = %d, want 0", c.calls)
	}
}

func TestEmbedAll_CachedNeedsNoConfirm(t *testing.T) {
	s := openTestStore(t)
	in := texts(3)
	c := &countingClient{}
	if _, err := embeddings.EmbedAll(context.Background(), c, s, in, embeddings.RunOptions{Confirm: true}); err != nil {
		t.Fatal(err)
	}
	c2 := &countingClient{}
	vecs, err := embeddings.EmbedAll(context.Background(), c2, s, in, embeddings.RunOptions{})
	if err != nil {
		t.Fatalf("fully cached run should not need confirm: %v", err)
	}
	if c2.calls != 0 || len(vecs) != 3 {
		t.Errorf("calls = %d, vecs = %d", c2.calls, len(vecs))
	}
}

func TestEmbedAll_BatchesDedupesAndKeepsOrder(t *testing.T) {
	c := &countingClient{}
	in := append(texts(5), "text number 001", "x") // one duplicate
	vecs, err := embeddings.EmbedAll(context.Background(), c, openTestStore(t), in,
		embeddings.RunOptions{Confirm: true, BatchSize: 2})
	if err != nil {
		t.Fatal(err)
	}
	if c.calls != 3 { // 6 distinct texts / 2 per batch
		t.Errorf("calls = %d, want 3", c.calls)
	}
	if len(vecs) != len(in) || vecs[6][0] != 1 || vecs[5][0] != vecs[1][0] {
		t.Errorf("vectors not aligned to input: %v", vecs)
	}
}

func TestEmbedAll_Budget(t *testing.T) {
	c := &countingClient{}
	// Each text is 15 chars ≈ 3.75 tokens at 4 chars/token; 2 per batch ≈ 7.5.
	_, err := embeddings.EmbedAll(context.Background(), c, openTestStore(t), texts(10),
		embeddings.RunOptions{Confirm: true, BatchSize: 2, MaxTokens: 20})
	if !errors.Is(err, embeddings.ErrBudget) {
		t.Fatalf("err = %v, want ErrBudget", err)
	}
	if c.calls != 2 {
		t.Errorf("calls = %d, want 2 (stop before exceeding the cap)", c.calls)
	}
}

func TestEmbedAll_RetriesOnlyRetryable(t *testing.T) {
	c := &countingClient{failWith: []error{tempNetErr{}}}
	if _, err := embeddings.EmbedAll(context.Background(), c, openTestStore(t), texts(2),
		embeddings.RunOptions{Confirm: true, RetryDelay: 1}); err != nil {
		t.Fatalf("retryable error should be retried: %v", err)
	}
	if c.calls != 2 {
		t.Errorf("calls = %d, want 2", c.calls)
	}

	bad := &countingClient{failWith: []error{fmt.Errorf("x: %w", embeddings.ErrInvalidResponse)}}
	if _, err := embeddings.EmbedAll(context.Background(), bad, openTestStore(t), texts(2),
		embeddings.RunOptions{Confirm: true, RetryDelay: 1}); err == nil {
		t.Fatal("expected error")
	}
	if bad.calls != 1 {
		t.Errorf("calls = %d, want 1 (not retried)", bad.calls)
	}
}

func TestEmbedAll_KeepsProgressOnFailure(t *testing.T) {
	s := openTestStore(t)
	c := &countingClient{}
	in := texts(6)
	// Budget lets 2 of 3 batches through.
	_, _ = embeddings.EmbedAll(context.Background(), c, s, in,
		embeddings.RunOptions{Confirm: true, BatchSize: 2, MaxTokens: 16})
	if s.Len() != 4 {
		t.Fatalf("stored = %d, want 4 after partial run", s.Len())
	}
	c2 := &countingClient{}
	if _, err := embeddings.EmbedAll(context.Background(), c2, s, in,
		embeddings.RunOptions{Confirm: true, BatchSize: 2}); err != nil {
		t.Fatal(err)
	}
	if c2.calls != 1 {
		t.Errorf("resume calls = %d, want 1 (only the missing batch)", c2.calls)
	}
}

func TestEstimateCost(t *testing.T) {
	s := openTestStore(t)
	_ = s.Add([]string{"cached"}, [][]float64{{1, 2, 3}})
	e := embeddings.EstimateCost(s, []string{"cached", "12345678", "12345678", "abcd"}, embeddings.RunOptions{BatchSize: 1})
	if e.Texts != 4 || e.Missing != 2 || e.Requests != 2 || e.Chars != 12 || e.Tokens != 3 {
		t.Errorf("Estimate = %+v", e)
	}
}
