// Package rerank calls the local cross-encoder server (tools/rerank_server.py,
// bge-reranker-v2-m3) and caches its scores on disk by content hash, so a
// pair is scored once.
package rerank

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"sync"
)

// Client scores (query, passage) pairs.
type Client struct {
	URL   string // e.g. http://127.0.0.1:8765/score
	Model string // cache namespace, e.g. "bge-reranker-v2-m3"
	Batch int    // pairs per request; 0 → 32

	mu    sync.Mutex
	path  string
	cache map[string]float64
	Calls int // pairs actually scored (cache misses)
}

type line struct {
	K string  `json:"k"`
	S float64 `json:"s"`
}

// Open loads (or creates) the cache file.
func Open(url, model, cachePath string) (*Client, error) {
	c := &Client{URL: url, Model: model, path: cachePath, cache: map[string]float64{}}
	f, err := os.Open(cachePath)
	if errors.Is(err, fs.ErrNotExist) {
		return c, os.MkdirAll(filepath.Dir(cachePath), 0o755)
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<16), 1<<20)
	for sc.Scan() {
		var l line
		if err := json.Unmarshal(sc.Bytes(), &l); err != nil {
			return nil, fmt.Errorf("rerank: corrupt cache line: %w", err)
		}
		c.cache[l.K] = l.S
	}
	return c, sc.Err()
}

func (c *Client) key(q, p string) string {
	h := sha256.Sum256([]byte(c.Model + "\x00" + q + "\x00" + p))
	return hex.EncodeToString(h[:16])
}

// Score returns one score per pair, in order.
func (c *Client) Score(ctx context.Context, pairs [][2]string) ([]float64, error) {
	out := make([]float64, len(pairs))
	var miss []int
	c.mu.Lock()
	for i, p := range pairs {
		if s, ok := c.cache[c.key(p[0], p[1])]; ok {
			out[i] = s
		} else {
			miss = append(miss, i)
		}
	}
	c.mu.Unlock()
	batch := c.Batch
	if batch <= 0 {
		batch = 32
	}
	for start := 0; start < len(miss); start += batch {
		idx := miss[start:min(start+batch, len(miss))]
		req := struct {
			Pairs [][2]string `json:"pairs"`
		}{}
		for _, i := range idx {
			req.Pairs = append(req.Pairs, pairs[i])
		}
		body, _ := json.Marshal(req)
		hr, err := http.NewRequestWithContext(ctx, http.MethodPost, c.URL, bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		hr.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(hr)
		if err != nil {
			return nil, fmt.Errorf("rerank: %w (is tools/rerank_server.py running?)", err)
		}
		var r struct {
			Scores []float64 `json:"scores"`
		}
		err = json.NewDecoder(resp.Body).Decode(&r)
		resp.Body.Close()
		if err != nil || len(r.Scores) != len(idx) {
			return nil, fmt.Errorf("rerank: bad response (status %d): %v", resp.StatusCode, err)
		}
		if err := c.store(pairs, idx, r.Scores, out); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func (c *Client) store(pairs [][2]string, idx []int, scores, out []float64) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	f, err := os.OpenFile(c.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	w := bufio.NewWriter(f)
	for j, i := range idx {
		k := c.key(pairs[i][0], pairs[i][1])
		c.cache[k] = scores[j]
		out[i] = scores[j]
		b, _ := json.Marshal(line{K: k, S: scores[j]})
		w.Write(append(b, '\n'))
	}
	c.Calls += len(idx)
	return w.Flush()
}
