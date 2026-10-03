// Package inferred builds graph connections for content that does not cite
// itself — a company's procedures, wiki pages, notes. Unlike the legal
// extractor (explicit "Article N" citations), every edge here is inferred and
// marked Origin "inferred". Two cheap, LLM-free methods:
//
//   - MentionEdges: node A mentions node B's title (an entity-linking proxy).
//   - SimilarityEdges: each node links to its k most similar nodes.
package inferred

import (
	"regexp"
	"sort"
	"strings"

	"jev/internal/evaluator"
	"jev/internal/graphmodel"
)

const minTitleLen = 4

// MentionEdges links A → B when A's content mentions B's title (from the
// "title" property) as a whole phrase, case-insensitively. Nodes sharing a
// title don't link to each other, and titles shorter than 4 characters are
// ignored (too ambiguous).
func MentionEdges(nodes []*graphmodel.Node) []graphmodel.Edge {
	byTitle := map[string][]*graphmodel.Node{}
	for _, n := range nodes {
		t := strings.ToLower(strings.TrimSpace(n.Properties["title"]))
		if len(t) >= minTitleLen {
			byTitle[t] = append(byTitle[t], n)
		}
	}
	titles := make([]string, 0, len(byTitle))
	for t := range byTitle {
		titles = append(titles, t)
	}
	sort.Strings(titles)

	lower := make([]string, len(nodes))
	for i, n := range nodes {
		lower[i] = strings.ToLower(n.Content)
	}

	var edges []graphmodel.Edge
	for _, t := range titles {
		re := regexp.MustCompile(`(^|[^\pL\pN])` + regexp.QuoteMeta(t) + `($|[^\pL\pN])`)
		for i, n := range nodes {
			if strings.EqualFold(strings.TrimSpace(n.Properties["title"]), t) || !strings.Contains(lower[i], t) {
				continue
			}
			if !re.MatchString(lower[i]) {
				continue
			}
			for _, target := range byTitle[t] {
				edges = append(edges, graphmodel.Edge{F: n, T: target, Type: "mentions", Origin: "inferred", Weight: 1})
			}
		}
	}
	return edges
}

// SimilarityEdges links every node to its k most similar nodes (cosine of
// Embedding) when the similarity is at least minSim. Use reduced vectors
// (e.g. 256 dims) for large corpora: this is O(n²).
func SimilarityEdges(nodes []*graphmodel.Node, k int, minSim float64) []graphmodel.Edge {
	type cand struct {
		j   int
		sim float64
	}
	var edges []graphmodel.Edge
	for i, a := range nodes {
		var best []cand
		for j, b := range nodes {
			if i == j {
				continue
			}
			s := evaluator.CosineSimilarity(a.Embedding, b.Embedding)
			if s < minSim {
				continue
			}
			best = append(best, cand{j, s})
		}
		sort.Slice(best, func(x, y int) bool {
			if best[x].sim != best[y].sim {
				return best[x].sim > best[y].sim
			}
			return best[x].j < best[y].j
		})
		for _, c := range best[:min(k, len(best))] {
			edges = append(edges, graphmodel.Edge{F: a, T: nodes[c.j], Type: "similar", Origin: "inferred", Weight: c.sim})
		}
	}
	return edges
}
