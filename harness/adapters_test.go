package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// fakeAPI serves canned replies in order and keeps every request body for inspection.
type fakeAPI struct {
	t        *testing.T
	path     string
	header   string // header that must carry the key
	key      string
	replies  []string
	requests []map[string]any
}

func (f *fakeAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !strings.HasSuffix(r.URL.Path, f.path) {
		f.t.Errorf("request to %s, want ...%s", r.URL.Path, f.path)
	}
	if got := r.Header.Get(f.header); !strings.Contains(got, f.key) {
		f.t.Errorf("header %s = %q, want the API key", f.header, got)
	}
	body, _ := io.ReadAll(r.Body)
	var parsed map[string]any
	if err := json.Unmarshal(body, &parsed); err != nil {
		f.t.Fatalf("request is not JSON: %v", err)
	}
	f.requests = append(f.requests, parsed)
	if len(f.replies) == 0 {
		f.t.Fatal("no more canned replies")
	}
	reply := f.replies[0]
	f.replies = f.replies[1:]
	_, _ = io.WriteString(w, reply)
}

func serve(t *testing.T, api *fakeAPI) string {
	server := httptest.NewServer(api)
	t.Cleanup(server.Close)
	return server.URL
}

var testTools = []Tool{{Name: "list_files", Description: "list", Schema: map[string]any{"type": "object", "properties": map[string]any{}, "required": []string{}}}}

// converse runs a two-call conversation: the model calls list_files, gets the result, answers.
func converse(t *testing.T, model Model) (first, second Response) {
	t.Helper()
	ctx := context.Background()
	messages := []Message{{Role: "user", Text: "build it"}}
	first, err := model.Chat(ctx, "system prompt", messages, testTools)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Message.ToolCalls) != 1 || first.Message.ToolCalls[0].Name != "list_files" {
		t.Fatalf("first reply should call list_files: %+v", first.Message)
	}
	call := first.Message.ToolCalls[0]
	messages = append(messages, first.Message, Message{
		Role:        "user",
		Text:        "Budget: 3 model calls left.",
		ToolResults: []ToolResult{{CallID: call.ID, Name: call.Name, Output: "main.go (10 bytes)"}},
	})
	second, err = model.Chat(ctx, "system prompt", messages, testTools)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(second.Message.Text, "DONE") || len(second.Message.ToolCalls) != 0 {
		t.Fatalf("second reply should be the final answer: %+v", second.Message)
	}
	return first, second
}

func TestAnthropic(t *testing.T) {
	api := &fakeAPI{t: t, path: "/v1/messages", header: "x-api-key", key: "sk-ant-test-key", replies: []string{
		`{"content":[{"type":"thinking","thinking":"plan","signature":"sig-1"},{"type":"tool_use","id":"tu_1","name":"list_files","input":{"path":"."}}],"stop_reason":"tool_use","usage":{"input_tokens":100,"output_tokens":20,"cache_read_input_tokens":50}}`,
		`{"content":[{"type":"text","text":"DONE built it"}],"stop_reason":"end_turn","usage":{"input_tokens":200,"output_tokens":10}}`,
	}}
	first, _ := converse(t, NewAnthropic("claude-x", 1000, "sk-ant-test-key", serve(t, api)))

	if first.Usage != (Usage{InputTokens: 150, OutputTokens: 20, CachedTokens: 50}) {
		t.Errorf("usage %+v: cache reads count as input", first.Usage)
	}
	second := api.requests[1]
	messages := second["messages"].([]any)
	assistant := messages[1].(map[string]any)["content"].([]any)
	if assistant[0].(map[string]any)["signature"] != "sig-1" {
		t.Errorf("the thinking block must go back unchanged: %v", assistant[0])
	}
	result := messages[2].(map[string]any)["content"].([]any)[0].(map[string]any)
	if result["type"] != "tool_result" || result["tool_use_id"] != "tu_1" {
		t.Errorf("tool result block: %v", result)
	}
	if second["max_tokens"].(float64) != 1000 || second["system"] != "system prompt" {
		t.Errorf("request settings: %v", second)
	}
}

