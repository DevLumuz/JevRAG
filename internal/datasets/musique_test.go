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

func TestSupportRoles(t *testing.T) {
	raw := musiqueRaw("2hop__1_2", true, true)
	raw["paragraphs"] = append(raw["paragraphs"].([]any), map[string]any{"idx": float64(2), "title": "Walt Disney", "paragraph_text": "Founder.", "is_supporting": true})
	raw["question_decomposition"] = []any{
		map[string]any{"question": "Who created Mickey?", "answer": "Walt Disney", "paragraph_support_idx": float64(0)},
		map[string]any{"question": "Who founded #1?", "answer": "x", "paragraph_support_idx": float64(2)},
	}
	r, err := datasets.ParseMuSiQueRow(raw)
	if err != nil {
		t.Fatal(err)
	}
	roles := r.SupportRoles()
	if roles["Mickey Mouse | Mickey was created by Walt Disney."] != "bridge" || roles["Walt Disney | Founder."] != "final" {
		t.Errorf("roles = %v", roles)
	}
}

func TestResolvedSteps(t *testing.T) {
	one, two := 0, 1
	r := &datasets.MuSiQueRow{
		Paragraphs: []datasets.MuSiQueParagraph{{Idx: 0, Title: "A", Text: "a"}, {Idx: 1, Title: "B", Text: "b"}},
		Decomposition: []datasets.MuSiQueStep{
			{Question: "Who founded X?", Answer: "Ann", ParagraphIdx: &one},
			{Question: "Where was #1 born?", Answer: "Oslo", ParagraphIdx: &two},
			{Question: "When was #2 founded by #1 ?", Answer: "1048"},
		},
	}
	s := r.ResolvedSteps()
	if s[1].Question != "Where was Ann born?" || len(s[1].Deps) != 1 || s[1].ParagraphKey != "B | b" {
		t.Errorf("step 2 = %+v", s[1])
	}
	if s[2].Question != "When was Oslo founded by Ann ?" || len(s[2].Deps) != 2 || s[2].ParagraphKey != "" {
		t.Errorf("step 3 = %+v", s[2])
	}
	if got := datasets.Ancestors(s, 2); len(got) != 2 || got[0] != 0 || got[1] != 1 {
		t.Errorf("ancestors = %v", got)
	}
}
