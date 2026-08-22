// Package security owns controller-side credential/key abstractions. Secrets
// are never written to logs, prompts, SSE, or shell environments.
package security

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strings"
)

// KeyProvider returns a 32-byte envelope encryption key.
type KeyProvider interface {
	Key(ctx context.Context) ([]byte, error)
}

// EnvKeyProvider reads the key from PERPETUAL_ENCRYPTION_KEY (32 bytes hex).
type EnvKeyProvider struct{}

// Key returns the configured key, or a random development key when unset.
func (EnvKeyProvider) Key(_ context.Context) ([]byte, error) {
	configured := strings.TrimSpace(os.Getenv("PERPETUAL_ENCRYPTION_KEY"))
	if configured == "" {
		key := make([]byte, 32)
		if _, err := rand.Read(key); err != nil {
			return nil, fmt.Errorf("generate dev encryption key: %w", err)
		}
		return key, nil
	}
	key, err := hex.DecodeString(configured)
	if err != nil || len(key) != 32 {
		return nil, errors.New("PERPETUAL_ENCRYPTION_KEY must be 32 bytes of hex")
	}
	return key, nil
}

// KMSKeyProvider is the envelope-key seam for AWS KMS. It is intentionally a
// stub until KMS wiring is implemented; callers fail closed rather than falling
// back to a local key.
type KMSKeyProvider struct{}

// Key returns a clear error so KMS is never silently replaced by local keys.
func (KMSKeyProvider) Key(_ context.Context) ([]byte, error) {
	return nil, errors.New("KMS key provider is not implemented; keep PERPETUAL_ENCRYPTION_KEY configured")
}
