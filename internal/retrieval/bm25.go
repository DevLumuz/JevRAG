package retrieval

import (
	"math"
	"sort"
	"strings"
	"unicode"

	"jev/internal/graphmodel"
)

// BM25 is a keyword index (Okapi BM25) over node title + content. It finds
// exact terms — names, codes, article numbers — that embedding search can
// miss. Domain-agnostic: lowercase word tokens, no stemming, no stop list.
type BM25 struct {
	nodes    []*graphmodel.Node
	postings map[string][]posting
	docLen   []float64
	avgLen   float64
	k1, b    float64
}

type posting struct {
	doc int
	tf  float64
}

// Tokenize splits text into lowercase letter/digit tokens.
func Tokenize(s string) []string {
	return strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
}

// NewBM25 indexes nodes. text returns the text indexed for a node (e.g.
// title + content).
func NewBM25(nodes []*graphmodel.Node, text func(*graphmodel.Node) string) *BM25 {
	ix := &BM25{nodes: nodes, postings: map[string][]posting{}, docLen: make([]float64, len(nodes)), k1: 1.2, b: 0.75}
	total := 0.0
	for i, n := range nodes {
		tf := map[string]float64{}
		toks := Tokenize(text(n))
		for _, t := range toks {
			tf[t]++
		}
		for t, c := range tf {
			ix.postings[t] = append(ix.postings[t], posting{doc: i, tf: c})
		}
		ix.docLen[i] = float64(len(toks))
		total += ix.docLen[i]
	}
	if len(nodes) > 0 {
		ix.avgLen = total / float64(len(nodes))
	}
	return ix
}

// Search returns the top k nodes for the query, best first.
func (ix *BM25) Search(query string, k int) []Scored {
	scores := map[int]float64{}
	n := float64(len(ix.nodes))
	seen := map[string]bool{}
	for _, t := range Tokenize(query) {
		if seen[t] {
			continue
		}
		seen[t] = true
		ps := ix.postings[t]
		if len(ps) == 0 {
			continue
		}
		idf := math.Log(1 + (n-float64(len(ps))+0.5)/(float64(len(ps))+0.5))
		for _, p := range ps {
			norm := p.tf * (ix.k1 + 1) / (p.tf + ix.k1*(1-ix.b+ix.b*ix.docLen[p.doc]/ix.avgLen))
			scores[p.doc] += idf * norm
		}
	}
	out := make([]Scored, 0, len(scores))
	for d, s := range scores {
		out = append(out, Scored{Node: ix.nodes[d], Score: s})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		return out[i].Node.Key < out[j].Node.Key
	})
	if k < len(out) {
		out = out[:k]
	}
	return out
}

// FuseRRF merges ranked lists with reciprocal rank fusion (score = Σ 1/(c+rank)),
// the standard way to combine keyword and embedding results without tuning.
func FuseRRF(c float64, k int, lists ...[]Scored) []Scored {
	score := map[*graphmodel.Node]float64{}
	for _, l := range lists {
		for r, s := range l {
			score[s.Node] += 1 / (c + float64(r+1))
		}
	}
	out := make([]Scored, 0, len(score))
	for n, s := range score {
		out = append(out, Scored{Node: n, Score: s})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		return out[i].Node.Key < out[j].Node.Key
	})
	if k < len(out) {
		out = out[:k]
	}
	return out
}
