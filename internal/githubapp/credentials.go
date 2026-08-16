package githubapp

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Credentials struct {
	AccessToken string `json:"access_token"`
	User        User   `json:"user"`
}

func DefaultCredentialsPath() (string, error) {
	root, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve config directory: %w", err)
	}
	return filepath.Join(root, "ayati", "github.json"), nil
}

func LoadCredentials(path string) (Credentials, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Credentials{}, err
	}
	var credentials Credentials
	if err := json.Unmarshal(data, &credentials); err != nil {
		return Credentials{}, fmt.Errorf("decode GitHub credentials: %w", err)
	}
	if strings.TrimSpace(credentials.AccessToken) == "" || credentials.User.ID == 0 {
		return Credentials{}, errors.New("GitHub credentials are incomplete")
	}
	return credentials, nil
}

func SaveCredentials(path string, credentials Credentials) error {
	if strings.TrimSpace(credentials.AccessToken) == "" || credentials.User.ID == 0 {
		return errors.New("GitHub token and user are required")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create GitHub config directory: %w", err)
	}
	if err := os.Chmod(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("secure GitHub config directory: %w", err)
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".github-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary GitHub credentials: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return err
	}
	if err := json.NewEncoder(temporary).Encode(credentials); err != nil {
		temporary.Close()
		return fmt.Errorf("write GitHub credentials: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("replace GitHub credentials: %w", err)
	}
	return nil
}

func RemoveCredentials(path string) error {
	err := os.Remove(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}
