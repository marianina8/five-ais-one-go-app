package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
)

// Model is one AI model behind its vendor's API. Each adapter (anthropic.go, openai.go,
// gemini.go, bedrock.go) translates these plain types to and from its own API.
type Model interface {
	Chat(ctx context.Context, system string, messages []Message, tools []Tool) (Response, error)
}

// Message is one turn of the conversation.
type Message struct {
	Role        string       // "user" or "assistant"
	Text        string       // what was said; may be empty when the turn is only tool calls or results
	ToolCalls   []ToolCall   // assistant turns: tools the model wants to run
	ToolResults []ToolResult // user turns: what those tools returned
	// Raw is the assistant turn exactly as the vendor's API returned it, for APIs that need
	// it sent back verbatim (Gemini's thought signatures). Other adapters ignore it.
	Raw json.RawMessage
}

// ToolCall is the model asking to run one tool.
type ToolCall struct {
	ID    string          // set by the API; ties the result to the call
	Name  string          // which tool
	Input json.RawMessage // the tool's arguments, a JSON object
}

// ToolResult is what goes back to the model after a tool ran.
type ToolResult struct {
	CallID  string
	Name    string
	Output  string
	IsError bool
}

// Tool describes a tool to the model: its name, when to use it, and its arguments as JSON schema.
type Tool struct {
	Name        string
	Description string
	Schema      map[string]any
}

// Response is the model's reply and what it cost.
type Response struct {
	Message    Message
	Usage      Usage
	StopReason string // the API's own reason, e.g. "end_turn", "max_tokens"
}

// Usage counts tokens. Output includes any reasoning or thinking tokens, which every
// vendor bills as output.
type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
	CachedTokens int `json:"cached_tokens"` // part of InputTokens the vendor served from its cache
}

// Add sums another call's usage into u.
func (u *Usage) Add(other Usage) {
	u.InputTokens += other.InputTokens
	u.OutputTokens += other.OutputTokens
	u.CachedTokens += other.CachedTokens
}

// apiClient posts JSON to a vendor API and retries the failures worth retrying.
type apiClient struct {
	http    *http.Client
	retries int
	backoff time.Duration // first wait; doubles each retry
}

func newAPIClient() *apiClient {
	return &apiClient{http: &http.Client{Timeout: 10 * time.Minute}, retries: 5, backoff: 2 * time.Second}
}

// statusError is an HTTP error from a vendor API. The body is the vendor's error message,
// which never contains the API key.
type statusError struct {
	status int
	body   string
}

func (e *statusError) Error() string { return fmt.Sprintf("HTTP %d: %s", e.status, e.body) }

// retryable reports whether an HTTP status is worth retrying: rate limits and server errors.
func retryable(status int) bool {
	return status == http.StatusTooManyRequests || status == 529 || status >= 500
}

// postJSON sends body to url and decodes the JSON reply into out, retrying rate limits,
// server errors and dropped connections with exponential backoff.
func (c *apiClient) postJSON(ctx context.Context, url string, headers map[string]string, body, out any) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}
	wait := c.backoff
	for attempt := 0; ; attempt++ {
		retryAfter, err := c.post(ctx, url, headers, payload, out)
		if err == nil || !c.shouldRetry(ctx, err, attempt) {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(max(wait, retryAfter)):
		}
		wait *= 2
	}
}

// shouldRetry retries rate limits, server errors and dropped connections, while there are
// attempts and time left. Any other HTTP error (a bad request, a wrong key) is final.
func (c *apiClient) shouldRetry(ctx context.Context, err error, attempt int) bool {
	var httpErr *statusError
	if errors.As(err, &httpErr) && !retryable(httpErr.status) {
		return false
	}
	return attempt < c.retries && ctx.Err() == nil
}

// post makes one attempt. It returns the server's Retry-After, if any, for the caller's backoff.
func (c *apiClient) post(ctx context.Context, url string, headers map[string]string, payload []byte, out any) (time.Duration, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	for name, value := range headers {
		req.Header.Set(name, value)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return 0, err
	}
	defer func() { _ = resp.Body.Close() }() // a read error below already reports any problem
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, err
	}
	if resp.StatusCode != http.StatusOK {
		seconds, _ := strconv.Atoi(resp.Header.Get("Retry-After"))
		return time.Duration(seconds) * time.Second, &statusError{status: resp.StatusCode, body: truncate(string(data), 2000)}
	}
	if err := json.Unmarshal(data, out); err != nil {
		return 0, fmt.Errorf("decode response: %w", err)
	}
	return 0, nil
}
