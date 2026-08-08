package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	agentruntime "github.com/Saieshwar5/ayati-code/internal/runtime"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func TestOpenAIChatMaintainsToolCallProtocol(t *testing.T) {
	responses := []string{
		`{"choices":[{"message":{"role":"assistant","tool_calls":[{"id":"call-1","type":"function","function":{"name":"shell","arguments":"{\"command\":\"pwd\"}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":10,"completion_tokens":2,"total_tokens":12}}`,
		`{"choices":[{"message":{"role":"assistant","content":"done"},"finish_reason":"stop"}],"usage":{"prompt_tokens":15,"completion_tokens":1,"total_tokens":16}}`,
	}
	calls := 0
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Fatal(err)
		}
		var payload struct {
			Messages []chatMessage `json:"messages"`
		}
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if calls == 1 {
			last := payload.Messages[len(payload.Messages)-1]
			if last.Role != "tool" || last.ToolCallID != "call-1" || !strings.Contains(last.Content, `"exit_code":0`) || strings.Contains(last.Content, "pwd") {
				t.Fatalf("unexpected tool result message: %+v", last)
			}
		}
		response := responses[calls]
		calls++
		return jsonResponse(response), nil
	})}
	model := &OpenAIChat{Config: Config{APIKey: "key", Model: "model", Endpoint: "https://example.test/chat", Client: client}}
	conversation, err := model.Start("system", "user", agentruntime.ShellDefinition)
	if err != nil {
		t.Fatal(err)
	}
	decision, err := conversation.Next(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if decision.ShellCall == nil || decision.ShellCall.Command != "pwd" || decision.Usage.TotalTokens != 12 {
		t.Fatalf("unexpected first decision: %+v", decision)
	}
	decision, err = conversation.Next(context.Background(), &agentruntime.ToolResult{ExitCode: 0, Stdout: "/workspace\n"})
	if err != nil {
		t.Fatal(err)
	}
	if decision.ShellCall != nil || decision.Text != "done" || calls != 2 {
		t.Fatalf("unexpected second decision: %+v, calls=%d", decision, calls)
	}
}

func TestNewSelectsNativeProviderAdapter(t *testing.T) {
	config := Config{APIKey: "key", Model: "model", Endpoint: "https://example.test"}
	tests := []struct {
		kind string
		want any
	}{
		{kind: "openai-chat", want: &OpenAIChat{}},
		{kind: "openai-responses", want: &OpenAIResponses{}},
		{kind: "anthropic", want: &Anthropic{}},
	}
	for _, test := range tests {
		t.Run(test.kind, func(t *testing.T) {
			model, err := New(test.kind, config)
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			if fmt.Sprintf("%T", model) != fmt.Sprintf("%T", test.want) {
				t.Fatalf("adapter = %T, want %T", model, test.want)
			}
		})
	}
	if _, err := New("unknown", config); err == nil {
		t.Fatal("expected unsupported-provider error")
	}
}

func TestOpenAIChatRejectsParallelToolCalls(t *testing.T) {
	response := `{"choices":[{"message":{"role":"assistant","tool_calls":[` +
		`{"id":"one","type":"function","function":{"name":"shell","arguments":"{\"command\":\"one\"}"}},` +
		`{"id":"two","type":"function","function":{"name":"shell","arguments":"{\"command\":\"two\"}"}}]},"finish_reason":"tool_calls"}]}`
	client := clientReturning(response)
	model := &OpenAIChat{Config: Config{APIKey: "key", Model: "model", Endpoint: "https://example.test/chat", Client: client}}
	conversation, err := model.Start("system", "user", agentruntime.ShellDefinition)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conversation.Next(context.Background(), nil); err == nil || !strings.Contains(err.Error(), "permits one") {
		t.Fatalf("expected parallel-call error, got %v", err)
	}
}

