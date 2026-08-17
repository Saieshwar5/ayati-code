package openaichat

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

func TestClientSendsOneShellToolAndDecodesToolCall(t *testing.T) {
	client := testClient(t, MaxCompletionTokens, true, func(request *http.Request) *http.Response {
		if request.URL.Path != "/chat/completions" || request.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("request = %s, authorization = %q", request.URL.Path, request.Header.Get("Authorization"))
		}
		var body struct {
			MaxTokens           int             `json:"max_tokens"`
			MaxCompletionTokens int             `json:"max_completion_tokens"`
			ParallelToolCalls   bool            `json:"parallel_tool_calls"`
			Messages            []agent.Message `json:"messages"`
			Tools               []chatTool      `json:"tools"`
		}
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if body.MaxTokens != 0 || body.MaxCompletionTokens != maxOutputTokens || body.ParallelToolCalls ||
			len(body.Tools) != 1 || body.Tools[0].Function.Name != "shell" || len(body.Messages) != 2 {
			t.Errorf("body = %#v", body)
		}
		return response(http.StatusOK,
			`{"choices":[{"message":{"role":"assistant","content":null,"tool_calls":[{"id":"call-1","type":"function","function":{"name":"shell","arguments":"{\"command\":\"pwd\"}"}}]}}]}`)
	})
	message, err := client.Next(context.Background(), agent.Request{
		Model: "test-model", SystemPrompt: "system", Messages: []agent.Message{{Role: "user", Content: "where"}},
	})
	if err != nil || len(message.ToolCalls) != 1 || message.ToolCalls[0].Function.Name != "shell" {
		t.Fatalf("message = %#v, error = %v", message, err)
	}
}

func TestClientUsesLegacyTokenFieldAndOmitsDisabledShell(t *testing.T) {
	client := testClient(t, MaxTokens, false, func(request *http.Request) *http.Response {
		var body map[string]any
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if _, exists := body["tools"]; exists || body["max_tokens"] != float64(maxOutputTokens) {
			t.Errorf("request body = %#v", body)
		}
		if _, exists := body["parallel_tool_calls"]; exists {
			t.Errorf("request included unsupported parallel_tool_calls: %#v", body)
		}
		if _, exists := body["max_completion_tokens"]; exists {
			t.Errorf("request included max_completion_tokens: %#v", body)
		}
		return response(http.StatusOK, `{"choices":[{"message":{"role":"assistant","content":"done"}}]}`)
	})
	message, err := client.Next(context.Background(), agent.Request{Model: "test", DisableShell: true})
	if err != nil || message.Content != "done" {
		t.Fatalf("message = %#v, error = %v", message, err)
	}
}

func TestClientChecksModelsEndpointAndHidesErrorBody(t *testing.T) {
	client := testClient(t, MaxTokens, false, func(request *http.Request) *http.Response {
		if request.Method != http.MethodGet || request.URL.Path != "/models" {
			t.Errorf("request = %s %s", request.Method, request.URL.Path)
		}
		return response(http.StatusUnauthorized, `{"error":"test-key"}`)
	})
	err := client.Check(context.Background())
	if err == nil || strings.Contains(err.Error(), "test-key") {
		t.Fatalf("Check error = %v", err)
	}
}

func TestClientRejectsInvalidOptions(t *testing.T) {
	for _, options := range []Options{
		{},
		{ProviderName: "Test", Endpoint: "https://example.invalid"},
		{ProviderName: "Test", Endpoint: "https://example.invalid", APIKey: "key", TokenLimitField: "other"},
	} {
		if _, err := New(options); err == nil {
			t.Fatalf("New accepted %#v", options)
		}
	}
}

func testClient(
	t *testing.T, field TokenLimitField, parallelControl bool,
	handler func(*http.Request) *http.Response,
) *Client {
	t.Helper()
	return &Client{
		providerName: "Test provider", endpoint: "https://example.invalid", apiKey: "test-key",
		tokenLimitField: field, supportsParallelToolControl: parallelControl,
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
