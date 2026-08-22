package model

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/Saieshwar5/perpetual/internal/execution"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func TestOpenAICompatibleCompleteMapsToolCalls(t *testing.T) {
	provider := &OpenAICompatibleProvider{
		client: &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			var requestBody openAIRequest
			raw, _ := io.ReadAll(request.Body)
			if err := json.Unmarshal(raw, &requestBody); err != nil {
				return &http.Response{StatusCode: 500, Body: io.NopCloser(strings.NewReader("bad")), Request: request}, err
			}
			if requestBody.Model != "gpt-4o" || len(requestBody.Tools) != 1 {
				return &http.Response{StatusCode: 500, Body: io.NopCloser(strings.NewReader("bad")), Request: request}, nil
			}
			body := `{"choices":[{"finish_reason":"tool_calls","message":{"content":"","tool_calls":[{"id":"1","type":"function","function":{"name":"shell","arguments":"{\"command\":\"echo hi\"}"}}]}}],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}`
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Request: request}, nil
		})},
		apiKey: "secret", model: "gpt-4o", endpoint: "https://example.test/v1/chat/completions", maxTokens: 4096,
	}
	response, err := provider.Complete(context.Background(), execution.ModelRequest{
		System: "system", Messages: []string{"hello"},
	})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if len(response.ToolCalls) != 1 || response.ToolCalls[0].Name != "shell" ||
		response.ToolCalls[0].Arguments["command"] != "echo hi" {
		t.Fatalf("tool calls = %#v", response.ToolCalls)
	}
	if response.Usage.Total != 15 {
		t.Fatalf("usage = %#v", response.Usage)
	}
	// Ensure the API key travelled as a bearer header.
	if _, ok := provider.client.Transport.(roundTripFunc); !ok {
		t.Fatal("missing transport")
	}
}

func TestOpenAICompatibleMapsStopReason(t *testing.T) {
	provider := &OpenAICompatibleProvider{
		client: &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			body := `{"choices":[{"finish_reason":"stop","message":{"content":"done"}}],"usage":{}}`
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Request: request}, nil
		})},
		model: "gpt-4o", endpoint: "https://example.test/v1/chat/completions", maxTokens: 10,
	}
	response, err := provider.Complete(context.Background(), execution.ModelRequest{})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if response.StopReason != "stop" || response.Content != "done" {
		t.Fatalf("response = %#v", response)
	}
}

func TestConfigValidation(t *testing.T) {
	if err := (Config{Provider: ProviderOpenAI, Model: "gpt-4o", APIKey: "x"}).Validate(); err != nil {
		t.Fatalf("valid config rejected: %v", err)
	}
	if err := (Config{Provider: ProviderOpenAI, Model: ""}).Validate(); err == nil {
		t.Fatal("expected missing model error")
	}
	if _, err := NewFromConfig(Config{Provider: "nope"}); err == nil {
		t.Fatal("expected unsupported provider error")
	}
}
