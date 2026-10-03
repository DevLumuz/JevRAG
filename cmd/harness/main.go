// Command harness runs one retrieval option over a KoBLEX split and reports
// the metrics of section 10 of plan.md.
//
//	go run ./cmd/harness --check                        # free: data + graph sanity
//	go run ./cmd/harness --smoke --confirm-embed        # ~US$0.03 live API check
//	go run ./cmd/harness --embed-only --confirm-embed   # one-time corpus embedding
//	go run ./cmd/harness --mode=dev --option=1
//	go run ./cmd/harness --mode=dev --option=2 --limit=10
//
// Downloads and embeddings are cached under --cache. Embeddings are keyed by
// the hash of the exact text embedded, so they are paid for once; any run
// that would need new (paid) embeddings prints an estimate and stops unless
// --confirm-embed is given.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"math"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"syscall"
	"time"

	"jev/internal/datasets"
	"jev/internal/embeddings"
	"jev/internal/evaluator"
	"jev/internal/graphmodel"
	"jev/internal/jev"
	"jev/internal/judge"
	"jev/internal/llm"
	"jev/internal/retrieval"
)

var reportKs = []int{1, 5, 10}

type config struct {
	mode        string
	option      int
	dataset     string
	k           int
	candidates  int
	devFraction float64
	limit       int
	minScore    float64
	concurrency int
	maxJEV      int64
	seeds       int
	neighbors   int
	hops        int
	maxCalls    int
	damping     float64
	seedTemp    float64
	unjudged    float64
	dims        int
	cacheDir    string
	envFile     string
	check       bool

	embeddingsDir  string
	resultsDir     string
	confirmEmbed   bool
	maxEmbedTokens int64
	embedOnly      bool
	smoke          bool
	queryTask      string
	queryVariant   string

	musiqueN      int
	inferEdges    string
	gate          bool
	gateTop       int
	gateThreshold float64
	probeBank     bool
	notebookProbe bool
	llmModel      string
	maxLLM        int64
}

func parseFlags() config {
	var c config
	flag.StringVar(&c.mode, "mode", "dev", "split to run: dev (tune thresholds) or test (frozen thresholds only)")
	flag.IntVar(&c.option, "option", 1, "retrieval option 1..5 (see plan.md section 6)")
	flag.StringVar(&c.dataset, "dataset", "koblex", "benchmark: koblex (statutes, explicit citations) or musique (Wikipedia paragraphs, inferred links, unanswerable questions)")
	flag.IntVar(&c.musiqueN, "musique-n", 150, "musique: answerable and unanswerable questions sampled (each) to build the memory")
	flag.StringVar(&c.inferEdges, "infer-edges", "both", "musique: inferred connections: mentions, similar or both")
	flag.BoolVar(&c.gate, "gate", false, "after retrieval, ask JEV whether the top passages suffice; abstain if not (1 call/question)")
	flag.IntVar(&c.gateTop, "gate-top", 5, "passages shown to the sufficiency gate")
	flag.Float64Var(&c.gateThreshold, "gate-threshold", 0.5, "abstain when the gate's probability is below this (tune on dev)")
	flag.BoolVar(&c.probeBank, "build-probe-bank", false, "write labeled (query, passage) pairs from the dev split to data/probe and exit")
	flag.BoolVar(&c.notebookProbe, "notebook-probe", false, "musique: plan v2 P1 — measure reach with notebook facts (T1) and write the T2/T3 probe inputs to data/probe, then exit (embeds a few hundred new query texts: needs --confirm-embed)")
	flag.StringVar(&c.llmModel, "llm-model", "gemini-3.8-flash", "option 4: Gemini model used as judge")
	flag.Int64Var(&c.maxLLM, "max-llm-tokens", 1_000_000, "option 4: stop after this many LLM input tokens (gemini-3.8-flash ≈ US$0.375 per million)")
	flag.IntVar(&c.k, "k", 10, "keys returned per question (must be >= the largest reported K)")
	flag.IntVar(&c.candidates, "candidates", 30, "similarity candidates sent to the judge (option 2)")
	flag.Float64Var(&c.devFraction, "dev-fraction", 0.3, "approximate share of questions in the dev split")
	flag.IntVar(&c.limit, "limit", 0, "run only the first N questions of the split (0 = all)")
	flag.Float64Var(&c.minScore, "min-score", 0, "option 1: abstain if the best similarity is below this")
	flag.IntVar(&c.concurrency, "concurrency", 8, "parallel judge calls per question")
	flag.IntVar(&c.seeds, "seeds", 30, "options 3-5: entry points by similarity before graph traversal (30 = option 2 candidates, frozen on dev)")
	flag.IntVar(&c.neighbors, "neighbors", 5, "option 5: connections judged per accepted node")
	flag.IntVar(&c.hops, "hops", 2, "option 5: how many steps the judge navigates from the seeds")
	flag.IntVar(&c.maxCalls, "max-judge-calls", 60, "option 5: judge calls per question (seeds + connections)")
	flag.Float64Var(&c.seedTemp, "seed-temp", 0.05, "options 3-5: seed weight sharpness exp((sim-best)/T); 0 = raw similarity (tuned on dev)")
	flag.Float64Var(&c.unjudged, "unjudged-mult", 1, "options 4-5: multiplier for connections the judge did not score")
	flag.Float64Var(&c.damping, "damping", 0.3, "options 3-5: PPR probability of following a connection (HippoRAG: 0.5; 0.3 tuned on dev)")
	flag.Int64Var(&c.maxJEV, "max-jev-tokens", 3_000_000, "stop the run after this many JEV input tokens (~$0.042 per million; 0 = no cap)")
	flag.StringVar(&c.cacheDir, "cache", ".cache", "directory for downloaded dataset rows (regenerable, not tracked)")
	flag.StringVar(&c.envFile, "env", ".env", "file with API keys (variables already set win)")
	flag.IntVar(&c.dims, "dims", 3072, "dimensions used for retrieval (stored at 3072; 768/1536 derived locally for free)")
	flag.BoolVar(&c.check, "check", false, "validate data and graph only (no API keys, no embeddings)")
	flag.StringVar(&c.embeddingsDir, "embeddings", "data/embeddings", "embedding stores (tracked in git via LFS: they cost money to rebuild)")
	flag.StringVar(&c.resultsDir, "results", "results", "per-run outputs (tracked in git)")
	flag.BoolVar(&c.confirmEmbed, "confirm-embed", false, "allow paid embedding calls for texts not yet cached")
	flag.Int64Var(&c.maxEmbedTokens, "max-embed-tokens", 16_000_000, "estimated-token cap for paid embedding in this run (~US$0.15 per million)")
	flag.BoolVar(&c.embedOnly, "embed-only", false, "embed the corpus and all questions, then exit")
	flag.BoolVar(&c.smoke, "smoke", false, "run the live embedding smoke test, then exit (needs --confirm-embed)")
	flag.StringVar(&c.queryTask, "query-task", "retrieval_query", "task type for question embeddings: retrieval_query or question_answering")
	flag.StringVar(&c.queryVariant, "query-text", "full", "question text to embed: full (background + question) or question")
	flag.Parse()
	return c
}

