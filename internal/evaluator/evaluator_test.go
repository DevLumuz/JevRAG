package evaluator_test

import (
	"testing"

	"jev/internal/evaluator"
)

// --- Recall@K ---

func TestRecallAtK(t *testing.T) {
	tests := []struct {
		name     string
		expected []string
		got      []string
		k        int
		want     float64
	}{
		{"all found", []string{"art_40", "art_41"}, []string{"art_40", "art_41", "art_50"}, 3, 1.0},
		{"none found", []string{"art_40"}, []string{"art_99"}, 1, 0.0},
		{"partial", []string{"art_40", "art_41"}, []string{"art_40", "art_99"}, 2, 0.5},
		{"K smaller than results", []string{"art_40"}, []string{"art_99", "art_40"}, 1, 0.0},
		{"K larger than results", []string{"art_40"}, []string{"art_40"}, 5, 1.0},
		{"empty expected", []string{}, []string{"art_40"}, 3, 1.0}, // vacuous truth
		{"empty got", []string{"art_40"}, []string{}, 3, 0.0},
		{"duplicates in got", []string{"art_40"}, []string{"art_40", "art_40"}, 2, 1.0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := evaluator.RecallAtK(tt.expected, tt.got, tt.k)
			if got != tt.want {
				t.Errorf("RecallAtK() = %v, want %v", got, tt.want)
			}
		})
	}
}

// --- MRR ---

func TestMRR(t *testing.T) {
	tests := []struct {
		name     string
		queries  []evaluator.RankedResult
		want     float64
	}{
		{
			"first result correct",
			[]evaluator.RankedResult{
				{Expected: []string{"art_40"}, Got: []string{"art_40", "art_41"}},
			},
			1.0,
		},
		{
			"second result correct",
			[]evaluator.RankedResult{
				{Expected: []string{"art_40"}, Got: []string{"art_99", "art_40"}},
			},
			0.5,
		},
		{
			"not found",
			[]evaluator.RankedResult{
				{Expected: []string{"art_40"}, Got: []string{"art_99", "art_98"}},
			},
			0.0,
		},
		{
			"average of two queries",
			[]evaluator.RankedResult{
				{Expected: []string{"art_40"}, Got: []string{"art_40"}},        // RR = 1.0
				{Expected: []string{"art_41"}, Got: []string{"x", "art_41"}},   // RR = 0.5
			},
			0.75,
		},
		{
			"multi-hop: any correct counts",
			[]evaluator.RankedResult{
				{Expected: []string{"art_40", "art_41"}, Got: []string{"art_99", "art_41", "art_40"}},
			},
			0.5, // first correct is at position 2 → RR = 1/2
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := evaluator.MRR(tt.queries)
			if got != tt.want {
				t.Errorf("MRR() = %v, want %v", got, tt.want)
			}
		})
	}
}

// --- Abstention accuracy ---

func TestAbstentionAccuracy(t *testing.T) {
	tests := []struct {
		name string
		preds []evaluator.AbstentionPrediction
		want  float64
	}{
		{
			"all correct",
			[]evaluator.AbstentionPrediction{
				{ShouldAbstain: true, DidAbstain: true},
				{ShouldAbstain: false, DidAbstain: false},
			},
			1.0,
		},
		{
			"all wrong",
			[]evaluator.AbstentionPrediction{
				{ShouldAbstain: true, DidAbstain: false},
				{ShouldAbstain: false, DidAbstain: true},
			},
			0.0,
		},
		{
			"mixed",
			[]evaluator.AbstentionPrediction{
				{ShouldAbstain: true, DidAbstain: true},
				{ShouldAbstain: true, DidAbstain: false},
				{ShouldAbstain: false, DidAbstain: false},
				{ShouldAbstain: false, DidAbstain: true},
			},
			0.5,
		},
		{
			"empty",
			[]evaluator.AbstentionPrediction{},
			1.0, // vacuous truth
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := evaluator.AbstentionAccuracy(tt.preds)
			if got != tt.want {
				t.Errorf("AbstentionAccuracy() = %v, want %v", got, tt.want)
			}
		})
	}
}

// --- Cosine similarity ---

func TestCosineSimilarity(t *testing.T) {
	tests := []struct {
		name string
		a, b []float64
		want float64
	}{
		{"identical", []float64{1, 0, 0}, []float64{1, 0, 0}, 1.0},
		{"orthogonal", []float64{1, 0}, []float64{0, 1}, 0.0},
		{"opposite", []float64{1, 0}, []float64{-1, 0}, -1.0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := evaluator.CosineSimilarity(tt.a, tt.b)
			if diff := got - tt.want; diff > 1e-9 || diff < -1e-9 {
				t.Errorf("CosineSimilarity() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCosineSimilarity_ZeroVector(t *testing.T) {
	got := evaluator.CosineSimilarity([]float64{0, 0}, []float64{1, 0})
	if got != 0.0 {
		t.Errorf("expected 0 for zero vector, got %v", got)
	}
}

// --- TopK ---

func TestTopKBySimilarity(t *testing.T) {
	query := []float64{1, 0, 0}
	candidates := []evaluator.Candidate{
		{Key: "a", Embedding: []float64{0, 1, 0}},  // orthogonal
		{Key: "b", Embedding: []float64{1, 0, 0}},  // identical
		{Key: "c", Embedding: []float64{0.9, 0.1, 0}}, // close
	}

	top := evaluator.TopKBySimilarity(query, candidates, 2)
	if len(top) != 2 {
		t.Fatalf("len = %d, want 2", len(top))
	}
	if top[0].Key != "b" {
		t.Errorf("top[0] = %q, want b (identical)", top[0].Key)
	}
	if top[1].Key != "c" {
		t.Errorf("top[1] = %q, want c (close)", top[1].Key)
	}
}
