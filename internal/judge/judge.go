// Package judge defines how a retrieval option decides whether a connection
// (edge) is relevant to the current query. The same Judge interface drives
// options 2, 4 and 5 of the harness; only the implementation changes.
//
// A Judge scores a CONNECTION, not an isolated node: where the traversal comes
// from, the candidate it could go to, the query, and what was already
// confirmed — the design CatRAG validated on top of HippoRAG 2.
package judge

import (
	"context"
	"fmt"

	"jev/internal/graphmodel"
)

// RelevanceTier is the discrete level a Judge assigns to a connection.
type RelevanceTier int

const (
	Irrelevant RelevanceTier = iota // contributes nothing to this query
	Weak                            // valid but tangential link
	High                            // critical step in the reasoning chain
	Direct                          // the target node IS or contains the answer
)

var tierNames = [...]string{"irrelevant", "weak", "high", "direct"}

// String returns the lowercase label used on the wire (e.g. as a JEV choice).
func (t RelevanceTier) String() string {
	if t < 0 || int(t) >= len(tierNames) {
		return fmt.Sprintf("tier(%d)", int(t))
	}
	return tierNames[t]
}

// ParseTier converts a label produced by String back into a tier.
func ParseTier(s string) (RelevanceTier, error) {
	for i, name := range tierNames {
		if s == name {
			return RelevanceTier(i), nil
		}
	}
	return Irrelevant, fmt.Errorf("judge: unknown relevance tier %q", s)
}

// Multiplier maps the tier to a factor over the edge's static Weight.
// Suppresses the irrelevant, amplifies the direct — CatRAG's non-linear mapping.
func (t RelevanceTier) Multiplier() float64 {
	switch t {
	case Irrelevant:
		return 0
	case Weak:
		return 0.25
	case High:
		return 2.5
	case Direct:
		return 5.0
	}
	return 0
}

// Decision is the result of judging one connection against the query.
type Decision struct {
	Tier       RelevanceTier
	Confidence float64 // 0-1, as reported by the judge
	Sufficient bool    // together with what was confirmed, this is enough evidence
}

// Judge scores a connection: edge.F (may be nil when there is no graph, as in
// option 2) → edge.T, against the query and the nodes confirmed so far.
// Option 3 (pure PPR) does not use a Judge — it is a different algorithm.
type Judge interface {
	ScoreEdge(ctx context.Context, edge graphmodel.Edge, query string, confirmed []*graphmodel.Node) (Decision, error)
}

// EffectiveWeight combines the static strength of the connection (fixed, from
// the graph) with the judge's verdict for this query (dynamic).
func EffectiveWeight(edge graphmodel.Edge, d Decision) float64 {
	return edge.Weight * d.Tier.Multiplier()
}

// FakeJudge returns fixed decisions keyed by the target node's Key, for
// deterministic tests. Unknown keys get the zero Decision (Irrelevant).
type FakeJudge struct {
	Decisions map[string]Decision
}

// ScoreEdge looks up the decision for edge.T.Key.
func (f *FakeJudge) ScoreEdge(_ context.Context, edge graphmodel.Edge, _ string, _ []*graphmodel.Node) (Decision, error) {
	return f.Decisions[edge.T.Key], nil
}