func TestOpenAI(t *testing.T) {
	api := &fakeAPI{t: t, path: "/v1/responses", header: "Authorization", key: "sk-proj-test-key", replies: []string{
		`{"id":"resp_1","status":"completed","output":[{"type":"reasoning","summary":[]},{"type":"function_call","call_id":"call_1","name":"list_files","arguments":"{\"path\":\".\"}"}],"usage":{"input_tokens":100,"output_tokens":30,"input_tokens_details":{"cached_tokens":40}}}`,
		`{"id":"resp_2","status":"completed","output":[{"type":"message","content":[{"type":"output_text","text":"DONE built it"}]}],"usage":{"input_tokens":150,"output_tokens":5}}`,
	}}
	first, _ := converse(t, NewOpenAI("gpt-x", 2000, "sk-proj-test-key", serve(t, api)))

	if first.Usage.CachedTokens != 40 || first.Usage.InputTokens != 100 {
		t.Errorf("usage %+v", first.Usage)
	}
	if _, ok := api.requests[0]["previous_response_id"]; ok {
		t.Error("the first call has no previous response")
	}
	checkOpenAIFollowUp(t, api.requests[1])
}

// checkOpenAIFollowUp checks the second call chains on the first and sends only what's new.
func checkOpenAIFollowUp(t *testing.T, second map[string]any) {
	t.Helper()
	if second["previous_response_id"] != "resp_1" || second["instructions"] != "system prompt" || second["store"] != true {
		t.Errorf("follow-up must chain on resp_1 and resend instructions: %v", second)
	}
	input := second["input"].([]any)
	if len(input) != 2 {
		t.Fatalf("follow-up should send only the tool output and the budget note, got %v", input)
	}
	output := input[0].(map[string]any)
	if output["type"] != "function_call_output" || output["call_id"] != "call_1" {
		t.Errorf("tool output item: %v", output)
	}
}

func TestOpenAIBadArguments(t *testing.T) {
	if got := string(jsonObject(`{"path": "main.go`)); !strings.Contains(got, "invalid_arguments") {
		t.Errorf("cut-off arguments should be wrapped, got %s", got)
	}
	if got := string(jsonObject(`{"path":"x"}`)); got != `{"path":"x"}` {
		t.Errorf("valid arguments changed: %s", got)
	}
}

func TestGemini(t *testing.T) {
	api := &fakeAPI{t: t, path: "/v1beta/models/gemini-x:generateContent", header: "x-goog-api-key", key: "AIza-test-key", replies: []string{
		`{"candidates":[{"content":{"role":"model","parts":[{"functionCall":{"id":"fc_1","name":"list_files","args":{"path":"."}},"thoughtSignature":"c2lnbmF0dXJl"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":100,"candidatesTokenCount":10,"thoughtsTokenCount":90}}`,
		`{"candidates":[{"content":{"role":"model","parts":[{"text":"thinking...","thought":true},{"text":"DONE built it"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":200,"candidatesTokenCount":5}}`,
	}}
	first, second := converse(t, NewGemini("gemini-x", 3000, "AIza-test-key", serve(t, api)))

	if first.Usage.OutputTokens != 100 {
		t.Errorf("thinking tokens are billed as output: %+v", first.Usage)
	}
	if second.Message.Text != "DONE built it" {
		t.Errorf("thought parts must not be part of the answer: %q", second.Message.Text)
	}
	req := api.requests[1]
	contents := req["contents"].([]any)
	modelTurn := contents[1].(map[string]any)["parts"].([]any)[0].(map[string]any)
	if modelTurn["thoughtSignature"] != "c2lnbmF0dXJl" {
		t.Errorf("the model turn must go back with its thought signature: %v", modelTurn)
	}
	response := contents[2].(map[string]any)["parts"].([]any)[0].(map[string]any)["functionResponse"].(map[string]any)
	if response["id"] != "fc_1" || response["name"] != "list_files" {
		t.Errorf("function response: %v", response)
	}
	declaration := req["tools"].([]any)[0].(map[string]any)["functionDeclarations"].([]any)[0].(map[string]any)
	if _, has := declaration["parameters"].(map[string]any)["required"]; has {
		t.Error("an empty required list must be dropped for Gemini")
	}
}

func TestRetries(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts++
		if attempts < 3 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		_, _ = io.WriteString(w, `{"ok":true}`)
	}))
	defer server.Close()
	client := &apiClient{http: server.Client(), retries: 5, backoff: time.Millisecond}
	var out map[string]bool
	if err := client.postJSON(context.Background(), server.URL, nil, map[string]string{}, &out); err != nil || !out["ok"] {
		t.Fatalf("should succeed after two 429s: %v", err)
	}

	attempts = 0
	server.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts++
		w.WriteHeader(http.StatusBadRequest)
	})
	if err := client.postJSON(context.Background(), server.URL, nil, map[string]string{}, &out); err == nil || attempts != 1 {
		t.Fatalf("a 400 must not be retried: attempts %d, err %v", attempts, err)
	}
}
