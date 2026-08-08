package protocol

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	agentruntime "github.com/Saieshwar5/ayati-code/internal/runtime"
)

const maxConfigBytes = 1 << 20

type Config struct {
	Version  int            `json:"version,omitempty"`
	Provider ProviderConfig `json:"provider"`
	Limits   LimitConfig    `json:"limits"`
	Shell    ShellConfig    `json:"shell,omitempty"`
}

type ProviderConfig struct {
	Kind                string `json:"kind"`
	Model               string `json:"model"`
	Endpoint            string `json:"endpoint,omitempty"`
	APIKeyEnv           string `json:"api_key_env,omitempty"`
	MaxOutputTokens     int    `json:"max_output_tokens,omitempty"`
	ContextWindowTokens int    `json:"context_window_tokens"`
}

type LimitConfig struct {
	MaxSteps            int `json:"max_steps,omitempty"`
	MaxContextRollovers int `json:"max_context_rollovers,omitempty"`
	RunTimeoutSeconds   int `json:"run_timeout_seconds,omitempty"`
	ModelTimeoutSeconds int `json:"model_timeout_seconds,omitempty"`
	ShellTimeoutSeconds int `json:"shell_timeout_seconds,omitempty"`
	MaxToolOutputBytes  int `json:"max_tool_output_bytes,omitempty"`
}

type ShellConfig struct {
	Path    string   `json:"path,omitempty"`
	PassEnv []string `json:"pass_env,omitempty"`
}

func LoadConfig(path string) (Config, error) {
	file, err := os.Open(path)
	if err != nil {
		return Config{}, fmt.Errorf("open runtime config: %w", err)
	}
	defer file.Close()
	var config Config
	if err := decodeLimited(file, maxConfigBytes, &config); err != nil {
		return Config{}, fmt.Errorf("decode runtime config: %w", err)
	}
	config.applyDefaults()
	if err := config.Validate(); err != nil {
		return Config{}, err
	}
	return config, nil
}

func (c *Config) applyDefaults() {
	if c.Version == 0 {
		c.Version = agentruntime.ProtocolVersion
	}
	if c.Provider.APIKeyEnv == "" {
		c.Provider.APIKeyEnv = "AYATI_API_KEY"
	}
	if c.Provider.MaxOutputTokens == 0 {
		c.Provider.MaxOutputTokens = 8192
	}
	if c.Shell.Path == "" {
		c.Shell.Path = "/bin/bash"
	}
	switch c.Provider.Kind {
	case "openai-responses":
		if c.Provider.Endpoint == "" {
			c.Provider.Endpoint = "https://api.openai.com/v1/responses"
		}
	case "anthropic":
		if c.Provider.Endpoint == "" {
			c.Provider.Endpoint = "https://api.anthropic.com/v1/messages"
		}
	}
}

func (c Config) Validate() error {
	if c.Version != agentruntime.ProtocolVersion {
		return fmt.Errorf("unsupported config version %d", c.Version)
	}
	switch c.Provider.Kind {
	case "openai-chat", "openai-responses", "anthropic":
	default:
		return fmt.Errorf("unsupported provider kind %q", c.Provider.Kind)
	}
	if strings.TrimSpace(c.Provider.Model) == "" {
		return fmt.Errorf("provider.model is required")
	}
	if strings.TrimSpace(c.Provider.Endpoint) == "" {
		return fmt.Errorf("provider.endpoint is required for %s", c.Provider.Kind)
	}
	if strings.TrimSpace(c.Provider.APIKeyEnv) == "" {
		return fmt.Errorf("provider.api_key_env is required")
	}
	if c.Limits.MaxSteps < 0 || c.Limits.MaxContextRollovers < 0 || c.Limits.RunTimeoutSeconds < 0 || c.Limits.ModelTimeoutSeconds < 0 || c.Limits.ShellTimeoutSeconds < 0 || c.Limits.MaxToolOutputBytes < 0 {
		return fmt.Errorf("runtime limits cannot be negative")
	}
	if c.Provider.MaxOutputTokens < 0 {
		return fmt.Errorf("provider.max_output_tokens cannot be negative")
	}
	contextPolicy := agentruntime.ContextPolicy{
		WindowTokens:    c.Provider.ContextWindowTokens,
		MaxOutputTokens: c.Provider.MaxOutputTokens,
	}
	if err := contextPolicy.Validate(); err != nil {
		return fmt.Errorf("provider context policy: %w", err)
	}
	for _, name := range c.Shell.PassEnv {
		if name == c.Provider.APIKeyEnv {
			return fmt.Errorf("shell.pass_env cannot expose provider API key variable %s", name)
		}
	}
	return nil
}

func (c Config) RuntimeLimits() agentruntime.Limits {
	return agentruntime.Limits{
		MaxSteps:            c.Limits.MaxSteps,
		MaxContextRollovers: c.Limits.MaxContextRollovers,
		RunTimeout:          time.Duration(c.Limits.RunTimeoutSeconds) * time.Second,
		ModelTimeout:        time.Duration(c.Limits.ModelTimeoutSeconds) * time.Second,
		ShellTimeout:        time.Duration(c.Limits.ShellTimeoutSeconds) * time.Second,
		MaxOutputBytes:      c.Limits.MaxToolOutputBytes,
	}
}

func decodeStrict(reader io.Reader, destination any) error {
	decoder := json.NewDecoder(reader)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return fmt.Errorf("unexpected trailing JSON value")
		}
		return err
	}
	return nil
}

func decodeLimited(reader io.Reader, maximum int64, destination any) error {
	content, err := io.ReadAll(io.LimitReader(reader, maximum+1))
	if err != nil {
		return err
	}
	if int64(len(content)) > maximum {
		return fmt.Errorf("JSON input exceeds %d bytes", maximum)
	}
	return decodeStrict(bytes.NewReader(content), destination)
}
