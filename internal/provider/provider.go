package provider

import (
	"context"

	"github.com/sai-eshwar/no-nonsense-coding-ai/internal/chat"
)

type Provider interface {
	Complete(context.Context, []chat.Message) (chat.Message, error)
}

type Summarizer interface {
	Summarize(context.Context, string) (string, error)
}

type ContextLimitProvider interface {
	ContextLimit(context.Context) (int, error)
}
