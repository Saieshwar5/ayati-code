package workspace

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const (
	environmentKeyBytes = 32
	maxEnvironmentCount = 100
	maxEnvironmentName  = 128
	maxEnvironmentValue = 64 << 10
)

var environmentName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

var reservedEnvironment = map[string]bool{
	"HOME": true, "PATH": true, "PWD": true, "OLDPWD": true, "SHELL": true,
	"USER": true, "LOGNAME": true, "SHLVL": true, "GIT_ASKPASS": true,
	"TMPDIR": true, "XDG_CACHE_HOME": true, "GOCACHE": true, "GOMODCACHE": true,
	"COREPACK_HOME": true, "npm_config_cache": true, "PIP_CACHE_DIR": true, "CARGO_HOME": true,
	"CARGO_TARGET_DIR": true, "PYTHONDONTWRITEBYTECODE": true,
}

type EnvironmentInput struct {
	Name              string `json:"name"`
	Value             string `json:"value"`
	ExposeDuringSetup bool   `json:"expose_during_setup"`
}

type EnvironmentVariable struct {
	Name              string    `json:"name"`
	Configured        bool      `json:"configured"`
	ExposeDuringSetup bool      `json:"expose_during_setup"`
	UpdatedAt         time.Time `json:"updated_at"`
}

type environmentSealer struct{ aead cipher.AEAD }

func newEnvironmentSealer(databasePath string, requireExistingKey bool) (*environmentSealer, error) {
	key := make([]byte, environmentKeyBytes)
	if databasePath == ":memory:" {
		if _, err := rand.Read(key); err != nil {
			return nil, fmt.Errorf("generate environment key: %w", err)
		}
	} else {
		value, err := loadOrCreateEnvironmentKey(
			filepath.Join(filepath.Dir(databasePath), "environment.key"), requireExistingKey,
		)
		if err != nil {
			return nil, err
		}
		copy(key, value)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("create environment cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create environment sealer: %w", err)
	}
	return &environmentSealer{aead: aead}, nil
}

