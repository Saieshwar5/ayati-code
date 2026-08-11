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

type Values struct {
	FireworksAPIKey string `json:"fireworks_api_key"`
	Model           string `json:"model"`
}

func DefaultPath() (string, error) {
	root, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve config directory: %w", err)
	}
	return filepath.Join(root, "ayati", "config.json"), nil
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
	var values Values
	if err := decoder.Decode(&values); err != nil {
		return Values{}, fmt.Errorf("decode configuration: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return Values{}, fmt.Errorf("decode configuration: multiple JSON values")
		}
		return Values{}, fmt.Errorf("decode configuration: %w", err)
	}
	if err := values.normalizeAndValidate(); err != nil {
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

func (v *Values) normalizeAndValidate() error {
	v.FireworksAPIKey = strings.TrimSpace(v.FireworksAPIKey)
	v.Model = strings.TrimSpace(v.Model)
	if v.FireworksAPIKey == "" {
		return fmt.Errorf("Fireworks API key is required")
	}
	if v.Model == "" {
		return fmt.Errorf("Fireworks model is required")
	}
	return nil
}
