package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"harness/harness/events"
)

var reTag = regexp.MustCompile(`<[^>]+>`)

type searchResult struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet"`
}

// webSearchTool searches the web. With BRAVE_API_KEY set it uses the Brave Search
// API (full web results); otherwise it falls back to Wikipedia's search API, which
// needs no key and works for most topics. Needs the internet either way.
func webSearchTool() *Tool {
	return &Tool{
		Name:        "web_search",
		Description: "Search the web for information outside your knowledge or the project. Needs the internet. Returns a list of {title, url, snippet}. If it returns no results, tell the user you could not find it rather than retrying many times.",
		Params:      json.RawMessage(`{"type":"object","properties":{"query":{"type":"string","description":"Search query."},"max_results":{"type":"integer","description":"Number of results.","default":5}},"required":["query"]}`),
		WebSafe:     true,
		Run: func(env Env, args Args) (any, *events.Diff, error) {
			query := args.Str("query")
			if query == "" {
				return nil, nil, fmt.Errorf("query is required")
			}
			max := args.Int("max_results", 5)
			if max <= 0 {
				max = 5
			}
			var results []searchResult
			var err error
			if key := os.Getenv("BRAVE_API_KEY"); key != "" {
				results, err = braveSearch(env.Ctx, query, max, key)
			} else {
				results, err = wikiSearch(env.Ctx, query, max)
			}
			if err != nil {
				return nil, nil, fmt.Errorf("web search failed (are you online?): %w", err)
			}
			out := map[string]any{"query": query, "results": results}
			if len(results) == 0 {
				out["note"] = "no results found for this query"
			}
			return out, nil, nil
		},
	}
}

func httpGetJSON(ctx context.Context, endpoint string, headers map[string]string, v any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "local-pilot/1.0")
	for k, val := range headers {
		req.Header.Set(k, val)
	}
	resp, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("search returned %s", resp.Status)
	}
	return json.NewDecoder(resp.Body).Decode(v)
}

// wikiSearch queries Wikipedia's search API (no key required).
func wikiSearch(ctx context.Context, query string, max int) ([]searchResult, error) {
	endpoint := fmt.Sprintf("https://en.wikipedia.org/w/api.php?action=query&list=search&format=json&srlimit=%d&srsearch=%s", max, url.QueryEscape(query))
	var body struct {
		Query struct {
			Search []struct {
				Title   string `json:"title"`
				Snippet string `json:"snippet"`
			} `json:"search"`
		} `json:"query"`
	}
	if err := httpGetJSON(ctx, endpoint, nil, &body); err != nil {
		return nil, err
	}
	var results []searchResult
	for _, s := range body.Query.Search {
		results = append(results, searchResult{
			Title:   s.Title,
			URL:     "https://en.wikipedia.org/wiki/" + url.PathEscape(strings.ReplaceAll(s.Title, " ", "_")),
			Snippet: cleanHTML(s.Snippet),
		})
	}
	return results, nil
}

// braveSearch queries the Brave Search API (requires BRAVE_API_KEY).
func braveSearch(ctx context.Context, query string, max int, key string) ([]searchResult, error) {
	endpoint := fmt.Sprintf("https://api.search.brave.com/res/v1/web/search?count=%d&q=%s", max, url.QueryEscape(query))
	var body struct {
		Web struct {
			Results []struct {
				Title       string `json:"title"`
				URL         string `json:"url"`
				Description string `json:"description"`
			} `json:"results"`
		} `json:"web"`
	}
	headers := map[string]string{"Accept": "application/json", "X-Subscription-Token": key}
	if err := httpGetJSON(ctx, endpoint, headers, &body); err != nil {
		return nil, err
	}
	var results []searchResult
	for _, r := range body.Web.Results {
		results = append(results, searchResult{Title: r.Title, URL: r.URL, Snippet: cleanHTML(r.Description)})
	}
	return results, nil
}

func cleanHTML(s string) string {
	return strings.TrimSpace(html.UnescapeString(reTag.ReplaceAllString(s, "")))
}
