package retrieval_test

import (
	"testing"

	"jev/internal/graphmodel"
	"jev/internal/retrieval"
)

func TestBM25(t *testing.T) {
	nodes := []*graphmodel.Node{
		{Key: "a", Content: "Jim Wilson played first base for the Cleveland Indians."},
		{Key: "b", Content: "The Cleveland Indians won 26 games in a row in 1916."},
		{Key: "c", Content: "A recipe for apple pie with cinnamon."},
	}
	ix := retrieval.NewBM25(nodes, func(n *graphmodel.Node) string { return n.Content })
	res := ix.Search("Cleveland Indians won games", 3) // no stemming: "won" must appear literally
	if len(res) != 2 {
		t.Fatalf("results = %d, want 2 (c shares no term)", len(res))
	}
	if res[0].Node.Key != "b" {
		t.Errorf("top = %s, want b (more query terms)", res[0].Node.Key)
	}
	if got := ix.Search("Jim Wilson", 1); got[0].Node.Key != "a" {
		t.Errorf("exact name search top = %s, want a", got[0].Node.Key)
	}
}

func TestTokenize(t *testing.T) {
	got := retrieval.Tokenize("Art. 4-2, CIVIL Act (1990)")
	want := []string{"art", "4", "2", "civil", "act", "1990"}
	if len(got) != len(want) {
		t.Fatalf("got %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("got %v, want %v", got, want)
		}
	}
}

func TestFuseRRF(t *testing.T) {
	a := &graphmodel.Node{Key: "a"}
	b := &graphmodel.Node{Key: "b"}
	c := &graphmodel.Node{Key: "c"}
	fused := retrieval.FuseRRF(60, 3,
		[]retrieval.Scored{{Node: a}, {Node: b}},
		[]retrieval.Scored{{Node: b}, {Node: c}})
	if fused[0].Node.Key != "b" {
		t.Errorf("top = %s, want b (in both lists)", fused[0].Node.Key)
	}
	if len(fused) != 3 {
		t.Errorf("len = %d", len(fused))
	}
}
