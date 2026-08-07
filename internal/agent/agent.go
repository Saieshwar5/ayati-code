package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/sai-eshwar/no-nonsense-coding-ai/internal/chat"
	"github.com/sai-eshwar/no-nonsense-coding-ai/internal/provider"
	"github.com/sai-eshwar/no-nonsense-coding-ai/internal/session"
	"github.com/sai-eshwar/no-nonsense-coding-ai/internal/shell"
)

type Shell interface {
	Run(context.Context, string) (shell.Result, error)
}

type Agent struct {
	Provider              provider.Provider
	Shell                 Shell
	Store                 session.Store
	Session               *session.Session
	Output                io.Writer
	MaxToolCalls          int
	MaxContextToolPairs   int
	ContextPercent        int
	FallbackContextTokens int
	modelContextTokens    int
}

func (a *Agent) Prompt(ctx context.Context, input string) error {
	input = strings.TrimSpace(input)
	if input == "" {
		return nil
	}
	if err := a.append(chat.Message{Role: "user", Content: input}); err != nil {
		return err
	}

	maxCalls := a.MaxToolCalls
	if maxCalls <= 0 {
		maxCalls = 30
	}
	emptyRetries := 0
	overflowRetries := 0
	for callCount := 0; ; {
		messages, err := a.prepareContext(ctx)
		if err != nil {
			return err
		}
		response, err := a.Provider.Complete(ctx, messages)
		if err != nil {
			if overflowRetries == 0 && isContextOverflow(err) {
				overflowRetries++
				if _, compactErr := a.compact(ctx, true); compactErr == nil {
					continue
				}
			}
			return err
		}
		if strings.TrimSpace(response.Content) == "" && len(response.ToolCalls) == 0 {
			if emptyRetries == 0 {
				emptyRetries++
				continue
			}
			return fmt.Errorf("provider returned an empty response twice")
		}
		emptyRetries = 0
		overflowRetries = 0
		response.Role = "assistant"
		if err := a.append(response); err != nil {
			return err
		}
		if response.Content != "" {
			fmt.Fprintln(a.Output, response.Content)
		}
		if len(response.ToolCalls) == 0 {
			return nil
		}

		for _, toolCall := range response.ToolCalls {
			callCount++
			if callCount > maxCalls {
				return fmt.Errorf("tool-call limit reached (%d)", maxCalls)
			}
			result := a.runTool(ctx, toolCall)
			if err := a.append(chat.Message{Role: "tool", ToolCallID: toolCall.ID, Content: result}); err != nil {
				return err
			}
		}
	}
}

func (a *Agent) Compact(ctx context.Context) (bool, error) {
	return a.compact(ctx, true)
}

func (a *Agent) ResetModelContext(fallback int) {
	a.modelContextTokens = 0
	a.FallbackContextTokens = fallback
}

func (a *Agent) runTool(ctx context.Context, call chat.ToolCall) string {
	if call.Function.Name != "shell" {
		return fmt.Sprintf(`{"error":"unknown tool %s"}`, call.Function.Name)
	}
	var arguments struct {
		Command string `json:"command"`
	}
	if err := json.Unmarshal([]byte(call.Function.Arguments), &arguments); err != nil {
		return fmt.Sprintf(`{"error":%q}`, "invalid shell arguments: "+err.Error())
	}
	fmt.Fprintf(a.Output, "\n$ %s\n", arguments.Command)
	result, err := a.Shell.Run(ctx, arguments.Command)
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	if result.Stdout != "" {
		fmt.Fprint(a.Output, result.Stdout)
	}
	if result.Stderr != "" {
		fmt.Fprint(a.Output, result.Stderr)
	}
	if result.Stdout != "" && !strings.HasSuffix(result.Stdout, "\n") {
		fmt.Fprintln(a.Output)
	}
	return result.JSON()
}

func (a *Agent) append(message chat.Message) error {
	return a.Store.Append(a.Session, message)
}

