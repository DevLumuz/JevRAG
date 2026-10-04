package brief_test

import (
	"strings"
	"testing"

	"jev/internal/brief"
)

func TestSearchText(t *testing.T) {
	got := brief.SearchText("Where was {1} born, and when did [the director] die?")
	if got != "Where was born, and when did die?" {
		t.Errorf("got %q", got)
	}
}

func TestPromptVariants(t *testing.T) {
	if !strings.Contains(brief.Prompt("q", brief.Blind), "private") {
		t.Error("blind prompt must say the documents are private")
	}
	if strings.Contains(brief.Prompt("q", brief.Knowledge), "private") {
		t.Error("knowledge prompt must not say private")
	}
}

func TestParse(t *testing.T) {
	b, err := brief.Parse(`{"question":"q","steps":[{"id":1,"ask":"a","needs":[]}],"requirements":["r"],"terms":["t"],"hypothetical":"h","answer_type":"date"}`)
	if err != nil || len(b.Steps) != 1 || b.Steps[0].Ask != "a" {
		t.Fatalf("got %+v, %v", b, err)
	}
}
