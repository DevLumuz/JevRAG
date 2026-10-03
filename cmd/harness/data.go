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
	rows, err := downloadRows(ctx, hf, cacheFile+".pages", dataset, config, split)
	if err != nil {
		return nil, err
	}
	if err := writeJSON(cacheFile, rows); err != nil {
		return nil, err
	}
	return rows, os.RemoveAll(cacheFile + ".pages")
}

// downloadRows pages through a split, saving each page under pagesDir as it
// arrives. Big splits (44k statutes = 443 pages) hit HF rate limits; an
// interrupted download resumes from the pages already on disk.
func downloadRows(ctx context.Context, hf *datasets.Client, pagesDir, dataset, config, split string) ([]map[string]any, error) {
	const pageSize = 100
	var all []map[string]any
	for offset, total := 0, -1; total < 0 || offset < total; {
		pageFile := filepath.Join(pagesDir, fmt.Sprintf("%07d.json", offset))
		var page datasets.HFResponse
		ok, err := readJSON(pageFile, &page)
		if err != nil {
			return nil, err
		}
		if !ok {
			p, err := hf.Rows(ctx, dataset, config, split, offset, pageSize)
			if err != nil {
				return nil, fmt.Errorf("offset %d: %w", offset, err)
			}
			page = *p
			if err := writeJSON(pageFile, page); err != nil {
				return nil, err
			}
		}
		if len(page.Rows) == 0 {
			break
		}
		for _, rw := range page.Rows {
			all = append(all, rw.Row)
		}
		total = page.NumRowsTotal
		offset += len(page.Rows)
		if (offset/pageSize)%50 == 0 {
			log.Printf("  %d/%d rows", offset, total)
		}
	}
	return all, nil
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

// writeJSON writes v to path atomically (temp file + rename), so a crash
// never leaves a half-written cache file behind.
func writeJSON(path string, v any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// sampledRows downloads `pages` pages of 100 rows spread evenly over a split
// (big splits need not be downloaded whole) and caches them in one file.
func sampledRows(ctx context.Context, hf *datasets.Client, cacheFile, dataset, config, split string, pages int) ([]map[string]any, error) {
	var rows []map[string]any
	if ok, err := readJSON(cacheFile, &rows); ok || err != nil {
		return rows, err
	}
	first, err := hf.Rows(ctx, dataset, config, split, 0, 1)
	if err != nil {
		return nil, err
	}
	total := first.NumRowsTotal
	log.Printf("sampling %d pages of %s (%s/%s, %d rows)...", pages, dataset, config, split, total)
	for i := 0; i < pages; i++ {
		offset := (total / pages * i) / 100 * 100
		p, err := hf.Rows(ctx, dataset, config, split, offset, 100)
		if err != nil {
			return nil, fmt.Errorf("offset %d: %w", offset, err)
		}
		for _, rw := range p.Rows {
			rows = append(rows, rw.Row)
		}
	}
	return rows, writeJSON(cacheFile, rows)
}
