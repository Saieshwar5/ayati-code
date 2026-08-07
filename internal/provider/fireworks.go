package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/sai-eshwar/no-nonsense-coding-ai/internal/chat"
)

const DefaultFireworksURL = "https://api.fireworks.ai/inference/v1/chat/completions"
const DefaultFireworksModelsURL = "https://api.fireworks.ai/v1/"

type Fireworks struct {
	APIKey    string
	Model     string
	BaseURL   string
	ModelsURL string
	Client    *http.Client
}

type completionRequest struct {
	Model       string         `json:"model"`
	Messages    []chat.Message `json:"messages"`
	Tools       []tool         `json:"tools,omitempty"`
	ToolChoice  string         `json:"tool_choice,omitempty"`
	Temperature float64        `json:"temperature"`
	MaxTokens   int            `json:"max_tokens,omitempty"`
}

type tool struct {
	Type     string       `json:"type"`
	Function toolFunction `json:"function"`
}

type toolFunction struct {
	Name        string     `json:"name"`
	Description string     `json:"description"`
	Parameters  toolSchema `json:"parameters"`
}

type toolSchema struct {
	Type       string                    `json:"type"`
	Properties map[string]map[string]any `json:"properties"`
	Required   []string                  `json:"required"`
}

type completionResponse struct {
	Choices []struct {
		Message chat.Message `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func (f *Fireworks) Complete(ctx context.Context, messages []chat.Message) (chat.Message, error) {
	return f.complete(ctx, messages, true)
}

func (f *Fireworks) Summarize(ctx context.Context, transcript string) (string, error) {
	messages := []chat.Message{
		{
			Role:    "system",
			Content: "Summarize this coding session for reliable continuation. Be factual and concise. Preserve the task, decisions, files changed, commands or tests that matter, failures, current state, and next step. Do not invent completed work. Return plain text with short labeled sections.",
		},
		{Role: "user", Content: transcript},
	}
	message, err := f.complete(ctx, messages, false)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(message.Content) == "" {
		return "", fmt.Errorf("Fireworks returned an empty summary")
	}
	return message.Content, nil
}

func (f *Fireworks) ContextLimit(ctx context.Context) (int, error) {
	if strings.TrimSpace(f.APIKey) == "" {
		return 0, fmt.Errorf("FIREWORKS_API_KEY is not set")
	}
	if !strings.HasPrefix(f.Model, "accounts/") {
		return 0, fmt.Errorf("cannot query metadata for model %q", f.Model)
	}
	baseURL := f.ModelsURL
	if baseURL == "" {
		baseURL = DefaultFireworksModelsURL
	}
	requestURL := strings.TrimRight(baseURL, "/") + "/" + strings.TrimLeft(f.Model, "/")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return 0, fmt.Errorf("create Fireworks model request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+f.APIKey)
	client := f.httpClient()
	resp, err := client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("query Fireworks model: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return 0, fmt.Errorf("read Fireworks model metadata: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return 0, fmt.Errorf("Fireworks model metadata returned %s: %s", resp.Status, compactError(body))
	}
	var metadata struct {
		ContextLength int `json:"contextLength"`
	}
	if err := json.Unmarshal(body, &metadata); err != nil {
		return 0, fmt.Errorf("decode Fireworks model metadata: %w", err)
	}
	if metadata.ContextLength <= 0 {
		return 0, fmt.Errorf("Fireworks model metadata has no context length")
	}
	return metadata.ContextLength, nil
}

func (f *Fireworks) complete(ctx context.Context, messages []chat.Message, withTools bool) (chat.Message, error) {
	if strings.TrimSpace(f.APIKey) == "" {
		return chat.Message{}, fmt.Errorf("FIREWORKS_API_KEY is not set")
	}
	if strings.TrimSpace(f.Model) == "" {
		return chat.Message{}, fmt.Errorf("Fireworks model is not set")
	}

	requestBody := completionRequest{
		Model:       f.Model,
		Messages:    messages,
		Temperature: 0,
	}
	if withTools {
		requestBody.Tools = []tool{shellTool()}
		requestBody.ToolChoice = "auto"
	} else {
		requestBody.MaxTokens = 2048
	}
	body, err := json.Marshal(requestBody)
	if err != nil {
		return chat.Message{}, fmt.Errorf("encode Fireworks request: %w", err)
	}

	url := f.BaseURL
	if url == "" {
		url = DefaultFireworksURL
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return chat.Message{}, fmt.Errorf("create Fireworks request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+f.APIKey)
	req.Header.Set("Content-Type", "application/json")

	client := f.httpClient()
	resp, err := client.Do(req)
	if err != nil {
		return chat.Message{}, fmt.Errorf("call Fireworks: %w", err)
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return chat.Message{}, fmt.Errorf("read Fireworks response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return chat.Message{}, fmt.Errorf("Fireworks returned %s: %s", resp.Status, compactError(responseBody))
	}

	var decoded completionResponse
	if err := json.Unmarshal(responseBody, &decoded); err != nil {
		return chat.Message{}, fmt.Errorf("decode Fireworks response: %w", err)
	}
	if decoded.Error != nil {
		return chat.Message{}, fmt.Errorf("Fireworks: %s", decoded.Error.Message)
	}
	if len(decoded.Choices) == 0 {
		return chat.Message{}, fmt.Errorf("Fireworks returned no choices")
	}
	return decoded.Choices[0].Message, nil
}

func (f *Fireworks) httpClient() *http.Client {
	if f.Client != nil {
		return f.Client
	}
	return &http.Client{Timeout: 5 * time.Minute}
}

func shellTool() tool {
	return tool{
		Type: "function",
		Function: toolFunction{
			Name:        "shell",
			Description: "Run one shell command in the project working directory. Use it to inspect, edit, test, build, and operate on the codebase.",
			Parameters: toolSchema{
				Type: "object",
				Properties: map[string]map[string]any{
					"command": {"type": "string", "description": "The shell command to execute."},
				},
				Required: []string{"command"},
			},
		},
	}
}

func compactError(body []byte) string {
	const limit = 1000
	text := strings.TrimSpace(string(body))
	if len(text) > limit {
		return text[:limit] + "..."
	}
	return text
}
