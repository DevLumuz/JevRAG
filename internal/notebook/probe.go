package notebook

// Inputs for the P1 notebook probes (plan.md §22.3), written by
// `harness --notebook-probe` and read by `cmd/notebook`.

// ProbeInput is the whole probe file.
type ProbeInput struct {
	Dataset   string           `json:"dataset"`
	Targets   []ProbeTarget    `json:"targets"`   // T2: recognition of a later-step passage
	Sentences []ProbeParagraph `json:"sentences"` // T3: pick the key sentence of a gold passage
}

// ProbeTarget is one later reasoning step (it depends on earlier steps) of a
// question: its gold passage plus non-gold passages from the same search pool,
// and the notebook views the judge may see.
type ProbeTarget struct {
	QueryID  string `json:"query_id"`
	Query    string `json:"query"`
	Split    string `json:"split"` // dev or test (of the earlier MuSiQue runs)
	Hops     int    `json:"hops"`
	Step     int    `json:"step"` // 0-based index in the decomposition
	StepKind string `json:"step_kind"`

	// Oracle notebook: silver facts of every earlier step this one depends
	// on, and the entities they lead to (answers of its direct dependencies).
	Facts    []Fact   `json:"facts"`
	Frontier []string `json:"frontier"`
	// Wrong notebook: the same view taken from another question.
	WrongFacts    []Fact   `json:"wrong_facts"`
	WrongFrontier []string `json:"wrong_frontier"`
	// Ancestor steps whose gold passage JEV reads in T3 (for the condition
	// where the notebook holds JEV's own picks instead of silver facts).
	Ancestors []int `json:"ancestors"`

	Passages []ProbeCandidate `json:"passages"` // first one is gold
}

// ProbeCandidate is a passage shown to the judge.
type ProbeCandidate struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Text  string `json:"text"`
	Gold  bool   `json:"gold"`
	Rank  int    `json:"rank"` // position in the search pool it was drawn from
}

// ProbeParagraph is one gold passage split into sentences, with the silver
// sentence for its step.
type ProbeParagraph struct {
	QueryID   string   `json:"query_id"`
	Query     string   `json:"query"`
	Step      int      `json:"step"`
	StepKind  string   `json:"step_kind"`
	Facts     []Fact   `json:"facts"` // oracle facts of earlier steps
	Title     string   `json:"title"`
	Sentences []string `json:"sentences"`
	Silver    int      `json:"silver"` // index into Sentences
	Exact     bool     `json:"exact"`  // silver sentence contains the step's answer
}

// Step kinds.
const (
	StepFirst  = "first"  // depends on no other step
	StepMiddle = "middle" // depends on earlier steps; not the last one
	StepFinal  = "final"  // last step: states the answer
)
