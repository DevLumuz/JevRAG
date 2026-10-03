package retrieval_test

import (
	"context"
	"math"
	"testing"

	"jev/internal/graphmodel"
	"jev/internal/judge"
	"jev/internal/retrieval"
)

// chain builds a -- b -- c -- d (undirected via citation edges) plus an
// isolated node z. Embeddings put a closest to the query {1,0}, then z; b, c, d
// are far from it, so only the graph can surface them.
func chain() ([]*graphmodel.Node, []graphmodel.Edge) {
	a := &graphmodel.Node{NumericID: 0, Key: "a", Embedding: []float64{1, 0}}
	b := &graphmodel.Node{NumericID: 1, Key: "b", Embedding: []float64{0, 1}}
	c := &graphmodel.Node{NumericID: 2, Key: "c", Embedding: []float64{-0.2, 1}}
	d := &graphmodel.Node{NumericID: 3, Key: "d", Embedding: []float64{-0.4, 1}}
	z := &graphmodel.Node{NumericID: 4, Key: "z", Embedding: []float64{0.9, 0.3}}
	nodes := []*graphmodel.Node{a, b, c, d, z}
	edges := []graphmodel.Edge{
		{F: a, T: b, Weight: 1},
		{F: c, T: b, Weight: 1}, // direction must not matter
		{F: c, T: d, Weight: 1},
	}
	return nodes, edges
}

func TestPersonalizedPageRank(t *testing.T) {
	nodes, edges := chain()
	g := retrieval.NewGraph(nodes, edges)
	p := g.PageRank(map[int]float64{0: 1}, nil, 0.5)

	sum := 0.0
	for _, x := range p {
		sum += x
	}
	if math.Abs(sum-1) > 1e-6 {
		t.Errorf("scores sum to %v, want 1", sum)
	}
	// Mass decays with distance from the seed; the isolated node gets none.
	if !(p[0] > p[1] && p[1] > p[2] && p[2] > p[3]) {
		t.Errorf("scores not decaying along the chain: %v", p)
	}
	if p[4] != 0 {
		t.Errorf("isolated node score = %v, want 0", p[4])
	}
}

func TestPersonalizedPageRank_WeightOverride(t *testing.T) {
	nodes, edges := chain()
	g := retrieval.NewGraph(nodes, edges)
	// Cutting a–b (multiplier 0) keeps all mass on the seed.
	p := g.PageRank(map[int]float64{0: 1}, map[[2]int]float64{{0, 1}: 0}, 0.5)
	if p[1] != 0 || math.Abs(p[0]-1) > 1e-9 {
		t.Errorf("cut edge still carries mass: %v", p)
	}
}

func TestGraphPPR_NoJudgeFindsNeighbors(t *testing.T) {
	nodes, edges := chain()
	r := &retrieval.GraphPPR{Graph: retrieval.NewGraph(nodes, edges), Seeds: 1, K: 3, Damping: 0.5}
	res, err := r.Retrieve(context.Background(), retrieval.Query{Text: "q", Embedding: []float64{1, 0}})
	if err != nil {
		t.Fatal(err)
	}
	// Seed a, then its graph neighbours b, c — not z, which is closer by
	// embedding but unconnected and not a seed.
	want := []string{"a", "b", "c"}
	for i, k := range want {
		if i >= len(res.Keys) || res.Keys[i] != k {
			t.Fatalf("Keys = %v, want %v", res.Keys, want)
		}
	}
	if res.Judged != 0 {
		t.Errorf("Judged = %d, want 0 without a judge", res.Judged)
	}
}

func TestGraphPPR_JudgeNavigatesTwoHops(t *testing.T) {
	nodes, edges := chain()
	fj := &judge.FakeJudge{Decisions: map[string]judge.Decision{
		"a": {Tier: judge.Direct}, // seed accepted
		"b": {Tier: judge.High},   // bridge accepted → expanded on hop 2
		"c": {Tier: judge.Direct}, // reached only on the 2nd hop
		// d: irrelevant
	}}
	r := &retrieval.GraphPPR{
		Graph: retrieval.NewGraph(nodes, edges), Judge: fj,
		Seeds: 1, Neighbors: 5, Hops: 2, MaxJudgeCalls: 20, K: 4, Damping: 0.5,
	}
	res, err := r.Retrieve(context.Background(), retrieval.Query{Text: "q", Embedding: []float64{1, 0}})
	if err != nil {
		t.Fatal(err)
	}
	// Judged: seed a; hop 1: a→b; hop 2: b→c. c→d is never judged because
	// c is not expanded past Hops. d's edge keeps its base weight.
	if res.Judged != 3 {
		t.Errorf("Judged = %d, want 3", res.Judged)
	}
	pos := map[string]int{}
	for i, k := range res.Keys {
		pos[k] = i
	}
	if _, ok := pos["c"]; !ok {
		t.Fatalf("2-hop node c missing: %v", res.Keys)
	}
}

