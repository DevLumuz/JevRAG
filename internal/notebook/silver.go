package notebook

import (
	"strings"

	"jev/internal/retrieval"
)

// Fact is one entry of the evidence notebook: a sentence copied verbatim from
// a passage, with where it came from.
type Fact struct {
	Text   string `json:"text"`
	Source string `json:"source"` // passage title / identifier
}

// Silver labels how a silver sentence was found.
type Silver int

const (
	SilverNone    Silver = iota // no usable sentence
	SilverPartial               // best overlap with the answer and the sub-question
	SilverExact                 // the sentence contains the step's answer
)

// SilverSentence picks the sentence of a gold passage that states a step's
// answer: one containing the whole answer (ties broken by overlap with the
// sub-question, then position); otherwise the sentence covering most of the
// answer's words, then the sub-question's. It returns the index and how it
// was found.
func SilverSentence(sentences []string, answer, subQuestion string) (int, Silver) {
	ans := retrieval.Tokenize(answer)
	if len(ans) == 0 || len(sentences) == 0 {
		return -1, SilverNone
	}
	needle := " " + strings.Join(ans, " ") + " "
	subq := retrieval.Tokenize(subQuestion)

	best, bestScore, kind := -1, -1.0, SilverNone
	for i, s := range sentences {
		toks := retrieval.Tokenize(s)
		set := make(map[string]bool, len(toks))
		for _, t := range toks {
			set[t] = true
		}
		score := overlap(subq, set)
		k := SilverPartial
		if strings.Contains(" "+strings.Join(toks, " ")+" ", needle) {
			k = SilverExact
		} else {
			score += 2 * overlap(ans, set)
		}
		switch {
		case k > kind, k == kind && score > bestScore:
			best, bestScore, kind = i, score, k
		}
	}
	if kind == SilverPartial && bestScore <= 0 {
		return -1, SilverNone
	}
	return best, kind
}

// overlap is the share of tokens found in set.
func overlap(tokens []string, set map[string]bool) float64 {
	if len(tokens) == 0 {
		return 0
	}
	n := 0
	for _, t := range tokens {
		if set[t] {
			n++
		}
	}
	return float64(n) / float64(len(tokens))
}
