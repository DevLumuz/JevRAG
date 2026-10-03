package evaluator

import (
	"sort"
	"time"
)

// QueryOutcome is what one retrieval option produced for one question.
type QueryOutcome struct {
	ID            string
	Expected      []string // gold keys
	Got           []string // ranked keys returned (empty if abstained)
	Abstained     bool
	ShouldAbstain bool // ground truth; false for every KoBLEX question
	Latency       time.Duration
	JudgeCalls    int
	Hops          int // reasoning steps (gold articles) the question needs
}

// Report aggregates the metrics of section 10 of the plan over a split.
type Report struct {
	Queries            int
	RecallAt           map[int]float64 // mean Recall@K, keyed by K
	CompleteAt         map[int]float64 // share of queries with ALL expected keys in the top K
	MRR                float64
	AbstentionAccuracy float64
	Abstained          int
	MeanLatency        time.Duration
	P50Latency         time.Duration
	JudgeCalls         int
}

// Summarize computes mean Recall@K for each k in ks, MRR, abstention accuracy
// and latency over all outcomes. An abstention counts as retrieving nothing.
func Summarize(outcomes []QueryOutcome, ks []int) Report {
	r := Report{Queries: len(outcomes), RecallAt: make(map[int]float64, len(ks)), CompleteAt: make(map[int]float64, len(ks))}
	if len(outcomes) == 0 {
		return r
	}

	ranked := make([]RankedResult, len(outcomes))
	abst := make([]AbstentionPrediction, len(outcomes))
	lats := make([]time.Duration, len(outcomes))
	var total time.Duration

	for i, o := range outcomes {
		for _, k := range ks {
			rec := RecallAtK(o.Expected, o.Got, k)
			r.RecallAt[k] += rec
			if rec == 1 {
				r.CompleteAt[k]++
			}
		}
		ranked[i] = RankedResult{Expected: o.Expected, Got: o.Got}
		abst[i] = AbstentionPrediction{ShouldAbstain: o.ShouldAbstain, DidAbstain: o.Abstained}
		if o.Abstained {
			r.Abstained++
		}
		lats[i] = o.Latency
		total += o.Latency
		r.JudgeCalls += o.JudgeCalls
	}

	n := float64(len(outcomes))
	for _, k := range ks {
		r.RecallAt[k] /= n
		r.CompleteAt[k] /= n
	}
	r.MRR = MRR(ranked)
	r.AbstentionAccuracy = AbstentionAccuracy(abst)
	r.MeanLatency = total / time.Duration(len(outcomes))

	sort.Slice(lats, func(i, j int) bool { return lats[i] < lats[j] })
	r.P50Latency = lats[len(lats)/2]
	return r
}

// GroupByHops splits outcomes by the number of reasoning steps they need.
func GroupByHops(outcomes []QueryOutcome) map[int][]QueryOutcome {
	g := map[int][]QueryOutcome{}
	for _, o := range outcomes {
		g[o.Hops] = append(g[o.Hops], o)
	}
	return g
}
