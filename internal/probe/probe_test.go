package probe_test

import (
	"context"
	"math"
	"path/filepath"
	"strings"
	"testing"

	"jev/internal/jev"
	"jev/internal/judge"
	"jev/internal/probe"
)

func TestAUC(t *testing.T) {
	tests := []struct {
		pos, neg []float64
		want     float64
	}{
		{[]float64{0.9, 0.8}, []float64{0.1, 0.2}, 1},
		{[]float64{0.1}, []float64{0.9}, 0},
		{[]float64{0.5}, []float64{0.5}, 0.5},
		{[]float64{0.9, 0.3}, []float64{0.5, 0.1}, 0.75},
	}
	for _, tt := range tests {
		if got := probe.AUC(tt.pos, tt.neg); math.Abs(got-tt.want) > 1e-9 {
			t.Errorf("AUC(%v, %v) = %v, want %v", tt.pos, tt.neg, got, tt.want)
		}
	}
	if !math.IsNaN(probe.AUC(nil, []float64{1})) {
		t.Error("AUC with no positives must be NaN")
	}
}

func TestFitLogistic(t *testing.T) {
	// Feature a predicts the label; feature b is noise.
	var x [][]float64
	var y []float64
	for i := 0; i < 200; i++ {
		lab := float64(i % 2)
		x = append(x, []float64{lab*0.8 + 0.1*float64(i%5)/5, float64((i*7)%11) / 11})
		y = append(y, lab)
	}
	m := probe.FitLogistic([]string{"a", "b"}, x, y)
	if math.Abs(m.Weights[0]) <= math.Abs(m.Weights[1]) {
		t.Errorf("weights = %v, want a to dominate", m.Weights)
	}
	if m.Score([]float64{0.9, 0.5}) <= m.Score([]float64{0.1, 0.5}) {
		t.Error("higher a must score higher")
	}
}

func TestRun_CachesResponses(t *testing.T) {
	set := &judge.QuestionSet{Name: "s", Questions: map[string]jev.Question{
		"answers": jev.Noul("The `passage` answers `query`."),
		"role":    jev.Choice("Role?", []jev.Option{{Label: "final"}, {Label: "other"}}),
	}}
	client := &jev.FakeClient{Respond: func(state any, _ map[string]jev.Question) (*jev.Response, error) {
		st := state.(probe.State)
		p := 0.2
		if strings.Contains(st.Passage.Text, "gold") {
			p = 0.9
		}
		return &jev.Response{Answers: map[string]jev.Answer{
			"answers": {Type: "noul", Noul: p},
			"role":    {Type: "choice", Choice: "final", Probabilities: map[string]float64{"final": p, "other": 1 - p}},
		}, Usage: jev.Usage{InputTokens: 100}}, nil
	}}
	pairs := []probe.Pair{
		{QueryID: "q1", Query: "q", Text: "gold text", Role: probe.RoleFinal},
		{QueryID: "q1", Query: "q", Text: "other", Role: probe.RoleHardNegative},
	}
	cachePath := filepath.Join(t.TempDir(), "c.jsonl")
	cache, err := probe.OpenCache(cachePath)
	if err != nil {
		t.Fatal(err)
	}
	feats, u, err := probe.Run(context.Background(), client, "m", set, pairs, cache, 2, 0)
	if err != nil {
		t.Fatal(err)
	}
	if feats[0]["answers"] != 0.9 || feats[1]["role=final"] != 0.2 || u.Calls != 2 || u.InputTokens != 200 {
		t.Errorf("feats = %v, usage = %+v", feats, u)
	}

	// Second run: everything from cache, also after reopening the file.
	cache2, err := probe.OpenCache(cachePath)
	if err != nil {
		t.Fatal(err)
	}
	_, u2, err := probe.Run(context.Background(), client, "m", set, pairs, cache2, 2, 0)
	if err != nil || u2.Calls != 0 || u2.CacheHits != 2 {
		t.Errorf("cached run: usage = %+v, err = %v", u2, err)
	}
	if len(client.Calls) != 2 {
		t.Errorf("client calls = %d, want 2", len(client.Calls))
	}
}

func TestStateFor_Truncates(t *testing.T) {
	long := strings.Repeat("é", probe.MaxPassageChars+10)
	st := probe.StateFor(probe.Pair{Query: "q", Source: "s", Text: long})
	if n := len([]rune(st.Passage.Text)); n != probe.MaxPassageChars {
		t.Errorf("len = %d", n)
	}
}
