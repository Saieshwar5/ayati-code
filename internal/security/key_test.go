package security

import (
	"context"
	"encoding/hex"
	"strings"
	"testing"
)

func TestEnvKeyProviderReadsConfiguredKey(t *testing.T) {
	t.Setenv("PERPETUAL_ENCRYPTION_KEY", strings.Repeat("ab", 32))
	key, err := (EnvKeyProvider{}).Key(context.Background())
	if err != nil {
		t.Fatalf("Key: %v", err)
	}
	if hex.EncodeToString(key) != strings.Repeat("ab", 32) {
		t.Fatalf("key = %x", key)
	}
}

func TestEnvKeyProviderRejectsBadKey(t *testing.T) {
	t.Setenv("PERPETUAL_ENCRYPTION_KEY", "short")
	if _, err := (EnvKeyProvider{}).Key(context.Background()); err == nil {
		t.Fatal("expected error for short key")
	}
}

func TestKMSKeyProviderFailsClosed(t *testing.T) {
	if _, err := (KMSKeyProvider{}).Key(context.Background()); err == nil {
		t.Fatal("KMS stub should fail closed")
	}
}
