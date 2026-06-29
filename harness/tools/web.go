package tools

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"harness/harness/events"
)

// webSearchTool searches the web. It needs the internet, so it fails cleanly
// when offline. It uses the DuckDuckGo instant-answer API, which returns JSON.
func webSearchTool() *Tool {
	return &Tool{
		Name:        "web_search",
		Description: "Search the web for current information. Only works when the machine is online. Use for facts outside the model's knowledge and outside the project.",
		Params:      json.RawMessage(`{"type":"object","properties":{"query":{"type":"string","description":"Search query."},"max_results":{"type":"integer","description":"Number of results.","default":5}},"required":["query"]}`),
		WebSafe:     true,
		Run: func(env Env, args Args) (any, *events.Diff, error) {
			query := args.Str("query")
			if query == "" {
				return nil, nil, fmt.Errorf("query is required")
			}
			max := args.Int("max_results", 5)
			endpoint := "https://api.duckduckgo.com/?format=json&no_html=1&q=" + url.QueryEscape(query)
			req, err := http.NewRequestWithContext(env.Ctx, http.MethodGet, endpoint, nil)
			if err != nil {
				return nil, nil, err
			}
			client := &http.Client{Timeout: 15 * time.Second}
			resp, err := client.Do(req)
			if err != nil {
				return nil, nil, fmt.Errorf("web search failed (are you online?): %w", err)
			}
			defer resp.Body.Close()

			var ddg struct {
				AbstractText  string `json:"AbstractText"`
				AbstractURL   string `json:"AbstractURL"`
				Heading       string `json:"Heading"`
				RelatedTopics []struct {
					Text     string `json:"Text"`
					FirstURL string `json:"FirstURL"`
				} `json:"RelatedTopics"`
			}
			if err := json.NewDecoder(resp.Body).Decode(&ddg); err != nil {
				return nil, nil, fmt.Errorf("decode search response: %w", err)
			}

			type result struct {
				Title   string `json:"title"`
				URL     string `json:"url"`
				Snippet string `json:"snippet"`
			}
			var results []result
			if ddg.AbstractText != "" {
				results = append(results, result{Title: ddg.Heading, URL: ddg.AbstractURL, Snippet: ddg.AbstractText})
			}
			for _, t := range ddg.RelatedTopics {
				if len(results) >= max {
					break
				}
				if t.Text == "" {
					continue
				}
				results = append(results, result{Title: t.Text, URL: t.FirstURL, Snippet: t.Text})
			}
			return map[string]any{"results": results}, nil, nil
		},
	}
}
