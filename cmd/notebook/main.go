// Command notebook runs the JEV parts of the P1 notebook probes (plan.md
// §22.3) on the inputs written by `harness --notebook-probe`:
//
//	T3 — key-sentence selection: given the question and the facts already in
//	the notebook, can JEV pick the sentence of a gold passage that carries
//	the step's fact? (one choice over the sentences, and one yes/no per
//	sentence), against simple baselines.
//
//	T2 — recognition with a notebook: does JEV rank a later step's gold
//	passage above non-gold passages of the same search pool better when the
//	state carries the notebook (oracle facts, wrong facts, JEV's own picks)?
//	Measured as AUC within each target, with 95% CIs resampling questions.
//
// Responses are cached by content hash (data/probe/cache.jsonl).
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"jev/internal/jev"
	"jev/internal/notebook"
	"jev/internal/probe"
	"jev/internal/retrieval"
)

const jevPricePerMTok = 0.042

func main() {
	log.SetFlags(0)
	inPath := flag.String("in", "data/probe/notebook-musique.json", "probe inputs from harness --notebook-probe")
	cachePath := flag.String("cache", "data/probe/cache.jsonl", "response cache")
	out := flag.String("out", "results/probe", "where reports are written")
	model := flag.String("model", "jev-1.13.0", "JEV model (pinned)")
	conc := flag.Int("concurrency", 8, "parallel requests")
	maxTok := flag.Int64("max-jev-tokens", 8_000_000, "cap on paid input tokens per stage (~US$0.042 per million)")
	only := flag.String("only", "t3,t2", "stages to run")
	limit := flag.Int("limit", 0, "use only the first N targets and N paragraphs (smoke test)")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	data, err := os.ReadFile(*inPath)
	if err != nil {
		log.Fatal(err)
	}
	var in notebook.ProbeInput
	if err := json.Unmarshal(data, &in); err != nil {
		log.Fatal(err)
	}
	if *limit > 0 {
		in.Targets = in.Targets[:min(*limit, len(in.Targets))]
		in.Sentences = in.Sentences[:min(*limit, len(in.Sentences))]
	}
	client, err := jev.NewHTTPClient(jev.Options{Model: *model})
	if err != nil {
		log.Fatal(err)
	}
	cache, err := probe.OpenCache(*cachePath)
	if err != nil {
		log.Fatal(err)
	}
	r := &runner{ctx: ctx, client: client, model: *model, cache: cache, conc: *conc, maxTok: *maxTok}

	var picks map[string]notebook.Fact
	if strings.Contains(*only, "t3") || strings.Contains(*only, "t2") {
		rep, p := r.t3(in)
		picks = p
		if strings.Contains(*only, "t3") {
			write(*out, "notebook-T3-"+in.Dataset+".md", rep+r.usageLine())
		}
	}
	if strings.Contains(*only, "t2") {
		r.used = probe.Usage{}
		write(*out, "notebook-T2-"+in.Dataset+".md", r.t2(in, picks)+r.usageLine())
	}
}

type runner struct {
	ctx    context.Context
	client jev.Client
	model  string
	cache  *probe.Cache
	conc   int
	maxTok int64
	used   probe.Usage
}

func (r *runner) run(items []probe.Item) []*jev.Response {
	resps, u, err := probe.RunItems(r.ctx, r.client, r.model, items, r.cache, r.conc, r.maxTok)
	r.used.Calls += u.Calls
	r.used.CacheHits += u.CacheHits
	r.used.InputTokens += u.InputTokens
	if err != nil {
		log.Fatalf("notebook: %v", err)
	}
	return resps
}

func (r *runner) usageLine() string {
	return fmt.Sprintf("\n---\n%d paid calls · %d cache hits · %d input tokens (~US$%.3f)\n",
		r.used.Calls, r.used.CacheHits, r.used.InputTokens, float64(r.used.InputTokens)/1e6*jevPricePerMTok)
}

func write(dir, name, s string) {
	fmt.Print(s)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		log.Fatal(err)
	}
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
		log.Fatal(err)
	}
	log.Printf("report: %s", p)
}

// --- T3: key-sentence selection ---

func key(qid string, step int) string { return fmt.Sprintf("%s#%d", qid, step) }

