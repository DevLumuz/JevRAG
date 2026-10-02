package embeddings_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"jev/internal/embeddings"
)

var meta = embeddings.StoreMeta{Model: "m", TaskType: "RETRIEVAL_DOCUMENT", Dims: 3}

func TestStore_AddGetPersist(t *testing.T) {
	dir := t.TempDir()
	s, err := embeddings.OpenStore(dir, meta)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Add([]string{"a", "b"}, [][]float64{{1, 2, 3}, {4, 5, 6}}); err != nil {
		t.Fatal(err)
	}
	if v, ok := s.Get("b"); !ok || v[0] != 4 {
		t.Errorf("Get(b) = %v, %v", v, ok)
	}

	// Reopen: vectors come back from disk, keyed by text.
	s2, err := embeddings.OpenStore(dir, meta)
	if err != nil {
		t.Fatal(err)
	}
	if s2.Len() != 2 {
		t.Errorf("Len() = %d, want 2", s2.Len())
	}
	if v, ok := s2.Get("a"); !ok || v[2] != 3 {
		t.Errorf("Get(a) after reopen = %v, %v", v, ok)
	}
	if _, ok := s2.Get("A"); ok {
		t.Error("a different text must not hit")
	}
}

func TestStore_MissingDedupes(t *testing.T) {
	s, _ := embeddings.OpenStore(t.TempDir(), meta)
	_ = s.Add([]string{"a"}, [][]float64{{1, 1, 1}})
	got := s.Missing([]string{"a", "b", "c", "b"})
	if len(got) != 2 || got[0] != "b" || got[1] != "c" {
		t.Errorf("Missing() = %v, want [b c]", got)
	}
}

func TestStore_MetaMismatchFails(t *testing.T) {
	dir := t.TempDir()
	if _, err := embeddings.OpenStore(dir, meta); err != nil {
		t.Fatal(err)
	}
	other := meta
	other.Dims = 768
	_, err := embeddings.OpenStore(dir, other)
	if err == nil || !strings.Contains(err.Error(), "dims") {
		t.Errorf("err = %v, want a dims mismatch", err)
	}
}

func TestStore_AddRejectsWrongDims(t *testing.T) {
	s, _ := embeddings.OpenStore(t.TempDir(), meta)
	if err := s.Add([]string{"a"}, [][]float64{{1, 2}}); err == nil {
		t.Error("expected error for wrong dims")
	}
	if err := s.Add([]string{"a", "b"}, [][]float64{{1, 2, 3}}); err == nil {
		t.Error("expected error for count mismatch")
	}
}

func TestStore_CorruptSegmentFails(t *testing.T) {
	dir := t.TempDir()
	s, _ := embeddings.OpenStore(dir, meta)
	_ = s.Add([]string{"a"}, [][]float64{{1, 2, 3}})

	segs, _ := filepath.Glob(filepath.Join(dir, "*.seg"))
	if len(segs) != 1 {
		t.Fatalf("segments = %v", segs)
	}
	info, _ := os.Stat(segs[0])
	if err := os.Truncate(segs[0], info.Size()-2); err != nil {
		t.Fatal(err)
	}
	if _, err := embeddings.OpenStore(dir, meta); err == nil {
		t.Error("expected error for truncated segment")
	}
}

func TestReduce(t *testing.T) {
	v := embeddings.Reduce([]float64{3, 4, 12}, 2)
	if len(v) != 2 || v[0] != 0.6 || v[1] != 0.8 {
		t.Errorf("Reduce() = %v, want [0.6 0.8]", v)
	}
	full := []float64{3, 4}
	if got := embeddings.Reduce(full, 0); len(got) != 2 || got[0] != 0.6 {
		t.Errorf("Reduce(dims 0) = %v, want normalized full vector", got)
	}
}
