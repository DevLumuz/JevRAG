package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"jev/internal/embeddings"
	"jev/internal/evaluator"
	"jev/internal/explorer"
	"jev/internal/jev"
	"jev/internal/probe"
	"jev/internal/rerank"
	"jev/internal/retrieval"
)

// JEV input price, US$ per million tokens.
const jevPricePerMTok = 0.042

// exploreVariant is one system compared in Phase 2 (plan.md §22.3).
type exploreVariant struct {
	name string
	run  func(ctx context.Context, q benchQuestion) (*explorer.Trace, error)
}

// runExplore is Phase 2: the notebook loop against single-pass controls on
// the answerable questions of the selected split.
func runExplore(ctx context.Context, cfg config, bench *benchmark, qs []benchQuestion, qEmb map[string][]float64, querySpace *embedSpace) error {
	ex, loop, err := newExplorer(cfg, bench, querySpace)
	if err != nil {
		return err
	}
	heur := loop
	heur.Heuristic = true

	var answerable []benchQuestion
	for _, q := range qs {
		if q.Answerable {
			answerable = append(answerable, q)
		}
	}
	vecOnly := func(_ context.Context, q benchQuestion) (*explorer.Trace, error) {
		tr := &explorer.Trace{}
		for _, s := range retrieval.VectorSearch(qEmb[q.ID], bench.Nodes, 30) {
			tr.Ranked = append(tr.Ranked, s.Node.Key)
		}
		return tr, nil
	}
	variants := []exploreVariant{
		{"vector, question only (option 1)", vecOnly},
		{"notebook loop, no JEV (word-overlap rules)", func(ctx context.Context, q benchQuestion) (*explorer.Trace, error) {
			return ex.Explore(ctx, q.Text, qEmb[q.ID], heur)
		}},
		{"vector single pass + JEV scores 30", func(ctx context.Context, q benchQuestion) (*explorer.Trace, error) {
			return ex.Single(ctx, q.Text, qEmb[q.ID], 30, "jev")
		}},
		{fmt.Sprintf("vector single pass + JEV scores %d (equal budget)", cfg.exControlN), func(ctx context.Context, q benchQuestion) (*explorer.Trace, error) {
			return ex.Single(ctx, q.Text, qEmb[q.ID], cfg.exControlN, "jev")
		}},
		{"notebook loop + JEV", func(ctx context.Context, q benchQuestion) (*explorer.Trace, error) {
			return ex.Explore(ctx, q.Text, qEmb[q.ID], loop)
		}},
	}

	if cfg.exVariants == "phase1" || cfg.exVariants == "phase1final" {
		rr, err := rerank.Open(cfg.rerankURL, cfg.rerankModel, filepath.Join("data", "rerank", "cache.jsonl"))
		if err != nil {
			return err
		}
		ex.Rerank = rr.Score
		with := func(passages, sentences string) explorer.Config {
			c := loop
			c.Passages, c.Sentences = passages, sentences
			return c
		}
		single := func(n int, who string) func(context.Context, benchQuestion) (*explorer.Trace, error) {
			return func(ctx context.Context, q benchQuestion) (*explorer.Trace, error) {
				return ex.Single(ctx, q.Text, qEmb[q.ID], n, who)
			}
		}
		explore := func(c explorer.Config) func(context.Context, benchQuestion) (*explorer.Trace, error) {
			return func(ctx context.Context, q benchQuestion) (*explorer.Trace, error) {
				return ex.Explore(ctx, q.Text, qEmb[q.ID], c)
			}
		}
		variants = []exploreVariant{
			{"vector, question only (option 1)", vecOnly},
			{"single pass + JEV scores 30", single(30, "jev")},
			{"single pass + reranker scores 30", single(30, "rerank")},
			{"single pass + reranker scores 60", single(60, "rerank")},
			{"single pass + JEV & reranker mean, 30", single(30, "mix")},
			{"notebook loop · reranker scores, reranker picks sentences (no JEV)", explore(with("rerank", "rerank"))},
			{"notebook loop · reranker scores, JEV picks sentences", explore(with("rerank", "jev"))},
			{"notebook loop · JEV & reranker mean scores, JEV picks sentences", explore(with("mix", "jev"))},
			{"notebook loop + JEV", explore(loop)},
		}
		if cfg.exVariants == "phase1final" { // plan §27: the reranker rows that matter, within budget
			variants = []exploreVariant{
				{"vector, question only (option 1)", vecOnly},
				{"single pass + reranker scores 30", single(30, "rerank")},
				{"single pass + JEV & reranker mean, 30", single(30, "mix")},
				{"single pass + JEV scores 30", single(30, "jev")},
				{"notebook loop + JEV", explore(loop)},
			}
		}
	}

	if cfg.exVariants == "koblex" { // plan §29: out-of-domain test, frozen loop
		variants = []exploreVariant{
			{"vector, question only (option 1)", vecOnly},
			{"notebook loop + JEV", func(ctx context.Context, q benchQuestion) (*explorer.Trace, error) {
				return ex.Explore(ctx, q.Text, qEmb[q.ID], loop)
			}},
		}
	}

	type row struct {
		name     string
		outcomes []evaluator.QueryOutcome
		traces   []*explorer.Trace
		calls    int
		tokens   int64
	}
	var rows []row
	var spent int64
	for _, v := range variants {
		r := row{name: v.name}
		for i, q := range answerable {
			start := time.Now()
			tr, err := v.run(ctx, q)
			if err != nil {
				return fmt.Errorf("%s · %s: %w", v.name, q.ID, err)
			}
			got := bench.Canon(tr.Ranked)
			r.outcomes = append(r.outcomes, evaluator.QueryOutcome{ID: q.ID, Expected: q.Gold, Got: got,
				Hops: q.Hops, Latency: time.Since(start), JudgeCalls: tr.Calls})
			r.traces = append(r.traces, tr)
			r.calls += tr.Calls
			r.tokens += tr.Tokens
			if spent += tr.Tokens; cfg.maxJEV > 0 && spent > cfg.maxJEV {
				return fmt.Errorf("JEV budget reached: %d paid tokens > --max-jev-tokens %d (results so far are cached)", spent, cfg.maxJEV)
			}
			if (i+1)%10 == 0 {
				log.Printf("  %s: %d/%d", v.name, i+1, len(answerable))
			}
		}
		rows = append(rows, r)
		log.Printf("%s done: %d JEV calls, %d paid tokens", v.name, r.calls, r.tokens)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "# Phase 2 — notebook loop vs. single pass (%s, %s split)\n\n", bench.Name, cfg.mode)
	fmt.Fprintf(&b, "%d answerable questions · memory %d passages · loop: %d rounds × %d new passages, reads the %d best per round, ≤ 2 facts per passage (P ≥ %.2f), notebook ≤ 8 facts.\n\n",
		len(answerable), len(bench.Nodes), cfg.exRounds, cfg.exPerRound, cfg.exReadTop, cfg.exFactThreshold)
	b.WriteString("Chain@10 = all gold passages of the question in the top 10. Found = all gold passages anywhere in what the system returns (all passages the loop saw).\n\n")
	b.WriteString("| system | R@5 | R@10 | chain@5 | chain@10 | found (any rank) | JEV calls / q | US$ / q |\n|---|---|---|---|---|---|---|---|\n")
	for _, r := range rows {
		s := evaluator.Summarize(r.outcomes, []int{5, 10, 1000})
		n := float64(len(r.outcomes))
		fmt.Fprintf(&b, "| %s | %.3f | %.3f | %.3f | %.3f | %.3f | %.1f | %.4f |\n", r.name,
			s.RecallAt[5], s.RecallAt[10], s.CompleteAt[5], s.CompleteAt[10], s.CompleteAt[1000],
			float64(r.calls)/n, float64(r.tokens)/n/1e6*jevPricePerMTok)
	}
	b.WriteString("\nUS$/q counts only paid (uncached) tokens of this run.\n\n## Paired differences, chain@10 (95% CI resampling questions)\n\n")
	last := rows[len(rows)-1]
	for _, r := range rows[:len(rows)-1] {
		d := make([]float64, len(answerable))
		cl := make([]string, len(answerable))
		for i := range answerable {
			d[i] = complete(last.outcomes[i], 10) - complete(r.outcomes[i], 10)
			cl[i] = answerable[i].ID
		}
		m, lo, hi := probe.ClusterMean(d, cl, 4000, 3)
		fmt.Fprintf(&b, "- notebook loop + JEV − %s: %+.3f [%+.3f..%+.3f]\n", r.name, m, lo, hi)
	}
	b.WriteString("\n## By number of hops (chain@10)\n\n| system |")
	hopSet := map[int]bool{}
	for _, q := range answerable {
		hopSet[q.Hops] = true
	}
	var hops []int
	for h := range hopSet {
		hops = append(hops, h)
	}
	sort.Ints(hops)
	for _, h := range hops {
		fmt.Fprintf(&b, " %d hops |", h)
	}
	b.WriteString("\n|---|" + strings.Repeat("---|", len(hops)) + "\n")
	for _, r := range rows {
		fmt.Fprintf(&b, "| %s |", r.name)
		for _, h := range hops {
			c, n := 0.0, 0
			for _, o := range r.outcomes {
				if o.Hops == h {
					c += complete(o, 10)
					n++
				}
			}
			if n == 0 {
				b.WriteString(" — |")
			} else {
				fmt.Fprintf(&b, " %.2f (n=%d) |", c/float64(n), n)
			}
		}
		b.WriteString("\n")
	}
	fmt.Print(b.String())
	stamp := time.Now().Format("20060102-150405")
	out := filepath.Join(cfg.resultsDir, fmt.Sprintf("%s-%s-explore-%s-%s.md", stamp, bench.Name, cfg.exVariants, cfg.mode))
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(out, []byte(b.String()), 0o644); err != nil {
		return err
	}
	tracesOut := strings.TrimSuffix(out, ".md") + "-traces.json"
	traces := map[string]map[string]*explorer.Trace{}
	for _, r := range rows {
		traces[r.name] = map[string]*explorer.Trace{}
		for i, q := range answerable {
			traces[r.name][q.ID] = r.traces[i]
		}
	}
	if err := writeJSON(tracesOut, traces); err != nil {
		return err
	}
	log.Printf("report: %s · traces: %s", out, tracesOut)
	return nil
}

