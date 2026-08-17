package openai

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/Saieshwar5/ayati-code/internal/agent"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (function roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}

func TestClientSendsShellToolAndDecodesToolCall(t *testing.T) {
	client := testClient(t, func(request *http.Request) *http.Response {
		if request.URL.Path != "/chat/completions" || request.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("request = %s, authorization = %q", request.URL.Path, request.Header.Get("Authorization"))
		}
		var body struct {
			MaxCompletionTokens int             `json:"max_completion_tokens"`
			ParallelToolCalls   bool            `json:"parallel_tool_calls"`
			Messages            []agent.Message `json:"messages"`
			Tools               []chatTool      `json:"tools"`
		}
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if body.MaxCompletionTokens != maxOutputTokens || body.ParallelToolCalls ||
			len(body.Tools) != 1 || body.Tools[0].Function.Name != "shell" || len(body.Messages) != 2 {
			t.Errorf("body = %#v", body)
		}
		return response(http.StatusOK,
			`{"choices":[{"message":{"role":"assistant","content":null,"tool_calls":[{"id":"call-1","type":"function","function":{"name":"shell","arguments":"{\"command\":\"pwd\"}"}}]}}]}`)
	})
	message, err := client.Next(context.Background(), agent.Request{
		Model: "gpt-test", SystemPrompt: "system", Messages: []agent.Message{{Role: "user", Content: "where"}},
	})
	if err != nil || len(message.ToolCalls) != 1 || message.ToolCalls[0].Function.Name != "shell" {
		t.Fatalf("message = %#v, error = %v", message, err)
	}
}

func TestClientOmitsShellAndDoesNotExposeErrorBody(t *testing.T) {
	client := testClient(t, func(request *http.Request) *http.Response {
		var body map[string]any
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if _, exists := body["tools"]; exists {
			t.Errorf("request included tools: %#v", body)
		}
		return response(http.StatusUnauthorized, `{"error":"test-key"}`)
	})
	_, err := client.Next(context.Background(), agent.Request{Model: "gpt-test", DisableShell: true})
	if err == nil || strings.Contains(err.Error(), "test-key") {
		t.Fatalf("error = %v", err)
	}
}

func TestCheckModelUsesModelsEndpoint(t *testing.T) {
	client := testClient(t, func(request *http.Request) *http.Response {
		if request.Method != http.MethodGet || request.URL.Path != "/models/gpt-test" {
			t.Errorf("request = %s %s", request.Method, request.URL.Path)
		}
		return response(http.StatusOK, `{}`)
	})
	if err := client.CheckModel(context.Background(), "gpt-test"); err != nil {
		t.Fatalf("CheckModel: %v", err)
	}
}

func testClient(t *testing.T, handler func(*http.Request) *http.Response) *Client {
	t.Helper()
	return &Client{
		apiKey: "test-key", endpoint: "https://example.invalid",
		httpClient: &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			return handler(request), nil
		})},
	}
}

func response(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status, Status: http.StatusText(status), Body: io.NopCloser(strings.NewReader(body)),
	}
}
