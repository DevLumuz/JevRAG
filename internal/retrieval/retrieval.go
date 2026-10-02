// Package retrieval implements the harness's retrieval options behind one
// Retriever interface, so the CLI and the evaluator treat them the same way.
//
//	Option 1 — PlainVector:  top-K by embedding similarity, no judge.
//	Option 2 — JudgedVector: generous top-N by similarity, each candidate
//	                          judged, reranked by EffectiveWeight.
package retrieval

import (
	"context"
	"fmt"
	"sort"
	"sync"

	"jev/internal/evaluator"
	"jev/internal/graphmodel"
	"jev/internal/judge"
)

// Query is one question with its precomputed embedding.
type Query struct {
	Text      string
	Embedding []float64
}

// Result is a ranked list of node keys, or an explicit abstention.
type Result struct {
	Keys      []string // ranked, best first; empty when Abstained
	Abstained bool     // the option judged the evidence insufficient
	Judged    int      // judge calls made (0 for options without a judge)
}

// Retriever is one retrieval option.
type Retriever interface {
	Retrieve(ctx context.Context, q Query) (Result, error)
}

// Scored pairs a node with its similarity to the query.
type Scored struct {
	Node  *graphmodel.Node
	Score float64
}

// VectorSearch returns the k nodes most similar to the query embedding,
// best first. Ties keep the input order.
func VectorSearch(queryEmb []float64, nodes []*graphmodel.Node, k int) []Scored {
	scored := make([]Scored, len(nodes))
	for i, n := range nodes {
		scored[i] = Scored{Node: n, Score: evaluator.CosineSimilarity(queryEmb, n.Embedding)}
	}
	sort.SliceStable(scored, func(i, j int) bool { return scored[i].Score > scored[j].Score })
	if k < len(scored) {
		scored = scored[:k]
	}
	return scored
}

// --- Option 1 ---

// PlainVector is option 1: vector RAG with no judge. It abstains only when
// even the best match scores below MinScore (0 disables abstention).
type PlainVector struct {
	Nodes    []*graphmodel.Node
	K        int
	MinScore float64
}

// Retrieve returns the top-K nodes by similarity.
func (p *PlainVector) Retrieve(_ context.Context, q Query) (Result, error) {
	top := VectorSearch(q.Embedding, p.Nodes, p.K)
	if len(top) == 0 || top[0].Score < p.MinScore {
		return Result{Abstained: true}, nil
	}
	keys := make([]string, len(top))
	for i, s := range top {
		keys[i] = s.Node.Key
	}
	return Result{Keys: keys}, nil
}

// --- Option 2 ---

const defaultConcurrency = 8

// JudgedVector is option 2: vector RAG + judge, no graph. The top Candidates
// nodes by similarity are judged independently (each as an edge from the
// query, with the similarity as static Weight), Irrelevant ones are dropped,
// and the rest are ranked by judge.EffectiveWeight. It abstains when the
// judge keeps nothing.
type JudgedVector struct {
	Nodes       []*graphmodel.Node
	Judge       judge.Judge
	Candidates  int // how many similarity results go to the judge; keep generous
	K           int // how many keys to return
	Concurrency int // parallel judge calls; 0 → 8
}

type judged struct {
	scored    Scored
	effWeight float64
	decision  judge.Decision
}

// Retrieve judges the similarity candidates and returns the reranked top-K.
func (j *JudgedVector) Retrieve(ctx context.Context, q Query) (Result, error) {
	cands := VectorSearch(q.Embedding, j.Nodes, j.Candidates)

	results := make([]judged, len(cands))
	errs := make([]error, len(cands))

	conc := j.Concurrency
	if conc <= 0 {
		conc = defaultConcurrency
	}
	sem := make(chan struct{}, conc)
	var wg sync.WaitGroup
	for i, c := range cands {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int, c Scored) {
			defer wg.Done()
			defer func() { <-sem }()
			edge := graphmodel.Edge{T: c.Node, Type: "query_match", Origin: "inferred", Weight: c.Score}
			d, err := j.Judge.ScoreEdge(ctx, edge, q.Text, nil)
			results[i] = judged{scored: c, effWeight: judge.EffectiveWeight(edge, d), decision: d}
			errs[i] = err
		}(i, c)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			return Result{}, fmt.Errorf("retrieval: judging %q: %w", cands[i].Node.Key, err)
		}
	}

	kept := results[:0]
	for _, r := range results {
		if r.decision.Tier != judge.Irrelevant {
			kept = append(kept, r)
		}
	}
	sort.SliceStable(kept, func(a, b int) bool { return kept[a].effWeight > kept[b].effWeight })

	res := Result{Judged: len(cands)}
	if len(kept) == 0 {
		res.Abstained = true
		return res, nil
	}
	for i := 0; i < len(kept) && i < j.K; i++ {
		res.Keys = append(res.Keys, kept[i].scored.Node.Key)
	}
	return res, nil
}