func TestOpenAIChatToolDisabledResponseOmitsTools(t *testing.T) {
	responses := []string{
		`{"choices":[{"message":{"role":"assistant","tool_calls":[{"id":"call-1","type":"function","function":{"name":"shell","arguments":"{\"command\":\"true\"}"}}]},"finish_reason":"tool_calls"}]}`,
		`{"choices":[{"message":{"role":"assistant","content":"truthful handoff"},"finish_reason":"stop"}]}`,
	}
	calls := 0
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		var payload map[string]any
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if calls == 1 {
			if _, exists := payload["tools"]; exists {
				t.Fatalf("tools present in tool-disabled request: %#v", payload["tools"])
			}
			messages := payload["messages"].([]any)
			if messages[len(messages)-2].(map[string]any)["role"] != "tool" || !strings.Contains(messages[len(messages)-1].(map[string]any)["content"].(string), "handoff") {
				t.Fatalf("unexpected finalization messages: %#v", messages)
			}
		}
		response := responses[calls]
		calls++
		return jsonResponse(response), nil
	})}
	conversation, err := (&OpenAIChat{Config: Config{APIKey: "key", Model: "model", Endpoint: "https://example.test/chat", Client: client}}).Start("system", "user", agentruntime.ShellDefinition)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conversation.Next(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	decision, err := conversation.RespondWithoutTools(context.Background(), &agentruntime.ToolResult{ExitCode: 0}, "write a handoff")
	if err != nil || decision.Text != "truthful handoff" {
		t.Fatalf("decision=%+v err=%v", decision, err)
	}
}

func TestOpenAIResponsesUsesPreviousResponseAndCallID(t *testing.T) {
	responses := []string{
		`{"id":"resp-1","status":"completed","output":[{"type":"function_call","call_id":"call-1","name":"shell","arguments":"{\"command\":\"go test ./...\"}"}],"usage":{"input_tokens":20,"output_tokens":4,"total_tokens":24}}`,
		`{"id":"resp-2","status":"completed","output":[{"type":"message","content":[{"type":"output_text","text":"verified"}]}],"usage":{"input_tokens":25,"output_tokens":2,"total_tokens":27}}`,
	}
	calls := 0
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		var payload map[string]any
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if calls == 1 {
			if payload["previous_response_id"] != "resp-1" {
				t.Fatalf("previous_response_id = %v", payload["previous_response_id"])
			}
			items, ok := payload["input"].([]any)
			if !ok || len(items) != 1 {
				t.Fatalf("input = %#v", payload["input"])
			}
			item := items[0].(map[string]any)
			if item["call_id"] != "call-1" || strings.Contains(item["output"].(string), "go test") {
				t.Fatalf("function output = %#v", item)
			}
		}
		response := responses[calls]
		calls++
		return jsonResponse(response), nil
	})}
	model := &OpenAIResponses{Config: Config{APIKey: "key", Model: "model", Endpoint: "https://example.test/responses", Client: client}}
	conversation, err := model.Start("system", "user", agentruntime.ShellDefinition)
	if err != nil {
		t.Fatal(err)
	}
	first, err := conversation.Next(context.Background(), nil)
	if err != nil || first.ShellCall == nil || first.ShellCall.Command != "go test ./..." {
		t.Fatalf("first decision = %+v, err=%v", first, err)
	}
	second, err := conversation.Next(context.Background(), &agentruntime.ToolResult{ExitCode: 0, Stdout: "ok"})
	if err != nil || second.Text != "verified" || second.ShellCall != nil {
		t.Fatalf("second decision = %+v, err=%v", second, err)
	}
}

func TestOpenAIResponsesToolDisabledResponseOmitsTools(t *testing.T) {
	responses := []string{
		`{"id":"resp-1","status":"completed","output":[{"type":"function_call","call_id":"call-1","name":"shell","arguments":"{\"command\":\"true\"}"}]}`,
		`{"id":"resp-2","status":"completed","output":[{"type":"message","content":[{"type":"output_text","text":"checkpoint"}]}]}`,
	}
	calls := 0
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		var payload map[string]any
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if calls == 1 {
			if _, exists := payload["tools"]; exists {
				t.Fatalf("tools present in tool-disabled request: %#v", payload["tools"])
			}
			if payload["previous_response_id"] != "resp-1" {
				t.Fatalf("previous response = %#v", payload["previous_response_id"])
			}
			items := payload["input"].([]any)
			if len(items) != 2 || items[0].(map[string]any)["type"] != "function_call_output" || !strings.Contains(items[1].(map[string]any)["content"].(string), "checkpoint") {
				t.Fatalf("unexpected checkpoint input: %#v", items)
			}
		}
		response := responses[calls]
		calls++
		return jsonResponse(response), nil
	})}
	conversation, err := (&OpenAIResponses{Config: Config{APIKey: "key", Model: "model", Endpoint: "https://example.test/responses", Client: client}}).Start("system", "user", agentruntime.ShellDefinition)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conversation.Next(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	decision, err := conversation.RespondWithoutTools(context.Background(), &agentruntime.ToolResult{ExitCode: 0}, "write a checkpoint")
	if err != nil || decision.Text != "checkpoint" {
		t.Fatalf("decision=%+v err=%v", decision, err)
	}
}

