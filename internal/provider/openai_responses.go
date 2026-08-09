package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	agentruntime "github.com/Saieshwar5/ayati-runtime/internal/runtime"
)

type OpenAIResponses struct {
	Config Config
}

func (p *OpenAIResponses) Start(systemPrompt, userPrompt string, tool agentruntime.ToolDefinition) (agentruntime.Conversation, error) {
	if err := p.Config.validate(); err != nil {
		return nil, err
	}
	return &openAIResponsesConversation{config: p.Config, systemPrompt: systemPrompt, userPrompt: userPrompt, tool: tool}, nil
}

type openAIResponsesConversation struct {
	config             Config
	systemPrompt       string
	userPrompt         string
	tool               agentruntime.ToolDefinition
	previousResponseID string
	pendingCallID      string
}

func (c *openAIResponsesConversation) Next(ctx context.Context, observation *agentruntime.ToolResult) (agentruntime.Decision, error) {
	return c.complete(ctx, observation, "", true)
}

func (c *openAIResponsesConversation) RespondWithoutTools(ctx context.Context, observation *agentruntime.ToolResult, instruction string) (agentruntime.Decision, error) {
	return c.complete(ctx, observation, instruction, false)
}

func (c *openAIResponsesConversation) complete(ctx context.Context, observation *agentruntime.ToolResult, instruction string, toolsEnabled bool) (agentruntime.Decision, error) {
	var input any = c.userPrompt
	pendingCallID := c.pendingCallID
	if observation != nil {
		if pendingCallID == "" || c.previousResponseID == "" {
			return agentruntime.Decision{}, fmt.Errorf("received shell result without a pending response tool call")
		}
		input = []any{map[string]any{"type": "function_call_output", "call_id": pendingCallID, "output": observation.ModelOutput()}}
		pendingCallID = ""
	}
	if !toolsEnabled {
		if strings.TrimSpace(instruction) == "" {
			return agentruntime.Decision{}, fmt.Errorf("tool-disabled response instruction is required")
		}
		message := map[string]any{"role": "user", "content": instruction}
		if items, ok := input.([]any); ok {
			input = append(items, message)
		} else {
			input = []any{map[string]any{"role": "user", "content": c.userPrompt}, message}
		}
	}
	request := map[string]any{
		"model": c.config.Model, "instructions": c.systemPrompt, "input": input,
	}
	if toolsEnabled {
		request["parallel_tool_calls"] = false
		request["tools"] = []any{map[string]any{
			"type": "function", "name": c.tool.Name, "description": c.tool.Description,
			"parameters": shellParameters(), "strict": true,
		}}
	}
	if c.previousResponseID != "" {
		request["previous_response_id"] = c.previousResponseID
	}
	if c.config.MaxOutputTokens > 0 {
		request["max_output_tokens"] = c.config.MaxOutputTokens
	}
	var response struct {
		ID     string `json:"id"`
		Status string `json:"status"`
		Output []struct {
			Type      string `json:"type"`
			CallID    string `json:"call_id"`
			Name      string `json:"name"`
			Arguments string `json:"arguments"`
			Content   []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"output"`
		Usage struct {
			InputTokens  int64 `json:"input_tokens"`
			OutputTokens int64 `json:"output_tokens"`
			TotalTokens  int64 `json:"total_tokens"`
			InputDetails struct {
				CachedTokens int64 `json:"cached_tokens"`
			} `json:"input_tokens_details"`
			OutputDetails struct {
				ReasoningTokens int64 `json:"reasoning_tokens"`
			} `json:"output_tokens_details"`
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
	if response.ID == "" {
		return agentruntime.Decision{}, fmt.Errorf("provider returned no response id")
	}
	decision := agentruntime.Decision{
		StopReason:        response.Status,
		ProviderRequestID: requestID(headers),
		Usage: agentruntime.Usage{
			InputTokens:     response.Usage.InputTokens,
			OutputTokens:    response.Usage.OutputTokens,
			CachedTokens:    response.Usage.InputDetails.CachedTokens,
			ReasoningTokens: response.Usage.OutputDetails.ReasoningTokens,
			TotalTokens:     response.Usage.TotalTokens,
		},
	}
	if decision.ProviderRequestID == "" {
		decision.ProviderRequestID = response.ID
	}
	toolCalls := 0
	var text []string
	for _, item := range response.Output {
		switch item.Type {
		case "function_call":
			toolCalls++
			if !toolsEnabled {
				return agentruntime.Decision{}, fmt.Errorf("provider returned a tool call while tools were disabled")
			}
			if item.Name != c.tool.Name {
				return agentruntime.Decision{}, fmt.Errorf("provider requested unknown tool %q", item.Name)
			}
			var arguments agentruntime.ShellCall
			if err := json.Unmarshal([]byte(item.Arguments), &arguments); err != nil {
				return agentruntime.Decision{}, fmt.Errorf("decode shell arguments: %w", err)
			}
			decision.ShellCall = &arguments
			pendingCallID = item.CallID
		case "message":
			for _, content := range item.Content {
				if content.Type == "output_text" && content.Text != "" {
					text = append(text, content.Text)
				}
			}
		}
	}
	if toolCalls > 1 {
		return agentruntime.Decision{}, fmt.Errorf("provider returned %d tool calls; Ayati permits one per decision", toolCalls)
	}
	decision.Text = strings.Join(text, "\n")
	c.previousResponseID = response.ID
	c.pendingCallID = pendingCallID
	return decision, nil
}
