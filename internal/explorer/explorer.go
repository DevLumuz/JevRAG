// Package explorer is the plan v2 retrieval loop (plan.md §22.2): rounds of
// search that build an evidence notebook.
//
//	round 1: vector search with the question
//	each round: JEV scores the new passages with the current notebook view
//	            (it reorders, it never drops), reads the sentences of the best
//	            ones and the still-needed facts go into the notebook
//	next round: vector search with question + notebook facts, fused with
//	            keyword search on the newest facts
//	output: every passage seen, ranked (notebook sources first), and the notebook
//
// Nothing here is domain-specific: states are {query, notebook view, passage}.
package explorer

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"jev/internal/graphmodel"
	"jev/internal/jev"
	"jev/internal/notebook"
	"jev/internal/probe"
	"jev/internal/retrieval"
)

// Config sets the loop's budget and rules.
type Config struct {
	Rounds        int     // search rounds (1 = single pass)
	PerRound      int     // new passages judged per round
	ReadTop       int     // best passages per round whose sentences are read
	MaxFacts      int     // notebook capacity (view shown to JEV)
	FactsPerPass  int     // facts taken from one passage at most
	FactThreshold float64 // P(needed_fact) to keep a sentence
	// Heuristic replaces JEV with no-model rules (ablation): passages keep
	// search order; the sentence with most words in common with question +
	// notebook is taken from each read passage.
	Heuristic bool
	// Passages and Sentences pick who scores passages and who picks notebook
	// sentences: "jev" (default), "rerank" (cross-encoder; query = question +
	// notebook), "mix" (passages only: mean of JEV and reranker).
	// A reranker picks the single best sentence of each read passage.
	Passages, Sentences string
}

// Explorer holds the memory and the services the loop calls.
type Explorer struct {
	Nodes   []*graphmodel.Node
	Keyword *retrieval.BM25
	Title   func(*graphmodel.Node) string
	// Embed returns the query embedding of a text (cached by the caller).
	Embed func(ctx context.Context, text string) ([]float64, error)
	JEV   jev.Client
	Model string
	Cache *probe.Cache
	Conc  int
	// Rerank scores (query, passage) pairs with a cross-encoder, in [0, 1].
	Rerank func(ctx context.Context, pairs [][2]string) ([]float64, error)
}

// Trace is what one exploration did.
type Trace struct {
	Ranked   []string        `json:"ranked"`   // passage keys, best first
	Notebook []notebook.Fact `json:"notebook"` // facts collected, in order
	Rounds   [][]string      `json:"rounds"`   // passages judged per round
	Calls    int             `json:"calls"`    // JEV calls (cache hits included)
	Tokens   int64           `json:"tokens"`   // paid JEV input tokens
	// MaxAnswer is the highest P(answers_query) JEV gave any passage.
	MaxAnswer float64 `json:"max_answer"`
}

type seen struct {
	node   *graphmodel.Node
	score  float64 // JEV max(next_needed, answers_query); heuristic: -order
	order  int     // first-seen order
	source bool    // contributed a fact
}