// t3 returns its report and JEV's pick (choice) for every gold paragraph.
func (r *runner) t3(in notebook.ProbeInput) (string, map[string]notebook.Fact) {
	ps := in.Sentences
	var choiceItems, sentItems []probe.Item
	var sentOwner []int
	for i, p := range ps {
		choiceItems = append(choiceItems, probe.Item{
			State:     notebook.T3State{Query: p.Query, KnownFacts: notebook.View(p.Facts), Passage: notebook.Passage{Title: p.Title, Text: strings.Join(p.Sentences, " ")}},
			Questions: notebook.PickQuestion(p.Sentences),
		})
		for _, s := range p.Sentences {
			sentItems = append(sentItems, probe.Item{
				State:     notebook.SentenceState{Query: p.Query, KnownFacts: notebook.View(p.Facts), Source: p.Title, Sentence: s},
				Questions: notebook.SentenceQuestions,
			})
			sentOwner = append(sentOwner, i)
		}
	}
	log.Printf("T3: %d paragraphs (choice) + %d sentences (yes/no)", len(choiceItems), len(sentItems))
	choice := r.run(choiceItems)
	sent := r.run(sentItems)

	sentScores := make([][]float64, len(ps))
	for j, resp := range sent {
		sentScores[sentOwner[j]] = append(sentScores[sentOwner[j]], resp.Answers["needed_fact"].Noul)
	}

	type method struct {
		name  string
		score func(i int) []float64 // one score per sentence, higher = pick
	}
	methods := []method{
		{"first sentence (baseline)", func(i int) []float64 {
			s := make([]float64, len(ps[i].Sentences))
			s[0] = 1
			return s
		}},
		{"word overlap with question (baseline)", func(i int) []float64 { return overlapScores(ps[i].Query, ps[i].Sentences) }},
		{"word overlap with question + known facts (baseline)", func(i int) []float64 {
			q := ps[i].Query
			for _, f := range ps[i].Facts {
				q += " " + f.Text
			}
			return overlapScores(q, ps[i].Sentences)
		}},
		{"JEV choice over sentences", func(i int) []float64 {
			a := choice[i].Answers["key_sentence"]
			s := make([]float64, len(ps[i].Sentences))
			for k := range s {
				s[k] = a.Probabilities[fmt.Sprintf("s%d", k+1)]
			}
			return s
		}},
		{"JEV yes/no per sentence", func(i int) []float64 { return sentScores[i] }},
	}

	picks := map[string]notebook.Fact{}
	for i, p := range ps {
		a := choice[i].Answers["key_sentence"]
		var k int
		if _, err := fmt.Sscanf(a.Choice, "s%d", &k); err == nil && k >= 1 && k <= len(p.Sentences) {
			picks[key(p.QueryID, p.Step)] = notebook.Fact{Text: p.Sentences[k-1], Source: p.Title}
		}
	}

	var b strings.Builder
	b.WriteString("# P1 · T3 — can JEV pick the key sentence of a gold passage?\n\n")
	fmt.Fprintf(&b, "%d gold passages (one per reasoning step), split into sentences; the state carries the question and the oracle facts of earlier steps. ", len(ps))
	b.WriteString("Target = the silver sentence (contains the step's answer). top-1 = the highest-scored sentence is the silver one; top-2 = it is in the two best. ")
	b.WriteString("JEV choice also has a last option \"none\"; it is reported apart.\n\n")
	nSent, firstSilver := 0, 0
	for _, p := range ps {
		nSent += len(p.Sentences)
		if p.Silver == 0 {
			firstSilver++
		}
	}
	fmt.Fprintf(&b, "Mean sentences per passage: %.1f (random pick top-1 ≈ %.2f). Silver sentence is the first one in %d of %d passages.\n\n",
		float64(nSent)/float64(len(ps)), meanInv(ps), firstSilver, len(ps))
	for _, filter := range []struct {
		name string
		keep func(p notebook.ProbeParagraph) bool
	}{
		{"all steps", func(notebook.ProbeParagraph) bool { return true }},
		{"first steps", func(p notebook.ProbeParagraph) bool { return p.StepKind == notebook.StepFirst }},
		{"middle steps", func(p notebook.ProbeParagraph) bool { return p.StepKind == notebook.StepMiddle }},
		{"final steps", func(p notebook.ProbeParagraph) bool { return p.StepKind == notebook.StepFinal }},
		{"passages with ≥ 3 sentences, silver not first", func(p notebook.ProbeParagraph) bool { return len(p.Sentences) >= 3 && p.Silver > 0 }},
	} {
		var idx []int
		for i, p := range ps {
			if filter.keep(p) {
				idx = append(idx, i)
			}
		}
		fmt.Fprintf(&b, "## %s (n = %d)\n\n| method | top-1 | top-2 |\n|---|---|---|\n", filter.name, len(idx))
		for _, m := range methods {
			t1 := make([]float64, len(idx))
			t2 := make([]float64, len(idx))
			cl := make([]string, len(idx))
			for j, i := range idx {
				rank := rankOfIdx(m.score(i), ps[i].Silver)
				if rank == 1 {
					t1[j] = 1
				}
				if rank <= 2 {
					t2[j] = 1
				}
				cl[j] = ps[i].QueryID
			}
			m1, l1, h1 := probe.ClusterMean(t1, cl, 2000, 1)
			m2, l2, h2 := probe.ClusterMean(t2, cl, 2000, 2)
			fmt.Fprintf(&b, "| %s | %.2f [%.2f–%.2f] | %.2f [%.2f–%.2f] |\n", m.name, m1, l1, h1, m2, l2, h2)
		}
		b.WriteString("\n")
	}
	none := 0
	conf := 0.0
	for i := range ps {
		a := choice[i].Answers["key_sentence"]
		if a.Choice == "none" {
			none++
		}
		conf += a.Confidence
	}
	fmt.Fprintf(&b, "JEV choice answered \"none\" in %d of %d passages (all contain the step's fact). Mean confidence %.2f.\n", none, len(ps), conf/float64(len(ps)))
	return b.String(), picks
}