func (a *Agent) prepareContext(ctx context.Context) ([]chat.Message, error) {
	limit := a.contextLimit(ctx)
	budget := limit * a.contextPercent() / 100
	messages := a.contextMessages()
	tooManyTools := countToolResults(messages) > a.maxContextToolPairs()
	if !tooManyTools && estimateTokens(messages) <= budget {
		return messages, nil
	}

	compacted, err := a.compactForBudget(ctx, budget, false)
	if err != nil {
		// The exact session remains authoritative. If maintenance fails, retain a
		// safe recent context that always includes the current user request.
		return a.boundedContext(budget), nil
	}
	if compacted {
		messages = a.contextMessages()
	}
	if estimateTokens(messages) > budget || countToolResults(messages) > a.maxContextToolPairs() {
		messages = a.boundedContext(budget)
	}
	return messages, nil
}

func (a *Agent) contextMessages() []chat.Message {
	messages := []chat.Message{{Role: "system", Content: SystemPrompt}}
	covered := 0
	if summary := a.Session.Summary; summary != nil {
		covered = summary.CoveredMessages
		messages = append(messages, chat.Message{
			Role:    "system",
			Content: "SESSION SUMMARY\n" + summary.Content + "\n\nThe exact append-only session is stored at " + a.Session.Path + ". If an omitted historical detail is necessary, use shell to search a bounded part of that file; do not load the whole file.",
		})
	}
	currentUser := latestUserIndex(a.Session.Messages)
	if currentUser >= 0 && currentUser < covered {
		messages = append(messages, a.Session.Messages[currentUser])
	}
	for _, message := range a.Session.Messages[covered:] {
		if meaningfulMessage(message) {
			messages = append(messages, message)
		}
	}
	return messages
}

func (a *Agent) compact(ctx context.Context, force bool) (bool, error) {
	budget := a.contextLimit(ctx) * a.contextPercent() / 100
	return a.compactForBudget(ctx, budget, force)
}

func (a *Agent) compactForBudget(ctx context.Context, budget int, force bool) (bool, error) {
	summarizer, ok := a.Provider.(provider.Summarizer)
	if !ok {
		return false, fmt.Errorf("provider does not support context summarization")
	}
	covered := 0
	previousSummary := ""
	if a.Session.Summary != nil {
		covered = a.Session.Summary.CoveredMessages
		previousSummary = a.Session.Summary.Content
	}
	if covered >= len(a.Session.Messages) {
		return false, nil
	}

	cutoff := a.recentStart(covered, budget/2, a.maxContextToolPairs())
	if force {
		latestUser := latestUserIndex(a.Session.Messages)
		if latestUser > covered {
			cutoff = latestUser
		} else {
			cutoff = a.recentStart(covered, budget/3, 20)
		}
	}
	if cutoff <= covered {
		return false, nil
	}

	transcript := summaryTranscript(previousSummary, a.Session.Messages[covered:cutoff])
	summary, err := summarizer.Summarize(ctx, transcript)
	if err != nil {
		return false, fmt.Errorf("compact session context: %w", err)
	}
	checkpoint := session.Summary{Content: strings.TrimSpace(summary), CoveredMessages: cutoff}
	if err := a.Store.AppendSummary(a.Session, checkpoint); err != nil {
		return false, err
	}
	return true, nil
}

func (a *Agent) recentStart(minimum, targetTokens, maxToolPairs int) int {
	messages := a.Session.Messages
	start := len(messages)
	used := 0
	toolPairs := 0
	for index := len(messages) - 1; index >= minimum; index-- {
		if messages[index].Role == "tool" {
			toolPairs++
			if toolPairs > maxToolPairs {
				break
			}
		}
		size := estimateMessageTokens(messages[index])
		if used+size > targetTokens && start < len(messages) {
			break
		}
		used += size
		start = index
	}
	if start < len(messages) && messages[start].Role == "tool" {
		callID := messages[start].ToolCallID
		for index := start - 1; index >= minimum; index-- {
			if containsToolCall(messages[index], callID) {
				start = index
				break
			}
		}
	}
	return start
}

