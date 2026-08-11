package fireworks

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
	client := &Client{
		apiKey: "test-key", endpoint: "https://example.invalid/chat",
		httpClient: &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			if request.Header.Get("Authorization") != "Bearer test-key" {
				t.Errorf("authorization = %q", request.Header.Get("Authorization"))
			}
			var body struct {
				Stream            bool            `json:"stream"`
				ParallelToolCalls bool            `json:"parallel_tool_calls"`
				Messages          []agent.Message `json:"messages"`
				Tools             []chatTool      `json:"tools"`
			}
			if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
				t.Errorf("decode request: %v", err)
			}
			if body.Stream || body.ParallelToolCalls || len(body.Tools) != 1 || body.Tools[0].Function.Name != "shell" {
				t.Errorf("request = %#v", body)
			}
			if len(body.Messages) != 2 || body.Messages[0].Role != "system" || body.Messages[1].Role != "user" {
				t.Errorf("messages = %#v", body.Messages)
			}
			response := `{"choices":[{"message":{"role":"assistant","content":null,"tool_calls":[{"id":"call-1","type":"function","function":{"name":"shell","arguments":"{\"command\":\"pwd\"}"}}]}}]}`
			return &http.Response{
				StatusCode: http.StatusOK, Status: "200 OK", Body: io.NopCloser(strings.NewReader(response)),
			}, nil
		})},
	}
	message, err := client.Next(context.Background(), agent.Request{
		Model: "test-model", SystemPrompt: "system",
		Messages: []agent.Message{{Role: "user", Content: "where"}},
	})
	if err != nil {
		t.Fatalf("Next: %v", err)
	}
	if len(message.ToolCalls) != 1 || message.ToolCalls[0].Function.Name != "shell" {
		t.Fatalf("message = %#v", message)
	}
}

func TestClientDoesNotIncludeErrorBody(t *testing.T) {
	client := &Client{
		apiKey: "secret-key", endpoint: "https://example.invalid/chat",
		httpClient: &http.Client{Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusUnauthorized, Status: "401 Unauthorized",
				Body: io.NopCloser(strings.NewReader(`{"error":"secret-key"}`)),
			}, nil
		})},
	}
	_, err := client.Next(context.Background(), agent.Request{Model: "test"})
	if err == nil || strings.Contains(err.Error(), "secret-key") {
		t.Fatalf("error = %v", err)
	}
}