// Explore runs the loop for one question.
func (e *Explorer) Explore(ctx context.Context, query string, queryEmb []float64, cfg Config) (*Trace, error) {
	tr := &Trace{}
	got := map[*graphmodel.Node]*seen{}
	var facts, lastFacts []factP

	for round := 0; round < cfg.Rounds; round++ {
		// Round 1: vector search with the question (keywords do badly on long
		// questions, P1 T1). Later rounds: vector search with question +
		// notebook, fused with keyword search on the facts found last round
		// (short, entity-rich text, where keywords do well).
		var pool []retrieval.Scored
		if round == 0 || len(lastFacts) == 0 {
			pool = retrieval.VectorSearch(queryEmb, e.Nodes, 300)
		} else {
			v, err := e.Embed(ctx, query+"\n"+factText(facts))
			if err != nil {
				return nil, err
			}
			pool = retrieval.FuseRRF(60, 300,
				retrieval.VectorSearch(v, e.Nodes, 300),
				e.Keyword.Search(factText(lastFacts), 300))
		}
		var fresh []*graphmodel.Node
		for _, s := range pool {
			if len(fresh) == cfg.PerRound {
				break
			}
			if got[s.Node] == nil {
				fresh = append(fresh, s.Node)
			}
		}
		if len(fresh) == 0 {
			break
		}
		keys := make([]string, len(fresh))
		for i, n := range fresh {
			keys[i] = n.Key
		}
		tr.Rounds = append(tr.Rounds, keys)

		// Score the new passages with the current notebook view.
		scores := make([]float64, len(fresh))
		if !cfg.Heuristic {
			var err error
			if scores, err = e.scorePassages(ctx, cfg.Passages, query, plain(facts), fresh, tr); err != nil {
				return nil, err
			}
		}
		for i, n := range fresh {
			s := &seen{node: n, score: scores[i], order: len(got)}
			if cfg.Heuristic {
				s.score = -float64(s.order)
			}
			got[n] = s
		}

		// Read the best new passages; keep their still-needed sentences.
		idx := make([]int, len(fresh))
		for i := range idx {
			idx[i] = i
		}
		sort.SliceStable(idx, func(a, b int) bool { return scores[idx[a]] > scores[idx[b]] })
		idx = idx[:min(cfg.ReadTop, len(idx))]
		var newFacts []factP
		if cfg.Heuristic {
			for _, i := range idx {
				n := fresh[i]
				sents := notebook.SplitSentences(n.Content)
				best := bestOverlap(query+" "+factText(facts), sents)
				if best >= 0 {
					newFacts = append(newFacts, factP{notebook.Fact{Text: sents[best], Source: e.Title(n)}, 1, n})
				}
			}
		} else if cfg.Sentences == "rerank" {
			for _, i := range idx {
				n := fresh[i]
				sents := notebook.SplitSentences(n.Content)
				q := withNotebook(query, plain(facts))
				pairs := make([][2]string, len(sents))
				for j, st := range sents {
					pairs[j] = [2]string{q, e.Title(n) + ": " + st}
				}
				sc, err := e.Rerank(ctx, pairs)
				if err != nil {
					return nil, err
				}
				best := -1
				for j := range sc {
					if best < 0 || sc[j] > sc[best] {
						best = j
					}
				}
				if best >= 0 {
					newFacts = append(newFacts, factP{notebook.Fact{Text: sents[best], Source: e.Title(n)}, sc[best], n})
				}
			}
		} else {
			var items []probe.Item
			var owner []int
			var sentText []string
			for _, i := range idx {
				n := fresh[i]
				for _, s := range notebook.SplitSentences(n.Content) {
					items = append(items, probe.Item{State: notebook.SentenceState{
						Query: query, KnownFacts: notebook.View(plain(facts)), Source: e.Title(n), Sentence: s,
					}, Questions: notebook.SentenceQuestions})
					owner = append(owner, i)
					sentText = append(sentText, s)
				}
			}
			resps, err := e.run(ctx, items, tr)
			if err != nil {
				return nil, err
			}
			perPass := map[int][]factP{}
			for j, r := range resps {
				p := r.Answers["needed_fact"].Noul
				if p >= cfg.FactThreshold {
					n := fresh[owner[j]]
					perPass[owner[j]] = append(perPass[owner[j]], factP{notebook.Fact{Text: sentText[j], Source: e.Title(n)}, p, n})
				}
			}
			for _, i := range idx {
				fs := perPass[i]
				sort.SliceStable(fs, func(a, b int) bool { return fs[a].p > fs[b].p })
				newFacts = append(newFacts, fs[:min(cfg.FactsPerPass, len(fs))]...)
			}
		}
		for _, f := range newFacts {
			got[f.node].source = true
		}
		facts = append(facts, newFacts...)
		lastFacts = newFacts
		if len(facts) > cfg.MaxFacts { // keep the most confident, in collection order
			keep := append([]factP{}, facts...)
			sort.SliceStable(keep, func(a, b int) bool { return keep[a].p > keep[b].p })
			cut := map[string]bool{}
			for _, f := range keep[cfg.MaxFacts:] {
				cut[f.Text] = true
			}
			var kept []factP
			for _, f := range facts {
				if !cut[f.Text] {
					kept = append(kept, f)
				}
			}
			facts = kept
		}
	}

	all := make([]*seen, 0, len(got))
	for _, s := range got {
		all = append(all, s)
	}
	sort.Slice(all, func(i, j int) bool {
		a, b := all[i], all[j]
		if a.source != b.source {
			return a.source
		}
		if a.score != b.score {
			return a.score > b.score
		}
		return a.order < b.order
	})
	for _, s := range all {
		tr.Ranked = append(tr.Ranked, s.node.Key)
	}
	tr.Notebook = plain(facts)
	return tr, nil
}

