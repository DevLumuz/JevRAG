// Package datasets provides a client for the HuggingFace Datasets Server API
// (datasets-server.huggingface.co). It fetches rows from any public dataset
// without depending on Python.
package datasets

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

const (
	defaultBaseURL = "https://datasets-server.huggingface.co"
	maxLength      = 100 // HF API hard limit per request
	defaultRetries = 6
	defaultBackoff = 2 * time.Second
	maxBackoff     = time.Minute
)

// HFResponse is the top-level JSON envelope returned by the /rows endpoint.
type HFResponse struct {
	Rows           []HFRowWrapper `json:"rows"`
	NumRowsTotal   int            `json:"num_rows_total"`
	NumRowsPerPage int            `json:"num_rows_per_page"`
	Partial        bool           `json:"partial"`
}

// HFRowWrapper wraps a single row with its index and truncation info.
type HFRowWrapper struct {
	RowIdx         int            `json:"row_idx"`
	Row            map[string]any `json:"row"`
	TruncatedCells []string       `json:"truncated_cells"`
}

// Options configures the datasets client.
type Options struct {
	BaseURL    string        // overrides the default HF endpoint (useful for tests)
	Token      string        // optional HF bearer token; increases rate limits
	HTTPClient *http.Client  // optional; defaults to http.DefaultClient
	MaxRetries int           // retries on 429/5xx; 0 → 6, negative disables
	Backoff    time.Duration // first retry delay, doubled each time; 0 → 2s
}

// Client talks to the HuggingFace Datasets Server.
type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client
	maxRetries int
	backoff    time.Duration
}

// NewClient creates a datasets client with the given options.
func NewClient(opts Options) *Client {
	base := opts.BaseURL
	if base == "" {
		base = defaultBaseURL
	}
	hc := opts.HTTPClient
	if hc == nil {
		hc = http.DefaultClient
	}
	retries := opts.MaxRetries
	if retries == 0 {
		retries = defaultRetries
	}
	backoff := opts.Backoff
	if backoff == 0 {
		backoff = defaultBackoff
	}
	return &Client{
		baseURL:    base,
		token:      opts.Token,
		httpClient: hc,
		maxRetries: max(retries, 0),
		backoff:    backoff,
	}
}

// Rows fetches a page of rows from the given dataset/config/split.
// Length is silently clamped to 100 (the API maximum).
func (c *Client) Rows(ctx context.Context, dataset, config, split string, offset, length int) (*HFResponse, error) {
	if length > maxLength {
		length = maxLength
	}
	if length < 1 {
		length = 1
	}

	q := url.Values{}
	q.Set("dataset", dataset)
	q.Set("config", config)
	q.Set("split", split)
	q.Set("offset", strconv.Itoa(offset))
	q.Set("length", strconv.Itoa(length))
	u := c.baseURL + "/rows?" + q.Encode()

	resp, err := c.get(ctx, u)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result HFResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("datasets: decoding response: %w", err)
	}
	return &result, nil
}

// AllRows fetches every row from a dataset by paginating automatically.
// Returns the raw row maps in order.
func (c *Client) AllRows(ctx context.Context, dataset, config, split string) ([]map[string]any, error) {
	var all []map[string]any
	offset := 0

	for {
		page, err := c.Rows(ctx, dataset, config, split, offset, maxLength)
		if err != nil {
			return nil, err
		}
		for _, rw := range page.Rows {
			all = append(all, rw.Row)
		}
		offset += len(page.Rows)
		if offset >= page.NumRowsTotal || len(page.Rows) == 0 {
			break
		}
	}
	return all, nil
}

// get performs a GET, retrying rate limits (429) and server errors (5xx) with
// exponential backoff. A Retry-After header in seconds overrides the backoff.
func (c *Client) get(ctx context.Context, u string) (*http.Response, error) {
	delay := c.backoff
	for attempt := 0; ; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			return nil, fmt.Errorf("datasets: creating request: %w", err)
		}
		if c.token != "" {
			req.Header.Set("Authorization", "Bearer "+c.token)
		}

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("datasets: request failed: %w", err)
		}
		if resp.StatusCode == http.StatusOK {
			return resp, nil
		}
		resp.Body.Close()

		retryable := resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500
		if !retryable || attempt >= c.maxRetries {
			return nil, fmt.Errorf("datasets: HTTP %d from %s", resp.StatusCode, u)
		}

		wait := delay
		if s, err := strconv.Atoi(resp.Header.Get("Retry-After")); err == nil {
			wait = time.Duration(s) * time.Second
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(wait):
		}
		delay = min(delay*2, maxBackoff)
	}
}
