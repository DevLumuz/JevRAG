// Package datasets provides a client for the HuggingFace Datasets Server API
// (datasets-server.huggingface.co). It fetches rows from any public dataset
// without depending on Python.
package datasets

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
)

const (
	defaultBaseURL = "https://datasets-server.huggingface.co"
	maxLength      = 100 // HF API hard limit per request
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
	BaseURL    string       // overrides the default HF endpoint (useful for tests)
	Token      string       // optional HF bearer token; increases rate limits
	HTTPClient *http.Client // optional; defaults to http.DefaultClient
}

// Client talks to the HuggingFace Datasets Server.
type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client
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
	return &Client{
		baseURL:    base,
		token:      opts.Token,
		httpClient: hc,
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

	url := fmt.Sprintf("%s/rows?dataset=%s&config=%s&split=%s&offset=%s&length=%s",
		c.baseURL,
		dataset,
		config,
		split,
		strconv.Itoa(offset),
		strconv.Itoa(length),
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
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
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("datasets: HTTP %d from %s", resp.StatusCode, url)
	}

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
