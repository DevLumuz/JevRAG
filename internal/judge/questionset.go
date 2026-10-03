package judge

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"

	"jev/internal/jev"
)

// QuestionSet is a versioned, reviewable set of JEV questions, stored as a
// JSON file next to the code (questionsets/*.json) rather than inside it.
// Every question in a set is asked about the same state in one request.
//
// File format:
//
//	{
//	  "name": "passage-v2",
//	  "description": "what the set is for",
//	  "state": "short description of the state schema it expects",
//	  "questions": { "<id>": {"type": "noul|choice|score", "instructions": ..., "criteria": ...} }
//	}
type QuestionSet struct {
	Name        string                  `json:"name"`
	Description string                  `json:"description"`
	State       string                  `json:"state"`
	Questions   map[string]jev.Question `json:"questions"`
}

// LoadQuestionSet reads and validates a question set file.
func LoadQuestionSet(path string) (*QuestionSet, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var qs QuestionSet
	if err := json.Unmarshal(data, &qs); err != nil {
		return nil, fmt.Errorf("judge: parsing %s: %w", path, err)
	}
	if qs.Name == "" || len(qs.Questions) == 0 {
		return nil, fmt.Errorf("judge: %s needs a name and at least one question", path)
	}
	for id, q := range qs.Questions {
		switch q.Type {
		case "noul":
		case "choice", "score":
			if q.Criteria == nil {
				return nil, fmt.Errorf("judge: %s: %s question %q needs criteria", path, q.Type, id)
			}
		default:
			return nil, fmt.Errorf("judge: %s: question %q has unknown type %q", path, id, q.Type)
		}
		if q.Instructions == nil {
			return nil, fmt.Errorf("judge: %s: question %q needs instructions", path, id)
		}
	}
	return &qs, nil
}

// IDs returns the question ids in a stable order.
func (qs *QuestionSet) IDs() []string {
	ids := make([]string, 0, len(qs.Questions))
	for id := range qs.Questions {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}