func TestGraphPPR_IrrelevantEdgeBlocks(t *testing.T) {
	nodes, edges := chain()
	fj := &judge.FakeJudge{Decisions: map[string]judge.Decision{
		"a": {Tier: judge.Direct},
		// b: irrelevant → the a–b connection is cut, nothing flows past a
	}}
	r := &retrieval.GraphPPR{
		Graph: retrieval.NewGraph(nodes, edges), Judge: fj,
		Seeds: 1, Neighbors: 5, Hops: 2, MaxJudgeCalls: 20, K: 4, Damping: 0.5,
	}
	res, _ := r.Retrieve(context.Background(), retrieval.Query{Text: "q", Embedding: []float64{1, 0}})
	if len(res.Keys) != 1 || res.Keys[0] != "a" {
		t.Errorf("Keys = %v, want only [a]", res.Keys)
	}
}

func TestGraphPPR_RespectsCallBudget(t *testing.T) {
	nodes, edges := chain()
	all := map[string]judge.Decision{}
	for _, n := range nodes {
		all[n.Key] = judge.Decision{Tier: judge.High}
	}
	r := &retrieval.GraphPPR{
		Graph: retrieval.NewGraph(nodes, edges), Judge: &judge.FakeJudge{Decisions: all},
		Seeds: 2, Neighbors: 5, Hops: 3, MaxJudgeCalls: 3, K: 5, Damping: 0.5,
	}
	res, _ := r.Retrieve(context.Background(), retrieval.Query{Text: "q", Embedding: []float64{1, 0}})
	if res.Judged > 3 {
		t.Errorf("Judged = %d, exceeds budget 3", res.Judged)
	}
}

func TestGraphPPR_AbstainsWhenSeedsIrrelevant(t *testing.T) {
	nodes, edges := chain()
	r := &retrieval.GraphPPR{
		Graph: retrieval.NewGraph(nodes, edges), Judge: &judge.FakeJudge{},
		Seeds: 2, Neighbors: 5, Hops: 2, MaxJudgeCalls: 20, K: 5, Damping: 0.5,
	}
	res, _ := r.Retrieve(context.Background(), retrieval.Query{Text: "q", Embedding: []float64{1, 0}})
	if !res.Abstained || len(res.Keys) != 0 {
		t.Errorf("Result = %+v, want abstention", res)
	}
}

func TestGraphPPR_UnjudgedMult(t *testing.T) {
	nodes, edges := chain()
	fj := &judge.FakeJudge{Decisions: map[string]judge.Decision{"a": {Tier: judge.Direct}, "b": {Tier: judge.Weak}}}
	run := func(m float64) []string {
		r := &retrieval.GraphPPR{
			Graph: retrieval.NewGraph(nodes, edges), Judge: fj, UnjudgedMult: m,
			Seeds: 1, Neighbors: 5, Hops: 1, MaxJudgeCalls: 20, K: 5, Damping: 0.5,
		}
		res, _ := r.Retrieve(context.Background(), retrieval.Query{Text: "q", Embedding: []float64{1, 0}})
		return res.Keys
	}
	// b is weak (not expanded), so b–c is never judged. With a tiny
	// multiplier on unjudged links almost nothing reaches c and d.
	if keys := run(1); len(keys) != 4 {
		t.Errorf("UnjudgedMult 1: keys = %v, want a,b,c,d", keys)
	}
	if keys := run(1e-12); len(keys) < 2 || keys[0] != "a" || keys[1] != "b" {
		t.Errorf("UnjudgedMult ~0: keys = %v, want a then b first", keys)
	}
}
