package judge

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync/atomic"

	"jev/internal/graphmodel"
)

// Generator produces a JSON answer for a prompt, constrained by a JSON
// schema. It reports billable tokens: input, and output including any hidden
// reasoning ("thinking") tokens.
type Generator interface {
	GenerateJSON(ctx context.Context, prompt string, schema map[string]any) (text string, inputTokens, outputTokens int64, err error)
}

// LLMJudge implements Judge with a generative LLM (option 4), the role the
// LLM plays in HippoRAG 2 / CatRAG. It sees exactly what JEVJudge sees (same
// state, same tier definitions, same sufficiency and contradiction checks) so
// that options 4 and 5 differ only in who judges.
type LLMJudge struct {
	gen Generator

	// MaxInputTokens caps billable input tokens; 0 = no cap.
	MaxInputTokens int64

	inputTokens  atomic.Int64
	outputTokens atomic.Int64
}

// NewLLMJudge creates an LLMJudge.
func NewLLMJudge(gen Generator) *LLMJudge { return &LLMJudge{gen: gen} }

var llmSchema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"relevance":   map[string]any{"type": "string", "enum": []string{"irrelevant", "weak", "high", "direct"}},
		"sufficient":  map[string]any{"type": "boolean"},
		"contradicts": map[string]any{"type": "boolean"},
	},
	"required":         []string{"relevance", "sufficient", "contradicts"},
	"propertyOrdering": []string{"relevance", "sufficient", "contradicts"},
}

type llmAnswer struct {
	Relevance   string `json:"relevance"`
	Sufficient  bool   `json:"sufficient"`
	Contradicts bool   `json:"contradicts"`
}

// ScoreEdge asks the LLM for relevance, sufficiency and contradiction in one
// structured call. Safe for concurrent use.
func (j *LLMJudge) ScoreEdge(ctx context.Context, edge graphmodel.Edge, query string, confirmed []*graphmodel.Node) (Decision, error) {
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

	text, in, out, err := j.gen.GenerateJSON(ctx, llmPrompt(state), llmSchema)
	j.inputTokens.Add(in)
	j.outputTokens.Add(out)
	if err != nil {
		return Decision{}, fmt.Errorf("judge: llm: %w", err)
	}

	var a llmAnswer
	if err := json.Unmarshal([]byte(text), &a); err != nil {
		return Decision{}, fmt.Errorf("judge: llm answer %q: %w", text, err)
	}
	tier, err := ParseTier(a.Relevance)
	if err != nil {
		return Decision{}, err
	}
	// A generative model gives no calibrated confidence; report 1.
	d := Decision{Tier: tier, Confidence: 1, Sufficient: a.Sufficient}
	if a.Contradicts && len(confirmed) > 0 { // same contradiction rule as JEVJudge
		d.Tier, d.Sufficient = Irrelevant, false
	}
	return d, nil
}

// Tokens returns the billable input and output tokens used so far.
func (j *LLMJudge) Tokens() (input, output int64) {
	return j.inputTokens.Load(), j.outputTokens.Load()
}

func llmPrompt(s EdgeState) string {
	st, _ := json.MarshalIndent(s, "", "  ")
	var b strings.Builder
	b.WriteString("You judge evidence for a retrieval system. Read the state and answer three questions.\n\n")
	b.WriteString("STATE (JSON):\n")
	b.Write(st)
	b.WriteString("\n\n1. relevance — " + relevanceInstructions + "\n")
	for _, o := range relevanceOptions {
		fmt.Fprintf(&b, "   - %s: %s\n", o.Label, o.Description)
	}
	b.WriteString("2. sufficient — true if: " + sufficientInstructions + "\n")
	b.WriteString("3. contradicts — true if: " + contradictsInstructions + " (false when `confirmed` is empty)\n")
	b.WriteString("\nAnswer only with the JSON object.")
	return b.String()
}
