package web_search

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
)

func TestSerpbaseProviderSearchMapping(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/google/search" {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("X-API-Key"); got != "serp-test" {
			t.Fatalf("X-API-Key = %q", got)
		}
		var request serpbaseSearchRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		if request.Query != "hello" || request.Device != "default" {
			t.Fatalf("request body = %+v", request)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(serpbaseSearchResponse{
			Status: 0, SearchType: "search",
			Organic: []serpbaseOrganicResult{
				{
					Rank: 1, Title: "One", Link: "https://example.com/1",
					Snippet: "first", PublishedAt: "2026-05-01T00:00:00Z",
				},
				// link empty → falls back to the normalized url alias
				{Rank: 2, Title: "Two", URL: "https://example.com/2", Snippet: "  second  "},
				{Rank: 3, Title: "Three", Link: "https://example.com/3", Date: "2026-06-02"},
				{Rank: 4},
			},
		})
	}))
	defer srv.Close()

	p := &SerpbaseProvider{client: srv.Client(), baseURL: srv.URL + "/google/search", apiKey: "serp-test"}
	results, err := p.Search(context.Background(), "hello", 3, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 3 {
		t.Fatalf("len(results) = %d, want 3 (cap + skip empty)", len(results))
	}
	if results[0].URL != "https://example.com/1" || results[0].Snippet != "first" || results[0].Source != "serpbase" {
		t.Fatalf("unexpected first result: %+v", results[0])
	}
	if results[0].PublishedAt == nil || !results[0].PublishedAt.Equal(time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("unexpected date: %v", results[0].PublishedAt)
	}
	if results[1].URL != "https://example.com/2" || results[1].Snippet != "second" {
		t.Fatalf("unexpected url-alias result: %+v", results[1])
	}
	// raw snippet date text still parses when published_at is absent
	if results[2].PublishedAt == nil || !results[2].PublishedAt.Equal(time.Date(2026, 6, 2, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("unexpected raw-date fallback: %v", results[2].PublishedAt)
	}
}

func TestSerpbaseProviderBusinessError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// SerpBase reports auth/credit failures in the body with HTTP 200
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": 1001, "error": "unauthorized", "request_id": "req-1", "credits_charged": 0,
		})
	}))
	defer srv.Close()

	p := &SerpbaseProvider{client: srv.Client(), baseURL: srv.URL, apiKey: "bad"}
	_, err := p.Search(context.Background(), "q", 1, false)
	if err == nil || !strings.Contains(err.Error(), "1001") || !strings.Contains(err.Error(), "unauthorized") {
		t.Fatalf("expected business status error, got %v", err)
	}
}

func TestSerpbaseProviderValidationAndHTTPStatus(t *testing.T) {
	if _, err := NewSerpbaseProvider(types.WebSearchProviderParameters{}); err == nil {
		t.Fatal("expected missing API key error")
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"status":1029,"error":"rate limited"}`))
	}))
	defer srv.Close()
	p := &SerpbaseProvider{client: srv.Client(), baseURL: srv.URL, apiKey: "key"}
	if _, err := p.Search(context.Background(), "q", 1, false); err == nil {
		t.Fatal("expected status error")
	}
	if _, err := p.Search(context.Background(), " ", 1, false); err == nil {
		t.Fatal("expected empty query error")
	}
}
