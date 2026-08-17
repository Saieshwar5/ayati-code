package openaichat

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
	maxResponseBytes = 4 << 20
	maxOutputTokens  = 8192
)

type TokenLimitField string

const (
	MaxTokens           TokenLimitField = "max_tokens"
	MaxCompletionTokens TokenLimitField = "max_completion_tokens"
)

type Options struct {
	ProviderName                string
	Endpoint                    string
	APIKey                      string
	TokenLimitField             TokenLimitField
	SupportsParallelToolControl bool
}

type Client struct {
	providerName                string
	endpoint                    string
	apiKey                      string
	tokenLimitField             TokenLimitField
	supportsParallelToolControl bool
	httpClient                  *http.Client
}

func New(options Options) (*Client, error) {
	options.ProviderName = strings.TrimSpace(options.ProviderName)
	options.Endpoint = strings.TrimRight(strings.TrimSpace(options.Endpoint), "/")
	options.APIKey = strings.TrimSpace(options.APIKey)
	if options.ProviderName == "" || options.Endpoint == "" {
		return nil, fmt.Errorf("provider name and endpoint are required")
	}
	if options.APIKey == "" {
		return nil, fmt.Errorf("%s API key is required", options.ProviderName)
	}
	if options.TokenLimitField == "" {
		options.TokenLimitField = MaxTokens
	}
	if options.TokenLimitField != MaxTokens && options.TokenLimitField != MaxCompletionTokens {
		return nil, fmt.Errorf("unsupported token limit field %q", options.TokenLimitField)
	}
	return &Client{
		providerName: options.ProviderName, endpoint: options.Endpoint, apiKey: options.APIKey,
		tokenLimitField:             options.TokenLimitField,
		supportsParallelToolControl: options.SupportsParallelToolControl,
		httpClient:                  &http.Client{Timeout: 2 * time.Minute},
	}, nil
}

func Verify(ctx context.Context, providerName, endpoint, apiKey string) error {
	client, err := New(Options{ProviderName: providerName, Endpoint: endpoint, APIKey: apiKey})
	if err != nil {
		return err
	}
	return client.Check(ctx)
}

func (c *Client) Check(ctx context.Context) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.endpoint+"/models", nil)
	if err != nil {
		return fmt.Errorf("create %s connection request: %w", c.providerName, err)
	}
	c.authorize(request)
	response, err := c.httpClient.Do(request)
	if err != nil {
		return fmt.Errorf("check %s connection: %w", c.providerName, err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("%s returned %s", c.providerName, response.Status)
	}
	return nil
}

func (c *Client) Next(ctx context.Context, request agent.Request) (agent.Message, error) {
	if strings.TrimSpace(request.Model) == "" {
		return agent.Message{}, fmt.Errorf("%s model is required", c.providerName)
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
		if c.supportsParallelToolControl {
			disabled := false
			parallelToolCalls = &disabled
		}
	}
	body := chatRequest{
		Model: request.Model, Messages: messages, Tools: tools,
		ParallelToolCalls: parallelToolCalls, Stream: false,
	}
	if c.tokenLimitField == MaxCompletionTokens {
		body.MaxCompletionTokens = maxOutputTokens
	} else {
		body.MaxTokens = maxOutputTokens
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		return agent.Message{}, fmt.Errorf("encode %s request: %w", c.providerName, err)
	}
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.endpoint+"/chat/completions", bytes.NewReader(encoded))
	if err != nil {
		return agent.Message{}, fmt.Errorf("create %s request: %w", c.providerName, err)
	}
	c.authorize(httpRequest)
	response, err := c.httpClient.Do(httpRequest)
	if err != nil {
		return agent.Message{}, fmt.Errorf("send %s request: %w", c.providerName, err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return agent.Message{}, fmt.Errorf("%s returned %s", c.providerName, response.Status)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil {
		return agent.Message{}, fmt.Errorf("read %s response: %w", c.providerName, err)
	}
	if len(data) > maxResponseBytes {
		return agent.Message{}, fmt.Errorf("%s response exceeds %d bytes", c.providerName, maxResponseBytes)
	}
	var decoded chatResponse
	if err := json.Unmarshal(data, &decoded); err != nil {
		return agent.Message{}, fmt.Errorf("decode %s response: %w", c.providerName, err)
	}
	if len(decoded.Choices) != 1 {
		return agent.Message{}, fmt.Errorf("%s returned %d choices", c.providerName, len(decoded.Choices))
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
	MaxTokens           int             `json:"max_tokens,omitempty"`
	MaxCompletionTokens int             `json:"max_completion_tokens,omitempty"`
	Stream              bool            `json:"stream"`
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
