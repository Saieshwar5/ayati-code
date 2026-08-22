package model

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Saieshwar5/perpetual/internal/execution"
)

const shellToolSchema = `{
  "type": "function",
  "function": {
    "name": "shell",
    "description": "Execute a shell command in the workspace environment",
    "parameters": {
      "type": "object",
      "properties": {
        "command": { "type": "string" }
      },
      "required": ["command"]
    }
  }
}`

// OpenAICompatibleProvider talks to any /chat/completions endpoint (OpenAI,
// Groq, Ollama, vLLM, etc.). It never receives or stores credentials beyond
// the controller process.
type OpenAICompatibleProvider struct {
	client    *http.Client
	apiKey    string
	model     string
	endpoint  string
	maxTokens int64
}

// NewOpenAICompatible builds a provider from config.
func NewOpenAICompatible(config Config) (*OpenAICompatibleProvider, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	endpoint := strings.TrimSpace(config.BaseURL)
	if endpoint == "" {
		endpoint = DefaultBaseURL
	}
	if !strings.Contains(endpoint, "/chat/completions") {
		endpoint = strings.TrimSuffix(endpoint, "/") + "/chat/completions"
	}
	maxTokens := config.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 4096
	}
	return &OpenAICompatibleProvider{
		client:    &http.Client{Timeout: 90 * time.Second},
		apiKey:    config.APIKey,
		model:     config.Model,
		endpoint:  endpoint,
		maxTokens: maxTokens,
	}, nil
}

// Complete implements execution.ModelProvider.
func (p *OpenAICompatibleProvider) Complete(ctx context.Context, req execution.ModelRequest) (execution.ModelResponse, error) {
	request := openAIRequest{
		Model:     p.model,
		MaxTokens: p.maxTokens,
		Tools:     []json.RawMessage{json.RawMessage(shellToolSchema)},
	}
	if strings.TrimSpace(req.System) != "" {
		request.Messages = append(request.Messages, chatMessage{
			Role: "system", Content: req.System,
		})
	}
	content := strings.Join(req.Messages, "\n")
	if content != "" {
		request.Messages = append(request.Messages, chatMessage{
			Role: "user", Content: content,
		})
	}
	body, err := json.Marshal(request)
	if err != nil {
		return execution.ModelResponse{}, fmt.Errorf("encode model request: %w", err)
	}
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint, bytes.NewReader(body))
	if err != nil {
		return execution.ModelResponse{}, err
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	if p.apiKey != "" {
		httpRequest.Header.Set("Authorization", "Bearer "+p.apiKey)
	}
	response, err := p.client.Do(httpRequest)
	if err != nil {
		return execution.ModelResponse{}, err
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return execution.ModelResponse{}, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return execution.ModelResponse{}, fmt.Errorf("model endpoint status %d: %s", response.StatusCode, strings.TrimSpace(string(raw)))
	}
	var decoded openAIResponse
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return execution.ModelResponse{}, fmt.Errorf("decode model response: %w", err)
	}
	if len(decoded.Choices) == 0 {
		return execution.ModelResponse{}, errors.New("model returned no choices")
	}
	choice := decoded.Choices[0]
	modelResponse := execution.ModelResponse{
		Content:    choice.Message.Content,
		StopReason: finishReason(choice.FinishReason),
		Usage: execution.TokenUsage{
			Input: int64(decoded.Usage.PromptTokens), Output: int64(decoded.Usage.CompletionTokens),
			Total: int64(decoded.Usage.TotalTokens),
		},
	}
	for _, call := range choice.Message.ToolCalls {
		arguments := map[string]any{}
		if err := json.Unmarshal([]byte(call.Function.Arguments), &arguments); err == nil {
			modelResponse.ToolCalls = append(modelResponse.ToolCalls, execution.ToolCall{
				Name: call.Function.Name, Arguments: arguments,
			})
		}
	}
	return modelResponse, nil
}

func finishReason(value string) string {
	switch value {
	case "stop":
		return "stop"
	case "length":
		return "length"
	case "tool_calls":
		return ""
	case "error":
		return "error"
	default:
		return ""
	}
}

type openAIRequest struct {
	Model     string            `json:"model"`
	Messages  []chatMessage     `json:"messages"`
	MaxTokens int64             `json:"max_tokens"`
	Tools     []json.RawMessage `json:"tools"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openAIResponse struct {
	Choices []struct {
		FinishReason string `json:"finish_reason"`
		Message      struct {
			Content   string `json:"content"`
			ToolCalls []struct {
				ID       string `json:"id"`
				Type     string `json:"type"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"message"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}
