package provider

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/sai-eshwar/no-nonsense-coding-ai/internal/chat"
)

func TestFireworksCompleteSendsOnlyShellTool(t *testing.T) {
	transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.Header.Get("Authorization") != "Bearer secret" {
			t.Errorf("unexpected authorization header")
		}
		var body completionRequest
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if len(body.Tools) != 1 || body.Tools[0].Function.Name != "shell" {
			t.Errorf("unexpected tools: %+v", body.Tools)
		}
		return jsonResponse(http.StatusOK, `{"choices":[{"message":{"role":"assistant","tool_calls":[{"id":"call-1","type":"function","function":{"name":"shell","arguments":"{\"command\":\"pwd\"}"}}]}}]}`), nil
	})

	client := Fireworks{APIKey: "secret", Model: "test-model", Client: &http.Client{Transport: transport}}
	message, err := client.Complete(context.Background(), []chat.Message{{Role: "user", Content: "inspect"}})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if len(message.ToolCalls) != 1 || message.ToolCalls[0].Function.Name != "shell" {
		t.Fatalf("unexpected response: %+v", message)
	}
}

func TestFireworksCompleteReportsHTTPError(t *testing.T) {
	transport := roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusBadRequest, `{"error":{"message":"bad model"}}`), nil
	})
	client := Fireworks{APIKey: "secret", Model: "missing", Client: &http.Client{Transport: transport}}
	if _, err := client.Complete(context.Background(), nil); err == nil {
		t.Fatal("expected HTTP error")
	}
}

func TestFireworksSummarizeDoesNotExposeTools(t *testing.T) {
	transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		var body completionRequest
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if len(body.Tools) != 0 || body.ToolChoice != "" || body.MaxTokens != 2048 {
			t.Errorf("summarization exposed tools: %+v", body)
		}
		return jsonResponse(http.StatusOK, `{"choices":[{"message":{"role":"assistant","content":"Task: build the site. Next: test it."}}]}`), nil
	})
	client := Fireworks{APIKey: "secret", Model: "test-model", Client: &http.Client{Transport: transport}}
	summary, err := client.Summarize(context.Background(), "session records")
	if err != nil {
		t.Fatalf("Summarize: %v", err)
	}
	if summary != "Task: build the site. Next: test it." {
		t.Fatalf("unexpected summary %q", summary)
	}
}

func TestFireworksContextLimitReadsModelMetadata(t *testing.T) {
	transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.Method != http.MethodGet || request.URL.Path != "/accounts/fireworks/models/deepseek-v4-flash" {
			t.Errorf("unexpected metadata request: %s %s", request.Method, request.URL.Path)
		}
		return jsonResponse(http.StatusOK, `{"contextLength":1048576,"supportsTools":true}`), nil
	})
	client := Fireworks{
		APIKey:    "secret",
		Model:     "accounts/fireworks/models/deepseek-v4-flash",
		ModelsURL: "https://metadata.test",
		Client:    &http.Client{Transport: transport},
	}
	limit, err := client.ContextLimit(context.Background())
	if err != nil {
		t.Fatalf("ContextLimit: %v", err)
	}
	if limit != 1048576 {
		t.Fatalf("limit = %d", limit)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

func jsonResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Status:     http.StatusText(status),
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}