func loadOrCreateEnvironmentKey(path string, requireExisting bool) ([]byte, error) {
	value, err := os.ReadFile(path)
	if err == nil {
		if len(value) != environmentKeyBytes {
			return nil, errors.New("workspace environment key has an invalid length")
		}
		if err := os.Chmod(path, 0o600); err != nil {
			return nil, fmt.Errorf("secure workspace environment key: %w", err)
		}
		return value, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("read workspace environment key: %w", err)
	}
	if requireExisting {
		return nil, errors.New("workspace environment key is missing; restore environment.key from backup")
	}
	value = make([]byte, environmentKeyBytes)
	if _, err := rand.Read(value); err != nil {
		return nil, fmt.Errorf("generate workspace environment key: %w", err)
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".environment-key-*.tmp")
	if err != nil {
		return nil, fmt.Errorf("create workspace environment key: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return nil, err
	}
	if _, err := temporary.Write(value); err != nil {
		temporary.Close()
		return nil, fmt.Errorf("write workspace environment key: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return nil, err
	}
	if err := temporary.Close(); err != nil {
		return nil, err
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		if existing, readErr := os.ReadFile(path); readErr == nil && len(existing) == environmentKeyBytes {
			return existing, nil
		}
		return nil, fmt.Errorf("install workspace environment key: %w", err)
	}
	return value, nil
}

func validateEnvironmentInput(input *EnvironmentInput) error {
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" || len(input.Name) > maxEnvironmentName || !environmentName.MatchString(input.Name) {
		return errors.New("environment variable name must use letters, digits, and underscores")
	}
	if reservedEnvironment[input.Name] || strings.HasPrefix(input.Name, "PERPETUAL_") ||
		strings.HasPrefix(input.Name, "DOCKER_") {
		return fmt.Errorf("environment variable %s is reserved", input.Name)
	}
	if len(input.Value) > maxEnvironmentValue {
		return fmt.Errorf("environment variable %s exceeds 64 KiB", input.Name)
	}
	if strings.ContainsRune(input.Value, '\x00') {
		return fmt.Errorf("environment variable %s contains a NUL byte", input.Name)
	}
	return nil
}

func ValidateEnvironment(inputs []EnvironmentInput) error {
	if len(inputs) > maxEnvironmentCount {
		return fmt.Errorf("a workspace may contain at most %d environment variables", maxEnvironmentCount)
	}
	seen := make(map[string]bool, len(inputs))
	for index := range inputs {
		if err := validateEnvironmentInput(&inputs[index]); err != nil {
			return err
		}
		if seen[inputs[index].Name] {
			return fmt.Errorf("environment variable %s is duplicated", inputs[index].Name)
		}
		seen[inputs[index].Name] = true
	}
	return nil
}

func (s *environmentSealer) seal(workspaceID, name, value string) ([]byte, []byte, error) {
	nonce := make([]byte, s.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, nil, fmt.Errorf("generate environment nonce: %w", err)
	}
	context := []byte(workspaceID + "\x00" + name)
	return s.aead.Seal(nil, nonce, []byte(value), context), nonce, nil
}

func (s *environmentSealer) open(workspaceID, name string, ciphertext, nonce []byte) (string, error) {
	value, err := s.aead.Open(nil, nonce, ciphertext, []byte(workspaceID+"\x00"+name))
	if err != nil {
		return "", fmt.Errorf("decrypt environment variable %s: %w", name, err)
	}
	return string(value), nil
}

func (s *Store) insertEnvironment(
	ctx context.Context, tx *sql.Tx, workspaceID string, inputs []EnvironmentInput, now time.Time,
) error {
	if err := ValidateEnvironment(inputs); err != nil {
		return err
	}
	for index := range inputs {
		input := &inputs[index]
		ciphertext, nonce, err := s.sealer.seal(workspaceID, input.Name, input.Value)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO workspace_environment
			(workspace_id, name, ciphertext, nonce, expose_during_setup, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)`, workspaceID, input.Name, ciphertext, nonce,
			input.ExposeDuringSetup, formatTime(now), formatTime(now))
		if err != nil {
			return fmt.Errorf("save environment variable %s: %w", input.Name, err)
		}
	}
	return nil
}

func (s *Store) UpsertEnvironment(ctx context.Context, workspaceID string, input EnvironmentInput) (EnvironmentVariable, error) {
	if err := validateEnvironmentInput(&input); err != nil {
		return EnvironmentVariable{}, err
	}
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM workspace_environment WHERE workspace_id = ?`, workspaceID).Scan(&count); err != nil {
		return EnvironmentVariable{}, fmt.Errorf("count environment variables: %w", err)
	}
	var exists int
	_ = s.db.QueryRowContext(ctx, `SELECT 1 FROM workspace_environment WHERE workspace_id = ? AND name = ?`, workspaceID, input.Name).Scan(&exists)
	if exists == 0 && count >= maxEnvironmentCount {
		return EnvironmentVariable{}, fmt.Errorf("a workspace may contain at most %d environment variables", maxEnvironmentCount)
	}
	ciphertext, nonce, err := s.sealer.seal(workspaceID, input.Name, input.Value)
	if err != nil {
		return EnvironmentVariable{}, err
	}
	now := time.Now().UTC()
	result, err := s.db.ExecContext(ctx, `INSERT INTO workspace_environment
		(workspace_id, name, ciphertext, nonce, expose_during_setup, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(workspace_id, name) DO UPDATE SET ciphertext = excluded.ciphertext,
		nonce = excluded.nonce, expose_during_setup = excluded.expose_during_setup,
		updated_at = excluded.updated_at`, workspaceID, input.Name, ciphertext, nonce,
		input.ExposeDuringSetup, formatTime(now), formatTime(now))
	if err != nil {
		return EnvironmentVariable{}, fmt.Errorf("save environment variable: %w", err)
	}
	if affected, _ := result.RowsAffected(); affected != 1 {
		return EnvironmentVariable{}, sql.ErrNoRows
	}
	return EnvironmentVariable{Name: input.Name, Configured: true,
		ExposeDuringSetup: input.ExposeDuringSetup, UpdatedAt: now}, nil
}

func (s *Store) ListEnvironment(ctx context.Context, workspaceID string) ([]EnvironmentVariable, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT name, expose_during_setup, updated_at
		FROM workspace_environment WHERE workspace_id = ? ORDER BY name`, strings.TrimSpace(workspaceID))
	if err != nil {
		return nil, fmt.Errorf("list environment variables: %w", err)
	}
	defer rows.Close()
	var values []EnvironmentVariable
	for rows.Next() {
		var value EnvironmentVariable
		var updatedAt string
		if err := rows.Scan(&value.Name, &value.ExposeDuringSetup, &updatedAt); err != nil {
			return nil, err
		}
		value.Configured = true
		value.UpdatedAt, err = parseStoredTime(updatedAt)
		if err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, rows.Err()
}

func (s *Store) EnvironmentValues(ctx context.Context, workspaceID string, setupOnly bool) (map[string]string, error) {
	query := `SELECT name, ciphertext, nonce FROM workspace_environment WHERE workspace_id = ?`
	if setupOnly {
		query += ` AND expose_during_setup = 1`
	}
	rows, err := s.db.QueryContext(ctx, query, strings.TrimSpace(workspaceID))
	if err != nil {
		return nil, fmt.Errorf("load environment variables: %w", err)
	}
	defer rows.Close()
	values := make(map[string]string)
	for rows.Next() {
		var name string
		var ciphertext, nonce []byte
		if err := rows.Scan(&name, &ciphertext, &nonce); err != nil {
			return nil, err
		}
		value, err := s.sealer.open(workspaceID, name, ciphertext, nonce)
		if err != nil {
			return nil, err
		}
		values[name] = value
	}
	return values, rows.Err()
}

func (s *Store) DeleteEnvironment(ctx context.Context, workspaceID, name string) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM workspace_environment WHERE workspace_id = ? AND name = ?`,
		strings.TrimSpace(workspaceID), strings.TrimSpace(name))
	if err != nil {
		return fmt.Errorf("delete environment variable: %w", err)
	}
	if count, err := result.RowsAffected(); err != nil || count != 1 {
		return sql.ErrNoRows
	}
	return nil
}
