// Package model provides controller-side model providers for execution rooms.
// API keys are never exposed to run stores, shells, logs, or SSE streams.
package model

import (
	"errors"
	"os"
	"strconv"
	"strings"
)

// Provider names.
const (
	ProviderStub       = "stub"
	ProviderOpenAI     = "openai"
	ProviderCompatible = "openai-compatible"
)

// Config selects a model provider for execution rooms.
type Config struct {
	Provider  string
	Model     string
	APIKey    string
	BaseURL   string
	MaxTokens int64
}

// DefaultBaseURL is the OpenAI chat completions endpoint root.
const DefaultBaseURL = "https://api.openai.com/v1"

// LoadFromEnv reads provider configuration from the environment.
func LoadFromEnv() Config {
	provider := os.Getenv("PERPETUAL_MODEL_PROVIDER")
	if provider == "" {
		provider = ProviderStub
	}
	return Config{
		Provider:  strings.ToLower(strings.TrimSpace(provider)),
		Model:     strings.TrimSpace(os.Getenv("PERPETUAL_MODEL_NAME")),
		APIKey:    strings.TrimSpace(os.Getenv("PERPETUAL_MODEL_API_KEY")),
		BaseURL:   strings.TrimSpace(os.Getenv("PERPETUAL_MODEL_BASE_URL")),
		MaxTokens: envInt64("PERPETUAL_MODEL_MAX_TOKENS", 4096),
	}
}

// Validate checks the configuration for a non-stub provider.
func (c Config) Validate() error {
	switch c.Provider {
	case ProviderStub:
		return nil
	case ProviderOpenAI:
		if c.Model == "" {
			return errors.New("PERPETUAL_MODEL_NAME is required")
		}
		if c.APIKey == "" {
			return errors.New("PERPETUAL_MODEL_API_KEY is required")
		}
		return nil
	case ProviderCompatible:
		if c.Model == "" {
			return errors.New("PERPETUAL_MODEL_NAME is required")
		}
		if c.BaseURL == "" {
			return errors.New("PERPETUAL_MODEL_BASE_URL is required")
		}
		if c.APIKey == "" && !strings.Contains(c.BaseURL, "localhost") && !strings.Contains(c.BaseURL, "127.0.0.1") {
			return errors.New("PERPETUAL_MODEL_API_KEY is required for remote endpoints")
		}
		return nil
	default:
		return errors.New("unsupported PERPETUAL_MODEL_PROVIDER " + c.Provider)
	}
}

func envInt64(name string, fallback int64) int64 {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return fallback
	}
	return parsed
}
