package runtime

import (
	"strings"
	"testing"
)

func TestContextPolicyAllowsDisabledRollover(t *testing.T) {
	policy := ContextPolicy{MaxOutputTokens: 8192}
	if err := policy.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if got := contextCheckpointTokens(policy); got != 0 {
		t.Fatalf("checkpoint tokens = %d, want 0", got)
	}
}

func TestContextPolicyCalculatesCheckpointFromOneSafetyRule(t *testing.T) {
	policy := ContextPolicy{WindowTokens: 10_000, MaxOutputTokens: 1_000}
	if err := policy.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	const want = 3432
	if got := contextCheckpointTokens(policy); got != want {
		t.Fatalf("checkpoint tokens = %d, want %d", got, want)
	}
}

func TestContextPolicyRejectsInsufficientReserve(t *testing.T) {
	policy := ContextPolicy{WindowTokens: 10_000, MaxOutputTokens: 6_000}
	if err := policy.Validate(); err == nil || !strings.Contains(err.Error(), "leave more than") {
		t.Fatalf("expected safety-reserve error, got %v", err)
	}
}
