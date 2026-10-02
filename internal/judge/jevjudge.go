package judge

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"

	"jev/internal/graphmodel"
	"jev/internal/jev"
)

// Question names sent to JEV. They are only for code; JEV never sees them.
const (
	qRelevance   = "relevance"
	qSufficient  = "sufficient"
	qContradicts = "contradicts"
)

const relevanceInstructions = "How relevant is the connection from `from` to `candidate` " +
	"for answering `query`, given the evidence already in `confirmed`? " +
	"Judge the candidate's content, not how many links it has."

var relevanceCriteria = map[string]string{
	Irrelevant.String(): "The candidate adds nothing to answering the query, or contradicts the confirmed evidence.",
	Weak.String():       "The candidate is related to the topic but only tangential to this query.",
	High.String():       "The candidate is a necessary step in the reasoning chain toward the answer.",
	Direct.String():     "The candidate itself states or contains the answer to the query.",
}

const sufficientInstructions = "The `candidate` together with the `confirmed` evidence is enough " +
	"to fully answer `query` without looking for more provisions."

const contradictsInstructions = "The `candidate` contradicts at least one item in `confirmed`."

// ErrBudgetExceeded is returned once the judge has used its token budget.
var ErrBudgetExceeded = errors.New("judge: JEV input token budget exceeded")

// EdgeState is the state JEV reads for one ScoreEdge call. Field names are
// referenced by the instructions above with backticks.
//
// The IDs are the nodes' Keys: for legal articles they name the act and the
// article ("CIVIL ACT / Article. 40 / Capacity"), which the body text alone
// almost never states.
type EdgeState struct {
	Query       string   `json:"query"`
	FromID      string   `json:"from_id,omitempty"`
	From        string   `json:"from,omitempty"`
	CandidateID string   `json:"candidate_id"`
	Candidate   string   `json:"candidate"`
	Confirmed   []string `json:"confirmed,omitempty"`
}

// JEVJudge implements Judge with one JEV request per connection. All
// questions for a connection go in the same request, as TypeSafe recommends:
// they run in parallel and the contradiction rule is applied in code.
type JEVJudge struct {
	client jev.Client

	// SufficientThreshold: Noul probability at or above which the evidence
	// counts as sufficient. ContradictionThreshold: probability at or above
	// which the candidate is forced to Irrelevant. Tune both on the dev split.
	SufficientThreshold    float64
	ContradictionThreshold float64

	// MaxInputTokens caps billable input tokens across all calls; once
	// reached, ScoreEdge returns ErrBudgetExceeded. 0 means no cap. Calls
	// already in flight can overshoot the cap by one request each.
	MaxInputTokens int64

	inputTokens  atomic.Int64
	outputTokens atomic.Int64
}

// NewJEVJudge creates a JEVJudge with 0.5 thresholds.
func NewJEVJudge(client jev.Client) *JEVJudge {
	return &JEVJudge{client: client, SufficientThreshold: 0.5, ContradictionThreshold: 0.5}
}

// ScoreEdge asks JEV for the relevance tier, sufficiency and, when something
// is already confirmed, contradiction. Safe for concurrent use.
func (j *JEVJudge) ScoreEdge(ctx context.Context, edge graphmodel.Edge, query string, confirmed []*graphmodel.Node) (Decision, error) {
	if j.MaxInputTokens > 0 && j.inputTokens.Load() >= j.MaxInputTokens {
		return Decision{}, ErrBudgetExceeded
	}
	state := EdgeState{Query: query, CandidateID: edge.T.Key, Candidate: edge.T.Content}
	if edge.F != nil {
		state.FromID, state.From = edge.F.Key, edge.F.Content
	}
	for _, n := range confirmed {
		state.Confirmed = append(state.Confirmed, n.Content)
	}

	questions := map[string]jev.Question{
		qRelevance:  jev.Choice(relevanceInstructions, relevanceCriteria),
		qSufficient: jev.Noul(sufficientInstructions),
	}
	if len(confirmed) > 0 {
		questions[qContradicts] = jev.Noul(contradictsInstructions)
	}

	resp, err := j.client.SystemOne(ctx, state, questions)
	if err != nil {
		return Decision{}, fmt.Errorf("judge: jev: %w", err)
	}
	j.inputTokens.Add(int64(resp.Usage.InputTokens))
	j.outputTokens.Add(int64(resp.Usage.OutputTokens))

	rel, ok := resp.Answers[qRelevance]
	if !ok {
		return Decision{}, fmt.Errorf("judge: jev response missing %q answer", qRelevance)
	}
	tier, err := ParseTier(rel.Choice)
	if err != nil {
		return Decision{}, err
	}

	d := Decision{
		Tier:       tier,
		Confidence: rel.Confidence,
		Sufficient: resp.Answers[qSufficient].Noul >= j.SufficientThreshold,
	}
	// Contradiction rule (from CatRAG): a candidate that contradicts confirmed
	// evidence is irrelevant regardless of its static weight or tier.
	if c, asked := resp.Answers[qContradicts]; asked && c.Noul >= j.ContradictionThreshold {
		d.Tier = Irrelevant
		d.Sufficient = false
	}
	return d, nil
}

// Tokens returns the billable input and output tokens used so far.
func (j *JEVJudge) Tokens() (input, output int64) {
	return j.inputTokens.Load(), j.outputTokens.Load()
}
