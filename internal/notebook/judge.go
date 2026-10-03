package notebook

import (
	"fmt"

	"jev/internal/jev"
)

// JEV states and questions of the notebook (validated in P1, plan.md §23).
// Every state is domain-agnostic: the query, a short notebook view
// (known_facts, frontier) and one passage or sentence.

// --- States (domain-agnostic: query, notebook view, passage) ---

// FactView is a notebook fact as shown to JEV.
type FactView struct {
	Text   string `json:"text"`
	Source string `json:"source"`
}

// View turns facts into the notebook view.
func View(fs []Fact) []FactView {
	out := make([]FactView, 0, len(fs))
	for _, f := range fs {
		out = append(out, FactView{Text: f.Text, Source: f.Source})
	}
	return out
}

// Passage is a candidate passage inside a state.
type Passage struct {
	Title string `json:"title"`
	Text  string `json:"text"`
}

// T3State: the passage whose key sentence is picked.
type T3State struct {
	Query      string     `json:"query"`
	KnownFacts []FactView `json:"known_facts"`
	Passage    Passage    `json:"passage"`
}

// SentenceState: one sentence judged on its own (with its passage title).
type SentenceState struct {
	Query      string     `json:"query"`
	KnownFacts []FactView `json:"known_facts"`
	Source     string     `json:"source"`
	Sentence   string     `json:"sentence"`
}

// T2State: a candidate passage judged with a notebook view.
type T2State struct {
	Query      string     `json:"query"`
	KnownFacts []FactView `json:"known_facts"`
	Frontier   []string   `json:"frontier"`
	Passage    Passage    `json:"passage"`
}

// --- Questions ---

const pickInstructions = "Which sentence of `passage` states a fact that is still needed to answer `query`? " +
	"`known_facts` are facts already collected from other documents; a sentence that only repeats them is not needed. " +
	"Prefer the sentence that names the item, value or relation the question asks about or that links an item from `query` or `known_facts` to a new item."

// PickQuestion asks JEV to pick the key sentence (choice; "none" last).
func PickQuestion(sentences []string) map[string]jev.Question {
	opts := make([]jev.Option, 0, len(sentences)+1)
	for i, s := range sentences {
		opts = append(opts, jev.Option{Label: fmt.Sprintf("s%d", i+1), Description: s})
	}
	opts = append(opts, jev.Option{Label: "none", Description: "No sentence of the passage states a fact still needed to answer `query`."})
	return map[string]jev.Question{"key_sentence": jev.Choice(pickInstructions, opts)}
}

// SentenceQuestions judge one sentence: is it a still-needed fact?
var SentenceQuestions = map[string]jev.Question{
	"needed_fact": {
		Type: "noul",
		Instructions: jev.Ordered{
			{Key: "question", Value: "Does `sentence` state a fact that is still needed to answer `query`?"},
			{Key: "focus", Value: "`known_facts` were already collected from other documents. `source` is the title of the document the sentence comes from. The question may need several facts chained together; a fact counts if it is one link of that chain."},
		},
		Criteria: jev.Ordered{
			{Key: "true", Value: "The sentence states a value, name, date, place or relation about an item named in `query` or in `known_facts` (or about the item `source` names) that answers part of `query` and is not already stated in `known_facts`."},
			{Key: "false", Value: "The sentence is background, describes other aspects of the item, repeats `known_facts`, or concerns items unrelated to `query`."},
		},
	},
}

// PassageQuestions judge a passage with a notebook view: is it a still-needed
// link (next_needed), and does it give the final answer (answers_query)?
var PassageQuestions = map[string]jev.Question{
	"next_needed": {
		Type: "noul",
		Instructions: jev.Ordered{
			{Key: "question", Value: "Does `passage` state a fact that is still needed to answer `query`?"},
			{Key: "focus", Value: "Answering `query` may need several facts from different documents, chained together. `known_facts` are links already found; `frontier` names items those facts lead to, whose details may be the next link. Judge only what `passage` states."},
		},
		Criteria: jev.Ordered{
			{Key: "true", Value: "The passage states a value, name, date, place or relation about an item named in `query`, `known_facts` or `frontier` that is one link of the chain and is not already in `known_facts`."},
			{Key: "false", Value: "The passage only shares the topic, describes a different item with a similar name, repeats `known_facts`, or concerns items that neither `query` nor `known_facts` lead to."},
		},
	},
	"answers_query": {
		Type: "noul",
		Instructions: jev.Ordered{
			{Key: "question", Value: "Together with `known_facts`, does `passage` state the final answer to `query`?"},
			{Key: "focus", Value: "Use `known_facts` to resolve what each part of `query` refers to."},
		},
		Criteria: jev.Ordered{
			{Key: "true", Value: "The passage states the value `query` finally asks for, for the item that `query` and `known_facts` point to."},
			{Key: "false", Value: "The passage gives an intermediate fact only, or the value for a different item, or nothing the query asks."},
		},
	},
}

// CoverageState is what JEV sees to decide whether the notebook answers the
// question: only the question and the collected facts.
type CoverageState struct {
	Query      string     `json:"query"`
	KnownFacts []FactView `json:"known_facts"`
}

// CoverageQuestions decide abstention from the notebook (plan.md §22.3,
// Phase 3): is the final answer stated, and is a link of the chain missing?
var CoverageQuestions = map[string]jev.Question{
	"answer_stated": {
		Type: "noul",
		Instructions: jev.Ordered{
			{Key: "question", Value: "Taken together, do `known_facts` state the final answer to `query`?"},
			{Key: "focus", Value: "Answering may need several facts chained together: one fact identifies an item that `query` describes, the next states something about that item. Count only what the facts state; do not use outside knowledge."},
		},
		Criteria: jev.Ordered{
			{Key: "true", Value: "Following the facts from one to the next identifies every item `query` refers to and ends in the value `query` asks for."},
			{Key: "false", Value: "A link is missing (an item `query` depends on is never identified, or nothing states the asked value for it), the facts concern a different item, or they only give background."},
		},
	},
	"missing_link": {
		Type: "noul",
		Instructions: jev.Ordered{
			{Key: "question", Value: "Does `query` depend on an item that no fact in `known_facts` identifies?"},
			{Key: "focus", Value: "`query` often refers to items by description (\"the company that makes X\", \"the city where Y was born\"). Check each description against the facts."},
		},
		Criteria: jev.Ordered{
			{Key: "true", Value: "At least one described item, or the value asked for it, is not stated by any fact."},
			{Key: "false", Value: "Every described item is identified by some fact and the asked value is stated."},
		},
	},
}

// CoverageWithPassages adds the explorer's best passages to the coverage
// state, for facts the notebook did not copy.
type CoverageWithPassages struct {
	Query      string     `json:"query"`
	KnownFacts []FactView `json:"known_facts"`
	Passages   []Passage  `json:"passages"`
}
