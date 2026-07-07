package model

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Client struct {
	http *http.Client
}

func NewClient() *Client {
	return &Client{http: &http.Client{Timeout: 10 * time.Minute}}
}

// InstalledModels returns the model tags ollama currently has, via /api/tags.
func (c *Client) InstalledModels(url string) ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(url, "/")+"/api/tags", nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var body struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, err
	}
	names := make([]string, 0, len(body.Models))
	for _, m := range body.Models {
		names = append(names, m.Name)
	}
	return names, nil
}

// Reachable reports whether a backend is up, by probing ollama's /api/version.
func (c *Client) Reachable(url string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(url, "/")+"/api/version", nil)
	if err != nil {
		return false
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

type constrainedRequest struct {
	Model          string         `json:"model"`
	Messages       []Message      `json:"messages"`
	Stream         bool           `json:"stream"`
	Temperature    float64        `json:"temperature"`
	ResponseFormat responseFormat `json:"response_format"`
}

type responseFormat struct {
	Type       string     `json:"type"`
	JSONSchema schemaBlock `json:"json_schema"`
}

type schemaBlock struct {
	Name   string          `json:"name"`
	Schema json.RawMessage `json:"schema"`
}

type fullResponse struct {
	Choices []struct {
		Message      Message `json:"message"`
		FinishReason string  `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		TotalTokens int `json:"total_tokens"`
	} `json:"usage"`
}

type chatRequest struct {
	Model       string          `json:"model"`
	Messages    []Message       `json:"messages"`
	Tools       []ToolDef       `json:"tools,omitempty"`
	Stream      bool            `json:"stream"`
	Temperature float64         `json:"temperature"`
	CachePrompt bool            `json:"cache_prompt,omitempty"`
	JSONSchema  json.RawMessage `json:"json_schema,omitempty"`
}

func (c *Client) Chat(ctx context.Context, url, model string, msgs []Message, defs []ToolDef) (Message, int, error) {
	body := chatRequest{Model: model, Messages: msgs, Tools: defs, Stream: false, Temperature: 0.2}
	buf, err := json.Marshal(body)
	if err != nil {
		return Message{}, 0, err
	}
	endpoint := strings.TrimRight(url, "/") + "/v1/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(buf))
	if err != nil {
		return Message{}, 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return Message{}, 0, fmt.Errorf("reach backend %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return Message{}, 0, fmt.Errorf("backend returned %s: %s", resp.Status, strings.TrimSpace(string(b)))
	}
	var full fullResponse
	if err := json.NewDecoder(resp.Body).Decode(&full); err != nil {
		return Message{}, 0, fmt.Errorf("decode completion: %w", err)
	}
	if len(full.Choices) == 0 {
		return Message{}, 0, fmt.Errorf("model returned no choices")
	}
	return full.Choices[0].Message, full.Usage.TotalTokens, nil
}

func (c *Client) CompleteConstrained(ctx context.Context, url, model string, msgs []Message, schema json.RawMessage) (string, int, error) {
	body := constrainedRequest{
		Model:          model,
		Messages:       msgs,
		Stream:         false,
		Temperature:    0.2,
		ResponseFormat: responseFormat{Type: "json_schema", JSONSchema: schemaBlock{Name: "action", Schema: schema}},
	}
	buf, err := json.Marshal(body)
	if err != nil {
		return "", 0, err
	}
	endpoint := strings.TrimRight(url, "/") + "/v1/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(buf))
	if err != nil {
		return "", 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return "", 0, fmt.Errorf("reach backend %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", 0, fmt.Errorf("backend returned %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	var full fullResponse
	if err := json.NewDecoder(resp.Body).Decode(&full); err != nil {
		return "", 0, fmt.Errorf("decode completion: %w", err)
	}
	if len(full.Choices) == 0 {
		return "", 0, fmt.Errorf("model returned no choices")
	}
	return full.Choices[0].Message.Content, full.Usage.TotalTokens, nil
}