func main() {
	log.SetFlags(0)
	cfg := parseFlags()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, cfg); err != nil {
		log.Fatalf("harness: %v", err)
	}
}

func run(ctx context.Context, cfg config) error {
	if cfg.mode != "dev" && cfg.mode != "test" {
		return fmt.Errorf("--mode must be dev or test, got %q", cfg.mode)
	}
	if err := loadDotEnv(cfg.envFile); err != nil {
		return fmt.Errorf("reading %s: %w", cfg.envFile, err)
	}

	hf := datasets.NewClient(datasets.Options{Token: os.Getenv("HF_TOKEN")})
	if cfg.check {
		return runCheck(ctx, hf, cfg)
	}
	queryTaskType, ok := queryTasks[cfg.queryTask]
	if !ok {
		return fmt.Errorf("--query-task must be retrieval_query or question_answering, got %q", cfg.queryTask)
	}
	if cfg.queryVariant != "full" && cfg.queryVariant != "question" {
		return fmt.Errorf("--query-text must be full or question, got %q", cfg.queryVariant)
	}
	if cfg.dims < 1 || cfg.dims > storeDims {
		return fmt.Errorf("--dims must be in 1..%d, got %d", storeDims, cfg.dims)
	}

	if cfg.smoke {
		nodes, _, err := loadStatutes(ctx, hf, cfg)
		if err != nil {
			return fmt.Errorf("statutes: %w", err)
		}
		return runSmoke(ctx, cfg, nodes)
	}

	bench, err := loadBenchmark(ctx, hf, cfg)
	if err != nil {
		return err
	}
	nodes := bench.Nodes
	dev, test := splitQuestions(bench.Questions, cfg.devFraction)
	qs := dev
	if cfg.mode == "test" {
		qs = test
	}
	if cfg.limit > 0 && cfg.limit < len(qs) {
		qs = qs[:cfg.limit]
	}
	log.Printf("%s: %d nodes · questions %d total, dev %d, test %d → running %d (%s)",
		bench.Name, len(nodes), len(bench.Questions), len(dev), len(test), len(qs), cfg.mode)

	// Embed the whole memory and every question (all splits, so a later
	// test run never needs to pay again) for the selected query variant.
	docSpace, err := openSpace(ctx, cfg, embeddings.TaskRetrievalDocument)
	if err != nil {
		return err
	}
	querySpace, err := openSpace(ctx, cfg, queryTaskType)
	if err != nil {
		return err
	}
	docTexts := make([]string, len(nodes))
	for i, n := range nodes {
		docTexts[i] = bench.DocText(n)
	}
	qTexts := make([]string, len(bench.Questions))
	for i, q := range bench.Questions {
		qTexts[i] = q.EmbedText
	}
	vecs, err := embedAll(ctx, cfg, []embedJob{
		{name: "corpus", space: docSpace, texts: docTexts},
		{name: "questions", space: querySpace, texts: qTexts},
	})
	if err != nil {
		return err
	}
	if cfg.embedOnly {
		log.Printf("embedding done: corpus store %d vectors, question store %d vectors", docSpace.store.Len(), querySpace.store.Len())
		return nil
	}

	docVecs, err := reduceAll(vecs[0], cfg.dims)
	if err != nil {
		return fmt.Errorf("corpus vectors: %w", err)
	}
	for i, n := range nodes {
		n.Embedding = docVecs[i]
	}
	allQVecs, err := reduceAll(vecs[1], cfg.dims)
	if err != nil {
		return fmt.Errorf("question vectors: %w", err)
	}
	qEmb := make(map[string][]float64, len(bench.Questions))
	for i, q := range bench.Questions {
		qEmb[q.ID] = allQVecs[i]
	}

	if cfg.probeBank {
		_, err := buildProbeBank(bench, dev, qEmb, filepath.Join("data", "probe"))
		return err
	}
	if cfg.notebookProbe {
		return runNotebookProbe(ctx, cfg, bench, qEmb, querySpace)
	}

	edges := bench.Edges(nodes)
	setEdgeWeights(edges)
	log.Printf("graph: %d connections", len(edges))
	r, meter, err := buildRetriever(cfg, nodes, edges)
	if err != nil {
		return err
	}

	outcomes := make([]evaluator.QueryOutcome, 0, len(qs))
	for i, q := range qs {
		start := time.Now()
		res, err := r.Retrieve(ctx, retrieval.Query{Text: q.Text, Embedding: qEmb[q.ID]})
		if err != nil {
			return fmt.Errorf("question %s: %w", q.ID, err)
		}
		outcomes = append(outcomes, evaluator.QueryOutcome{
			ID:            q.ID,
			Expected:      q.Gold,
			Got:           bench.Canon(res.Keys),
			Abstained:     res.Abstained,
			ShouldAbstain: !q.Answerable,
			Sufficiency:   res.Sufficiency,
			Latency:       time.Since(start),
			JudgeCalls:    res.Judged,
			Hops:          q.Hops,
		})
		if (i+1)%20 == 0 {
			log.Printf("  %d/%d", i+1, len(qs))
		}
	}

	report := evaluator.Summarize(outcomes, reportKs)
	printReport(cfg, report, meter, outcomes)

	gateTag := ""
	if cfg.gate {
		gateTag = "-gate"
	}
	runFile := filepath.Join(cfg.resultsDir, fmt.Sprintf("%s-%s-option%d%s-%s.json",
		time.Now().Format("20060102-150405"), bench.Name, cfg.option, gateTag, cfg.mode))
	if err := writeJSON(runFile, outcomes); err != nil {
		return err
	}
	log.Printf("per-question results: %s", runFile)
	return nil
}