func (a *Agent) boundedContext(budget int) []chat.Message {
	base := []chat.Message{
		{Role: "system", Content: SystemPrompt},
		{Role: "system", Content: "Some older active context may be omitted. The exact append-only session is stored at " + a.Session.Path + ". If a missing detail is necessary, use shell to search a bounded part of that file."},
	}
	if a.Session.Summary != nil {
		base = append(base, chat.Message{Role: "system", Content: "SESSION SUMMARY\n" + a.Session.Summary.Content})
	}
	currentUser := latestUserIndex(a.Session.Messages)
	if currentUser < 0 {
		return base
	}
	base = append(base, a.Session.Messages[currentUser])
	used := estimateTokens(base)
	start := len(a.Session.Messages)
	toolPairs := 0
	for index := len(a.Session.Messages) - 1; index > currentUser; index-- {
		if a.Session.Messages[index].Role == "tool" {
			toolPairs++
			if toolPairs > a.maxContextToolPairs() {
				break
			}
		}
		size := estimateMessageTokens(a.Session.Messages[index])
		if used+size > budget && start < len(a.Session.Messages) {
			break
		}
		used += size
		start = index
	}
	if start < len(a.Session.Messages) && a.Session.Messages[start].Role == "tool" {
		callID := a.Session.Messages[start].ToolCallID
		for index := start - 1; index > currentUser; index-- {
			if containsToolCall(a.Session.Messages[index], callID) {
				start = index
				break
			}
		}
	}
	if start < len(a.Session.Messages) {
		for _, message := range a.Session.Messages[start:] {
			if meaningfulMessage(message) {
				base = append(base, message)
			}
		}
	}
	return base
}

func (a *Agent) contextLimit(ctx context.Context) int {
	if a.modelContextTokens > 0 {
		return a.modelContextTokens
	}
	fallback := a.FallbackContextTokens
	if fallback <= 0 {
		fallback = 128000
	}
	if source, ok := a.Provider.(provider.ContextLimitProvider); ok {
		if limit, err := source.ContextLimit(ctx); err == nil && limit > 0 {
			a.modelContextTokens = limit
			return limit
		}
	}
	a.modelContextTokens = fallback
	return fallback
}

func (a *Agent) contextPercent() int {
	if a.ContextPercent <= 0 || a.ContextPercent >= 100 {
		return 70
	}
	return a.ContextPercent
}

func (a *Agent) maxContextToolPairs() int {
	if a.MaxContextToolPairs <= 0 {
		return 100
	}
	return a.MaxContextToolPairs
}

func messageSize(message chat.Message) int {
	encoded, err := json.Marshal(message)
	if err != nil {
		return len(message.Content)
	}
	return len(encoded)
}

func estimateMessageTokens(message chat.Message) int {
	size := messageSize(message)
	return (size + 2) / 3
}

func estimateTokens(messages []chat.Message) int {
	total := 0
	for _, message := range messages {
		total += estimateMessageTokens(message)
	}
	return total
}

func latestUserIndex(messages []chat.Message) int {
	for index := len(messages) - 1; index >= 0; index-- {
		if messages[index].Role == "user" {
			return index
		}
	}
	return -1
}

func containsToolCall(message chat.Message, callID string) bool {
	if message.Role != "assistant" {
		return false
	}
	for _, call := range message.ToolCalls {
		if call.ID == callID {
			return true
		}
	}
	return false
}

func countToolResults(messages []chat.Message) int {
	count := 0
	for _, message := range messages {
		if message.Role == "tool" {
			count++
		}
	}
	return count
}

func meaningfulMessage(message chat.Message) bool {
	return message.Role != "assistant" || strings.TrimSpace(message.Content) != "" || len(message.ToolCalls) > 0
}

func summaryTranscript(previous string, messages []chat.Message) string {
	var builder strings.Builder
	if previous != "" {
		builder.WriteString("PREVIOUS SUMMARY\n")
		builder.WriteString(previous)
		builder.WriteString("\n\n")
	}
	builder.WriteString("EXACT SESSION RECORDS TO COMPACT\n")
	for _, message := range messages {
		encoded, err := json.Marshal(message)
		if err != nil {
			continue
		}
		builder.Write(encoded)
		builder.WriteByte('\n')
	}
	return builder.String()
}

func isContextOverflow(err error) bool {
	text := strings.ToLower(err.Error())
	return strings.Contains(text, "context") && (strings.Contains(text, "length") || strings.Contains(text, "token") || strings.Contains(text, "overflow"))
}
