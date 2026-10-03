// Package probe is the offline test bench for judge questions. It takes
// labeled (query, passage) pairs from dev splits — gold passages split by
// role, and hard negatives (high-similarity non-gold passages) — runs a
// question set over every pair, and measures how well each question, and a
// learned combination of all of them, separates gold from non-gold.
//
// Everything here is domain-agnostic: the state JEV sees is always
// {query, passage: {title, text}}, whatever the corpus.
package probe

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"

	"jev/internal/jev"
	"jev/internal/judge"
)

// Pair roles.
const (
	RoleFinal        = "final"         // gold: states the answer (last reasoning step)
	RoleBridge       = "bridge"        // gold: an intermediate step
	RoleGold         = "gold"          // gold, role unknown (e.g. statutes)
	RoleHardNegative = "hard-negative" // high-similarity, not gold
	RoleUnanswerable = "unanswerable"  // top passage for a question the memory cannot answer
)

// Pair is one labeled (query, passage) example.
type Pair struct {
	Dataset   string `json:"dataset"`
	QueryID   string `json:"query_id"`
	Query     string `json:"query"`
	Hops      int    `json:"hops"`
	PassageID string `json:"passage_id"`
	Source    string `json:"source"` // title or identifier of the passage's document
	Text      string `json:"text"`
	Role      string `json:"role"`
	Rank      int    `json:"rank"` // position in the embedding search (1-based; 0 = not retrieved)
}

// Gold reports whether the pair is a positive example.
func (p Pair) Gold() bool {
	return p.Role == RoleFinal || p.Role == RoleBridge || p.Role == RoleGold
}

// State is the domain-agnostic state JEV receives for a pair.
type State struct {
	Query   string  `json:"query"`
	Passage Passage `json:"passage"`
}

// Passage is the candidate passage inside State. Title carries the document
// title / heading path / identifier (e.g. "CIVIL ACT / Article. 40 / Capacity").
type Passage struct {
	Title string `json:"title"`
	Text  string `json:"text"`
}

// MaxPassageChars bounds the passage text sent to JEV (large states distract
// the model and cost more; the long tail of very long passages is cut).
const MaxPassageChars = 6000

// StateFor builds the state for a pair.
func StateFor(p Pair) State {
	t := []rune(p.Text)
	if len(t) > MaxPassageChars {
		t = t[:MaxPassageChars]
	}
	return State{Query: p.Query, Passage: Passage{Title: p.Source, Text: string(t)}}
}

// LoadPairs reads a bank file (a JSON array of pairs).
func LoadPairs(path string) ([]Pair, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var ps []Pair
	if err := json.Unmarshal(data, &ps); err != nil {
		return nil, fmt.Errorf("probe: parsing %s: %w", path, err)
	}
	return ps, nil
}

// --- Response cache ---

// Cache stores JEV responses keyed by the hash of (model, state, questions),
// so re-running a question set over the same pairs costs nothing. It is an
// append-only JSONL file.
type Cache struct {
	mu   sync.Mutex
	path string
	m    map[string]*jev.Response
}

type cacheLine struct {
	Key  string        `json:"key"`
	Resp *jev.Response `json:"resp"`
}

// OpenCache loads (or creates) a cache file.
func OpenCache(path string) (*Cache, error) {
	c := &Cache{path: path, m: map[string]*jev.Response{}}
	f, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return c, os.MkdirAll(filepath.Dir(path), 0o755)
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<24)
	for sc.Scan() {
		var l cacheLine
		if err := json.Unmarshal(sc.Bytes(), &l); err != nil {
			return nil, fmt.Errorf("probe: corrupt cache line in %s: %w", path, err)
		}
		c.m[l.Key] = l.Resp
	}
	return c, sc.Err()
}

func (c *Cache) get(k string) (*jev.Response, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	r, ok := c.m[k]
	return r, ok
}

func (c *Cache) put(k string, r *jev.Response) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.m[k] = r
	b, err := json.Marshal(cacheLine{Key: k, Resp: r})
	if err != nil {
		return err
	}
	f, err := os.OpenFile(c.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(append(b, '\n'))
	return err
}