// tokenMeter reports a judge's billable tokens.
type tokenMeter struct {
	name         string
	pricePerMTok float64 // input price, US$ per million tokens
	tokens       func() (input, output int64)
}

// buildRetriever returns the retriever for cfg.option, wrapped in the
// sufficiency gate when --gate is set, and the meter of whatever model it pays for.
func buildRetriever(cfg config, nodes []*graphmodel.Node, edges []graphmodel.Edge) (retrieval.Retriever, *tokenMeter, error) {
	var jj *judge.JEVJudge
	newJEV := func() (*judge.JEVJudge, error) {
		if jj != nil {
			return jj, nil
		}
		client, err := jev.NewHTTPClient(jev.Options{})
		if err != nil {
			return nil, err
		}
		jj = judge.NewJEVJudge(client)
		jj.MaxInputTokens = cfg.maxJEV
		return jj, nil
	}
	graphPPR := func(j judge.Judge) *retrieval.GraphPPR {
		return &retrieval.GraphPPR{
			Graph: retrieval.NewGraph(nodes, edges), Judge: j,
			Seeds: cfg.seeds, Neighbors: cfg.neighbors, Hops: cfg.hops, MaxJudgeCalls: cfg.maxCalls,
			Damping: cfg.damping, SeedTemp: cfg.seedTemp, UnjudgedMult: cfg.unjudged, K: cfg.k, Concurrency: cfg.concurrency,
		}
	}

	var r retrieval.Retriever
	var meter *tokenMeter
	switch cfg.option {
	case 1:
		r = &retrieval.PlainVector{Nodes: nodes, K: cfg.k, MinScore: cfg.minScore}
	case 2:
		j, err := newJEV()
		if err != nil {
			return nil, nil, err
		}
		r = &retrieval.JudgedVector{Nodes: nodes, Judge: j, Candidates: cfg.candidates, K: cfg.k, Concurrency: cfg.concurrency}
	case 3:
		r = graphPPR(nil)
	case 4:
		gen, err := llm.NewGemini(context.Background(), llm.Options{Model: cfg.llmModel})
		if err != nil {
			return nil, nil, err
		}
		lj := judge.NewLLMJudge(gen)
		lj.MaxInputTokens = cfg.maxLLM
		r = graphPPR(lj)
		meter = &tokenMeter{name: cfg.llmModel, pricePerMTok: 0.375, tokens: lj.Tokens}
	case 5:
		j, err := newJEV()
		if err != nil {
			return nil, nil, err
		}
		r = graphPPR(j)
	default:
		return nil, nil, fmt.Errorf("--option must be 1..5, got %d", cfg.option)
	}

	if cfg.gate {
		j, err := newJEV()
		if err != nil {
			return nil, nil, err
		}
		byKey := make(map[string]*graphmodel.Node, len(nodes))
		for _, n := range nodes {
			byKey[n.Key] = n
		}
		r = &retrieval.Gate{Inner: r, Checker: j, Nodes: byKey, TopN: cfg.gateTop, Threshold: cfg.gateThreshold}
	}
	if jj != nil && meter == nil {
		meter = &tokenMeter{name: "JEV", pricePerMTok: 0.042, tokens: jj.Tokens}
	} else if jj != nil {
		// Option 4 with the gate: the gate's JEV tokens are small; report the LLM.
		log.Printf("note: the sufficiency gate's JEV tokens are not included in the %s report", meter.name)
	}
	return r, meter, nil
}

