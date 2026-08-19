package config

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const maxConfigBytes = 64 << 10

const CurrentVersion = 1

type ProviderValues struct {
	APIKey       string `json:"api_key,omitempty"`
	DefaultModel string `json:"default_model"`
}

type Values struct {
	Version   int                       `json:"version"`
	Providers map[string]ProviderValues `json:"providers"`
}

type storedValues struct {
	Version         int                       `json:"version,omitempty"`
	Providers       map[string]ProviderValues `json:"providers,omitempty"`
	FireworksAPIKey string                    `json:"fireworks_api_key,omitempty"`
	Model           string                    `json:"model,omitempty"`
}

func DefaultPath() (string, error) {
	root, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve config directory: %w", err)
	}
	return filepath.Join(root, "perpetual", "config.json"), nil
}

func Load(path string) (Values, error) {
	file, err := os.Open(path)
	if err != nil {
		return Values{}, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return Values{}, fmt.Errorf("inspect configuration: %w", err)
	}
	if info.Size() > maxConfigBytes {
		return Values{}, fmt.Errorf("configuration exceeds %d bytes", maxConfigBytes)
	}
	decoder := json.NewDecoder(io.LimitReader(file, maxConfigBytes+1))
	decoder.DisallowUnknownFields()
	var stored storedValues
	if err := decoder.Decode(&stored); err != nil {
		return Values{}, fmt.Errorf("decode configuration: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return Values{}, fmt.Errorf("decode configuration: multiple JSON values")
		}
		return Values{}, fmt.Errorf("decode configuration: %w", err)
	}
	values, err := migrate(stored)
	if err != nil {
		return Values{}, err
	}
	return values, nil
}

func Save(path string, values Values) error {
	if err := values.normalizeAndValidate(); err != nil {
		return err
	}
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}
	if err := os.Chmod(directory, 0o700); err != nil {
		return fmt.Errorf("secure config directory: %w", err)
	}
	temporary, err := os.CreateTemp(directory, ".config-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary configuration: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return fmt.Errorf("secure temporary configuration: %w", err)
	}
	if err := json.NewEncoder(temporary).Encode(values); err != nil {
		temporary.Close()
		return fmt.Errorf("write configuration: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return fmt.Errorf("sync configuration: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close configuration: %w", err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("replace configuration: %w", err)
	}
	return nil
}

func (v Values) Provider(id string) (ProviderValues, bool) {
	value, exists := v.Providers[strings.TrimSpace(id)]
	return value, exists
}

func (v *Values) SetProvider(id string, value ProviderValues) {
	if v.Providers == nil {
		v.Providers = make(map[string]ProviderValues)
	}
	v.Providers[strings.TrimSpace(id)] = value
}

func (v *Values) DeleteProvider(id string) {
	delete(v.Providers, strings.TrimSpace(id))
}

func migrate(stored storedValues) (Values, error) {
	if stored.Version == 0 {
		if len(stored.Providers) != 0 {
			return Values{}, fmt.Errorf("configuration version is required")
		}
		values := Values{Version: CurrentVersion}
		values.SetProvider("fireworks", ProviderValues{
			APIKey: stored.FireworksAPIKey, DefaultModel: stored.Model,
		})
		if err := values.normalizeAndValidate(); err != nil {
			return Values{}, err
		}
		return values, nil
	}
	if stored.FireworksAPIKey != "" || stored.Model != "" {
		return Values{}, fmt.Errorf("legacy and versioned provider configuration cannot be combined")
	}
	values := Values{Version: stored.Version, Providers: stored.Providers}
	if err := values.normalizeAndValidate(); err != nil {
		return Values{}, err
	}
	return values, nil
}

func (v *Values) normalizeAndValidate() error {
	if v.Version == 0 {
		v.Version = CurrentVersion
	}
	if v.Version != CurrentVersion {
		return fmt.Errorf("unsupported configuration version %d", v.Version)
	}
	if len(v.Providers) > 64 {
		return fmt.Errorf("provider configuration exceeds 64 entries")
	}
	normalized := make(map[string]ProviderValues, len(v.Providers))
	for rawID, value := range v.Providers {
		id := strings.TrimSpace(rawID)
		value.APIKey = strings.TrimSpace(value.APIKey)
		value.DefaultModel = strings.TrimSpace(value.DefaultModel)
		if !validProviderID(id) {
			return fmt.Errorf("provider id %q is invalid", id)
		}
		if _, exists := normalized[id]; exists {
			return fmt.Errorf("provider %q is configured more than once", id)
		}
		if value.DefaultModel == "" {
			return fmt.Errorf("default model for provider %q is required", id)
		}
		if id == "fireworks" && value.APIKey == "" {
			return fmt.Errorf("Fireworks API key is required")
		}
		normalized[id] = value
	}
	v.Providers = normalized
	return nil
}

func validProviderID(value string) bool {
	if value == "" || len(value) > 64 {
		return false
	}
	for index, character := range value {
		if character >= 'a' && character <= 'z' || character >= '0' && character <= '9' ||
			index > 0 && (character == '-' || character == '_' || character == '.') {
			continue
		}
		return false
	}
	return true
}
