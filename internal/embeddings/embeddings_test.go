package embeddings_test

import (
	"context"
	"testing"

	"jev/internal/embeddings"
)

func TestFakeClient_Embed(t *testing.T) {
	fake := &embeddings.FakeClient{
		Vectors: map[string][]float64{
			"hello": {0.1, 0.2, 0.3},
		},
		Dimension: 3,
	}

	vec, err := fake.Embed(context.Background(), "hello")
	if err != nil {
		t.Fatalf("Embed() error: %v", err)
	}
	if len(vec) != 3 || vec[0] != 0.1 {
		t.Errorf("Embed() = %v, want [0.1, 0.2, 0.3]", vec)
	}
}

func TestFakeClient_EmbedBatch(t *testing.T) {
	fake := &embeddings.FakeClient{
		Vectors: map[string][]float64{
			"a": {1, 0},
			"b": {0, 1},
		},
		Dimension: 2,
	}

	vecs, err := fake.EmbedBatch(context.Background(), []string{"a", "b"})
	if err != nil {
		t.Fatalf("EmbedBatch() error: %v", err)
	}
	if len(vecs) != 2 {
		t.Fatalf("len = %d, want 2", len(vecs))
	}
	if vecs[0][0] != 1 || vecs[1][1] != 1 {
		t.Errorf("unexpected vectors: %v", vecs)
	}
}

func TestFakeClient_UnknownText(t *testing.T) {
	fake := &embeddings.FakeClient{Dimension: 3}

	vec, err := fake.Embed(context.Background(), "unknown text")
	if err != nil {
		t.Fatalf("Embed() error: %v", err)
	}
	// Should return a zero vector of the right dimension.
	if len(vec) != 3 {
		t.Fatalf("len = %d, want 3", len(vec))
	}
	for i, v := range vec {
		if v != 0 {
			t.Errorf("vec[%d] = %f, want 0", i, v)
		}
	}
}
