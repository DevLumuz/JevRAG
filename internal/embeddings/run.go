package embeddings

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"
)

// ErrNotConfirmed is returned when texts need (paid) embedding but the run was
// not explicitly confirmed. Print an Estimate first, then confirm.
var ErrNotConfirmed = errors.New("embeddings: texts need embedding but the run is not confirmed")

// ErrBudget is returned when the next batch would exceed RunOptions.MaxTokens.
// Batches embedded before stopping are kept in the store.
var ErrBudget = errors.New("embeddings: token budget reached")

const (
	defaultBatchSize     = 100 // Gemini batchEmbedContents limit
	defaultCharsPerToken = 4.0 // conservative; measured ~4.35 on KoBLEX
	defaultRetries       = 6
	defaultRetryDelay    = 2 * time.Second
)

// RunOptions controls a paid embedding run.
type RunOptions struct {
	Confirm       bool          // must be true to call the API for missing texts
	MaxTokens     int64         // estimated-token cap for this run; 0 = no cap
	BatchSize     int           // texts per request; 0 → 100
	CharsPerToken float64       // for estimates; 0 → 4.0
	RetryDelay    time.Duration // first retry delay, doubled each time; 0 → 2s
	Log           func(format string, args ...any)
}

func (o RunOptions) withDefaults() RunOptions {
	if o.BatchSize <= 0 {
		o.BatchSize = defaultBatchSize
	}
	if o.CharsPerToken <= 0 {
		o.CharsPerToken = defaultCharsPerToken
	}
	if o.RetryDelay <= 0 {
		o.RetryDelay = defaultRetryDelay
	}
	if o.Log == nil {
		o.Log = func(string, ...any) {}
	}
	return o
}

// Estimate describes what a run would embed.
type Estimate struct {
	Texts    int   // texts requested
	Missing  int   // distinct texts not in the store (to be paid for)
	Requests int   // API calls needed
	Chars    int64 // characters to embed
	Tokens   int64 // estimated tokens to embed
}

// USD returns the estimated cost at pricePerMTok dollars per million tokens.
func (e Estimate) USD(pricePerMTok float64) float64 {
	return float64(e.Tokens) / 1e6 * pricePerMTok
}

// EstimateCost reports what EmbedAll would send to the API for texts.
func EstimateCost(store *Store, texts []string, opts RunOptions) Estimate {
	opts = opts.withDefaults()
	missing := store.Missing(texts)
	e := Estimate{Texts: len(texts), Missing: len(missing)}
	e.Requests = (len(missing) + opts.BatchSize - 1) / opts.BatchSize
	for _, t := range missing {
		e.Chars += int64(len([]rune(t)))
	}
	e.Tokens = estimateTokens(e.Chars, opts.CharsPerToken)
	return e
}

func estimateTokens(chars int64, charsPerToken float64) int64 {
	return int64(math.Ceil(float64(chars) / charsPerToken))
}

// EmbedAll returns one vector per text, in order. Texts already in the store
// are free; missing ones are embedded in batches (only if opts.Confirm), each
// batch persisted to the store as soon as it returns. Only retryable errors
// (rate limits, 5xx, network) are retried.
func EmbedAll(ctx context.Context, client Client, store *Store, texts []string, opts RunOptions) ([][]float64, error) {
	opts = opts.withDefaults()
	missing := store.Missing(texts)
	if len(missing) > 0 && !opts.Confirm {
		return nil, fmt.Errorf("%w (%d texts)", ErrNotConfirmed, len(missing))
	}

	var spent int64
	for start := 0; start < len(missing); start += opts.BatchSize {
		batch := missing[start:min(start+opts.BatchSize, len(missing))]
		var chars int64
		for _, t := range batch {
			chars += int64(len([]rune(t)))
		}
		cost := estimateTokens(chars, opts.CharsPerToken)

		vecs, err := embedWithRetry(ctx, client, batch, opts, cost, &spent)
		if err != nil {
			return nil, fmt.Errorf("embedding texts %d-%d of %d missing: %w", start, start+len(batch), len(missing), err)
		}
		if err := store.Add(batch, vecs); err != nil {
			return nil, err
		}
		if n := start/opts.BatchSize + 1; n%25 == 0 || start+len(batch) == len(missing) {
			opts.Log("  embedded %d/%d texts (~%d tokens so far)", start+len(batch), len(missing), spent)
		}
	}

	out := make([][]float64, len(texts))
	for i, t := range texts {
		v, ok := store.Get(t)
		if !ok {
			return nil, fmt.Errorf("embeddings: text %d missing from store after run", i)
		}
		out[i] = v
	}
	return out, nil
}

// embedWithRetry calls the API, charging the estimated cost of every attempt
// against the budget before sending it.
func embedWithRetry(ctx context.Context, client Client, batch []string, opts RunOptions, cost int64, spent *int64) ([][]float64, error) {
	delay := opts.RetryDelay
	for attempt := 0; ; attempt++ {
		if opts.MaxTokens > 0 && *spent+cost > opts.MaxTokens {
			return nil, fmt.Errorf("%w: ~%d tokens spent, next batch ~%d, cap %d", ErrBudget, *spent, cost, opts.MaxTokens)
		}
		*spent += cost
		vecs, err := client.EmbedBatch(ctx, batch)
		if err == nil {
			return vecs, nil
		}
		if !IsRetryable(err) || attempt >= defaultRetries {
			return nil, err
		}
		opts.Log("  retryable error (attempt %d, waiting %v): %v", attempt+1, delay, err)
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(delay):
		}
		delay *= 2
	}
}
