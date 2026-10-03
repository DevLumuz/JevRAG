package datasets

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// MuSiQue (bdsaglam/musique, config "default" = MuSiQue-Full) pairs every
// answerable multi-hop question with an unanswerable twin whose key
// supporting paragraph was removed. Each row carries 20 Wikipedia paragraphs
// (supporting ones plus distractors) that do not cite each other.
const (
	MuSiQueDataset = "bdsaglam/musique"
	MuSiQueConfig  = "default"
	MuSiQueSplit   = "validation"
)

// MuSiQueParagraph is one paragraph attached to a question.
type MuSiQueParagraph struct {
	Idx          int    `json:"idx"`
	Title        string `json:"title"`
	Text         string `json:"paragraph_text"`
	IsSupporting bool   `json:"is_supporting"`
}

// MuSiQueStep is one step of a question's decomposition. ParagraphIdx points
// at the supporting paragraph for that step (nil for unanswerable rows).
type MuSiQueStep struct {
	Question     string `json:"question"`
	Answer       string `json:"answer"`
	ParagraphIdx *int   `json:"paragraph_support_idx"`
}

// MuSiQueRow is one question.
type MuSiQueRow struct {
	ID            string             `json:"id"`
	Question      string             `json:"question"`
	Answer        string             `json:"answer"`
	Answerable    bool               `json:"answerable"`
	Paragraphs    []MuSiQueParagraph `json:"paragraphs"`
	Decomposition []MuSiQueStep      `json:"question_decomposition"`
}

// SupportRoles maps each supporting paragraph's key to its role in the
// reasoning chain: "final" for the paragraph of the last step (it states the
// answer) and "bridge" for earlier steps (intermediate facts).
func (r *MuSiQueRow) SupportRoles() map[string]string {
	byIdx := map[int]MuSiQueParagraph{}
	for _, p := range r.Paragraphs {
		byIdx[p.Idx] = p
	}
	roles := map[string]string{}
	for i, s := range r.Decomposition {
		if s.ParagraphIdx == nil {
			continue
		}
		p, ok := byIdx[*s.ParagraphIdx]
		if !ok {
			continue
		}
		role := "bridge"
		if i == len(r.Decomposition)-1 {
			role = "final"
		}
		roles[ParagraphKey(p)] = role
	}
	return roles
}

// ResolvedStep is a decomposition step with its "#k" references replaced by
// the answers of the steps they point to.
type ResolvedStep struct {
	Question     string // sub-question, references resolved
	Answer       string
	ParagraphKey string // key of the step's supporting paragraph ("" if unknown)
	Deps         []int  // indexes of the steps this one references directly
}

// ResolvedSteps returns the decomposition with references resolved. Steps
// without "#k" are entry points (first hops); the last step states the answer;
// the others are middle hops.
func (r *MuSiQueRow) ResolvedSteps() []ResolvedStep {
	byIdx := map[int]MuSiQueParagraph{}
	for _, p := range r.Paragraphs {
		byIdx[p.Idx] = p
	}
	out := make([]ResolvedStep, len(r.Decomposition))
	for i, s := range r.Decomposition {
		q := s.Question
		var deps []int
		// Replace longest numbers first so "#1" never eats part of "#12".
		for k := len(r.Decomposition); k >= 1; k-- {
			ref := fmt.Sprintf("#%d", k)
			if k-1 != i && strings.Contains(q, ref) {
				q = strings.ReplaceAll(q, ref, r.Decomposition[k-1].Answer)
				deps = append(deps, k-1)
			}
		}
		sort.Ints(deps)
		out[i] = ResolvedStep{Question: q, Answer: s.Answer, Deps: deps}
		if s.ParagraphIdx != nil {
			if p, ok := byIdx[*s.ParagraphIdx]; ok {
				out[i].ParagraphKey = ParagraphKey(p)
			}
		}
	}
	return out
}

// Ancestors returns every step that step i depends on, directly or not, in
// increasing order.
func Ancestors(steps []ResolvedStep, i int) []int {
	seen := map[int]bool{}
	var walk func(int)
	walk = func(j int) {
		for _, d := range steps[j].Deps {
			if !seen[d] {
				seen[d] = true
				walk(d)
			}
		}
	}
	walk(i)
	out := make([]int, 0, len(seen))
	for d := range seen {
		out = append(out, d)
	}
	sort.Ints(out)
	return out
}

// Hops returns the number of reasoning steps encoded in the ID prefix
// ("2hop__…", "3hop1__…", "4hop3__…"), or 0 if absent.
func (r *MuSiQueRow) Hops() int {
	if len(r.ID) > 4 && r.ID[0] >= '2' && r.ID[0] <= '9' && strings.HasPrefix(r.ID[1:], "hop") {
		return int(r.ID[0] - '0')
	}
	return 0
}

// Supporting returns the supporting paragraphs (empty for unanswerable rows).
func (r *MuSiQueRow) Supporting() []MuSiQueParagraph {
	var out []MuSiQueParagraph
	for _, p := range r.Paragraphs {
		if p.IsSupporting {
			out = append(out, p)
		}
	}
	return out
}

// ParagraphKey identifies a paragraph by content, so the same paragraph
// shared by several questions becomes one memory node.
func ParagraphKey(p MuSiQueParagraph) string {
	return p.Title + " | " + p.Text
}

// ParseMuSiQueRow converts a raw row map into a typed MuSiQueRow.
func ParseMuSiQueRow(raw map[string]any) (*MuSiQueRow, error) {
	b, err := json.Marshal(raw)
	if err != nil {
		return nil, fmt.Errorf("musique: marshaling raw row: %w", err)
	}
	var r MuSiQueRow
	if err := json.Unmarshal(b, &r); err != nil {
		return nil, fmt.Errorf("musique: parsing row: %w", err)
	}
	if r.ID == "" || r.Question == "" || len(r.Paragraphs) == 0 {
		return nil, fmt.Errorf("musique: row missing required fields (id=%q)", r.ID)
	}
	if r.Answerable && len(r.Supporting()) == 0 {
		return nil, fmt.Errorf("musique: answerable row %q has no supporting paragraph", r.ID)
	}
	return &r, nil
}
