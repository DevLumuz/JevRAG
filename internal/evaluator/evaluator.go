// Package evaluator implements the metrics for comparing retrieval options:
// Recall@K, MRR, abstention accuracy, cosine similarity, and top-K ranking.
// All functions are pure — no API calls, no side effects.
package evaluator

import (
	"math"
	"sort"
)

// --- Recall@K ---

// RecallAtK computes what fraction of expected items appear in the first K
// positions of got. Returns 1.0 if expected is empty (vacuous truth).
func RecallAtK(expected, got []string, k int) float64 {
	if len(expected) == 0 {
		return 1.0
	}

	// Build set of what we got in the first K positions.
	topK := make(map[string]bool, k)
	for i := 0; i < k && i < len(got); i++ {
		topK[got[i]] = true
	}

	found := 0
	for _, e := range expected {
		if topK[e] {
			found++
		}
	}
	return float64(found) / float64(len(expected))
}

// --- MRR (Mean Reciprocal Rank) ---

// RankedResult holds one query's retrieval result for MRR computation.
type RankedResult struct {
	Expected []string // gold set: any of these counts as correct
	Got      []string // ranked list of retrieved keys
}

// MRR computes the mean reciprocal rank across all queries.
// For each query, the reciprocal rank is 1/(position of first correct result).
// If no correct result is found, RR = 0.
func MRR(queries []RankedResult) float64 {
	if len(queries) == 0 {
		return 0
	}

	sum := 0.0
	for _, q := range queries {
		gold := make(map[string]bool, len(q.Expected))
		for _, e := range q.Expected {
			gold[e] = true
		}
		for i, g := range q.Got {
			if gold[g] {
				sum += 1.0 / float64(i+1)
				break
			}
		}
	}
	return sum / float64(len(queries))
}

// --- Abstention ---

// AbstentionPrediction holds one prediction for abstention accuracy.
type AbstentionPrediction struct {
	ShouldAbstain bool // ground truth: was evidence insufficient?
	DidAbstain    bool // system prediction: did the system abstain?
}

// AbstentionAccuracy computes the fraction of correct abstention decisions.
// Returns 1.0 if preds is empty (vacuous truth).
func AbstentionAccuracy(preds []AbstentionPrediction) float64 {
	if len(preds) == 0 {
		return 1.0
	}

	correct := 0
	for _, p := range preds {
		if p.ShouldAbstain == p.DidAbstain {
			correct++
		}
	}
	return float64(correct) / float64(len(preds))
}

// --- Cosine similarity ---

// CosineSimilarity computes the cosine similarity between two vectors.
// Returns 0 if either vector has zero magnitude.
func CosineSimilarity(a, b []float64) float64 {
	if len(a) != len(b) {
		return 0
	}

	var dot, magA, magB float64
	for i := range a {
		dot += a[i] * b[i]
		magA += a[i] * a[i]
		magB += b[i] * b[i]
	}

	denom := math.Sqrt(magA) * math.Sqrt(magB)
	if denom == 0 {
		return 0
	}
	return dot / denom
}

// --- Top-K retrieval ---

// Candidate is a node with a precomputed embedding, ready for similarity search.
type Candidate struct {
	Key       string
	Embedding []float64
}

// ScoredCandidate pairs a candidate with its similarity score.
type ScoredCandidate struct {
	Key   string
	Score float64
}

// TopKBySimilarity returns the K candidates most similar to the query embedding,
// sorted by descending similarity.
func TopKBySimilarity(query []float64, candidates []Candidate, k int) []ScoredCandidate {
	scored := make([]ScoredCandidate, len(candidates))
	for i, c := range candidates {
		scored[i] = ScoredCandidate{
			Key:   c.Key,
			Score: CosineSimilarity(query, c.Embedding),
		}
	}

	sort.Slice(scored, func(i, j int) bool {
		return scored[i].Score > scored[j].Score
	})

	if k > len(scored) {
		k = len(scored)
	}
	return scored[:k]
}
