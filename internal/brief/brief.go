// Package brief defines the search brief: what the calling agent (a large
// LLM that has the whole conversation) writes once, before the memory runs,
// to steer the search. Nothing in a brief is evidence: it only shapes
// searches and the states JEV judges; the notebook keeps verbatim document
// sentences only.
package brief

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// Brief is the agent's search brief.
type Brief struct {
	Question     string   `json:"question"`     // self-contained question (conversation resolved)
	Steps        []Step   `json:"steps"`        // ordered sub-questions; later ones may depend on earlier answers
	Requirements []string `json:"requirements"` // what must be found to answer
	Terms        []string `json:"terms"`        // exact terms, official names, synonyms, acronyms
	Hypothetical string   `json:"hypothetical"` // how a passage that answers might read (search only)
	AnswerType   string   `json:"answer_type"`  // date, number, name, yes/no with conditions, list of steps…
}

// Step is one sub-question. Needs lists the ids of steps whose answers it
// uses; the placeholder {n} in Ask stands for step n's answer.
type Step struct {
	ID    int    `json:"id"`
	Ask   string `json:"ask"`
	Needs []int  `json:"needs"`
}

// Variants of what the agent may know.
const (
	Knowledge = "knowledge" // the agent may use its own general knowledge
	Blind     = "blind"     // private documents: the agent must not supply facts
)

// Schema is the JSON schema of a Brief (Gemini structured output).
var Schema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"question": map[string]any{"type": "string"},
		"steps": map[string]any{"type": "array", "items": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id":    map[string]any{"type": "integer"},
				"ask":   map[string]any{"type": "string"},
				"needs": map[string]any{"type": "array", "items": map[string]any{"type": "integer"}},
			},
			"required": []string{"id", "ask", "needs"},
		}},
		"requirements": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
		"terms":        map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
		"hypothetical": map[string]any{"type": "string"},
		"answer_type":  map[string]any{"type": "string"},
	},
	"required":         []string{"question", "steps", "requirements", "terms", "hypothetical", "answer_type"},
	"propertyOrdering": []string{"question", "steps", "requirements", "terms", "hypothetical", "answer_type"},
}

// Prompt is what the agent is asked. It is domain-agnostic: the same text for
// laws, procedures or encyclopedic questions.
func Prompt(question, variant string) string {
	var b strings.Builder
	b.WriteString("You help a search system find evidence in a document collection. Write a search brief for the question below. ")
	b.WriteString("The brief only guides searching; it is never shown to the user as evidence.\n\n")
	b.WriteString("Fields:\n")
	b.WriteString("- question: the question rewritten so it stands alone (resolve references; keep every condition).\n")
	b.WriteString("- steps: 1 to 4 sub-questions in the order they must be answered. If a step needs the answer of an earlier step, list that step's id in `needs` and write {id} where that answer goes.\n")
	b.WriteString("- requirements: 1 to 5 short items that the evidence must contain to answer the question.\n")
	b.WriteString("- terms: up to 8 exact search terms: official names, technical terms, synonyms, acronyms, article or section numbers.\n")
	b.WriteString("- hypothetical: at most 60 words, written like a passage from a document that would answer (it may be wrong; it is used only to search).\n")
	b.WriteString("- answer_type: the kind of answer expected (e.g. date, number, person, place, yes/no with conditions, list of steps).\n\n")
	if variant == Blind {
		b.WriteString("The documents are private: you do NOT know what they say. Never state or guess specific names, dates, numbers or answers that the question itself does not give. ")
		b.WriteString("Use bracketed placeholders instead, e.g. [the director], [the year], in steps, terms and the hypothetical passage.\n\n")
	} else {
		b.WriteString("You may use your general knowledge to choose better terms and steps.\n\n")
	}
	fmt.Fprintf(&b, "Question:\n%s\n", question)
	return b.String()
}

// Parse decodes a brief.
func Parse(text string) (*Brief, error) {
	var br Brief
	if err := json.Unmarshal([]byte(text), &br); err != nil {
		return nil, fmt.Errorf("brief: %w", err)
	}
	return &br, nil
}

var placeholder = regexp.MustCompile(`\{\d+\}|\[[^\]]*\]`)

// SearchText removes placeholders ({1}, [the director]) so a step can be
// searched before earlier answers are known.
func SearchText(s string) string {
	return strings.Join(strings.Fields(placeholder.ReplaceAllString(s, " ")), " ")
}