// Get returns a cached response.
func (c *Cache) Get(key string) (*jev.Response, bool) { return c.get(key) }

// Put stores a response.
func (c *Cache) Put(key string, r *jev.Response) error { return c.put(key, r) }

// CacheKey hashes (model, state, questions). Any state that marshals to the
// same JSON shares the entry.
func CacheKey(model string, state any, qs map[string]jev.Question) string {
	b, _ := json.Marshal(struct {
		M string                  `json:"m"`
		S any                     `json:"s"`
		Q map[string]jev.Question `json:"q"`
	}{model, state, qs})
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

// --- Running a question set ---

// Usage totals what a run paid for (cache hits cost nothing).
type Usage struct {
	Calls, CacheHits int
	InputTokens      int64
}

// Features turns a response into named numbers: a noul gives one feature
// (its probability); a choice gives one per label ("id=label" probability);
// a score gives its expected level.
func Features(resp *jev.Response) map[string]float64 {
	f := map[string]float64{}
	for id, a := range resp.Answers {
		switch a.Type {
		case "noul":
			f[id] = a.Noul
		case "choice":
			for label, p := range a.Probabilities {
				f[id+"="+label] = p
			}
		case "score":
			f[id] = a.Score
		}
	}
	return f
}

// Item is one JEV call: a state and the questions asked about it.
type Item struct {
	State     any
	Questions map[string]jev.Question
}

// Run asks the question set about every pair (cached responses are reused)
// and returns one feature map per pair, in order. maxInputTokens caps paid
// input tokens (0 = no cap).
func Run(ctx context.Context, client jev.Client, model string, set *judge.QuestionSet, pairs []Pair,
	cache *Cache, concurrency int, maxInputTokens int64) ([]map[string]float64, Usage, error) {
	items := make([]Item, len(pairs))
	for i, p := range pairs {
		items[i] = Item{State: StateFor(p), Questions: set.Questions}
	}
	resps, usage, err := RunItems(ctx, client, model, items, cache, concurrency, maxInputTokens)
	if err != nil {
		return nil, usage, err
	}
	out := make([]map[string]float64, len(resps))
	for i, r := range resps {
		out[i] = Features(r)
	}
	return out, usage, nil
}

// RunItems makes every call (cached responses are reused) and returns the
// responses in order. maxInputTokens caps paid input tokens (0 = no cap).
func RunItems(ctx context.Context, client jev.Client, model string, items []Item,
	cache *Cache, concurrency int, maxInputTokens int64) ([]*jev.Response, Usage, error) {

	out := make([]*jev.Response, len(items))
	var (
		mu    sync.Mutex
		usage Usage
		first error
		wg    sync.WaitGroup
	)
	sem := make(chan struct{}, max(concurrency, 1))
	// Identical items in one batch are asked once; the copies take the answer.
	firstOf := map[string]int{}
	dup := map[int]int{}
	for i, it := range items {
		key := CacheKey(model, it.State, it.Questions)
		if r, ok := cache.get(key); ok {
			out[i] = r
			usage.CacheHits++
			continue
		}
		if j, ok := firstOf[key]; ok {
			dup[i] = j
			continue
		}
		firstOf[key] = i
		mu.Lock()
		over := maxInputTokens > 0 && usage.InputTokens >= maxInputTokens
		stop := first != nil
		mu.Unlock()
		if over || stop {
			break
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(i int, it Item, key string) {
			defer wg.Done()
			defer func() { <-sem }()
			r, err := client.SystemOne(ctx, it.State, it.Questions)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				if first == nil {
					first = fmt.Errorf("probe: item %d: %w", i, err)
				}
				return
			}
			usage.Calls++
			usage.InputTokens += int64(r.Usage.InputTokens)
			if err := cache.put(key, r); err != nil && first == nil {
				first = err
			}
			out[i] = r
		}(i, it, key)
	}
	wg.Wait()
	if first != nil {
		return nil, usage, first
	}
	for i, j := range dup {
		out[i] = out[j]
		usage.CacheHits++
	}
	for i, r := range out {
		if r == nil {
			return nil, usage, fmt.Errorf("probe: stopped at item %d: input token cap %d reached", i, maxInputTokens)
		}
	}
	return out, usage, nil
}
