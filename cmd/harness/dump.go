package main

import "log"

// dumpMemory writes a memory's paragraphs and questions so offline tools
// (e.g. the conversation set checker) can verify what the memory contains.
func dumpMemory(path string, bench *benchmark) error {
	type para struct {
		Key   string `json:"key"`
		Title string `json:"title"`
		Text  string `json:"text"`
	}
	type question struct {
		ID         string   `json:"id"`
		Text       string   `json:"text"`
		Answerable bool     `json:"answerable"`
		Hops       int      `json:"hops"`
		Gold       []string `json:"gold,omitempty"`
	}
	out := struct {
		Name       string     `json:"name"`
		Paragraphs []para     `json:"paragraphs"`
		Questions  []question `json:"questions"`
	}{Name: bench.Name}
	for _, n := range bench.Nodes {
		out.Paragraphs = append(out.Paragraphs, para{Key: n.Key, Title: n.Properties["title"], Text: n.Content})
	}
	for _, q := range bench.Questions {
		out.Questions = append(out.Questions, question{ID: q.ID, Text: q.Text, Answerable: q.Answerable, Hops: q.Hops, Gold: q.Gold})
	}
	log.Printf("%s: %d paragraphs, %d questions → %s", bench.Name, len(out.Paragraphs), len(out.Questions), path)
	return writeJSON(path, out)
}
