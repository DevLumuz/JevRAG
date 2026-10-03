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

// relevanceOptions are listed from lowest to highest tier, in that order.
var relevanceOptions = []jev.Option{
	{Label: Irrelevant.String(), Description: "The candidate adds nothing to answering the query, or contradicts the confirmed evidence."},
	{Label: Weak.String(), Description: "The candidate is related to the topic but only tangential to this query."},
	{Label: High.String(), Description: "The candidate is a necessary step in the reasoning chain toward the answer."},
	{Label: Direct.String(), Description: "The candidate itself states or contains the answer to the query."},
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
		qRelevance:  jev.Choice(relevanceInstructions, relevanceOptions),
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

const evidenceInstructions = "The passages in `evidence`, taken together, contain all the information " +
	"needed to answer `query`. Answer yes only if nothing essential is missing."

// EvidenceItem is one retrieved passage shown to the sufficiency check.
type EvidenceItem struct {
	ID   string `json:"id"`
	Text string `json:"text"`
}

// EvidenceState is the state for the final sufficiency check.
type EvidenceState struct {
	Query    string         `json:"query"`
	Evidence []EvidenceItem `json:"evidence"`
}

// EvidenceSufficiency asks JEV, in one Noul call, how likely it is that the
// retrieved passages together are enough to answer the query. It drives
// abstention ("not enough evidence") for any retrieval option.
func (j *JEVJudge) EvidenceSufficiency(ctx context.Context, query string, evidence []*graphmodel.Node) (float64, error) {
	if j.MaxInputTokens > 0 && j.inputTokens.Load() >= j.MaxInputTokens {
		return 0, ErrBudgetExceeded
	}
	st := EvidenceState{Query: query}
	for _, n := range evidence {
		st.Evidence = append(st.Evidence, EvidenceItem{ID: n.Key, Text: n.Content})
	}
	resp, err := j.client.SystemOne(ctx, st, map[string]jev.Question{qSufficient: jev.Noul(evidenceInstructions)})
	if err != nil {
		return 0, fmt.Errorf("judge: jev: %w", err)
	}
	j.inputTokens.Add(int64(resp.Usage.InputTokens))
	j.outputTokens.Add(int64(resp.Usage.OutputTokens))
	a, ok := resp.Answers[qSufficient]
	if !ok {
		return 0, fmt.Errorf("judge: jev response missing %q answer", qSufficient)
	}
	return a.Noul, nil
}
