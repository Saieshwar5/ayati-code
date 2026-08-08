package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	agentruntime "github.com/Saieshwar5/ayati-code/internal/runtime"
)

type OpenAIChat struct {
	Config Config
}

func (p *OpenAIChat) Start(systemPrompt, userPrompt string, tool agentruntime.ToolDefinition) (agentruntime.Conversation, error) {
	if err := p.Config.validate(); err != nil {
		return nil, err
	}
	return &openAIChatConversation{
		config: p.Config,
		tool:   tool,
		messages: []chatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
	}, nil
}

type openAIChatConversation struct {
	config        Config
	tool          agentruntime.ToolDefinition
	messages      []chatMessage
	pendingCallID string
}

type chatMessage struct {
	Role       string         `json:"role"`
	Content    string         `json:"content,omitempty"`
	ToolCalls  []chatToolCall `json:"tool_calls,omitempty"`
	ToolCallID string         `json:"tool_call_id,omitempty"`
}

type chatToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

func (c *openAIChatConversation) Next(ctx context.Context, observation *agentruntime.ToolResult) (agentruntime.Decision, error) {
	return c.complete(ctx, observation, "", true)
}

func (c *openAIChatConversation) RespondWithoutTools(ctx context.Context, observation *agentruntime.ToolResult, instruction string) (agentruntime.Decision, error) {
	return c.complete(ctx, observation, instruction, false)
}

func (c *openAIChatConversation) complete(ctx context.Context, observation *agentruntime.ToolResult, instruction string, toolsEnabled bool) (agentruntime.Decision, error) {
	messages := append([]chatMessage(nil), c.messages...)
	pendingCallID := c.pendingCallID
	if observation != nil {
		if pendingCallID == "" {
			return agentruntime.Decision{}, fmt.Errorf("received shell result without a pending tool call")
		}
		messages = append(messages, chatMessage{Role: "tool", ToolCallID: pendingCallID, Content: observation.ModelOutput()})
		pendingCallID = ""
	}
	if !toolsEnabled {
		if strings.TrimSpace(instruction) == "" {
			return agentruntime.Decision{}, fmt.Errorf("tool-disabled response instruction is required")
		}
		messages = append(messages, chatMessage{Role: "user", Content: instruction})
	}

	type functionDefinition struct {
		Name        string         `json:"name"`
		Description string         `json:"description"`
		Parameters  map[string]any `json:"parameters"`
	}
	request := struct {
		Model      string        `json:"model"`
		Messages   []chatMessage `json:"messages"`
		Tools      []any         `json:"tools,omitempty"`
		ToolChoice string        `json:"tool_choice,omitempty"`
		MaxTokens  int           `json:"max_tokens,omitempty"`
	}{
		Model:     c.config.Model,
		Messages:  messages,
		MaxTokens: c.config.MaxOutputTokens,
	}
	if toolsEnabled {
		request.Tools = []any{map[string]any{"type": "function", "function": functionDefinition{Name: c.tool.Name, Description: c.tool.Description, Parameters: shellParameters()}}}
		request.ToolChoice = "auto"
	}
	var response struct {
		ID      string `json:"id"`
		Choices []struct {
			Message      chatMessage `json:"message"`
			FinishReason string      `json:"finish_reason"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int64 `json:"prompt_tokens"`
			CompletionTokens int64 `json:"completion_tokens"`
			TotalTokens      int64 `json:"total_tokens"`
			PromptDetails    struct {
				CachedTokens int64 `json:"cached_tokens"`
			} `json:"prompt_tokens_details"`
			CompletionDetails struct {
				ReasoningTokens int64 `json:"reasoning_tokens"`
			} `json:"completion_tokens_details"`
		} `json:"usage"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error,omitempty"`
	}
	headers, err := postJSON(ctx, c.config, map[string]string{"Authorization": "Bearer " + c.config.APIKey}, request, &response)
	if err != nil {
		return agentruntime.Decision{}, err
	}
	if response.Error != nil {
		return agentruntime.Decision{}, fmt.Errorf("provider: %s", response.Error.Message)
	}
	if len(response.Choices) == 0 {
		return agentruntime.Decision{}, fmt.Errorf("provider returned no choices")
	}
	choice := response.Choices[0]
	if len(choice.Message.ToolCalls) > 1 {
		return agentruntime.Decision{}, fmt.Errorf("provider returned %d tool calls; Ayati permits one per decision", len(choice.Message.ToolCalls))
	}
	decision := agentruntime.Decision{
		Text:              choice.Message.Content,
		StopReason:        choice.FinishReason,
		ProviderRequestID: requestID(headers),
		Usage: agentruntime.Usage{
			InputTokens:     response.Usage.PromptTokens,
			OutputTokens:    response.Usage.CompletionTokens,
			CachedTokens:    response.Usage.PromptDetails.CachedTokens,
			ReasoningTokens: response.Usage.CompletionDetails.ReasoningTokens,
			TotalTokens:     response.Usage.TotalTokens,
		},
	}
	if decision.ProviderRequestID == "" {
		decision.ProviderRequestID = response.ID
	}
	if len(choice.Message.ToolCalls) == 1 {
		if !toolsEnabled {
			return agentruntime.Decision{}, fmt.Errorf("provider returned a tool call while tools were disabled")
		}
		call := choice.Message.ToolCalls[0]
		if call.Function.Name != c.tool.Name {
			return agentruntime.Decision{}, fmt.Errorf("provider requested unknown tool %q", call.Function.Name)
		}
		var arguments agentruntime.ShellCall
		if err := json.Unmarshal([]byte(call.Function.Arguments), &arguments); err != nil {
			return agentruntime.Decision{}, fmt.Errorf("decode shell arguments: %w", err)
		}
		decision.ShellCall = &arguments
		pendingCallID = call.ID
	}
	messages = append(messages, choice.Message)
	c.messages = messages
	c.pendingCallID = pendingCallID
	return decision, nil
}

func requestID(headers http.Header) string {
	for _, name := range []string{"x-request-id", "request-id"} {
		if value := strings.TrimSpace(headers.Get(name)); value != "" {
			return value
		}
	}
	return ""
}
