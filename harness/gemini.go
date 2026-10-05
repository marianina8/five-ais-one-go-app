package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
)

// Gemini talks to the Gemini API's generateContent endpoint. Gemini 3 models attach thought
// signatures to their parts and need them back, so each model turn is resent exactly as
// it arrived (Message.Raw).
type Gemini struct {
	model     string
	maxTokens int
	apiKey    string
	baseURL   string
	client    *apiClient
}

func NewGemini(model string, maxTokens int, apiKey, baseURL string) *Gemini {
	if baseURL == "" {
		baseURL = "https://generativelanguage.googleapis.com"
	}
	return &Gemini{model: model, maxTokens: maxTokens, apiKey: apiKey, baseURL: baseURL, client: newAPIClient()}
}

type geminiRequest struct {
	SystemInstruction geminiContent     `json:"systemInstruction"`
	Contents          []json.RawMessage `json:"contents"`
	Tools             []geminiTools     `json:"tools,omitempty"`
	GenerationConfig  struct {
		MaxOutputTokens int `json:"maxOutputTokens"`
	} `json:"generationConfig"`
}

type geminiContent struct {
	Role  string       `json:"role,omitempty"`
	Parts []geminiPart `json:"parts"`
}

type geminiPart struct {
	Text             string                  `json:"text,omitempty"`
	Thought          bool                    `json:"thought,omitempty"`
	FunctionCall     *geminiFunctionCall     `json:"functionCall,omitempty"`
	FunctionResponse *geminiFunctionResponse `json:"functionResponse,omitempty"`
}

type geminiFunctionCall struct {
	ID   string         `json:"id,omitempty"`
	Name string         `json:"name"`
	Args map[string]any `json:"args"`
}

type geminiFunctionResponse struct {
	ID       string         `json:"id,omitempty"`
	Name     string         `json:"name"`
	Response map[string]any `json:"response"`
}

type geminiTools struct {
	FunctionDeclarations []geminiFunction `json:"functionDeclarations"`
}

type geminiFunction struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

type geminiResponse struct {
	Candidates []struct {
		Content      json.RawMessage `json:"content"`
		FinishReason string          `json:"finishReason"`
	} `json:"candidates"`
	PromptFeedback *struct {
		BlockReason string `json:"blockReason"`
	} `json:"promptFeedback"`
	UsageMetadata struct {
		PromptTokenCount        int `json:"promptTokenCount"`
		CandidatesTokenCount    int `json:"candidatesTokenCount"`
		ThoughtsTokenCount      int `json:"thoughtsTokenCount"`
		CachedContentTokenCount int `json:"cachedContentTokenCount"`
	} `json:"usageMetadata"`
}

func (g *Gemini) Chat(ctx context.Context, system string, messages []Message, tools []Tool) (Response, error) {
	req, err := g.request(system, messages, tools)
	if err != nil {
		return Response{}, err
	}
	endpoint := fmt.Sprintf("%s/v1beta/models/%s:generateContent", g.baseURL, url.PathEscape(g.model))
	var resp geminiResponse
	if err := g.client.postJSON(ctx, endpoint, map[string]string{"x-goog-api-key": g.apiKey}, req, &resp); err != nil {
		return Response{}, fmt.Errorf("gemini: %w", err)
	}

	usage := Usage{
		InputTokens:  resp.UsageMetadata.PromptTokenCount,
		OutputTokens: resp.UsageMetadata.CandidatesTokenCount + resp.UsageMetadata.ThoughtsTokenCount,
		CachedTokens: resp.UsageMetadata.CachedContentTokenCount,
	}
	if len(resp.Candidates) == 0 {
		reason := "no candidates"
		if resp.PromptFeedback != nil {
			reason = "blocked: " + resp.PromptFeedback.BlockReason
		}
		return Response{Usage: usage}, fmt.Errorf("gemini: %s", reason)
	}
	candidate := resp.Candidates[0]
	reply, err := fromGeminiContent(candidate.Content)
	if err != nil {
		return Response{}, err
	}
	return Response{Message: reply, Usage: usage, StopReason: candidate.FinishReason}, nil
}

func (g *Gemini) request(system string, messages []Message, tools []Tool) (geminiRequest, error) {
	var req geminiRequest
	req.SystemInstruction = geminiContent{Parts: []geminiPart{{Text: system}}}
	req.GenerationConfig.MaxOutputTokens = g.maxTokens
	if len(tools) > 0 {
		declarations := geminiTools{}
		for _, tool := range tools {
			declarations.FunctionDeclarations = append(declarations.FunctionDeclarations, geminiFunction{tool.Name, tool.Description, geminiSchema(tool.Schema)})
		}
		req.Tools = []geminiTools{declarations}
	}
	for _, msg := range messages {
		content, err := toGeminiContent(msg)
		if err != nil {
			return req, err
		}
		req.Contents = append(req.Contents, content)
	}
	return req, nil
}

// fromGeminiContent reads a model turn. It keeps the raw content to send back unchanged.
func fromGeminiContent(raw json.RawMessage) (Message, error) {
	var content geminiContent
	if err := json.Unmarshal(raw, &content); err != nil {
		return Message{}, fmt.Errorf("gemini: read content: %w", err)
	}
	reply := Message{Role: "assistant", Raw: raw}
	for i, part := range content.Parts {
		switch {
		case part.FunctionCall != nil:
			reply.ToolCalls = append(reply.ToolCalls, fromGeminiCall(i, part.FunctionCall))
		case part.Text != "" && !part.Thought:
			reply.Text += part.Text
		}
	}
	return reply, nil
}

func fromGeminiCall(index int, call *geminiFunctionCall) ToolCall {
	args := []byte("{}")
	if call.Args != nil {
		args, _ = json.Marshal(call.Args) // it was just decoded from JSON, so it encodes
	}
	id := call.ID
	if id == "" {
		id = fmt.Sprintf("call-%d", index) // older models don't number their calls
	}
	return ToolCall{ID: id, Name: call.Name, Input: args}
}

// toGeminiContent turns one of our messages into a Gemini content entry. A model turn is
// resent exactly as received; a user turn carries the tool results, then any text.
func toGeminiContent(msg Message) (json.RawMessage, error) {
	if msg.Role == "assistant" && len(msg.Raw) > 0 {
		return msg.Raw, nil
	}
	content := geminiContent{Role: "user"}
	if msg.Role == "assistant" {
		content.Role = "model"
	}
	for _, result := range msg.ToolResults {
		response := geminiFunctionResponse{Name: result.Name, Response: map[string]any{"output": result.Output}}
		if result.IsError {
			response.Response = map[string]any{"error": result.Output}
		}
		if !isGeneratedGeminiID(result.CallID) {
			response.ID = result.CallID
		}
		content.Parts = append(content.Parts, geminiPart{FunctionResponse: &response})
	}
	if msg.Text != "" {
		content.Parts = append(content.Parts, geminiPart{Text: msg.Text})
	}
	return json.Marshal(content)
}

func isGeneratedGeminiID(id string) bool {
	var n int
	_, err := fmt.Sscanf(id, "call-%d", &n)
	return err == nil
}

// geminiSchema drops the empty "required" list, which the Gemini API rejects.
func geminiSchema(schema map[string]any) map[string]any {
	out := map[string]any{}
	for key, value := range schema {
		if list, ok := value.([]string); ok && key == "required" && len(list) == 0 {
			continue
		}
		out[key] = value
	}
	return out
}
