package judge_test

import (
	"context"
	"errors"
	"testing"

	"jev/internal/graphmodel"
	"jev/internal/jev"
	"jev/internal/judge"
)

func TestMultiplier(t *testing.T) {
	tests := []struct {
		tier judge.RelevanceTier
		want float64
	}{
		{judge.Irrelevant, 0},
		{judge.Weak, 0.25},
		{judge.High, 2.5},
		{judge.Direct, 5.0},
		{judge.RelevanceTier(99), 0},
	}
	for _, tt := range tests {
		if got := tt.tier.Multiplier(); got != tt.want {
			t.Errorf("%v.Multiplier() = %v, want %v", tt.tier, got, tt.want)
		}
	}
}

func TestEffectiveWeight(t *testing.T) {
	e := graphmodel.Edge{Weight: 0.8}
	if got := judge.EffectiveWeight(e, judge.Decision{Tier: judge.High}); got != 2.0 {
		t.Errorf("EffectiveWeight() = %v, want 2.0", got)
	}
	if got := judge.EffectiveWeight(e, judge.Decision{Tier: judge.Irrelevant}); got != 0 {
		t.Errorf("EffectiveWeight(irrelevant) = %v, want 0", got)
	}
}

func TestParseTier(t *testing.T) {
	for _, tier := range []judge.RelevanceTier{judge.Irrelevant, judge.Weak, judge.High, judge.Direct} {
		got, err := judge.ParseTier(tier.String())
		if err != nil || got != tier {
			t.Errorf("ParseTier(%q) = %v, %v", tier.String(), got, err)
		}
	}
	if _, err := judge.ParseTier("bogus"); err == nil {
		t.Error("ParseTier(bogus) expected error")
	}
}

func TestFakeJudge(t *testing.T) {
	f := &judge.FakeJudge{Decisions: map[string]judge.Decision{"b": {Tier: judge.Direct}}}
	d, err := f.ScoreEdge(context.Background(), graphmodel.Edge{T: &graphmodel.Node{Key: "b"}}, "q", nil)
	if err != nil || d.Tier != judge.Direct {
		t.Errorf("FakeJudge known key = %+v, %v", d, err)
	}
	d, _ = f.ScoreEdge(context.Background(), graphmodel.Edge{T: &graphmodel.Node{Key: "zzz"}}, "q", nil)
	if d.Tier != judge.Irrelevant {
		t.Errorf("FakeJudge unknown key tier = %v, want Irrelevant", d.Tier)
	}
}

// answers builds a fake JEV response for the JEVJudge question names.
func answers(tier string, conf, sufficient float64, contradicts *float64) func(any, map[string]jev.Question) (*jev.Response, error) {
	return func(_ any, qs map[string]jev.Question) (*jev.Response, error) {
		a := map[string]jev.Answer{
			"relevance":  {Type: "choice", Choice: tier, Confidence: conf},
			"sufficient": {Type: "noul", Noul: sufficient},
		}
		if _, asked := qs["contradicts"]; asked && contradicts != nil {
			a["contradicts"] = jev.Answer{Type: "noul", Noul: *contradicts}
		}
		return &jev.Response{Answers: a, Usage: jev.Usage{InputTokens: 100, OutputTokens: 5}}, nil
	}
}

func ptr(f float64) *float64 { return &f }

func TestJEVJudge_ScoreEdge(t *testing.T) {
	from := &graphmodel.Node{Key: "a", Content: "Article 1 text"}
	to := &graphmodel.Node{Key: "b", Content: "Article 2 text"}
	confirmed := []*graphmodel.Node{{Key: "c", Content: "confirmed text"}}

	tests := []struct {
		name           string
		respond        func(any, map[string]jev.Question) (*jev.Response, error)
		confirmed      []*graphmodel.Node
		wantTier       judge.RelevanceTier
		wantSufficient bool
		wantConf       float64
	}{
		{"high and sufficient", answers("high", 0.9, 0.8, nil), nil, judge.High, true, 0.9},
		{"direct, insufficient", answers("direct", 0.7, 0.2, nil), nil, judge.Direct, false, 0.7},
		{"contradiction overrides tier", answers("direct", 0.95, 0.9, ptr(0.85)), confirmed, judge.Irrelevant, false, 0.95},
		{"no contradiction keeps tier", answers("weak", 0.6, 0.1, ptr(0.1)), confirmed, judge.Weak, false, 0.6},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &jev.FakeClient{Respond: tt.respond}
			j := judge.NewJEVJudge(client)

			d, err := j.ScoreEdge(context.Background(), graphmodel.Edge{F: from, T: to, Weight: 1}, "the query", tt.confirmed)
			if err != nil {
				t.Fatalf("ScoreEdge() error: %v", err)
			}
			if d.Tier != tt.wantTier || d.Sufficient != tt.wantSufficient || d.Confidence != tt.wantConf {
				t.Errorf("Decision = %+v, want tier=%v sufficient=%v conf=%v", d, tt.wantTier, tt.wantSufficient, tt.wantConf)
			}
		})
	}
}

