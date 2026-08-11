package fireworks

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Saieshwar5/ayati-code/internal/agent"
)

const (
	endpoint         = "https://api.fireworks.ai/inference/v1/chat/completions"
	maxResponseBytes = 4 << 20
	maxOutputTokens  = 8192
)

type Client struct {
	apiKey     string
	endpoint   string
	httpClient *http.Client
}

func New(apiKey string) (*Client, error) {
	if strings.TrimSpace(apiKey) == "" {
		return nil, fmt.Errorf("Fireworks API key is required")
	}
	return &Client{
		apiKey: apiKey, endpoint: endpoint,
		httpClient: &http.Client{Timeout: 2 * time.Minute},
	}, nil
}

func (c *Client) Next(ctx context.Context, request agent.Request) (agent.Message, error) {
	if strings.TrimSpace(request.Model) == "" {
		return agent.Message{}, fmt.Errorf("Fireworks model is required")
	}
	messages := make([]agent.Message, 0, len(request.Messages)+1)
	if strings.TrimSpace(request.SystemPrompt) != "" {
		messages = append(messages, agent.Message{Role: "system", Content: request.SystemPrompt})
	}
	messages = append(messages, request.Messages...)
	body, err := json.Marshal(chatRequest{
		Model: request.Model, Messages: messages, Tools: []chatTool{shellTool()},
		ParallelToolCalls: false, MaxTokens: maxOutputTokens, Stream: false,
	})
	if err != nil {
		return agent.Message{}, fmt.Errorf("encode Fireworks request: %w", err)
	}
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return agent.Message{}, fmt.Errorf("create Fireworks request: %w", err)
	}
	httpRequest.Header.Set("Authorization", "Bearer "+c.apiKey)
	httpRequest.Header.Set("Content-Type", "application/json")
	response, err := c.httpClient.Do(httpRequest)
	if err != nil {
		return agent.Message{}, fmt.Errorf("send Fireworks request: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return agent.Message{}, fmt.Errorf("Fireworks returned %s", response.Status)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil {
		return agent.Message{}, fmt.Errorf("read Fireworks response: %w", err)
	}
	if len(data) > maxResponseBytes {
		return agent.Message{}, fmt.Errorf("Fireworks response exceeds %d bytes", maxResponseBytes)
	}
	var decoded chatResponse
	if err := json.Unmarshal(data, &decoded); err != nil {
		return agent.Message{}, fmt.Errorf("decode Fireworks response: %w", err)
	}
	if len(decoded.Choices) != 1 {
		return agent.Message{}, fmt.Errorf("Fireworks returned %d choices", len(decoded.Choices))
	}
	message := decoded.Choices[0].Message
	content := ""
	if message.Content != nil {
		content = *message.Content
	}
	return agent.Message{
		Role: message.Role, Content: content, ToolCalls: message.ToolCalls,
	}, nil
}

type chatRequest struct {
	Model             string          `json:"model"`
	Messages          []agent.Message `json:"messages"`
	Tools             []chatTool      `json:"tools"`
	ParallelToolCalls bool            `json:"parallel_tool_calls"`
	MaxTokens         int             `json:"max_tokens"`
	Stream            bool            `json:"stream"`
}

type chatTool struct {
	Type     string       `json:"type"`
	Function chatFunction `json:"function"`
}

type chatFunction struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

type chatResponse struct {
	Choices []struct {
		Message struct {
			Role      string           `json:"role"`
			Content   *string          `json:"content"`
			ToolCalls []agent.ToolCall `json:"tool_calls"`
		} `json:"message"`
	} `json:"choices"`
}

func shellTool() chatTool {
	return chatTool{Type: "function", Function: chatFunction{
		Name: "shell", Description: "Run one shell command in the current project and return its bounded result.",
		Parameters: map[string]any{
			"type": "object", "additionalProperties": false,
			"properties": map[string]any{
				"command": map[string]any{"type": "string", "description": "Shell command to run."},
			},
			"required": []string{"command"},
		},
	}}
}
