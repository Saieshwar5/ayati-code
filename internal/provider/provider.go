package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	agentruntime "github.com/Saieshwar5/ayati-code/internal/runtime"
)

const maxResponseBytes = 8 << 20

type Config struct {
	APIKey          string
	Model           string
	Endpoint        string
	MaxOutputTokens int
	Client          *http.Client
}

// New constructs the native adapter for one configured provider kind.
func New(kind string, config Config) (agentruntime.Model, error) {
	switch kind {
	case "openai-chat":
		return &OpenAIChat{Config: config}, nil
	case "openai-responses":
		return &OpenAIResponses{Config: config}, nil
	case "anthropic":
		return &Anthropic{Config: config}, nil
	default:
		return nil, fmt.Errorf("unsupported provider kind %q", kind)
	}
}

func (c Config) validate() error {
	if strings.TrimSpace(c.APIKey) == "" {
		return fmt.Errorf("provider API key is required")
	}
	if strings.TrimSpace(c.Model) == "" {
		return fmt.Errorf("provider model is required")
	}
	if strings.TrimSpace(c.Endpoint) == "" {
		return fmt.Errorf("provider endpoint is required")
	}
	return nil
}

func (c Config) client() *http.Client {
	if c.Client != nil {
		return c.Client
	}
	return &http.Client{Timeout: 5 * time.Minute}
}

type HTTPError struct {
	StatusCode int
	Status     string
	Body       string
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("provider returned %s: %s", e.Status, e.Body)
}

func postJSON(ctx context.Context, config Config, headers map[string]string, payload, output any) (http.Header, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("encode provider request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, config.Endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create provider request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	for name, value := range headers {
		req.Header.Set(name, value)
	}
	resp, err := config.client().Do(req)
	if err != nil {
		return nil, fmt.Errorf("call provider: %w", err)
	}
	defer resp.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return resp.Header, fmt.Errorf("read provider response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return resp.Header, &HTTPError{StatusCode: resp.StatusCode, Status: resp.Status, Body: compactError(responseBody)}
	}
	if err := json.Unmarshal(responseBody, output); err != nil {
		return resp.Header, fmt.Errorf("decode provider response: %w", err)
	}
	return resp.Header, nil
}

func compactError(body []byte) string {
	const limit = 1000
	text := strings.TrimSpace(string(body))
	if len(text) > limit {
		return text[:limit] + "..."
	}
	return text
}

func shellParameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"command": map[string]any{"type": "string", "description": "The shell command to execute."},
		},
		"required":             []string{"command"},
		"additionalProperties": false,
	}
}
