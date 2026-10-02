package datasets_test

import (
	"testing"

	"jev/internal/datasets"
)

func TestParseKoBLEXRow(t *testing.T) {
	raw := map[string]any{
		"id":             "qa_19_1hop_28",
		"question":       "한국어 질문",
		"question_eng":   "English question about insurance",
		"answer":         "한국어 답",
		"answer_eng":     "English answer",
		"background":     "한국어 배경",
		"background_eng": "English background scenario",
		"n_hops":         float64(2),
		"contexts": []any{
			map[string]any{
				"index":         "상법 665조 손해보험자의 책임",
				"index_eng":     "COMMERCIAL ACT / Article. 665 / Liability of Non-Life Insurers",
				"hierarchy":     "상법 665조 1항",
				"hierarchy_eng": "COMMERCIAL ACT / Article. 665 / Paragraph. 1",
				"content":       "한국어 조문",
				"content_eng":   "English statutory text",
			},
			map[string]any{
				"index":         "상법 666조",
				"index_eng":     "COMMERCIAL ACT / Article. 666",
				"hierarchy":     "상법 666조",
				"hierarchy_eng": "COMMERCIAL ACT / Article. 666",
				"content":       "두번째 조문",
				"content_eng":   "Second provision",
			},
		},
	}

	row, err := datasets.ParseKoBLEXRow(raw)
	if err != nil {
		t.Fatalf("ParseKoBLEXRow() error: %v", err)
	}

	if row.ID != "qa_19_1hop_28" {
		t.Errorf("ID = %q, want qa_19_1hop_28", row.ID)
	}
	if row.QuestionEng != "English question about insurance" {
		t.Errorf("QuestionEng = %q", row.QuestionEng)
	}
	if row.NHops != 2 {
		t.Errorf("NHops = %d, want 2", row.NHops)
	}
	if len(row.Contexts) != 2 {
		t.Fatalf("len(Contexts) = %d, want 2", len(row.Contexts))
	}
	if row.Contexts[0].IndexEng != "COMMERCIAL ACT / Article. 665 / Liability of Non-Life Insurers" {
		t.Errorf("Contexts[0].IndexEng = %q", row.Contexts[0].IndexEng)
	}
	if row.Contexts[1].ContentEng != "Second provision" {
		t.Errorf("Contexts[1].ContentEng = %q", row.Contexts[1].ContentEng)
	}
}

func TestParseKoBLEXRow_MissingField(t *testing.T) {
	raw := map[string]any{
		"id": "incomplete",
		// missing required fields
	}

	_, err := datasets.ParseKoBLEXRow(raw)
	if err == nil {
		t.Fatal("expected error for incomplete row, got nil")
	}
}

func TestParseStatuteRow(t *testing.T) {
	raw := map[string]any{
		"index":       "상법 665조",
		"index_eng":   "COMMERCIAL ACT / Article. 665 / Liability of Non-Life Insurers",
		"content":     "한국어 조문 내용",
		"content_eng": "The insurer shall be liable...",
	}

	s, err := datasets.ParseStatuteRow(raw)
	if err != nil {
		t.Fatalf("ParseStatuteRow() error: %v", err)
	}
	if s.IndexEng != "COMMERCIAL ACT / Article. 665 / Liability of Non-Life Insurers" {
		t.Errorf("IndexEng = %q", s.IndexEng)
	}
	if s.ContentEng != "The insurer shall be liable..." {
		t.Errorf("ContentEng = %q", s.ContentEng)
	}
}