func overlapScores(q string, sentences []string) []float64 {
	qs := map[string]bool{}
	for _, t := range retrieval.Tokenize(q) {
		qs[t] = true
	}
	out := make([]float64, len(sentences))
	for i, s := range sentences {
		toks := retrieval.Tokenize(s)
		seen := map[string]bool{}
		for _, t := range toks {
			if qs[t] && !seen[t] {
				out[i]++
				seen[t] = true
			}
		}
	}
	return out
}

// rankOfIdx is the 1-based rank of index want under scores (ties count
// against it: the target must beat every other sentence to be ranked first).
func rankOfIdx(scores []float64, want int) int {
	r := 1
	for i, s := range scores {
		if i != want && s >= scores[want] {
			r++
		}
	}
	return r
}

func meanInv(ps []notebook.ProbeParagraph) float64 {
	s := 0.0
	for _, p := range ps {
		s += 1 / float64(len(p.Sentences))
	}
	return s / float64(len(ps))
}

// --- T2: recognition with a notebook ---

type condition struct {
	name  string
	state func(t notebook.ProbeTarget) (facts []notebook.Fact, frontier []string, ok bool)
}

func (r *runner) t2(in notebook.ProbeInput, picks map[string]notebook.Fact) string {
	conds := []condition{
		{"no notebook (today)", func(notebook.ProbeTarget) ([]notebook.Fact, []string, bool) { return nil, nil, true }},
		{"oracle facts + frontier", func(t notebook.ProbeTarget) ([]notebook.Fact, []string, bool) { return t.Facts, t.Frontier, true }},
		{"oracle facts only", func(t notebook.ProbeTarget) ([]notebook.Fact, []string, bool) { return t.Facts, nil, true }},
		{"wrong facts + frontier (other question)", func(t notebook.ProbeTarget) ([]notebook.Fact, []string, bool) {
			return t.WrongFacts, t.WrongFrontier, len(t.WrongFacts) > 0
		}},
		{"JEV-picked facts (T3), no frontier", func(t notebook.ProbeTarget) ([]notebook.Fact, []string, bool) {
			var fs []notebook.Fact
			for _, a := range t.Ancestors {
				if f, ok := picks[key(t.QueryID, a)]; ok {
					fs = append(fs, f)
				}
			}
			return fs, nil, true
		}},
	}

	var items []probe.Item
	type ref struct{ cond, target, passage int }
	var refs []ref
	for ci, c := range conds {
		for ti, t := range in.Targets {
			facts, frontier, ok := c.state(t)
			if !ok {
				continue
			}
			if frontier == nil {
				frontier = []string{}
			}
			for pi, p := range t.Passages {
				items = append(items, probe.Item{
					State: notebook.T2State{Query: t.Query, KnownFacts: notebook.View(facts), Frontier: frontier,
						Passage: notebook.Passage{Title: p.Title, Text: truncate(p.Text, probe.MaxPassageChars)}},
					Questions: notebook.PassageQuestions,
				})
				refs = append(refs, ref{ci, ti, pi})
			}
		}
	}
	log.Printf("T2: %d calls (%d targets, %d conditions)", len(items), len(in.Targets), len(conds))
	resps := r.run(items)

	// scores[cond][target][passage][feature]
	feats := []string{"next_needed", "answers_query", "max"}
	scores := make([][][]map[string]float64, len(conds))
	for ci := range conds {
		scores[ci] = make([][]map[string]float64, len(in.Targets))
	}
	for k, rf := range refs {
		a := resps[k].Answers
		f := map[string]float64{"next_needed": a["next_needed"].Noul, "answers_query": a["answers_query"].Noul}
		f["max"] = math.Max(f["next_needed"], f["answers_query"])
		if scores[rf.cond][rf.target] == nil {
			scores[rf.cond][rf.target] = make([]map[string]float64, len(in.Targets[rf.target].Passages))
		}
		scores[rf.cond][rf.target][rf.passage] = f
	}
	// within-target AUC: gold (passage 0) vs the target's non-gold passages.
	aucOf := func(ci, ti int, f string) float64 {
		ss := scores[ci][ti]
		if ss == nil || len(ss) < 2 {
			return math.NaN()
		}
		var neg []float64
		for _, s := range ss[1:] {
			neg = append(neg, s[f])
		}
		return probe.AUC([]float64{ss[0][f]}, neg)
	}
	rankAUC := func(ti int) float64 { // search pool order as a reference
		ps := in.Targets[ti].Passages
		pos := func(r int) float64 { // rank 0 = outside the pool: below every pool member
			if r == 0 {
				return math.Inf(-1)
			}
			return -float64(r)
		}
		var neg []float64
		for _, p := range ps[1:] {
			neg = append(neg, pos(p.Rank))
		}
		return probe.AUC([]float64{pos(ps[0].Rank)}, neg)
	}

	var b strings.Builder
	b.WriteString("# P1 · T2 — does a notebook help JEV recognize the next link?\n\n")
	fmt.Fprintf(&b, "%d targets = gold passages of later reasoning steps, each with up to %d non-gold passages from the top %d of the notebook search (question + facts by vector, question + frontier entity by keywords). ",
		len(in.Targets), len(in.Targets[0].Passages)-1, 30)
	b.WriteString("AUC within each target (1 = gold ranked above all its non-gold passages, 0.5 = chance), averaged; 95% CI resampling questions. Same passages in every condition; only the notebook view changes.\n\n")
	b.WriteString("Questions: `next_needed` (is this passage a still-needed link?), `answers_query` (does it give the final answer, with the facts?), `max` = the larger of both.\n\n")

	for _, kind := range []string{notebook.StepMiddle, notebook.StepFinal, ""} {
		label := kind + " steps"
		if kind == "" {
			label = "all later steps"
		}
		var tids []int
		for ti, t := range in.Targets {
			if kind == "" || t.StepKind == kind {
				tids = append(tids, ti)
			}
		}
		fmt.Fprintf(&b, "## %s (n = %d)\n\n| notebook in the state |", label, len(tids))
		for _, f := range feats {
			fmt.Fprintf(&b, " `%s` |", f)
		}
		fmt.Fprintf(&b, "\n|---|%s\n", strings.Repeat("---|", len(feats)))
		cl := make([]string, len(tids))
		for j, ti := range tids {
			cl[j] = in.Targets[ti].QueryID
		}
		vals := make([]float64, len(tids))
		for j, ti := range tids {
			vals[j] = rankAUC(ti)
		}
		m, lo, hi := probe.ClusterMean(vals, cl, 2000, 7)
		fmt.Fprintf(&b, "| _search-pool order (reference)_ | %.3f [%.3f–%.3f] |%s\n", m, lo, hi, strings.Repeat(" |", len(feats)-1))
		for ci, c := range conds {
			fmt.Fprintf(&b, "| %s |", c.name)
			for _, f := range feats {
				for j, ti := range tids {
					vals[j] = aucOf(ci, ti, f)
				}
				m, lo, hi := probe.ClusterMean(vals, cl, 2000, uint64(ci+1))
				fmt.Fprintf(&b, " %.3f [%.3f–%.3f] |", m, lo, hi)
			}
			b.WriteString("\n")
		}
		// paired differences against "no notebook" on `max`.
		b.WriteString("\nPaired difference vs. no notebook (`next_needed`, `max`):\n\n")
		for ci := 1; ci < len(conds); ci++ {
			fmt.Fprintf(&b, "- %s:", conds[ci].name)
			for _, f := range []string{"next_needed", "max"} {
				for j, ti := range tids {
					vals[j] = aucOf(ci, ti, f) - aucOf(0, ti, f)
				}
				m, lo, hi := probe.ClusterMean(vals, cl, 2000, uint64(10+ci))
				fmt.Fprintf(&b, " `%s` %+.3f [%+.3f..%+.3f];", f, m, lo, hi)
			}
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}
	b.WriteString("Gate (plan §22.3 P1): oracle raises middle-step AUC by ≥ 0.10 and wrong facts lower it by no more than 0.05.\n")
	return b.String()
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n])
	}
	return s
}
