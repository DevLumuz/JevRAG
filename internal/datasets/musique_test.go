package datasets_test

import (
	"testing"

	"jev/internal/datasets"
)

func musiqueRaw(id string, answerable bool, supporting bool) map[string]any {
	return map[string]any{
		"id": id, "question": "Who developed #1?", "answer": "Walt Disney", "answerable": answerable,
		"paragraphs": []any{
			map[string]any{"idx": float64(0), "title": "Mickey Mouse", "paragraph_text": "Mickey was created by Walt Disney.", "is_supporting": supporting},
			map[string]any{"idx": float64(1), "title": "Letterland", "paragraph_text": "Unrelated.", "is_supporting": false},
		},
	}
}

func TestParseMuSiQueRow(t *testing.T) {
	r, err := datasets.ParseMuSiQueRow(musiqueRaw("3hop1__1_2_3", true, true))
	if err != nil {
		t.Fatal(err)
	}
	if r.Hops() != 3 || len(r.Supporting()) != 1 || r.Supporting()[0].Title != "Mickey Mouse" {
		t.Errorf("row = %+v hops=%d", r, r.Hops())
	}
	if got := datasets.ParagraphKey(r.Paragraphs[0]); got != "Mickey Mouse | Mickey was created by Walt Disney." {
		t.Errorf("ParagraphKey = %q", got)
	}
}

func TestParseMuSiQueRow_Unanswerable(t *testing.T) {
	r, err := datasets.ParseMuSiQueRow(musiqueRaw("2hop__1_2", false, false))
	if err != nil || r.Answerable || len(r.Supporting()) != 0 || r.Hops() != 2 {
		t.Errorf("row = %+v, err = %v", r, err)
	}
}

func TestParseMuSiQueRow_AnswerableWithoutSupportFails(t *testing.T) {
	if _, err := datasets.ParseMuSiQueRow(musiqueRaw("2hop__1_2", true, false)); err == nil {
		t.Error("expected error")
	}
}
