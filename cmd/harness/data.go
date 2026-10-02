package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"

	"jev/internal/datasets"
	"jev/internal/embeddings"
	"jev/internal/extract/legal"
	"jev/internal/graphmodel"
)

// KoBLEX lives in two Hugging Face repos, each with a single config and split.
const (
	koblexQA            = "JihyungL/KoBLEX-koblex"
	koblexQASplit       = "test"
	koblexStatutes      = "JihyungL/KoBLEX-statute-eng"
	koblexStatutesSplit = "corpus"
	hfConfig            = "default"

	embedBatchSize = 100 // Gemini batch embedding limit per request
)

// loadDotEnv sets KEY=VALUE pairs from path without overriding variables that
// are already set. A missing file is not an error.
func loadDotEnv(path string) error {
	f, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k, v = strings.TrimSpace(k), strings.Trim(strings.TrimSpace(v), `"'`)
		if _, set := os.LookupEnv(k); !set {
			os.Setenv(k, v)
		}
	}
	return sc.Err()
}

// cachedRows returns all rows of a HF dataset split, reading them from
// cacheFile when present and downloading (then caching) them otherwise.
func cachedRows(ctx context.Context, hf *datasets.Client, cacheFile, dataset, config, split string) ([]map[string]any, error) {
	var rows []map[string]any
	if ok, err := readJSON(cacheFile, &rows); ok || err != nil {
		return rows, err
	}
	log.Printf("downloading %s (%s/%s)...", dataset, config, split)
	rows, err := hf.AllRows(ctx, dataset, config, split)
	if err != nil {
		return nil, err
	}
	return rows, writeJSON(cacheFile, rows)
}

// loadQuestions returns all KoBLEX questions.
func loadQuestions(ctx context.Context, hf *datasets.Client, cfg config) ([]*datasets.KoBLEXRow, error) {
	raw, err := cachedRows(ctx, hf, filepath.Join(cfg.cacheDir, "koblex-qa.rows.json"), koblexQA, hfConfig, koblexQASplit)
	if err != nil {
		return nil, err
	}
	qs := make([]*datasets.KoBLEXRow, 0, len(raw))
	for i, r := range raw {
		q, err := datasets.ParseKoBLEXRow(r)
		if err != nil {
			return nil, fmt.Errorf("question row %d: %w", i, err)
		}
		qs = append(qs, q)
	}
	return qs, nil
}

// loadStatutes returns the statute nodes and citation edges, without
// embeddings.
func loadStatutes(ctx context.Context, hf *datasets.Client, cfg config) ([]*graphmodel.Node, []graphmodel.Edge, error) {
	raw, err := cachedRows(ctx, hf, filepath.Join(cfg.cacheDir, "koblex-statutes.rows.json"), koblexStatutes, hfConfig, koblexStatutesSplit)
	if err != nil {
		return nil, nil, err
	}
	statutes := make([]*datasets.StatuteRow, 0, len(raw))
	for i, r := range raw {
		s, err := datasets.ParseStatuteRow(r)
		if err != nil {
			return nil, nil, fmt.Errorf("statute row %d: %w", i, err)
		}
		statutes = append(statutes, s)
	}
	nodes := legal.StatutesToNodes(statutes)
	return nodes, legal.BuildEdges(nodes, statutes), nil
}

// loadGraph returns the statute nodes (with embeddings) and citation edges,
// building and caching them on first use. Embedding the corpus is the one
// expensive step, so the result is persisted with graphmodel.SaveGraph.
func loadGraph(ctx context.Context, hf *datasets.Client, emb embeddings.Client, cfg config) ([]*graphmodel.Node, []graphmodel.Edge, error) {
	graphFile := filepath.Join(cfg.cacheDir, "koblex.graph.json")
	if _, err := os.Stat(graphFile); err == nil {
		return graphmodel.LoadGraph(graphFile)
	}

	nodes, edges, err := loadStatutes(ctx, hf, cfg)
	if err != nil {
		return nil, nil, err
	}

	texts := make([]string, len(nodes))
	for i, n := range nodes {
		texts[i] = n.Content
	}
	log.Printf("embedding %d statutes (%d requests)...", len(texts), (len(texts)+embedBatchSize-1)/embedBatchSize)
	vecs, err := embedAll(ctx, emb, texts)
	if err != nil {
		return nil, nil, err
	}
	for i, n := range nodes {
		n.Embedding = vecs[i]
	}
	return nodes, edges, graphmodel.SaveGraph(graphFile, nodes, edges)
}

// queryEmbeddings returns an embedding per question ID, computing only the
// ones missing from the cache.
func queryEmbeddings(ctx context.Context, emb embeddings.Client, cfg config, qs []*datasets.KoBLEXRow) (map[string][]float64, error) {
	cacheFile := filepath.Join(cfg.cacheDir, "koblex-qa.embeddings.json")
	cache := map[string][]float64{}
	if _, err := readJSON(cacheFile, &cache); err != nil {
		return nil, err
	}

	var missing []*datasets.KoBLEXRow
	for _, q := range qs {
		if _, ok := cache[q.ID]; !ok {
			missing = append(missing, q)
		}
	}
	if len(missing) == 0 {
		return cache, nil
	}

	texts := make([]string, len(missing))
	for i, q := range missing {
		texts[i] = queryText(q)
	}
	log.Printf("embedding %d questions...", len(texts))
	vecs, err := embedAll(ctx, emb, texts)
	if err != nil {
		return nil, err
	}
	for i, q := range missing {
		cache[q.ID] = vecs[i]
	}
	return cache, writeJSON(cacheFile, cache)
}

// queryText is what gets embedded and judged for a question: the scenario
// (background) plus the question itself, both in English.
func queryText(q *datasets.KoBLEXRow) string {
	if q.BackgroundEng == "" {
		return q.QuestionEng
	}
	return q.BackgroundEng + "\n\n" + q.QuestionEng
}

func embedAll(ctx context.Context, emb embeddings.Client, texts []string) ([][]float64, error) {
	out := make([][]float64, 0, len(texts))
	for start := 0; start < len(texts); start += embedBatchSize {
		end := min(start+embedBatchSize, len(texts))
		vecs, err := emb.EmbedBatch(ctx, texts[start:end])
		if err != nil {
			return nil, fmt.Errorf("embedding batch %d-%d: %w", start, end, err)
		}
		out = append(out, vecs...)
	}
	return out, nil
}

// readJSON decodes path into v. It reports false (and no error) if the file
// does not exist.
func readJSON(path string, v any) (bool, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if err := json.Unmarshal(data, v); err != nil {
		return false, fmt.Errorf("parsing %s: %w", path, err)
	}
	return true, nil
}

func writeJSON(path string, v any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
