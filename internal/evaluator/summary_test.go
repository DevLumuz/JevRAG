package evaluator_test

import (
	"math"
	"testing"
	"time"

	"jev/internal/evaluator"
)

func TestSummarize(t *testing.T) {
	outcomes := []evaluator.QueryOutcome{
		{Expected: []string{"a", "b"}, Got: []string{"a", "x", "b"}, Latency: 10 * time.Millisecond},
		{Expected: []string{"c"}, Got: []string{"x", "c"}, Latency: 30 * time.Millisecond},
		{Expected: []string{"d"}, Abstained: true, Latency: 20 * time.Millisecond},
	}
	r := evaluator.Summarize(outcomes, []int{1, 5})

	if r.Queries != 3 {
		t.Errorf("Queries = %d", r.Queries)
	}
	// Recall@1: (0.5 + 0 + 0)/3 ; Recall@5: (1 + 1 + 0)/3
	if !approx(r.RecallAt[1], 0.5/3) || !approx(r.RecallAt[5], 2.0/3) {
		t.Errorf("RecallAt = %v", r.RecallAt)
	}
	// MRR: (1 + 1/2 + 0)/3
	if !approx(r.MRR, 0.5) {
		t.Errorf("MRR = %v, want 0.5", r.MRR)
	}
	// None should abstain; one did → 2/3 correct.
	if !approx(r.AbstentionAccuracy, 2.0/3) || r.Abstained != 1 {
		t.Errorf("abstention = %v (%d)", r.AbstentionAccuracy, r.Abstained)
	}
	if r.MeanLatency != 20*time.Millisecond || r.P50Latency != 20*time.Millisecond {
		t.Errorf("latency mean=%v p50=%v", r.MeanLatency, r.P50Latency)
	}
}

func TestSummarize_Empty(t *testing.T) {
	r := evaluator.Summarize(nil, []int{5})
	if r.Queries != 0 || r.MRR != 0 {
		t.Errorf("empty summary = %+v", r)
	}
}

func approx(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func TestSummarize_CompleteAt(t *testing.T) {
	outcomes := []evaluator.QueryOutcome{
		{Expected: []string{"a", "b"}, Got: []string{"a", "x", "b"}}, // complete at 3+, not at 2
		{Expected: []string{"c"}, Got: []string{"c"}},                // complete at 1
	}
	r := evaluator.Summarize(outcomes, []int{1, 3})
	if !approx(r.CompleteAt[1], 0.5) || !approx(r.CompleteAt[3], 1.0) {
		t.Errorf("CompleteAt = %v, want {1:0.5, 3:1}", r.CompleteAt)
	}
}

func TestGroupByHops(t *testing.T) {
	g := evaluator.GroupByHops([]evaluator.QueryOutcome{{Hops: 2}, {Hops: 1}, {Hops: 2}})
	if len(g[1]) != 1 || len(g[2]) != 2 {
		t.Errorf("GroupByHops = %v", g)
	}
}

func TestSummarize_UnanswerableExcludedFromRecall(t *testing.T) {
	outcomes := []evaluator.QueryOutcome{
		{Expected: []string{"a"}, Got: []string{"a"}},               // answerable, found
		{ShouldAbstain: true, Got: []string{"x"}, Abstained: true},  // correctly abstained
		{ShouldAbstain: true, Got: []string{"y"}, Abstained: false}, // should have abstained
	}
	r := evaluator.Summarize(outcomes, []int{1})
	if r.Queries != 3 || r.Answerable != 1 || !approx(r.RecallAt[1], 1) || !approx(r.MRR, 1) {
		t.Errorf("report = %+v", r)
	}
	if !approx(r.AbstentionAccuracy, 2.0/3) {
		t.Errorf("AbstentionAccuracy = %v, want 2/3", r.AbstentionAccuracy)
	}
}