func TestAnthropicMaintainsNativeContentBlocks(t *testing.T) {
	responses := []string{
		`{"id":"msg-1","stop_reason":"tool_use","content":[{"type":"tool_use","id":"tool-1","name":"shell","input":{"command":"git status"}}],"usage":{"input_tokens":12,"output_tokens":3}}`,
		`{"id":"msg-2","stop_reason":"end_turn","content":[{"type":"text","text":"clean"}],"usage":{"input_tokens":16,"output_tokens":1}}`,
	}
	calls := 0
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		var payload map[string]any
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if request.Header.Get("x-api-key") != "key" || request.Header.Get("anthropic-version") == "" {
			t.Fatalf("missing Anthropic headers: %v", request.Header)
		}
		if calls == 1 {
			messages := payload["messages"].([]any)
			last := messages[len(messages)-1].(map[string]any)
			if last["role"] != "user" {
				t.Fatalf("tool result role = %v", last["role"])
			}
			content := last["content"].([]any)[0].(map[string]any)
			if content["type"] != "tool_result" || content["tool_use_id"] != "tool-1" || strings.Contains(content["content"].(string), "git status") {
				t.Fatalf("tool result block = %#v", content)
			}
		}
		response := responses[calls]
		calls++
		return jsonResponse(response), nil
	})}
	model := &Anthropic{Config: Config{APIKey: "key", Model: "model", Endpoint: "https://example.test/messages", Client: client}}
	conversation, err := model.Start("system", "user", agentruntime.ShellDefinition)
	if err != nil {
		t.Fatal(err)
	}
	first, err := conversation.Next(context.Background(), nil)
	if err != nil || first.ShellCall == nil || first.ShellCall.Command != "git status" {
		t.Fatalf("first decision = %+v, err=%v", first, err)
	}
	second, err := conversation.Next(context.Background(), &agentruntime.ToolResult{ExitCode: 0, Stdout: "clean"})
	if err != nil || second.Text != "clean" {
		t.Fatalf("second decision = %+v, err=%v", second, err)
	}
}

func TestAnthropicToolDisabledResponseOmitsTools(t *testing.T) {
	responses := []string{
		`{"id":"msg-1","stop_reason":"tool_use","content":[{"type":"tool_use","id":"tool-1","name":"shell","input":{"command":"true"}}]}`,
		`{"id":"msg-2","stop_reason":"end_turn","content":[{"type":"text","text":"final handoff"}]}`,
	}
	calls := 0
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		var payload map[string]any
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if calls == 1 {
			if _, exists := payload["tools"]; exists {
				t.Fatalf("tools present in tool-disabled request: %#v", payload["tools"])
			}
			messages := payload["messages"].([]any)
			content := messages[len(messages)-1].(map[string]any)["content"].([]any)
			if len(content) != 2 || content[0].(map[string]any)["type"] != "tool_result" || !strings.Contains(content[1].(map[string]any)["text"].(string), "handoff") {
				t.Fatalf("unexpected finalization content: %#v", content)
			}
		}
		response := responses[calls]
		calls++
		return jsonResponse(response), nil
	})}
	conversation, err := (&Anthropic{Config: Config{APIKey: "key", Model: "model", Endpoint: "https://example.test/messages", Client: client}}).Start("system", "user", agentruntime.ShellDefinition)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conversation.Next(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	decision, err := conversation.RespondWithoutTools(context.Background(), &agentruntime.ToolResult{ExitCode: 0}, "write a handoff")
	if err != nil || decision.Text != "final handoff" {
		t.Fatalf("decision=%+v err=%v", decision, err)
	}
}

func clientReturning(body string) *http.Client {
	return &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return jsonResponse(body), nil
	})}
}

func jsonResponse(body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Status:     "200 OK",
		Header:     http.Header{"X-Request-Id": []string{"request-1"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}
