package datasets

import (
	"encoding/json"
	"fmt"
)

// KoBLEXRow represents a single QA instance from JihyungL/KoBLEX-koblex.
type KoBLEXRow struct {
	ID            string            `json:"id"`
	Question      string            `json:"question"`
	QuestionEng   string            `json:"question_eng"`
	Answer        string            `json:"answer"`
	AnswerEng     string            `json:"answer_eng"`
	Background    string            `json:"background"`
	BackgroundEng string            `json:"background_eng"`
	NHops         int               `json:"n_hops"`
	Contexts      []KoBLEXProvision `json:"contexts"`
}

// KoBLEXProvision represents a legal article in the multi-hop reasoning chain.
type KoBLEXProvision struct {
	Index        string `json:"index"`
	IndexEng     string `json:"index_eng"`
	Hierarchy    string `json:"hierarchy"`
	HierarchyEng string `json:"hierarchy_eng"`
	Content      string `json:"content"`
	ContentEng   string `json:"content_eng"`
}

// StatuteRow represents a single article from JihyungL/KoBLEX-statute-eng.
type StatuteRow struct {
	Index      string `json:"index"`
	IndexEng   string `json:"index_eng"`
	Content    string `json:"content"`
	ContentEng string `json:"content_eng"`
}

// ParseKoBLEXRow converts a raw row map (from AllRows) into a typed KoBLEXRow.
// Returns an error if required fields are missing or malformed.
func ParseKoBLEXRow(raw map[string]any) (*KoBLEXRow, error) {
	// Re-marshal to JSON and decode into the typed struct.
	// Simple, correct, and fast enough for 226 rows.
	b, err := json.Marshal(raw)
	if err != nil {
		return nil, fmt.Errorf("koblex: marshaling raw row: %w", err)
	}
	var row KoBLEXRow
	if err := json.Unmarshal(b, &row); err != nil {
		return nil, fmt.Errorf("koblex: parsing row: %w", err)
	}
	if row.ID == "" || row.QuestionEng == "" {
		return nil, fmt.Errorf("koblex: row missing required fields (id=%q, question_eng=%q)", row.ID, row.QuestionEng)
	}
	return &row, nil
}

// ParseStatuteRow converts a raw row map into a typed StatuteRow.
func ParseStatuteRow(raw map[string]any) (*StatuteRow, error) {
	b, err := json.Marshal(raw)
	if err != nil {
		return nil, fmt.Errorf("statute: marshaling raw row: %w", err)
	}
	var s StatuteRow
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, fmt.Errorf("statute: parsing row: %w", err)
	}
	if s.IndexEng == "" || s.ContentEng == "" {
		return nil, fmt.Errorf("statute: row missing required fields (index_eng=%q)", s.IndexEng)
	}
	return &s, nil
}
