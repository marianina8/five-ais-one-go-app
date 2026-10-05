package main

import (
	"context"
	"encoding/json"
	"fmt"
)

// OpenAI talks to the OpenAI Responses API. The conversation lives on OpenAI's side:
// each call sends only what is new since the last reply, plus previous_response_id, so the
// model's reasoning items carry over between tool calls without being resent.
type OpenAI struct {
	model     string
	maxTokens int
	apiKey    string
	baseURL   string
	client    *apiClient

	previousID string // id of the last response, which holds everything before it
	sent       int    // how many of our messages OpenAI already has
}

func NewOpenAI(model string, maxTokens int, apiKey, baseURL string) *OpenAI {
	if baseURL == "" {
		baseURL = "https://api.openai.com"
	}
	return &OpenAI{model: model, maxTokens: maxTokens, apiKey: apiKey, baseURL: baseURL, client: newAPIClient()}
}

type openAIRequest struct {
	Model              string       `json:"model"`
	Instructions       string       `json:"instructions"`
	Input              []any        `json:"input"`
	Tools              []openAITool `json:"tools,omitempty"`
	MaxOutputTokens    int          `json:"max_output_tokens"`
	PreviousResponseID string       `json:"previous_response_id,omitempty"`
	Store              bool         `json:"store"` // previous_response_id needs the response stored
}

type openAITool struct {
	Type        string         `json:"type"`
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
	Strict      bool           `json:"strict"`
}

type openAIUserText struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openAIToolOutput struct {
	Type   string `json:"type"` // "function_call_output"
	CallID string `json:"call_id"`
	Output string `json:"output"`
}

type openAIResponse struct {
	ID                string `json:"id"`
	Status            string `json:"status"`
	IncompleteDetails *struct {
		Reason string `json:"reason"`
	} `json:"incomplete_details"`
	Output []struct {
		Type      string `json:"type"` // "message", "function_call", "reasoning", ...
		CallID    string `json:"call_id"`
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
		Content   []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	} `json:"output"`
	Usage struct {
		InputTokens        int `json:"input_tokens"`
		OutputTokens       int `json:"output_tokens"`
		InputTokensDetails struct {
			CachedTokens int `json:"cached_tokens"`
		} `json:"input_tokens_details"`
	} `json:"usage"`
}

func (o *OpenAI) Chat(ctx context.Context, system string, messages []Message, tools []Tool) (Response, error) {
	req := openAIRequest{
		Model:              o.model,
		Instructions:       system, // not carried over by previous_response_id, so sent every time
		Input:              newInput(messages[min(o.sent, len(messages)):]),
		MaxOutputTokens:    o.maxTokens,
		PreviousResponseID: o.previousID,
		Store:              true,
	}
	for _, tool := range tools {
		req.Tools = append(req.Tools, openAITool{Type: "function", Name: tool.Name, Description: tool.Description, Parameters: tool.Schema})
	}

	var resp openAIResponse
	headers := map[string]string{"Authorization": "Bearer " + o.apiKey}
	if err := o.client.postJSON(ctx, o.baseURL+"/v1/responses", headers, req, &resp); err != nil {
		return Response{}, fmt.Errorf("openai: %w", err)
	}
	o.previousID = resp.ID
	o.sent = len(messages) + 1 // the caller appends this reply

	stopReason := resp.Status
	if resp.IncompleteDetails != nil {
		stopReason = resp.IncompleteDetails.Reason
	}
	usage := Usage{
		InputTokens:  resp.Usage.InputTokens,
		OutputTokens: resp.Usage.OutputTokens,
		CachedTokens: resp.Usage.InputTokensDetails.CachedTokens,
	}
	return Response{Message: fromOpenAIOutput(resp), Usage: usage, StopReason: stopReason}, nil
}

// newInput turns the messages OpenAI hasn't seen into input items. OpenAI already has its
// own replies, so only our turns go: tool outputs, then any text.
func newInput(messages []Message) []any {
	var input []any
	for _, msg := range messages {
		if msg.Role != "user" {
			continue
		}
		for _, result := range msg.ToolResults {
			input = append(input, openAIToolOutput{Type: "function_call_output", CallID: result.CallID, Output: result.Output})
		}
		if msg.Text != "" {
			input = append(input, openAIUserText{Role: "user", Content: msg.Text})
		}
	}
	return input
}

func fromOpenAIOutput(resp openAIResponse) Message {
	reply := Message{Role: "assistant"}
	for _, item := range resp.Output {
		switch item.Type {
		case "message":
			for _, part := range item.Content {
				if part.Type == "output_text" {
					reply.Text += part.Text
				}
			}
		case "function_call":
			reply.ToolCalls = append(reply.ToolCalls, ToolCall{ID: item.CallID, Name: item.Name, Input: jsonObject(item.Arguments)})
		}
	}
	return reply
}

// jsonObject returns arguments as JSON if they are a valid JSON object. Otherwise (a reply
// cut off mid-call, say) it wraps them so the tool reports the problem to the model.
func jsonObject(arguments string) json.RawMessage {
	var probe map[string]any
	if json.Unmarshal([]byte(arguments), &probe) == nil {
		return json.RawMessage(arguments)
	}
	wrapped, _ := json.Marshal(map[string]string{"invalid_arguments": arguments})
	return wrapped
}
