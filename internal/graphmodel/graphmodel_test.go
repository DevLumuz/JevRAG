package graphmodel_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"jev/internal/graphmodel"
)

// --- Node ---

func TestNodeID(t *testing.T) {
	n := &graphmodel.Node{NumericID: 42}
	if n.ID() != 42 {
		t.Errorf("ID() = %d, want 42", n.ID())
	}
}

// --- Edge implements gonum graph.Edge ---

func TestEdgeEndpoints(t *testing.T) {
	a := &graphmodel.Node{NumericID: 1, Key: "a"}
	b := &graphmodel.Node{NumericID: 2, Key: "b"}
	e := graphmodel.Edge{F: a, T: b, Type: "references", Weight: 0.8}

	if e.From().ID() != 1 {
		t.Errorf("From().ID() = %d, want 1", e.From().ID())
	}
	if e.To().ID() != 2 {
		t.Errorf("To().ID() = %d, want 2", e.To().ID())
	}
}

func TestEdgeReversed(t *testing.T) {
	a := &graphmodel.Node{NumericID: 1, Key: "a"}
	b := &graphmodel.Node{NumericID: 2, Key: "b"}
	e := graphmodel.Edge{F: a, T: b, Type: "references", Origin: "extracted", Weight: 0.9}

	rev := e.ReversedEdge().(graphmodel.Edge)
	if rev.From().ID() != 2 || rev.To().ID() != 1 {
		t.Errorf("reversed endpoints wrong: from=%d to=%d", rev.From().ID(), rev.To().ID())
	}
	if rev.Type != "references" || rev.Origin != "extracted" || rev.Weight != 0.9 {
		t.Error("reversed edge lost metadata")
	}
}

// --- BuildGraph ---

func TestBuildGraph(t *testing.T) {
	n1 := &graphmodel.Node{NumericID: 1, Key: "art_1"}
	n2 := &graphmodel.Node{NumericID: 2, Key: "art_2"}
	n3 := &graphmodel.Node{NumericID: 3, Key: "art_3"}
	edges := []graphmodel.Edge{
		{F: n1, T: n2, Type: "references", Weight: 0.5},
		{F: n2, T: n3, Type: "modifies", Weight: 0.7},
	}

	g := graphmodel.BuildGraph([]*graphmodel.Node{n1, n2, n3}, edges)

	// Check node count.
	count := 0
	nodes := g.Nodes()
	for nodes.Next() {
		count++
	}
	if count != 3 {
		t.Errorf("node count = %d, want 3", count)
	}

	// Check edges exist.
	if !g.HasEdgeFromTo(1, 2) {
		t.Error("missing edge 1→2")
	}
	if !g.HasEdgeFromTo(2, 3) {
		t.Error("missing edge 2→3")
	}
	if g.HasEdgeFromTo(1, 3) {
		t.Error("unexpected edge 1→3")
	}
}

// --- JSON persistence ---

func makeTestData() ([]*graphmodel.Node, []graphmodel.Edge) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	n1 := &graphmodel.Node{
		NumericID:   1,
		Key:         "civil_code_art_40",
		Content:     "Artículo 40. Las personas son...",
		Label:       "LegalArticle",
		Properties:  map[string]string{"law": "Código Civil", "article_number": "40"},
		Embedding:   []float64{0.1, 0.2, 0.3},
		LastUpdated: now,
	}
	n2 := &graphmodel.Node{
		NumericID:   2,
		Key:         "civil_code_art_41",
		Content:     "Artículo 41. ...",
		Label:       "LegalArticle",
		Properties:  map[string]string{"law": "Código Civil", "article_number": "41"},
		Embedding:   []float64{0.4, 0.5, 0.6},
		LastUpdated: now,
	}
	edges := []graphmodel.Edge{
		{F: n1, T: n2, Type: "references", Origin: "extracted", Weight: 0.85,
			Properties: map[string]string{"note": "explicit citation"}},
	}
	return []*graphmodel.Node{n1, n2}, edges
}

func TestSaveAndLoadGraph(t *testing.T) {
	nodes, edges := makeTestData()

	dir := t.TempDir()
	path := filepath.Join(dir, "test.graph.json")

	// Save.
	if err := graphmodel.SaveGraph(path, nodes, edges); err != nil {
		t.Fatalf("SaveGraph: %v", err)
	}

	// File must exist and be valid JSON.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading file: %v", err)
	}
	if !json.Valid(data) {
		t.Fatal("saved file is not valid JSON")
	}

	// Load.
	loadedNodes, loadedEdges, err := graphmodel.LoadGraph(path)
	if err != nil {
		t.Fatalf("LoadGraph: %v", err)
	}

	if len(loadedNodes) != 2 {
		t.Fatalf("loaded %d nodes, want 2", len(loadedNodes))
	}
	if len(loadedEdges) != 1 {
		t.Fatalf("loaded %d edges, want 1", len(loadedEdges))
	}

	// Verify node data survived round-trip.
	n := loadedNodes[0]
	if n.Key != "civil_code_art_40" {
		t.Errorf("node.Key = %q", n.Key)
	}
	if n.Label != "LegalArticle" {
		t.Errorf("node.Label = %q", n.Label)
	}
	if n.Properties["law"] != "Código Civil" {
		t.Errorf("node.Properties[law] = %q", n.Properties["law"])
	}
	if len(n.Embedding) != 3 || n.Embedding[0] != 0.1 {
		t.Errorf("node.Embedding = %v", n.Embedding)
	}

	// Verify edge data survived round-trip.
	e := loadedEdges[0]
	if e.Type != "references" {
		t.Errorf("edge.Type = %q", e.Type)
	}
	if e.Origin != "extracted" {
		t.Errorf("edge.Origin = %q", e.Origin)
	}
	if e.Weight != 0.85 {
		t.Errorf("edge.Weight = %f", e.Weight)
	}
	if e.F.Key != "civil_code_art_40" || e.T.Key != "civil_code_art_41" {
		t.Errorf("edge endpoints: %q → %q", e.F.Key, e.T.Key)
	}
}

func TestLoadGraph_FileNotFound(t *testing.T) {
	_, _, err := graphmodel.LoadGraph("/nonexistent/path.json")
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}
