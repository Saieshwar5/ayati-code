package provider

import (
	"context"

	"github.com/Saieshwar5/ayati-code/internal/agent"
	"github.com/Saieshwar5/ayati-code/internal/fireworks"
	"github.com/Saieshwar5/ayati-code/internal/openaichat"
)

const (
	OpenAIProviderID     = "openai"
	OpenRouterProviderID = "openrouter"
	GroqProviderID       = "groq"
	TogetherProviderID   = "together"
	DeepSeekProviderID   = "deepseek"

	openAIEndpoint     = "https://api.openai.com/v1"
	openRouterEndpoint = "https://openrouter.ai/api/v1"
	groqEndpoint       = "https://api.groq.com/openai/v1"
	togetherEndpoint   = "https://api.together.ai/v1"
	deepSeekEndpoint   = "https://api.deepseek.com"
)

func BuiltinSpecifications() []Specification {
	return []Specification{
		{
			Definition: Definition{
				ID: agent.FireworksProviderID, Name: "Fireworks", Protocol: "openai-chat",
			},
			Factory: func(key string) (agent.Provider, error) { return fireworks.New(key) },
		},
		compatibleSpecification(OpenAIProviderID, "OpenAI", openAIEndpoint, openaichat.MaxCompletionTokens, true),
		compatibleSpecification(OpenRouterProviderID, "OpenRouter", openRouterEndpoint, openaichat.MaxTokens, true),
		compatibleSpecification(GroqProviderID, "Groq", groqEndpoint, openaichat.MaxTokens, true),
		compatibleSpecification(TogetherProviderID, "Together AI", togetherEndpoint, openaichat.MaxTokens, false),
		compatibleSpecification(DeepSeekProviderID, "DeepSeek", deepSeekEndpoint, openaichat.MaxTokens, false),
	}
}

func compatibleSpecification(
	id, name, endpoint string, tokenLimitField openaichat.TokenLimitField,
	parallelToolControl bool,
) Specification {
	return Specification{
		Definition: Definition{ID: id, Name: name, Protocol: "openai-chat"},
		Factory: func(key string) (agent.Provider, error) {
			return openaichat.New(openaichat.Options{
				ProviderName: name, Endpoint: endpoint, APIKey: key, TokenLimitField: tokenLimitField,
				SupportsParallelToolControl: parallelToolControl,
			})
		},
		Verifier: func(ctx context.Context, key, _ string) error {
			return openaichat.Verify(ctx, name, endpoint, key)
		},
	}
}