// setEdgeWeights gives each citation edge its static strength: the cosine
// similarity between the two articles' embeddings (plan section 4). It
// depends only on the two texts, never on a question.
func setEdgeWeights(edges []graphmodel.Edge) {
	for i := range edges {
		edges[i].Weight = math.Max(evaluator.CosineSimilarity(edges[i].F.Embedding, edges[i].T.Embedding), 0.01)
	}
}

func printReport(cfg config, r evaluator.Report, m *tokenMeter, outcomes []evaluator.QueryOutcome) {
	fmt.Printf("\n== %s · option %d · %s · %d questions (%d answerable) ==\n", cfg.dataset, cfg.option, cfg.mode, r.Queries, r.Answerable)
	ks := make([]int, 0, len(r.RecallAt))
	for k := range r.RecallAt {
		ks = append(ks, k)
	}
	sort.Ints(ks)
	for _, k := range ks {
		fmt.Printf("Recall@%-3d %.3f   complete chain@%-3d %.3f\n", k, r.RecallAt[k], k, r.CompleteAt[k])
	}
	fmt.Printf("MRR        %.3f\n", r.MRR)
	fmt.Printf("Abstention %.3f (%d abstained)\n", r.AbstentionAccuracy, r.Abstained)
	fmt.Printf("Latency    mean %v · p50 %v\n", r.MeanLatency.Round(time.Millisecond), r.P50Latency.Round(time.Millisecond))
	if m != nil {
		in, out := m.tokens()
		fmt.Printf("%-10s %d judge calls · %d input tokens · %d output tokens · ~US$%.3f input\n", m.name, r.JudgeCalls, in, out, float64(in)/1e6*m.pricePerMTok)
		if r.Queries > 0 {
			fmt.Printf("           %.0f input tokens / question\n", float64(in)/float64(r.Queries))
		}
	}

	groups := evaluator.GroupByHops(outcomes)
	hops := make([]int, 0, len(groups))
	for h := range groups {
		hops = append(hops, h)
	}
	sort.Ints(hops)
	fmt.Printf("\nby steps needed   n (ans)   R@5    R@10   chain@10  abstention  judge calls/q\n")
	for _, h := range hops {
		g := evaluator.Summarize(groups[h], []int{5, 10})
		fmt.Printf("  %d             %3d (%3d)   %.3f  %.3f  %.3f     %.3f       %.1f\n",
			h, g.Queries, g.Answerable, g.RecallAt[5], g.RecallAt[10], g.CompleteAt[10], g.AbstentionAccuracy, float64(g.JudgeCalls)/float64(g.Queries))
	}
}
