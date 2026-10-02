// Package legal extracts graphmodel.Node and graphmodel.Edge from KoBLEX
// statute data. It parses explicit cross-references ("Article N") within the
// English text of each provision, producing intra-act directed edges.
//
// This is the domain-specific extraction layer for the legal domain.
// Other domains would have their own extractor under /internal/extract/<domain>.
package legal

import (
	"regexp"
	"sort"
	"strings"

	"jev/internal/datasets"
	"jev/internal/graphmodel"
)

// citationRe matches "Article N" where N is a number (possibly with hyphen,
// e.g. "4-2"). Captures the article number.
var citationRe = regexp.MustCompile(`Article[s]?\s+(\d+(?:-\d+)?)`)

// articleInIndexRe matches "Article. N" in the index_eng field.
var articleInIndexRe = regexp.MustCompile(`Article\.\s*(\d+(?:-\d+)?)`)

// ExtractCitations returns all unique article numbers referenced in a statute's
// content text, in the order first seen. E.g. "Article 3 and Article 22" → ["3","22"].
func ExtractCitations(content string) []string {
	matches := citationRe.FindAllStringSubmatch(content, -1)
	if len(matches) == 0 {
		return nil
	}

	seen := make(map[string]bool)
	var result []string
	for _, m := range matches {
		num := m[1]
		if !seen[num] {
			seen[num] = true
			result = append(result, num)
		}
	}
	return result
}

// ExtractActName returns the act name from an index_eng string.
// E.g. "CIVIL ACT / Article. 40 / Capacity" → "CIVIL ACT".
func ExtractActName(indexEng string) string {
	parts := strings.SplitN(indexEng, " / ", 2)
	return strings.TrimSpace(parts[0])
}

// ExtractArticleNumber returns the article number from an index_eng string.
// E.g. "CIVIL ACT / Article. 40 / Capacity" → "40".
func ExtractArticleNumber(indexEng string) string {
	m := articleInIndexRe.FindStringSubmatch(indexEng)
	if m == nil {
		return ""
	}
	return m[1]
}

// StatutesToNodes converts statute rows into graph nodes.
// Each statute becomes a single node; NumericID is assigned sequentially.
func StatutesToNodes(statutes []*datasets.StatuteRow) []*graphmodel.Node {
	nodes := make([]*graphmodel.Node, len(statutes))
	for i, s := range statutes {
		props := map[string]string{
			"act":            ExtractActName(s.IndexEng),
			"article_number": ExtractArticleNumber(s.IndexEng),
			"index":          s.Index,
		}
		if strings.Contains(s.ContentEng, machineTranslatedMarker) {
			props["machine_translated"] = "true"
		}
		nodes[i] = &graphmodel.Node{
			NumericID:  int64(i),
			Key:        s.IndexEng,
			Content:    s.ContentEng,
			Label:      "LegalArticle",
			Properties: props,
		}
	}
	return nodes
}

// BuildEdges parses cross-references in each statute's content and creates
// directed edges to the referenced articles within the SAME act.
//
// Only intra-act references are resolved — cross-act references ("Article 330"
// in the CIVIL ACT referencing the CRIMINAL ACT) cannot be reliably matched
// by article number alone and are skipped.
func BuildEdges(nodes []*graphmodel.Node, statutes []*datasets.StatuteRow) []graphmodel.Edge {
	// Build lookup: act → article_number → *Node
	type actArticleKey struct {
		act    string
		artNum string
	}
	lookup := make(map[actArticleKey]*graphmodel.Node, len(nodes))
	for _, n := range nodes {
		key := actArticleKey{
			act:    n.Properties["act"],
			artNum: n.Properties["article_number"],
		}
		lookup[key] = n
	}

	var edges []graphmodel.Edge
	for i, s := range statutes {
		fromNode := nodes[i]
		fromAct := fromNode.Properties["act"]
		fromArtNum := fromNode.Properties["article_number"]

		citations := ExtractCitations(s.ContentEng)
		for _, citedNum := range citations {
			// Skip self-references.
			if citedNum == fromArtNum {
				continue
			}
			// Only resolve within the same act.
			target, ok := lookup[actArticleKey{act: fromAct, artNum: citedNum}]
			if !ok {
				continue
			}
			edges = append(edges, graphmodel.Edge{
				F:      fromNode,
				T:      target,
				Type:   "references",
				Origin: "extracted",
				Weight: 1.0, // uniform for now; can be refined later
			})
		}
	}

	// Sort for determinism in tests.
	sort.Slice(edges, func(i, j int) bool {
		if edges[i].F.Key != edges[j].F.Key {
			return edges[i].F.Key < edges[j].F.Key
		}
		return edges[i].T.Key < edges[j].T.Key
	})

	return edges
}

