package inferred_test

import (
	"testing"

	"jev/internal/extract/inferred"
	"jev/internal/graphmodel"
)

func node(key, title, content string, emb ...float64) *graphmodel.Node {
	return &graphmodel.Node{Key: key, Content: content, Properties: map[string]string{"title": title}, Embedding: emb}
}

func TestMentionEdges(t *testing.T) {
	disney := node("d", "Walt Disney", "Walt Disney founded a studio.")
	mickey := node("m", "Mickey Mouse", "Mickey Mouse was created by Walt Disney in 1928.")
	other := node("o", "Mouse", "A mouse is a rodent; see Mickey Mouseketeers.") // title "mouse" too
	edges := inferred.MentionEdges([]*graphmodel.Node{disney, mickey, other})

	got := map[string]bool{}
	for _, e := range edges {
		got[e.F.Key+"→"+e.T.Key] = true
		if e.Origin != "inferred" {
			t.Errorf("edge %s→%s origin %q", e.F.Key, e.T.Key, e.Origin)
		}
	}
	if !got["m→d"] {
		t.Error("missing m→d (Mickey mentions Walt Disney)")
	}
	if !got["m→o"] {
		t.Error("missing m→o (\"Mickey Mouse\" contains the whole word \"mouse\")")
	}
	if got["o→m"] {
		t.Error("o→m must not exist: \"Mickey Mouseketeers\" is not the phrase \"Mickey Mouse\"")
	}
	if got["d→d"] || got["o→o"] {
		t.Error("no self-mentions")
	}
}

func TestSimilarityEdges(t *testing.T) {
	a := node("a", "", "", 1, 0)
	b := node("b", "", "", 0.9, 0.1)
	c := node("c", "", "", 0, 1)
	edges := inferred.SimilarityEdges([]*graphmodel.Node{a, b, c}, 1, 0.5)

	got := map[string]float64{}
	for _, e := range edges {
		got[e.F.Key+"→"+e.T.Key] = e.Weight
	}
	if _, ok := got["a→b"]; !ok {
		t.Errorf("missing a→b: %v", got)
	}
	if _, ok := got["c→a"]; ok {
		t.Error("c→a is below minSim")
	}
	if len(edges) != 2 { // a→b, b→a; c is below minSim with both
		t.Errorf("edges = %v, want 2", got)
	}
}
