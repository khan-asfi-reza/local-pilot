package model

import (
	"bufio"
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

// PullModel downloads a model via ollama's /api/pull, streaming NDJSON progress.
// progress (when non-nil) is called for each event with the current status and
// byte counts, so a caller can render a download bar. It uses a client with no
// timeout, since a large pull can take many minutes; cancel via ctx instead.
func (c *Client) PullModel(ctx context.Context, url, name string, progress func(status string, completed, total int64)) error {
	body, _ := json.Marshal(map[string]any{"name": name, "stream": true})
	endpoint := strings.TrimRight(url, "/") + "/api/pull"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		return fmt.Errorf("reach backend %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("pull %s: %s: %s", name, resp.Status, strings.TrimSpace(string(b)))
	}
	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	var lastErr string
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var ev struct {
			Status    string `json:"status"`
			Total     int64  `json:"total"`
			Completed int64  `json:"completed"`
			Error     string `json:"error"`
		}
		if json.Unmarshal([]byte(line), &ev) != nil {
			continue
		}
		if ev.Error != "" {
			lastErr = ev.Error
		}
		if progress != nil {
			progress(ev.Status, ev.Completed, ev.Total)
		}
	}
	if err := sc.Err(); err != nil {
		return err
	}
	if lastErr != "" {
		return fmt.Errorf("pull %s: %s", name, lastErr)
	}
	return nil
}

// DeleteModel removes a model from ollama via /api/delete, freeing its disk.
func (c *Client) DeleteModel(url, name string) error {
	body, _ := json.Marshal(map[string]any{"name": name})
	endpoint := strings.TrimRight(url, "/") + "/api/delete"
	req, err := http.NewRequest(http.MethodDelete, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("delete %s: %s: %s", name, resp.Status, strings.TrimSpace(string(b)))
	}
	return nil
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
	Type       string      `json:"type"`
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
	Model         string          `json:"model"`
	Messages      []Message       `json:"messages"`
	Tools         []ToolDef       `json:"tools,omitempty"`
	Stream        bool            `json:"stream"`
	StreamOptions *streamOptions  `json:"stream_options,omitempty"`
	Temperature   float64         `json:"temperature"`
	CachePrompt   bool            `json:"cache_prompt,omitempty"`
	JSONSchema    json.RawMessage `json:"json_schema,omitempty"`
}

type streamOptions struct {
	IncludeUsage bool `json:"include_usage"`
}

// streamChunk is one Server-Sent Event from a streamed /v1 completion. The final
// chunk carries usage with an empty Choices list.
type streamChunk struct {
	Choices []struct {
		Delta struct {
			Content   string `json:"content"`
			Reasoning string `json:"reasoning"`
			ToolCalls []struct {
				Index    int    `json:"index"`
				ID       string `json:"id"`
				Type     string `json:"type"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		TotalTokens int `json:"total_tokens"`
	} `json:"usage"`
}

// Chat runs one native tool-calling turn, streaming the response. onDelta (when
// non-nil) is called for each token as it arrives — kind "content" for the answer
// and "reasoning" for the model's thinking — so the UI updates live. The fully
// assembled message and total token count are returned when the stream ends.
func (c *Client) Chat(ctx context.Context, url, model string, msgs []Message, defs []ToolDef, onDelta func(kind, text string)) (outMsg Message, outTokens int, outErr error) {
	start := time.Now()
	defer func() { logCall("chat", model, msgs, outMsg.Content, outMsg.ToolCalls, outTokens, start, outErr) }()
	body := chatRequest{
		Model:         model,
		Messages:      msgs,
		Tools:         defs,
		Stream:        true,
		StreamOptions: &streamOptions{IncludeUsage: true},
		Temperature:   0.2,
	}
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

	var content, reasoning strings.Builder
	toolAcc := map[int]*ToolCall{}
	var order []int
	tokens := 0

	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" || data == "[DONE]" {
			continue
		}
		var chunk streamChunk
		if json.Unmarshal([]byte(data), &chunk) != nil {
			continue
		}
		if chunk.Usage.TotalTokens > 0 {
			tokens = chunk.Usage.TotalTokens
		}
		if len(chunk.Choices) == 0 {
			continue
		}
		d := chunk.Choices[0].Delta
		if d.Content != "" {
			content.WriteString(d.Content)
			if onDelta != nil {
				onDelta("content", d.Content)
			}
		}
		if d.Reasoning != "" {
			reasoning.WriteString(d.Reasoning)
			if onDelta != nil {
				onDelta("reasoning", d.Reasoning)
			}
		}
		for _, tc := range d.ToolCalls {
			acc, ok := toolAcc[tc.Index]
			if !ok {
				acc = &ToolCall{Type: "function"}
				toolAcc[tc.Index] = acc
				order = append(order, tc.Index)
			}
			if tc.ID != "" {
				acc.ID = tc.ID
			}
			if tc.Function.Name != "" {
				acc.Function.Name = tc.Function.Name
			}
			acc.Function.Arguments += tc.Function.Arguments
		}
	}
	if err := sc.Err(); err != nil {
		return Message{}, 0, fmt.Errorf("read stream from %s: %w", url, err)
	}

	msg := Message{Role: "assistant", Content: content.String(), Reasoning: reasoning.String()}
	for _, i := range order {
		tc := toolAcc[i]
		if tc.ID == "" {
			tc.ID = fmt.Sprintf("call_%d", i)
		}
		msg.ToolCalls = append(msg.ToolCalls, *tc)
	}
	return msg, tokens, nil
}

func (c *Client) CompleteConstrained(ctx context.Context, url, model string, msgs []Message, schema json.RawMessage) (outContent string, outTokens int, outErr error) {
	start := time.Now()
	defer func() { logCall("constrained", model, msgs, outContent, nil, outTokens, start, outErr) }()
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
