package main

import (
	"context"
	"encoding/json"
	"fmt"
)

// Anthropic talks to the Anthropic Messages API (Claude).
type Anthropic struct {
	model     string
	maxTokens int
	apiKey    string
	baseURL   string
	client    *apiClient
}

func NewAnthropic(model string, maxTokens int, apiKey, baseURL string) *Anthropic {
	if baseURL == "" {
		baseURL = "https://api.anthropic.com"
	}
	return &Anthropic{model: model, maxTokens: maxTokens, apiKey: apiKey, baseURL: baseURL, client: newAPIClient()}
}

type anthropicRequest struct {
	Model     string             `json:"model"`
	MaxTokens int                `json:"max_tokens"`
	System    string             `json:"system"`
	Messages  []anthropicMessage `json:"messages"`
	Tools     []anthropicTool    `json:"tools,omitempty"`
}

type anthropicMessage struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"` // a list of blocks
}

// anthropicBlock is one content block: text, tool_use, tool_result (and, in replies,
// blocks such as thinking that we pass back untouched through Message.Raw).
type anthropicBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text,omitempty"`
	ID        string          `json:"id,omitempty"`
	Name      string          `json:"name,omitempty"`
	Input     json.RawMessage `json:"input,omitempty"`
	ToolUseID string          `json:"tool_use_id,omitempty"`
	Content   string          `json:"content,omitempty"`
	IsError   bool            `json:"is_error,omitempty"`
}

type anthropicTool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"input_schema"`
}

type anthropicResponse struct {
	Content    json.RawMessage `json:"content"`
	StopReason string          `json:"stop_reason"`
	Usage      struct {
		InputTokens          int `json:"input_tokens"`
		OutputTokens         int `json:"output_tokens"`
		CacheReadInputTokens int `json:"cache_read_input_tokens"`
	} `json:"usage"`
}

func (a *Anthropic) Chat(ctx context.Context, system string, messages []Message, tools []Tool) (Response, error) {
	req := anthropicRequest{Model: a.model, MaxTokens: a.maxTokens, System: system}
	for _, tool := range tools {
		req.Tools = append(req.Tools, anthropicTool{tool.Name, tool.Description, tool.Schema})
	}
	for _, msg := range messages {
		converted, err := toAnthropicMessage(msg)
		if err != nil {
			return Response{}, err
		}
		req.Messages = append(req.Messages, converted)
	}

	headers := map[string]string{"x-api-key": a.apiKey, "anthropic-version": "2023-06-01"}
	var resp anthropicResponse
	if err := a.client.postJSON(ctx, a.baseURL+"/v1/messages", headers, req, &resp); err != nil {
		return Response{}, fmt.Errorf("anthropic: %w", err)
	}

	var blocks []anthropicBlock
	if err := json.Unmarshal(resp.Content, &blocks); err != nil {
		return Response{}, fmt.Errorf("anthropic: read content: %w", err)
	}
	// Keep the content exactly as returned: thinking blocks must go back unchanged.
	reply := Message{Role: "assistant", Raw: resp.Content}
	for _, block := range blocks {
		switch block.Type {
		case "text":
			reply.Text += block.Text
		case "tool_use":
			reply.ToolCalls = append(reply.ToolCalls, ToolCall{ID: block.ID, Name: block.Name, Input: block.Input})
		}
	}
	usage := Usage{
		InputTokens:  resp.Usage.InputTokens + resp.Usage.CacheReadInputTokens,
		OutputTokens: resp.Usage.OutputTokens,
		CachedTokens: resp.Usage.CacheReadInputTokens,
	}
	return Response{Message: reply, Usage: usage, StopReason: resp.StopReason}, nil
}

// toAnthropicMessage turns one of our messages into Anthropic content blocks.
func toAnthropicMessage(msg Message) (anthropicMessage, error) {
	if msg.Role == "assistant" && len(msg.Raw) > 0 {
		return anthropicMessage{Role: msg.Role, Content: msg.Raw}, nil
	}
	var blocks []anthropicBlock
	// Tool results go first: the API wants them right after the assistant's tool calls.
	for _, result := range msg.ToolResults {
		blocks = append(blocks, anthropicBlock{Type: "tool_result", ToolUseID: result.CallID, Content: result.Output, IsError: result.IsError})
	}
	if msg.Text != "" {
		blocks = append(blocks, anthropicBlock{Type: "text", Text: msg.Text})
	}
	for _, call := range msg.ToolCalls {
		blocks = append(blocks, anthropicBlock{Type: "tool_use", ID: call.ID, Name: call.Name, Input: call.Input})
	}
	content, err := json.Marshal(blocks)
	return anthropicMessage{Role: msg.Role, Content: content}, err
}
