package protocol

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoadConfigAppliesProviderAndRuntimeDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "runtime.json")
	content := `{"provider":{"kind":"anthropic","model":"claude","context_window_tokens":200000},"limits":{"max_steps":12,"max_context_rollovers":3,"shell_timeout_seconds":9}}`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	config, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if config.Provider.Endpoint != "https://api.anthropic.com/v1/messages" || config.Provider.APIKeyEnv != "AYATI_API_KEY" || config.Shell.Path != "/bin/bash" {
		t.Fatalf("unexpected defaults: %+v", config)
	}
	limits := config.RuntimeLimits()
	if limits.MaxSteps != 12 || limits.MaxContextRollovers != 3 || limits.ShellTimeout != 9*time.Second {
		t.Fatalf("unexpected limits: %+v", limits)
	}
}

func TestConfigRejectsOutputLimitAtOrAboveContextWindow(t *testing.T) {
	config := Config{
		Version:  1,
		Provider: ProviderConfig{Kind: "openai-chat", Model: "model", Endpoint: "https://example.test", APIKeyEnv: "MODEL_KEY", MaxOutputTokens: 100, ContextWindowTokens: 100},
	}
	if err := config.Validate(); err == nil || !strings.Contains(err.Error(), "must be smaller") {
		t.Fatalf("expected context-window error, got %v", err)
	}
}

func TestLoadConfigRejectsUnknownFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "runtime.json")
	if err := os.WriteFile(path, []byte(`{"provider":{"kind":"anthropic","model":"claude"},"unknown":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadConfig(path); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("expected unknown-field error, got %v", err)
	}
}

func TestConfigRejectsProviderSecretInShellEnvironment(t *testing.T) {
	config := Config{
		Version:  1,
		Provider: ProviderConfig{Kind: "openai-chat", Model: "model", Endpoint: "https://example.test", APIKeyEnv: "MODEL_KEY", MaxOutputTokens: 1},
		Shell:    ShellConfig{PassEnv: []string{"PATH", "MODEL_KEY"}},
	}
	if err := config.Validate(); err == nil || !strings.Contains(err.Error(), "cannot expose") {
		t.Fatalf("expected secret exposure error, got %v", err)
	}
}

func TestLoadConfigRejectsNegativeProviderOutputLimit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "runtime.json")
	content := `{"version":1,"provider":{"kind":"anthropic","model":"claude","max_output_tokens":-1}}`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadConfig(path); err == nil || !strings.Contains(err.Error(), "cannot be negative") {
		t.Fatalf("expected negative-limit error, got %v", err)
	}
}

func TestLoadConfigRejectsUnsupportedVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "runtime.json")
	content := `{"version":2,"provider":{"kind":"anthropic","model":"claude"}}`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadConfig(path); err == nil || !strings.Contains(err.Error(), "unsupported config version") {
		t.Fatalf("expected version error, got %v", err)
	}
}
