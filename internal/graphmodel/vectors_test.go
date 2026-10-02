package graphmodel_test

import (
	"os"
	"path/filepath"
	"testing"

	"jev/internal/graphmodel"
)

func TestVectorsRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "v.bin")
	in := [][]float64{{0.5, -1.25, 3}, {0, 0.75, -0.5}}

	if err := graphmodel.SaveVectors(path, in); err != nil {
		t.Fatalf("SaveVectors() error: %v", err)
	}
	out, err := graphmodel.LoadVectors(path)
	if err != nil {
		t.Fatalf("LoadVectors() error: %v", err)
	}
	if len(out) != 2 || len(out[0]) != 3 {
		t.Fatalf("shape = %dx%d, want 2x3", len(out), len(out[0]))
	}
	for i := range in {
		for j := range in[i] {
			if out[i][j] != in[i][j] { // exactly representable in float32
				t.Errorf("out[%d][%d] = %v, want %v", i, j, out[i][j], in[i][j])
			}
		}
	}
}

func TestSaveVectors_RaggedFails(t *testing.T) {
	path := filepath.Join(t.TempDir(), "v.bin")
	if err := graphmodel.SaveVectors(path, [][]float64{{1, 2}, {1}}); err == nil {
		t.Error("expected error for vectors of different dims")
	}
}

func TestLoadVectors_Truncated(t *testing.T) {
	path := filepath.Join(t.TempDir(), "v.bin")
	if err := graphmodel.SaveVectors(path, [][]float64{{1, 2}, {3, 4}}); err != nil {
		t.Fatal(err)
	}
	if err := os.Truncate(path, 8+4*3); err != nil { // header + 3 of 4 floats
		t.Fatal(err)
	}
	if _, err := graphmodel.LoadVectors(path); err == nil {
		t.Error("expected error for truncated file")
	}
}