// CanonicalKey reduces an index_eng string to "ACT#article", so a statute
// ("CIVIL ACT / Article. 40 / Capacity") and a gold context that omits the
// title or points at one paragraph ("CIVIL ACT / Article. 40") compare equal.
// Strings without an article number are returned trimmed, unchanged.
func CanonicalKey(indexEng string) string {
	num := ExtractArticleNumber(indexEng)
	if num == "" {
		return strings.TrimSpace(indexEng)
	}
	return ExtractActName(indexEng) + "#" + num
}

// GoldKeys returns the canonical keys of a KoBLEX question's gold provisions,
// deduplicated, in order.
func GoldKeys(row *datasets.KoBLEXRow) []string {
	idx := make([]string, len(row.Contexts))
	for i, c := range row.Contexts {
		idx[i] = c.IndexEng
	}
	return CanonicalRanking(idx)
}

// CanonicalRanking maps a ranked list of index_eng keys to canonical keys,
// keeping the first occurrence of each so ranks stay meaningful.
func CanonicalRanking(indexEngs []string) []string {
	seen := make(map[string]bool, len(indexEngs))
	out := make([]string, 0, len(indexEngs))
	for _, s := range indexEngs {
		k := CanonicalKey(s)
		if !seen[k] {
			seen[k] = true
			out = append(out, k)
		}
	}
	return out
}

// --- Embedding text ---

// EmbeddingRecipe names the recipe EmbeddingText implements. Bump it whenever
// the recipe changes; texts change with it, so cached vectors (keyed by text
// hash) are never reused for a different recipe.
const EmbeddingRecipe = "legal-v1"

const machineTranslatedMarker = "%MACHINE_TRANSLATED%"

var (
	// Editorial tags carrying a year (<Amended on Mar. 2, 2020>, <Dec. 29,
	// 2020>, <by Act No. 1, Jan. 2, 2019>) and image placeholders with no
	// content (<img id="1"></img>). They are not legal text.
	editorialTagRe = regexp.MustCompile(`<(?:/?img[^<>]*|[^<>]*\b(?:19|20)\d\d\b[^<>]*)>`)
	spacesRe       = regexp.MustCompile(`\s+`)
	smallWords     = map[string]bool{"a": true, "an": true, "and": true, "as": true, "at": true, "by": true,
		"for": true, "from": true, "in": true, "of": true, "on": true, "or": true, "the": true, "to": true, "with": true}
)

// ActDisplayName turns an ALL-CAPS act name into title case, the way acts are
// cited in prose ("ENFORCEMENT DECREE OF THE INCOME TAX ACT" → "Enforcement
// Decree of the Income Tax Act"). Names already in mixed case are kept.
func ActDisplayName(act string) string {
	if act != strings.ToUpper(act) {
		return act
	}
	words := strings.Fields(strings.ToLower(act))
	for i, w := range words {
		if i > 0 && smallWords[w] {
			continue
		}
		r := []rune(w)
		r[0] = []rune(strings.ToUpper(string(r[0])))[0]
		words[i] = string(r)
	}
	return strings.Join(words, " ")
}

// CleanForEmbedding removes editorial noise from statute text: amendment and
// date tags, empty image placeholders and the machine-translation marker.
// Only the embedded text is cleaned; Node.Content keeps the raw source.
func CleanForEmbedding(content string) string {
	s := strings.ReplaceAll(content, machineTranslatedMarker, " ")
	s = editorialTagRe.ReplaceAllString(s, " ")
	return strings.TrimSpace(spacesRe.ReplaceAllString(s, " "))
}

// EmbeddingText is the text embedded for a statute node: the act name, which
// the article body almost never states (99.7% of KoBLEX), followed by the
// cleaned content. Everything here is in the corpus row itself; nothing comes
// from questions or gold answers.
func EmbeddingText(n *graphmodel.Node) string {
	return ActDisplayName(n.Properties["act"]) + " — " + CleanForEmbedding(n.Content)
}