func TestJEVJudge_AsksContradictionOnlyWithConfirmed(t *testing.T) {
	client := &jev.FakeClient{Respond: answers("weak", 0.5, 0.1, ptr(0))}
	j := judge.NewJEVJudge(client)
	to := &graphmodel.Node{Key: "b", Content: "x"}

	_, _ = j.ScoreEdge(context.Background(), graphmodel.Edge{T: to}, "q", nil)
	_, _ = j.ScoreEdge(context.Background(), graphmodel.Edge{T: to}, "q", []*graphmodel.Node{{Content: "c"}})

	if _, ok := client.Calls[0].Questions["contradicts"]; ok {
		t.Error("first call should not ask contradicts (nothing confirmed)")
	}
	if _, ok := client.Calls[1].Questions["contradicts"]; !ok {
		t.Error("second call should ask contradicts")
	}
	if _, ok := client.Calls[0].Questions["relevance"].Criteria.(map[string]any)["direct"]; !ok {
		t.Error("relevance criteria must include every tier")
	}
}

func TestJEVJudge_StateWithoutSource(t *testing.T) {
	// Option 2 (no graph): the edge has no From node, only the candidate.
	client := &jev.FakeClient{Respond: answers("high", 0.8, 0.6, nil)}
	j := judge.NewJEVJudge(client)

	if _, err := j.ScoreEdge(context.Background(), graphmodel.Edge{T: &graphmodel.Node{Content: "cand"}}, "q", nil); err != nil {
		t.Fatalf("ScoreEdge() error: %v", err)
	}
	st := client.Calls[0].State.(judge.EdgeState)
	if st.From != "" || st.Candidate != "cand" || st.Query != "q" {
		t.Errorf("state = %+v", st)
	}
}

func TestJEVJudge_CountsTokens(t *testing.T) {
	client := &jev.FakeClient{Respond: answers("weak", 0.5, 0.1, nil)}
	j := judge.NewJEVJudge(client)
	for i := 0; i < 3; i++ {
		_, _ = j.ScoreEdge(context.Background(), graphmodel.Edge{T: &graphmodel.Node{}}, "q", nil)
	}
	if in, out := j.Tokens(); in != 300 || out != 15 {
		t.Errorf("Tokens() = %d, %d, want 300, 15", in, out)
	}
}

func TestJEVJudge_Errors(t *testing.T) {
	tests := []struct {
		name    string
		respond func(any, map[string]jev.Question) (*jev.Response, error)
	}{
		{"client error", func(any, map[string]jev.Question) (*jev.Response, error) { return nil, errors.New("boom") }},
		{"unknown tier", answers("maybe", 0.5, 0.5, nil)},
		{"missing answer", func(any, map[string]jev.Question) (*jev.Response, error) {
			return &jev.Response{Answers: map[string]jev.Answer{}}, nil
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			j := judge.NewJEVJudge(&jev.FakeClient{Respond: tt.respond})
			if _, err := j.ScoreEdge(context.Background(), graphmodel.Edge{T: &graphmodel.Node{}}, "q", nil); err == nil {
				t.Error("expected error")
			}
		})
	}
}

func TestJEVJudge_Budget(t *testing.T) {
	client := &jev.FakeClient{Respond: answers("weak", 0.5, 0.1, nil)} // 100 input tokens per call
	j := judge.NewJEVJudge(client)
	j.MaxInputTokens = 250

	var err error
	calls := 0
	for ; calls < 10; calls++ {
		if _, err = j.ScoreEdge(context.Background(), graphmodel.Edge{T: &graphmodel.Node{}}, "q", nil); err != nil {
			break
		}
	}
	if !errors.Is(err, judge.ErrBudgetExceeded) {
		t.Fatalf("err = %v, want ErrBudgetExceeded", err)
	}
	if calls != 3 || len(client.Calls) != 3 {
		t.Errorf("successful calls = %d, client calls = %d; want 3, 3", calls, len(client.Calls))
	}
}
