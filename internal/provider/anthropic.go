package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	agentruntime "github.com/Saieshwar5/ayati-code/internal/runtime"
)

type Anthropic struct {
	Config Config
}

func (p *Anthropic) Start(systemPrompt, userPrompt string, tool agentruntime.ToolDefinition) (agentruntime.Conversation, error) {
	if err := p.Config.validate(); err != nil {
		return nil, err
	}
	return &anthropicConversation{
		config:       p.Config,
		systemPrompt: systemPrompt,
		tool:         tool,
		messages:     []anthropicMessage{{Role: "user", Content: userPrompt}},
	}, nil
}

type anthropicConversation struct {
	config        Config
	systemPrompt  string
	tool          agentruntime.ToolDefinition
	messages      []anthropicMessage
	pendingCallID string
}

type anthropicMessage struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
}

func (c *anthropicConversation) Next(ctx context.Context, observation *agentruntime.ToolResult) (agentruntime.Decision, error) {
	return c.complete(ctx, observation, "", true)
}

func (c *anthropicConversation) RespondWithoutTools(ctx context.Context, observation *agentruntime.ToolResult, instruction string) (agentruntime.Decision, error) {
	return c.complete(ctx, observation, instruction, false)
}

func (c *anthropicConversation) complete(ctx context.Context, observation *agentruntime.ToolResult, instruction string, toolsEnabled bool) (agentruntime.Decision, error) {
	if !toolsEnabled && strings.TrimSpace(instruction) == "" {
		return agentruntime.Decision{}, fmt.Errorf("tool-disabled response instruction is required")
	}
	messages := append([]anthropicMessage(nil), c.messages...)
	pendingCallID := c.pendingCallID
	if observation != nil {
		if pendingCallID == "" {
			return agentruntime.Decision{}, fmt.Errorf("received shell result without a pending Anthropic tool call")
		}
		content := []any{map[string]any{
			"type": "tool_result", "tool_use_id": pendingCallID, "content": observation.ModelOutput(),
		}}
		if !toolsEnabled {
			content = append(content, map[string]any{"type": "text", "text": instruction})
		}
		messages = append(messages, anthropicMessage{Role: "user", Content: content})
		pendingCallID = ""
	} else if !toolsEnabled {
		messages = append(messages, anthropicMessage{Role: "user", Content: instruction})
	}
	maxTokens := c.config.MaxOutputTokens
	if maxTokens <= 0 {
		maxTokens = 8192
	}
	request := map[string]any{
		"model":      c.config.Model,
		"max_tokens": maxTokens,
		"system":     c.systemPrompt,
		"messages":   messages,
	}
	if toolsEnabled {
		request["tools"] = []any{map[string]any{
			"name": c.tool.Name, "description": c.tool.Description, "input_schema": shellParameters(),
		}}
	}
	var response struct {
		ID         string            `json:"id"`
		StopReason string            `json:"stop_reason"`
		Content    []json.RawMessage `json:"content"`
		Usage      struct {
			InputTokens              int64 `json:"input_tokens"`
			OutputTokens             int64 `json:"output_tokens"`
			CacheCreationInputTokens int64 `json:"cache_creation_input_tokens"`
			CacheReadInputTokens     int64 `json:"cache_read_input_tokens"`
		} `json:"usage"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error,omitempty"`
	}
	headers, err := postJSON(ctx, c.config, map[string]string{
		"x-api-key": c.config.APIKey, "anthropic-version": "2023-06-01",
	}, request, &response)
	if err != nil {
		return agentruntime.Decision{}, err
	}
	if response.Error != nil {
		return agentruntime.Decision{}, fmt.Errorf("provider: %s", response.Error.Message)
	}
	decision := agentruntime.Decision{
		StopReason:        response.StopReason,
		ProviderRequestID: requestID(headers),
		Usage: agentruntime.Usage{
			InputTokens:  response.Usage.InputTokens,
			OutputTokens: response.Usage.OutputTokens,
			CachedTokens: response.Usage.CacheReadInputTokens,
			TotalTokens:  response.Usage.InputTokens + response.Usage.OutputTokens,
		},
	}
	if decision.ProviderRequestID == "" {
		decision.ProviderRequestID = response.ID
	}
	toolCalls := 0
	var text []string
	for _, raw := range response.Content {
		var block struct {
			Type  string          `json:"type"`
			Text  string          `json:"text"`
			ID    string          `json:"id"`
			Name  string          `json:"name"`
			Input json.RawMessage `json:"input"`
		}
		if err := json.Unmarshal(raw, &block); err != nil {
			return agentruntime.Decision{}, fmt.Errorf("decode Anthropic content block: %w", err)
		}
		switch block.Type {
		case "text":
			if block.Text != "" {
				text = append(text, block.Text)
			}
		case "tool_use":
			toolCalls++
			if !toolsEnabled {
				return agentruntime.Decision{}, fmt.Errorf("provider returned a tool call while tools were disabled")
			}
			if block.Name != c.tool.Name {
				return agentruntime.Decision{}, fmt.Errorf("provider requested unknown tool %q", block.Name)
			}
			var arguments agentruntime.ShellCall
			if err := json.Unmarshal(block.Input, &arguments); err != nil {
				return agentruntime.Decision{}, fmt.Errorf("decode shell arguments: %w", err)
			}
			decision.ShellCall = &arguments
			pendingCallID = block.ID
		}
	}
	if toolCalls > 1 {
		return agentruntime.Decision{}, fmt.Errorf("provider returned %d tool calls; Ayati permits one per decision", toolCalls)
	}
	decision.Text = strings.Join(text, "\n")
	messages = append(messages, anthropicMessage{Role: "assistant", Content: response.Content})
	c.messages = messages
	c.pendingCallID = pendingCallID
	return decision, nil
}
