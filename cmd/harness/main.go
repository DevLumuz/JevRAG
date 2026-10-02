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
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"syscall"
	"time"

	"jev/internal/datasets"
	"jev/internal/embeddings"
	"jev/internal/evaluator"
	"jev/internal/extract/legal"
	"jev/internal/graphmodel"
	"jev/internal/jev"
	"jev/internal/judge"
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
	dims        int
	cacheDir    string
	envFile     string
	check       bool

	confirmEmbed   bool
	maxEmbedTokens int64
	embedOnly      bool
	smoke          bool
	queryTask      string
	queryVariant   string
}

func parseFlags() config {
	var c config
	flag.StringVar(&c.mode, "mode", "dev", "split to run: dev (tune thresholds) or test (frozen thresholds only)")
	flag.IntVar(&c.option, "option", 1, "retrieval option 1..5 (see plan.md section 6)")
	flag.StringVar(&c.dataset, "dataset", "koblex", "dataset (only koblex for now)")
	flag.IntVar(&c.k, "k", 10, "keys returned per question (must be >= the largest reported K)")
	flag.IntVar(&c.candidates, "candidates", 30, "similarity candidates sent to the judge (option 2)")
	flag.Float64Var(&c.devFraction, "dev-fraction", 0.3, "approximate share of questions in the dev split")
	flag.IntVar(&c.limit, "limit", 0, "run only the first N questions of the split (0 = all)")
	flag.Float64Var(&c.minScore, "min-score", 0, "option 1: abstain if the best similarity is below this")
	flag.IntVar(&c.concurrency, "concurrency", 8, "parallel judge calls per question")
	flag.Int64Var(&c.maxJEV, "max-jev-tokens", 3_000_000, "stop the run after this many JEV input tokens (~$0.042 per million; 0 = no cap)")
	flag.StringVar(&c.cacheDir, "cache", ".cache", "directory for downloaded data, graph and embeddings")
	flag.StringVar(&c.envFile, "env", ".env", "file with API keys (variables already set win)")
	flag.IntVar(&c.dims, "dims", 3072, "dimensions used for retrieval (stored at 3072; 768/1536 derived locally for free)")
	flag.BoolVar(&c.check, "check", false, "validate data and graph only (no API keys, no embeddings)")
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
	if cfg.dataset != "koblex" {
		return fmt.Errorf("unknown dataset %q", cfg.dataset)
	}
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

	nodes, edges, err := loadStatutes(ctx, hf, cfg)
	if err != nil {
		return fmt.Errorf("statutes: %w", err)
	}
	log.Printf("graph: %d nodes, %d edges", len(nodes), len(edges))
	if cfg.smoke {
		return runSmoke(ctx, cfg, nodes)
	}

	all, err := loadQuestions(ctx, hf, cfg)
	if err != nil {
		return fmt.Errorf("questions: %w", err)
	}
	dev, test := datasets.SplitDevTest(all, cfg.devFraction)
	qs := dev
	if cfg.mode == "test" {
		qs = test
	}
	if cfg.limit > 0 && cfg.limit < len(qs) {
		qs = qs[:cfg.limit]
	}
	log.Printf("questions: %d total, dev %d, test %d → running %d (%s)", len(all), len(dev), len(test), len(qs), cfg.mode)

	// Embed the whole corpus and every question (all splits, so a later
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
		docTexts[i] = docText(n)
	}
	qTexts := make([]string, len(all))
	for i, q := range all {
		qTexts[i] = queryEmbedText(q, cfg.queryVariant)
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
	qEmb := make(map[string][]float64, len(all))
	for i, q := range all {
		qEmb[q.ID] = allQVecs[i]
	}

	r, jevJudge, err := buildRetriever(cfg, nodes)
	if err != nil {
		return err
	}

	outcomes := make([]evaluator.QueryOutcome, 0, len(qs))
	for i, q := range qs {
		start := time.Now()
		res, err := r.Retrieve(ctx, retrieval.Query{Text: queryText(q), Embedding: qEmb[q.ID]})
		if err != nil {
			return fmt.Errorf("question %s: %w", q.ID, err)
		}
		outcomes = append(outcomes, evaluator.QueryOutcome{
			ID:         q.ID,
			Expected:   legal.GoldKeys(q),
			Got:        legal.CanonicalRanking(res.Keys),
			Abstained:  res.Abstained,
			Latency:    time.Since(start),
			JudgeCalls: res.Judged,
		})
		if (i+1)%20 == 0 {
			log.Printf("  %d/%d", i+1, len(qs))
		}
	}

	report := evaluator.Summarize(outcomes, reportKs)
	printReport(cfg, report, jevJudge)

	runFile := filepath.Join(cfg.cacheDir, "runs", fmt.Sprintf("%s-option%d-%s.json", time.Now().Format("20060102-150405"), cfg.option, cfg.mode))
	if err := writeJSON(runFile, outcomes); err != nil {
		return err
	}
	log.Printf("per-question results: %s", runFile)
	return nil
}

// buildRetriever returns the retriever for cfg.option and, when the option
// uses JEV, the judge so its token usage can be reported.
func buildRetriever(cfg config, nodes []*graphmodel.Node) (retrieval.Retriever, *judge.JEVJudge, error) {
	switch cfg.option {
	case 1:
		return &retrieval.PlainVector{Nodes: nodes, K: cfg.k, MinScore: cfg.minScore}, nil, nil
	case 2:
		client, err := jev.NewHTTPClient(jev.Options{})
		if err != nil {
			return nil, nil, err
		}
		j := judge.NewJEVJudge(client)
		j.MaxInputTokens = cfg.maxJEV
		return &retrieval.JudgedVector{
			Nodes: nodes, Judge: j, Candidates: cfg.candidates, K: cfg.k, Concurrency: cfg.concurrency,
		}, j, nil
	case 3, 4, 5:
		return nil, nil, fmt.Errorf("option %d is not implemented yet (plan.md section 13, steps 4-5)", cfg.option)
	}
	return nil, nil, fmt.Errorf("--option must be 1..5, got %d", cfg.option)
}

func printReport(cfg config, r evaluator.Report, j *judge.JEVJudge) {
	fmt.Printf("\n== option %d · %s · %d questions ==\n", cfg.option, cfg.mode, r.Queries)
	ks := make([]int, 0, len(r.RecallAt))
	for k := range r.RecallAt {
		ks = append(ks, k)
	}
	sort.Ints(ks)
	for _, k := range ks {
		fmt.Printf("Recall@%-3d %.3f\n", k, r.RecallAt[k])
	}
	fmt.Printf("MRR        %.3f\n", r.MRR)
	fmt.Printf("Abstention %.3f (%d abstained)\n", r.AbstentionAccuracy, r.Abstained)
	fmt.Printf("Latency    mean %v · p50 %v\n", r.MeanLatency.Round(time.Millisecond), r.P50Latency.Round(time.Millisecond))
	if j != nil {
		in, out := j.Tokens()
		fmt.Printf("JEV        %d calls · %d input tokens · %d output tokens\n", r.JudgeCalls, in, out)
		if r.Queries > 0 {
			fmt.Printf("           %.0f input tokens / question\n", float64(in)/float64(r.Queries))
		}
	}
}
