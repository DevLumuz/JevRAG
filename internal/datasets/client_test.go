package datasets_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"jev/internal/datasets"
)

func TestRows_HappyPath(t *testing.T) {
	// Fake HF Datasets Server response matching the real envelope.
	resp := datasets.HFResponse{
		NumRowsTotal:  226,
		NumRowsPerPage: 100,
		Partial:        false,
		Rows: []datasets.HFRowWrapper{
			{
				RowIdx: 0,
				Row: map[string]any{
					"id":       "qa_19_1hop_28",
					"question": "보험자가 보험사고 발생 전에 피보험자의 고지의무 위반을 발견하면 무엇을 할 수 있나요?",
					"n_hops":   float64(1),
				},
			},
			{
				RowIdx: 1,
				Row: map[string]any{
					"id":       "qa_90_2hop_147",
					"question": "다른 질문",
					"n_hops":   float64(2),
				},
			},
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify query parameters.
		q := r.URL.Query()
		if q.Get("dataset") != "JihyungL/KoBLEX-koblex" {
			t.Errorf("dataset = %q, want JihyungL/KoBLEX-koblex", q.Get("dataset"))
		}
		if q.Get("config") != "default" {
			t.Errorf("config = %q, want default", q.Get("config"))
		}
		if q.Get("split") != "test" {
			t.Errorf("split = %q, want test", q.Get("split"))
		}
		if q.Get("offset") != "0" {
			t.Errorf("offset = %q, want 0", q.Get("offset"))
		}
		if q.Get("length") != "100" {
			t.Errorf("length = %q, want 100", q.Get("length"))
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	c := datasets.NewClient(datasets.Options{
		BaseURL: srv.URL,
	})

	result, err := c.Rows(context.Background(), "JihyungL/KoBLEX-koblex", "default", "test", 0, 100)
	if err != nil {
		t.Fatalf("Rows() error: %v", err)
	}

	if result.NumRowsTotal != 226 {
		t.Errorf("NumRowsTotal = %d, want 226", result.NumRowsTotal)
	}
	if len(result.Rows) != 2 {
		t.Fatalf("len(Rows) = %d, want 2", len(result.Rows))
	}
	if result.Rows[0].Row["id"] != "qa_19_1hop_28" {
		t.Errorf("Rows[0].Row[id] = %v, want qa_19_1hop_28", result.Rows[0].Row["id"])
	}
}

func TestRows_LengthClamped(t *testing.T) {
	// The HF API caps length at 100. The client must clamp silently.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("length") != "100" {
			t.Errorf("length should be clamped to 100, got %q", r.URL.Query().Get("length"))
		}
		json.NewEncoder(w).Encode(datasets.HFResponse{})
	}))
	defer srv.Close()

	c := datasets.NewClient(datasets.Options{BaseURL: srv.URL})
	_, err := c.Rows(context.Background(), "any", "default", "test", 0, 500)
	if err != nil {
		t.Fatalf("Rows() error: %v", err)
	}
}

func TestRows_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "rate limited", http.StatusTooManyRequests)
	}))
	defer srv.Close()

	c := datasets.NewClient(datasets.Options{BaseURL: srv.URL})
	_, err := c.Rows(context.Background(), "any", "default", "test", 0, 10)
	if err == nil {
		t.Fatal("expected error on HTTP 429, got nil")
	}
}

func TestRows_AuthHeader(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth != "Bearer test-token-123" {
			t.Errorf("Authorization = %q, want Bearer test-token-123", auth)
		}
		json.NewEncoder(w).Encode(datasets.HFResponse{})
	}))
	defer srv.Close()

	c := datasets.NewClient(datasets.Options{
		BaseURL: srv.URL,
		Token:   "test-token-123",
	})
	_, err := c.Rows(context.Background(), "any", "default", "test", 0, 10)
	if err != nil {
		t.Fatalf("Rows() error: %v", err)
	}
}

func TestAllRows_Pagination(t *testing.T) {
	// Simulates a dataset with 3 rows, server returns max 2 per page.
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		offset := r.URL.Query().Get("offset")

		var resp datasets.HFResponse
		resp.NumRowsTotal = 3
		resp.NumRowsPerPage = 2

		switch offset {
		case "0":
			resp.Rows = []datasets.HFRowWrapper{
				{RowIdx: 0, Row: map[string]any{"id": "row_0"}},
				{RowIdx: 1, Row: map[string]any{"id": "row_1"}},
			}
		case "2":
			resp.Rows = []datasets.HFRowWrapper{
				{RowIdx: 2, Row: map[string]any{"id": "row_2"}},
			}
		default:
			t.Errorf("unexpected offset: %s", offset)
		}

		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	c := datasets.NewClient(datasets.Options{BaseURL: srv.URL})
	rows, err := c.AllRows(context.Background(), "any", "default", "test")
	if err != nil {
		t.Fatalf("AllRows() error: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("len(rows) = %d, want 3", len(rows))
	}
	if rows[2]["id"] != "row_2" {
		t.Errorf("rows[2][id] = %v, want row_2", rows[2]["id"])
	}
	if callCount != 2 {
		t.Errorf("expected 2 HTTP calls for pagination, got %d", callCount)
	}
}
