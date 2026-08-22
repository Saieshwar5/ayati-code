package model

import (
	"fmt"

	"github.com/Saieshwar5/perpetual/internal/execution"
)

// NewFromConfig returns the ModelProvider selected by config. Provider "stub"
// returns the execution stub so local/dev runs need no credentials.
func NewFromConfig(config Config) (execution.ModelProvider, error) {
	switch config.Provider {
	case ProviderStub, "":
		return execution.StubProvider{}, nil
	case ProviderOpenAI, ProviderCompatible:
		return NewOpenAICompatible(config)
	default:
		return nil, fmt.Errorf("unsupported model provider %q", config.Provider)
	}
}