// Single is the equal-budget control: one vector search with the question;
// the top n passages are reordered (never dropped) by scorer: "jev",
// "rerank", "mix", or "" (search order, no model).
func (e *Explorer) Single(ctx context.Context, query string, queryEmb []float64, n int, scorer string) (*Trace, error) {
	tr := &Trace{}
	pool := retrieval.VectorSearch(queryEmb, e.Nodes, n)
	nodes := make([]*graphmodel.Node, len(pool))
	for i, s := range pool {
		nodes[i] = s.Node
	}
	sc := make([]float64, len(pool))
	if scorer != "" {
		var err error
		if sc, err = e.scorePassages(ctx, scorer, query, nil, nodes, tr); err != nil {
			return nil, err
		}
	}
	idx := make([]int, len(pool))
	for i := range idx {
		idx[i] = i
	}
	sort.SliceStable(idx, func(a, b int) bool { return sc[idx[a]] > sc[idx[b]] })
	for _, i := range idx {
		tr.Ranked = append(tr.Ranked, nodes[i].Key)
	}
	return tr, nil
}

// scorePassages scores passages for the question and the notebook view.
func (e *Explorer) scorePassages(ctx context.Context, who, query string, facts []notebook.Fact, nodes []*graphmodel.Node, tr *Trace) ([]float64, error) {
	var jevS, rrS []float64
	if who == "" || who == "jev" || who == "mix" {
		items := make([]probe.Item, len(nodes))
		for i, n := range nodes {
			items[i] = probe.Item{State: notebook.T2State{
				Query: query, KnownFacts: notebook.View(facts), Frontier: []string{},
				Passage: notebook.Passage{Title: e.Title(n), Text: truncate(n.Content, probe.MaxPassageChars)},
			}, Questions: notebook.PassageQuestions}
		}
		resps, err := e.run(ctx, items, tr)
		if err != nil {
			return nil, err
		}
		jevS = make([]float64, len(nodes))
		for i, r := range resps {
			jevS[i] = max(r.Answers["next_needed"].Noul, r.Answers["answers_query"].Noul)
			tr.MaxAnswer = max(tr.MaxAnswer, r.Answers["answers_query"].Noul)
		}
	}
	if who == "rerank" || who == "mix" {
		q := withNotebook(query, facts)
		pairs := make([][2]string, len(nodes))
		for i, n := range nodes {
			pairs[i] = [2]string{q, e.Title(n) + ": " + n.Content}
		}
		var err error
		if rrS, err = e.Rerank(ctx, pairs); err != nil {
			return nil, err
		}
	}
	switch {
	case jevS != nil && rrS != nil:
		out := make([]float64, len(nodes))
		for i := range out {
			out[i] = (jevS[i] + rrS[i]) / 2
		}
		return out, nil
	case rrS != nil:
		return rrS, nil
	}
	return jevS, nil
}

// withNotebook is the reranker's query: the question plus the notebook facts.
func withNotebook(query string, facts []notebook.Fact) string {
	if len(facts) == 0 {
		return query
	}
	parts := make([]string, len(facts))
	for i, f := range facts {
		parts[i] = f.Text
	}
	return query + "\nKnown facts: " + strings.Join(parts, " ")
}

func (e *Explorer) run(ctx context.Context, items []probe.Item, tr *Trace) ([]*jev.Response, error) {
	if len(items) == 0 {
		return nil, nil
	}
	resps, u, err := probe.RunItems(ctx, e.JEV, e.Model, items, e.Cache, e.Conc, 0)
	tr.Calls += len(items)
	tr.Tokens += u.InputTokens
	if err != nil {
		return nil, fmt.Errorf("explorer: %w", err)
	}
	return resps, nil
}

type factP struct {
	notebook.Fact
	p    float64
	node *graphmodel.Node
}

func plain(fs []factP) []notebook.Fact {
	out := make([]notebook.Fact, len(fs))
	for i, f := range fs {
		out[i] = f.Fact
	}
	return out
}

func factText(fs []factP) string {
	parts := make([]string, len(fs))
	for i, f := range fs {
		parts[i] = f.Text
	}
	return strings.Join(parts, " ")
}

// bestOverlap returns the sentence sharing most distinct words with q.
func bestOverlap(q string, sents []string) int {
	qs := map[string]bool{}
	for _, t := range retrieval.Tokenize(q) {
		qs[t] = true
	}
	best, bestN := -1, 0
	for i, s := range sents {
		n, dup := 0, map[string]bool{}
		for _, t := range retrieval.Tokenize(s) {
			if qs[t] && !dup[t] {
				n++
				dup[t] = true
			}
		}
		if n > bestN {
			best, bestN = i, n
		}
	}
	return best
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n])
	}
	return s
}
