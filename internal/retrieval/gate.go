package retrieval

import (
	"context"
	"fmt"

	"jev/internal/graphmodel"
)

// SufficiencyChecker estimates the probability that a set of passages is
// enough to answer a query (judge.JEVJudge implements it).
type SufficiencyChecker interface {
	EvidenceSufficiency(ctx context.Context, query string, evidence []*graphmodel.Node) (float64, error)
}

// Gate wraps any retrieval option with an evidence-sufficiency check: after
// retrieval, the top TopN passages go to Checker in one call, and the option
// abstains when the probability is below Threshold. The ranked keys are kept,
// so retrieval quality and the answer/abstain decision can be measured apart;
// the probability is returned so the threshold can be tuned on dev offline.
type Gate struct {
	Inner     Retriever
	Checker   SufficiencyChecker
	Nodes     map[string]*graphmodel.Node // by Key
	TopN      int
	Threshold float64
}

// Retrieve runs the inner option, then the sufficiency check.
func (g *Gate) Retrieve(ctx context.Context, q Query) (Result, error) {
	res, err := g.Inner.Retrieve(ctx, q)
	if err != nil || res.Abstained || len(res.Keys) == 0 {
		return res, err
	}
	var ev []*graphmodel.Node
	for _, k := range res.Keys[:min(g.TopN, len(res.Keys))] {
		if n, ok := g.Nodes[k]; ok {
			ev = append(ev, n)
		}
	}
	p, err := g.Checker.EvidenceSufficiency(ctx, q.Text, ev)
	if err != nil {
		return Result{}, fmt.Errorf("retrieval: sufficiency check: %w", err)
	}
	res.Judged++
	res.Sufficiency = p
	res.Abstained = p < g.Threshold
	return res, nil
}
