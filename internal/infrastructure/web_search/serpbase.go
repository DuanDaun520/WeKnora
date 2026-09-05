package web_search

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

const (
	defaultSerpbaseSearchURL = "https://api.serpbase.dev/google/search"
	defaultSerpbaseTimeout   = 30 * time.Second
	defaultSerpbaseResults   = 10
	maxSerpbaseResults       = 50
	maxSerpbaseResponseBytes = 4 << 20
	// SerpBase's own device routing default; explicit like the docs example.
	defaultSerpbaseDevice = "default"
)

// SerpbaseProvider implements web search using the SerpBase Google Search API
// (POST https://api.serpbase.dev/google/search, X-API-Key auth).
type SerpbaseProvider struct {
	client  *http.Client
	baseURL string
	apiKey  string
	hl      string
	gl      string
}

func NewSerpbaseProvider(params types.WebSearchProviderParameters) (interfaces.WebSearchProvider, error) {
	if err := ValidateSerpbaseParameters(params); err != nil {
		return nil, err
	}
	client, err := NewSearchHTTPClient(defaultSerpbaseTimeout, params.ProxyURL)
	if err != nil {
		return nil, err
	}
	return &SerpbaseProvider{
		client:  client,
		baseURL: defaultSerpbaseSearchURL,
		apiKey:  strings.TrimSpace(params.APIKey),
		hl:      strings.TrimSpace(params.ExtraConfig["hl"]),
		gl:      strings.TrimSpace(params.ExtraConfig["gl"]),
	}, nil
}

func ValidateSerpbaseParameters(params types.WebSearchProviderParameters) error {
	if strings.TrimSpace(params.APIKey) == "" {
		return fmt.Errorf("API key is required for SerpBase provider")
	}
	return nil
}

func (p *SerpbaseProvider) Name() string { return "serpbase" }

func (p *SerpbaseProvider) Search(ctx context.Context, query string, maxResults int, includeDate bool) ([]*types.WebSearchResult, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, fmt.Errorf("query is empty")
	}
	if maxResults <= 0 {
		maxResults = defaultSerpbaseResults
	}
	if maxResults > maxSerpbaseResults {
		maxResults = maxSerpbaseResults
	}

	body, err := json.Marshal(serpbaseSearchRequest{
		Query: query, HL: p.hl, GL: p.gl, Page: 1, Device: defaultSerpbaseDevice,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to marshal SerpBase request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create SerpBase request: %w", err)
	}
	req.Header.Set("X-API-Key", p.apiKey)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")

	logger.Infof(ctx, "[WebSearch][SerpBase] query=%q maxResults=%d", query, maxResults)
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute SerpBase request: %w", err)
	}
	defer resp.Body.Close()
	respBody, err := readSerpbaseResponseBody(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, serpbaseHTTPError(resp.StatusCode, respBody)
	}

	var response serpbaseSearchResponse
	if err := json.Unmarshal(respBody, &response); err != nil {
		return nil, fmt.Errorf("failed to unmarshal SerpBase response: %w", err)
	}
	// SerpBase reports business errors in the body (status != 0) even when
	// the transport succeeded — e.g. 1001 unauthorized, 1020 out of credits.
	if response.Status != 0 {
		return nil, fmt.Errorf("SerpBase API returned status %d: %s", response.Status, response.Error)
	}

	results := make([]*types.WebSearchResult, 0, len(response.Organic))
	for _, item := range response.Organic {
		link := strings.TrimSpace(item.Link)
		if link == "" {
			link = strings.TrimSpace(item.URL)
		}
		if strings.TrimSpace(item.Title) == "" && link == "" {
			continue
		}
		result := &types.WebSearchResult{
			Title:   item.Title,
			URL:     link,
			Snippet: strings.TrimSpace(item.Snippet),
			Source:  "serpbase",
		}
		if includeDate {
			if publishedAt, ok := parseSerpbaseDate(item.PublishedAt, item.Date); ok {
				result.PublishedAt = &publishedAt
			}
		}
		results = append(results, result)
		if len(results) >= maxResults {
			break
		}
	}
	logger.Infof(ctx, "[WebSearch][SerpBase] returned %d results", len(results))
	return results, nil
}

func readSerpbaseResponseBody(reader io.Reader) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(reader, maxSerpbaseResponseBytes+1))
	if err != nil {
		return nil, fmt.Errorf("failed to read SerpBase response: %w", err)
	}
	if len(body) > maxSerpbaseResponseBytes {
		return nil, fmt.Errorf("SerpBase response exceeds %d bytes", maxSerpbaseResponseBytes)
	}
	return body, nil
}

func serpbaseHTTPError(statusCode int, body []byte) error {
	var apiError struct {
		Error string `json:"error"`
	}
	detail := ""
	if json.Unmarshal(body, &apiError) == nil {
		detail = strings.TrimSpace(apiError.Error)
	}
	if detail == "" {
		detail = strings.TrimSpace(string(body))
		if len(detail) > 4096 {
			detail = detail[:4096]
		}
	}
	if detail == "" {
		return fmt.Errorf("SerpBase API returned status %d", statusCode)
	}
	return fmt.Errorf("SerpBase API returned status %d: %s", statusCode, detail)
}

// parseSerpbaseDate prefers the normalized published_at alias and falls back
// to the raw snippet date text; neither is guaranteed to be present.
func parseSerpbaseDate(values ...string) (time.Time, bool) {
	layouts := []string{time.RFC3339Nano, "2006-01-02 15:04:05", "2006-01-02"}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		for _, layout := range layouts {
			if parsed, err := time.Parse(layout, value); err == nil {
				return parsed, true
			}
		}
	}
	return time.Time{}, false
}

type serpbaseSearchRequest struct {
	Query  string `json:"q"`
	HL     string `json:"hl,omitempty"`
	GL     string `json:"gl,omitempty"`
	Page   int    `json:"page,omitempty"`
	Device string `json:"device,omitempty"`
}

type serpbaseSearchResponse struct {
	Status     int                     `json:"status"`
	Error      string                  `json:"error"`
	RequestID  string                  `json:"request_id"`
	SearchType string                  `json:"search_type"`
	Organic    []serpbaseOrganicResult `json:"organic"`
}

type serpbaseOrganicResult struct {
	Rank        int    `json:"rank"`
	Title       string `json:"title"`
	Link        string `json:"link"`
	URL         string `json:"url"`
	Snippet     string `json:"snippet"`
	Date        string `json:"date"`
	PublishedAt string `json:"published_at"`
}
