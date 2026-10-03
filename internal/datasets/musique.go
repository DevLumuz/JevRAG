package datasets

import (
	"encoding/json"
	"fmt"
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

// MuSiQueRow is one question.
type MuSiQueRow struct {
	ID         string             `json:"id"`
	Question   string             `json:"question"`
	Answer     string             `json:"answer"`
	Answerable bool               `json:"answerable"`
	Paragraphs []MuSiQueParagraph `json:"paragraphs"`
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