func complete(o evaluator.QueryOutcome, k int) float64 {
	if evaluator.RecallAtK(o.Expected, o.Got, k) == 1 {
		return 1
	}
	return 0
}

// newExplorer builds the notebook explorer over the benchmark's memory and the
// frozen loop configuration (plan.md §24).
func newExplorer(cfg config, bench *benchmark, querySpace *embedSpace) (*explorer.Explorer, explorer.Config, error) {
	client, err := jev.NewHTTPClient(jev.Options{Model: cfg.jevModel})
	if err != nil {
		return nil, explorer.Config{}, err
	}
	cache, err := probe.OpenCache(filepath.Join("data", "probe", "cache.jsonl"))
	if err != nil {
		return nil, explorer.Config{}, err
	}
	title := bench.Source
	ex := &explorer.Explorer{
		Nodes: bench.Nodes,
		// Index the embedded text (title + content, cut at maxEmbedChars): very
		// long statutes stay bounded; for short paragraphs it is the same tokens.
		Keyword: retrieval.NewBM25(bench.Nodes, bench.DocText),
		Title:   title,
		JEV:     client, Model: cfg.jevModel, Cache: cache, Conc: cfg.concurrency,
		Embed: func(ctx context.Context, text string) ([]float64, error) {
			v, err := embeddings.EmbedAll(ctx, querySpace.client, querySpace.store, []string{text},
				embeddings.RunOptions{Confirm: cfg.confirmEmbed, Log: func(string, ...any) {}})
			if err != nil {
				return nil, err
			}
			r, err := reduceAll(v, cfg.dims)
			if err != nil {
				return nil, err
			}
			return r[0], nil
		},
	}
	loop := explorer.Config{Rounds: cfg.exRounds, PerRound: cfg.exPerRound, ReadTop: cfg.exReadTop,
		MaxFacts: 8, FactsPerPass: 2, FactThreshold: cfg.exFactThreshold}
	return ex, loop, nil
}
