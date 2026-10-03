package retrieval_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"jev/internal/graphmodel"
	"jev/internal/judge"
	"jev/internal/retrieval"
)

// Four nodes on the unit circle-ish: similarity to query {1,0} is a > b > c > d.
func fixtureNodes() []*graphmodel.Node {
	return []*graphmodel.Node{
		{Key: "c", Embedding: []float64{0.2, 1}},
		{Key: "a", Embedding: []float64{1, 0}},
		{Key: "d", Embedding: []float64{-1, 0}},
		{Key: "b", Embedding: []float64{1, 0.5}},
	}
}

var query = retrieval.Query{Text: "q", Embedding: []float64{1, 0}}

func keys(r retrieval.Result) []string { return r.Keys }

func TestVectorSearch(t *testing.T) {
	got := retrieval.VectorSearch(query.Embedding, fixtureNodes(), 3)
	var ks []string
	for _, s := range got {
		ks = append(ks, s.Node.Key)
	}
	if want := []string{"a", "b", "c"}; !reflect.DeepEqual(ks, want) {
		t.Errorf("VectorSearch keys = %v, want %v", ks, want)
	}
	if got[0].Score < got[1].Score {
		t.Errorf("scores not descending: %v", got)
	}
	if n := len(retrieval.VectorSearch(query.Embedding, fixtureNodes(), 10)); n != 4 {
		t.Errorf("k > len(nodes): got %d results, want 4", n)
	}
}

func TestPlainVector(t *testing.T) {
	r := &retrieval.PlainVector{Nodes: fixtureNodes(), K: 2}
	res, err := r.Retrieve(context.Background(), query)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"a", "b"}; !reflect.DeepEqual(keys(res), want) {
		t.Errorf("Keys = %v, want %v", keys(res), want)
	}
	if res.Abstained {
		t.Error("should not abstain with MinScore 0")
	}
}

func TestPlainVector_AbstainsBelowMinScore(t *testing.T) {
	r := &retrieval.PlainVector{Nodes: fixtureNodes(), K: 2, MinScore: 1.5}
	res, _ := r.Retrieve(context.Background(), query)
	if !res.Abstained || len(res.Keys) != 0 {
		t.Errorf("Result = %+v, want abstention with no keys", res)
	}
}

func TestJudgedVector_ReranksAndFilters(t *testing.T) {
	fj := &judge.FakeJudge{Decisions: map[string]judge.Decision{
		"a": {Tier: judge.Weak},   // ~1.0 * 0.25
		"b": {Tier: judge.Direct}, // ~0.89 * 5
		"c": {Tier: judge.High},   // ~0.2 * 2.5
		// "d" missing → Irrelevant → dropped
	}}
	r := &retrieval.JudgedVector{Nodes: fixtureNodes(), Judge: fj, Candidates: 4, K: 10}

	res, err := r.Retrieve(context.Background(), query)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"b", "c", "a"}; !reflect.DeepEqual(keys(res), want) {
		t.Errorf("Keys = %v, want %v", keys(res), want)
	}
	if res.Abstained {
		t.Error("should not abstain when something was kept")
	}
	if res.Judged != 4 {
		t.Errorf("Judged = %d, want 4", res.Judged)
	}
}

func TestJudgedVector_TruncatesToK(t *testing.T) {
	fj := &judge.FakeJudge{Decisions: map[string]judge.Decision{
		"a": {Tier: judge.High}, "b": {Tier: judge.High}, "c": {Tier: judge.High},
	}}
	r := &retrieval.JudgedVector{Nodes: fixtureNodes(), Judge: fj, Candidates: 4, K: 2}
	res, _ := r.Retrieve(context.Background(), query)
	if want := []string{"a", "b"}; !reflect.DeepEqual(keys(res), want) {
		t.Errorf("Keys = %v, want %v", keys(res), want)
	}
}

func TestJudgedVector_AbstainsWhenAllIrrelevant(t *testing.T) {
	r := &retrieval.JudgedVector{Nodes: fixtureNodes(), Judge: &judge.FakeJudge{}, Candidates: 4, K: 5}
	res, _ := r.Retrieve(context.Background(), query)
	if !res.Abstained || len(res.Keys) != 0 {
		t.Errorf("Result = %+v, want abstention", res)
	}
}

type failingJudge struct{}

func (failingJudge) ScoreEdge(context.Context, graphmodel.Edge, string, []*graphmodel.Node) (judge.Decision, error) {
	return judge.Decision{}, errors.New("boom")
}

func TestJudgedVector_PropagatesJudgeError(t *testing.T) {
	r := &retrieval.JudgedVector{Nodes: fixtureNodes(), Judge: failingJudge{}, Candidates: 2, K: 2}
	if _, err := r.Retrieve(context.Background(), query); err == nil {
		t.Error("expected error from judge")
	}
}

type fixedChecker struct {
	p    float64
	seen []string
}

func (f *fixedChecker) EvidenceSufficiency(_ context.Context, _ string, ev []*graphmodel.Node) (float64, error) {
	for _, n := range ev {
		f.seen = append(f.seen, n.Key)
	}
	return f.p, nil
}

func TestGate(t *testing.T) {
	nodes := fixtureNodes()
	byKey := map[string]*graphmodel.Node{}
	for _, n := range nodes {
		byKey[n.Key] = n
	}
	for _, tt := range []struct {
		p       float64
		abstain bool
	}{{0.9, false}, {0.2, true}} {
		chk := &fixedChecker{p: tt.p}
		g := &retrieval.Gate{Inner: &retrieval.PlainVector{Nodes: nodes, K: 4}, Checker: chk, Nodes: byKey, TopN: 2, Threshold: 0.5}
		res, err := g.Retrieve(context.Background(), query)
		if err != nil {
			t.Fatal(err)
		}
		if res.Abstained != tt.abstain || res.Sufficiency != tt.p || len(res.Keys) != 4 || res.Judged != 1 {
			t.Errorf("p=%v: result = %+v", tt.p, res)
		}
		if !reflect.DeepEqual(chk.seen, []string{"a", "b"}) {
			t.Errorf("checker saw %v, want top 2 [a b]", chk.seen)
		}
	}
}
