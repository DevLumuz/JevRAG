package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"jev/internal/datasets"
	"jev/internal/graphmodel"
	"jev/internal/notebook"
	"jev/internal/probe"
	"jev/internal/retrieval"
)

const (
	nbPool      = 300 // depth of every ranked list (largest reach@K reported)
	nbJudgePool = 30  // T2 negatives come from the top of the hybrid notebook search
	nbNegatives = 4   // T2 non-gold passages per target
)

var nbReachKs = []int{30, 100, 300}

// nbTarget is a later reasoning step whose gold passage must be reached.
type nbTarget struct {
	q        benchQuestion
	step     int
	kind     string
	node     *graphmodel.Node
	facts    []notebook.Fact
	frontier []string
	anc      []int
	missing  int // ancestors without a silver fact
}

// runNotebookProbe is plan.md §22.3 P1 on MuSiQue. T1 (here): can the next
// search reach a later step's gold passage when the notebook holds the facts
// of the earlier steps? Compared with the query alone, keyword search on the
// entities those facts lead to, and an exact title lookup. It also writes the
// inputs of T2/T3 (JEV runs in cmd/notebook).
//
// The memory here holds questions of both earlier splits; P1 is a mechanism
// probe, not a final evaluation (those use fresh questions, plan.md §22.3).
func runNotebookProbe(ctx context.Context, cfg config, bench *benchmark, qEmb map[string][]float64, querySpace *embedSpace) error {
	if bench.Name != "musique" {
		return fmt.Errorf("--notebook-probe needs --dataset musique (it uses the question decompositions)")
	}
	nodes := bench.Nodes
	byKey := make(map[string]*graphmodel.Node, len(nodes))
	for _, n := range nodes {
		byKey[n.Key] = n
	}
	title := func(n *graphmodel.Node) string { return n.Properties["title"] }
	bm := retrieval.NewBM25(nodes, func(n *graphmodel.Node) string { return title(n) + " " + n.Content })

	// Silver facts for every step, the T3 paragraphs and the T1/T2 targets.
	var (
		targets    []*nbTarget
		paragraphs []notebook.ProbeParagraph
		silverHist = map[notebook.Silver]int{}
	)
	for _, q := range bench.Questions {
		if !q.Answerable || len(q.Steps) == 0 {
			continue
		}
		facts := make([]*notebook.Fact, len(q.Steps))
		kinds := make([]string, len(q.Steps))
		for i, s := range q.Steps {
			kinds[i] = stepKind(q.Steps, i)
			n := byKey[s.ParagraphKey]
			if n == nil {
				continue
			}
			sents := notebook.SplitSentences(n.Content)
			idx, sk := notebook.SilverSentence(sents, s.Answer, s.Question)
			silverHist[sk]++
			if sk == notebook.SilverNone {
				continue
			}
			facts[i] = &notebook.Fact{Text: sents[idx], Source: title(n)}
			paragraphs = append(paragraphs, notebook.ProbeParagraph{
				QueryID: q.ID, Query: q.Text, Step: i, StepKind: kinds[i],
				Facts: oracleFacts(facts, datasets.Ancestors(q.Steps, i)), Title: title(n),
				Sentences: sents, Silver: idx, Exact: sk == notebook.SilverExact,
			})
		}
		for i, s := range q.Steps {
			n := byKey[s.ParagraphKey]
			if kinds[i] == notebook.StepFirst || n == nil {
				continue
			}
			t := &nbTarget{q: q, step: i, kind: kinds[i], node: n, anc: datasets.Ancestors(q.Steps, i)}
			for _, a := range t.anc {
				if facts[a] == nil {
					t.missing++
				}
			}
			t.facts = oracleFacts(facts, t.anc)
			for _, d := range s.Deps {
				t.frontier = append(t.frontier, strings.Trim(q.Steps[d].Answer, " ,.;"))
			}
			targets = append(targets, t)
		}
	}
	log.Printf("notebook probe: %d targets (later steps) · %d gold paragraphs for sentence selection · silver facts: exact %d, partial %d, none %d",
		len(targets), len(paragraphs), silverHist[notebook.SilverExact], silverHist[notebook.SilverPartial], silverHist[notebook.SilverNone])

	// New query texts: question + notebook facts, facts alone, resolved sub-question.
	withFacts := func(t *nbTarget) string { return t.q.Text + "\n" + factText(t.facts) }
	var texts []string
	for _, t := range targets {
		texts = append(texts, withFacts(t), factText(t.facts), t.q.Steps[t.step].Question)
	}
	vecs, err := embedAll(ctx, cfg, []embedJob{{name: "notebook", space: querySpace, texts: texts}})
	if err != nil {
		return err
	}
	red, err := reduceAll(vecs[0], cfg.dims)
	if err != nil {
		return err
	}
	emb := map[string][]float64{}
	for i, s := range texts {
		emb[s] = red[i]
	}

	vec := func(e []float64) []retrieval.Scored { return retrieval.VectorSearch(e, nodes, nbPool) }
	kw := func(s string) []retrieval.Scored { return bm.Search(s, nbPool) }
	rrf := func(ls ...[]retrieval.Scored) []retrieval.Scored { return retrieval.FuseRRF(60, nbPool, ls...) }
	methods := []struct {
		name string
		rank func(t *nbTarget) []retrieval.Scored
	}{
		{"vector · question alone (today)", func(t *nbTarget) []retrieval.Scored { return vec(qEmb[t.q.ID]) }},
		{"keywords · question alone", func(t *nbTarget) []retrieval.Scored { return kw(t.q.Text) }},
		{"hybrid · question alone", func(t *nbTarget) []retrieval.Scored { return rrf(vec(qEmb[t.q.ID]), kw(t.q.Text)) }},
		{"vector · question + notebook facts", func(t *nbTarget) []retrieval.Scored { return vec(emb[withFacts(t)]) }},
		{"vector · notebook facts alone", func(t *nbTarget) []retrieval.Scored { return vec(emb[factText(t.facts)]) }},
		{"keywords · question + frontier entity", func(t *nbTarget) []retrieval.Scored {
			return kw(t.q.Text + " " + strings.Join(t.frontier, " "))
		}},
		{"keywords · frontier entity alone", func(t *nbTarget) []retrieval.Scored { return kw(strings.Join(t.frontier, " ")) }},
		{"hybrid · question + facts + frontier", func(t *nbTarget) []retrieval.Scored { return nbHybrid(t, vec(emb[withFacts(t)]), kw) }},
		{"title lookup · frontier entity", func(t *nbTarget) []retrieval.Scored { return titleLookup(nodes, t.frontier, title) }},
		{"(upper bound) vector · resolved sub-question", func(t *nbTarget) []retrieval.Scored { return vec(emb[t.q.Steps[t.step].Question]) }},
	}

	var rep strings.Builder
	fmt.Fprintf(&rep, "# P1 · T1 — reach of later-step passages with a notebook (MuSiQue)\n\n")
	fmt.Fprintf(&rep, "Memory: %d paragraphs. Targets: gold passages of steps that depend on earlier steps (middle: not last; final: states the answer). ", len(nodes))
	fmt.Fprintf(&rep, "Notebook = silver facts of the earlier steps (the sentence of their gold passage that contains their answer); frontier = the answers of the steps this one depends on.\n\n")
	fmt.Fprintf(&rep, "Silver facts: %d exact, %d partial, %d none. Targets with an incomplete notebook: %d.\n\n",
		silverHist[notebook.SilverExact], silverHist[notebook.SilverPartial], silverHist[notebook.SilverNone], countMissing(targets))
	fmt.Fprintf(&rep, "Reach@K = share of targets whose gold passage is in the top K (95%% CI resampling questions).\n\n")
	for _, kind := range []string{notebook.StepMiddle, notebook.StepFinal} {
		var sel []*nbTarget
		for _, t := range targets {
			if t.kind == kind {
				sel = append(sel, t)
			}
		}
		fmt.Fprintf(&rep, "## %s steps (n = %d)\n\n| search | reach@30 | reach@100 | reach@300 |\n|---|---|---|---|\n", kind, len(sel))
		for _, m := range methods {
			fmt.Fprintf(&rep, "| %s |", m.name)
			ranks := make([]int, len(sel))
			clusters := make([]string, len(sel))
			for i, t := range sel {
				ranks[i] = rankOf(m.rank(t), t.node)
				clusters[i] = t.q.ID
			}
			for _, k := range nbReachKs {
				vals := make([]float64, len(sel))
				for i, r := range ranks {
					if r > 0 && r <= k {
						vals[i] = 1
					}
				}
				mean, lo, hi := probe.ClusterMean(vals, clusters, 2000, uint64(k))
				fmt.Fprintf(&rep, " %.2f [%.2f–%.2f] |", mean, lo, hi)
			}
			rep.WriteString("\n")
		}
		rep.WriteString("\n")
	}
	fmt.Print(rep.String())
	reportPath := filepath.Join(cfg.resultsDir, "probe", "notebook-T1-musique.md")
	if err := os.MkdirAll(filepath.Dir(reportPath), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(reportPath, []byte(rep.String()), 0o644); err != nil {
		return err
	}
	log.Printf("report: %s", reportPath)

	// T2/T3 inputs.
	in := notebook.ProbeInput{Dataset: bench.Name, Sentences: paragraphs}
	order := append([]*nbTarget{}, targets...)
	sort.SliceStable(order, func(i, j int) bool { return hash32("w"+tid(order[i])) < hash32("w"+tid(order[j])) })
	wrong := map[*nbTarget]*nbTarget{}
	for i, t := range order {
		for j := 1; j < len(order); j++ {
			if o := order[(i+j)%len(order)]; o.q.ID != t.q.ID && len(o.facts) > 0 {
				wrong[t] = o
				break
			}
		}
	}
	for _, t := range targets {
		if len(t.facts) == 0 {
			continue
		}
		gold := map[string]bool{}
		for _, g := range t.q.Gold {
			gold[g] = true
		}
		pool := nbHybrid(t, vec(emb[withFacts(t)]), kw)
		var negs []retrieval.Scored
		rank := map[*graphmodel.Node]int{}
		for i, s := range pool[:min(nbJudgePool, len(pool))] {
			rank[s.Node] = i + 1
			if !gold[s.Node.Key] {
				negs = append(negs, s)
			}
		}
		sort.SliceStable(negs, func(i, j int) bool {
			return hash32(tid(t)+negs[i].Node.Key) < hash32(tid(t)+negs[j].Node.Key)
		})
		pt := notebook.ProbeTarget{
			QueryID: t.q.ID, Query: t.q.Text, Split: splitOf(t.q, cfg), Hops: t.q.Hops, Step: t.step, StepKind: t.kind,
			Facts: t.facts, Frontier: t.frontier, Ancestors: t.anc,
			Passages: []notebook.ProbeCandidate{{ID: t.node.Key, Title: title(t.node), Text: t.node.Content, Gold: true, Rank: rank[t.node]}},
		}
		if w := wrong[t]; w != nil {
			pt.WrongFacts, pt.WrongFrontier = w.facts, w.frontier
		}
		for _, s := range negs[:min(nbNegatives, len(negs))] {
			pt.Passages = append(pt.Passages, notebook.ProbeCandidate{ID: s.Node.Key, Title: title(s.Node), Text: s.Node.Content, Rank: rank[s.Node]})
		}
		in.Targets = append(in.Targets, pt)
	}
	path := filepath.Join("data", "probe", "notebook-musique.json")
	if err := writeJSON(path, in); err != nil {
		return err
	}
	log.Printf("T2/T3 inputs: %s (%d targets, %d paragraphs)", path, len(in.Targets), len(in.Sentences))
	return nil
}

// stepKind classifies step i of a decomposition.
func stepKind(steps []datasets.ResolvedStep, i int) string {
	switch {
	case len(steps[i].Deps) == 0:
		return notebook.StepFirst
	case i == len(steps)-1:
		return notebook.StepFinal
	}
	return notebook.StepMiddle
}

func oracleFacts(facts []*notebook.Fact, idx []int) []notebook.Fact {
	var out []notebook.Fact
	for _, i := range idx {
		if facts[i] != nil {
			out = append(out, *facts[i])
		}
	}
	return out
}

func factText(fs []notebook.Fact) string {
	parts := make([]string, len(fs))
	for i, f := range fs {
		parts[i] = f.Text
	}
	return strings.Join(parts, " ")
}

// nbHybrid is the notebook search of plan v2: question + facts by vector,
// question + frontier entities by keywords, fused.
func nbHybrid(t *nbTarget, vecFacts []retrieval.Scored, kw func(string) []retrieval.Scored) []retrieval.Scored {
	return retrieval.FuseRRF(60, nbPool, vecFacts, kw(t.q.Text+" "+strings.Join(t.frontier, " ")))
}

// titleLookup returns passages whose title equals a frontier entity.
func titleLookup(nodes []*graphmodel.Node, entities []string, title func(*graphmodel.Node) string) []retrieval.Scored {
	var out []retrieval.Scored
	for _, n := range nodes {
		for _, e := range entities {
			if e != "" && strings.EqualFold(strings.TrimSpace(title(n)), e) {
				out = append(out, retrieval.Scored{Node: n, Score: 1})
				break
			}
		}
	}
	return out
}

func rankOf(list []retrieval.Scored, n *graphmodel.Node) int {
	for i, s := range list {
		if s.Node == n {
			return i + 1
		}
	}
	return 0
}

func countMissing(ts []*nbTarget) int {
	c := 0
	for _, t := range ts {
		if t.missing > 0 {
			c++
		}
	}
	return c
}

func tid(t *nbTarget) string { return fmt.Sprintf("%s#%d", t.q.ID, t.step) }

func splitOf(q benchQuestion, cfg config) string {
	if datasets.InDev(q.ID, cfg.devFraction) {
		return "dev"
	}
	return "test"
}
