package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Saieshwar5/ayati-code/internal/agent"
)

const (
	ProviderID       = "openai"
	defaultEndpoint  = "https://api.openai.com/v1"
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
		return nil, fmt.Errorf("OpenAI API key is required")
	}
	return &Client{
		apiKey: strings.TrimSpace(apiKey), endpoint: defaultEndpoint,
		httpClient: &http.Client{Timeout: 2 * time.Minute},
	}, nil
}

func Verify(ctx context.Context, apiKey, model string) error {
	client, err := New(apiKey)
	if err != nil {
		return err
	}
	return client.CheckModel(ctx, model)
}

func (c *Client) CheckModel(ctx context.Context, model string) error {
	model = strings.TrimSpace(model)
	if model == "" {
		return fmt.Errorf("OpenAI model is required")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet,
		c.endpoint+"/models/"+url.PathEscape(model), nil)
	if err != nil {
		return fmt.Errorf("create OpenAI model request: %w", err)
	}
	c.authorize(request)
	response, err := c.httpClient.Do(request)
	if err != nil {
		return fmt.Errorf("check OpenAI connection: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("OpenAI returned %s", response.Status)
	}
	return nil
}

func (c *Client) Next(ctx context.Context, request agent.Request) (agent.Message, error) {
	if strings.TrimSpace(request.Model) == "" {
		return agent.Message{}, fmt.Errorf("OpenAI model is required")
	}
	messages := make([]agent.Message, 0, len(request.Messages)+1)
	if strings.TrimSpace(request.SystemPrompt) != "" {
		messages = append(messages, agent.Message{Role: "system", Content: request.SystemPrompt})
	}
	messages = append(messages, request.Messages...)
	tools := []chatTool(nil)
	var parallelToolCalls *bool
	if !request.DisableShell {
		tools = []chatTool{shellTool()}
		disabled := false
		parallelToolCalls = &disabled
	}
	body, err := json.Marshal(chatRequest{
		Model: request.Model, Messages: messages, Tools: tools,
		ParallelToolCalls: parallelToolCalls, MaxCompletionTokens: maxOutputTokens,
	})
	if err != nil {
		return agent.Message{}, fmt.Errorf("encode OpenAI request: %w", err)
	}
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.endpoint+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return agent.Message{}, fmt.Errorf("create OpenAI request: %w", err)
	}
	c.authorize(httpRequest)
	response, err := c.httpClient.Do(httpRequest)
	if err != nil {
		return agent.Message{}, fmt.Errorf("send OpenAI request: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return agent.Message{}, fmt.Errorf("OpenAI returned %s", response.Status)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil {
		return agent.Message{}, fmt.Errorf("read OpenAI response: %w", err)
	}
	if len(data) > maxResponseBytes {
		return agent.Message{}, fmt.Errorf("OpenAI response exceeds %d bytes", maxResponseBytes)
	}
	var decoded chatResponse
	if err := json.Unmarshal(data, &decoded); err != nil {
		return agent.Message{}, fmt.Errorf("decode OpenAI response: %w", err)
	}
	if len(decoded.Choices) != 1 {
		return agent.Message{}, fmt.Errorf("OpenAI returned %d choices", len(decoded.Choices))
	}
	message := decoded.Choices[0].Message
	content := ""
	if message.Content != nil {
		content = *message.Content
	}
	return agent.Message{Role: message.Role, Content: content, ToolCalls: message.ToolCalls}, nil
}

func (c *Client) authorize(request *http.Request) {
	request.Header.Set("Authorization", "Bearer "+c.apiKey)
	request.Header.Set("Content-Type", "application/json")
}

type chatRequest struct {
	Model               string          `json:"model"`
	Messages            []agent.Message `json:"messages"`
	Tools               []chatTool      `json:"tools,omitempty"`
	ParallelToolCalls   *bool           `json:"parallel_tool_calls,omitempty"`
	MaxCompletionTokens int             `json:"max_completion_tokens"`
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
				"command": map[string]any{
					"type": "string", "description": "Shell command to run in the persistent workspace sandbox.",
				},
			},
			"required": []string{"command"},
		},
	}}
}
