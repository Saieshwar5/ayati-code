package accounts

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
	"strings"
	"time"

	appdatabase "github.com/Saieshwar5/perpetual/internal/database"
)

const (
	githubCredentialKeyBytes = 32
	githubCredentialKeyName  = "github.key"
)

func githubCredentialSchema(dialect appdatabase.Provider) string {
	if dialect == appdatabase.ProviderPostgres {
		return `CREATE TABLE IF NOT EXISTS github_credentials (
			user_id TEXT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
			ciphertext BYTEA NOT NULL,
			nonce BYTEA NOT NULL,
			updated_at TEXT NOT NULL
		)`
	}
	return `CREATE TABLE IF NOT EXISTS github_credentials (
		user_id TEXT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
		ciphertext BLOB NOT NULL,
		nonce BLOB NOT NULL,
		updated_at TEXT NOT NULL
	)`
}

type credentialSealer struct{ aead cipher.AEAD }

func newCredentialSealer(databasePath string) (*credentialSealer, error) {
	key := make([]byte, githubCredentialKeyBytes)
	if databasePath == ":memory:" || strings.TrimSpace(databasePath) == "" {
		if _, err := rand.Read(key); err != nil {
			return nil, fmt.Errorf("generate GitHub credential key: %w", err)
		}
	} else {
		path := filepath.Join(filepath.Dir(databasePath), githubCredentialKeyName)
		value, err := loadOrCreateGitHubCredentialKey(path)
		if err != nil {
			return nil, err
		}
		copy(key, value)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("create GitHub credential cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create GitHub credential sealer: %w", err)
	}
	return &credentialSealer{aead: aead}, nil
}

func loadOrCreateGitHubCredentialKey(path string) ([]byte, error) {
	value, err := os.ReadFile(path)
	if err == nil {
		if len(value) != githubCredentialKeyBytes {
			return nil, errors.New("GitHub credential key has an invalid length")
		}
		if err := os.Chmod(path, 0o600); err != nil {
			return nil, fmt.Errorf("secure GitHub credential key: %w", err)
		}
		return value, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("read GitHub credential key: %w", err)
	}
	value = make([]byte, githubCredentialKeyBytes)
	if _, err := rand.Read(value); err != nil {
		return nil, fmt.Errorf("generate GitHub credential key: %w", err)
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".github-key-*.tmp")
	if err != nil {
		return nil, fmt.Errorf("create GitHub credential key: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return nil, err
	}
	if _, err := temporary.Write(value); err != nil {
		temporary.Close()
		return nil, fmt.Errorf("write GitHub credential key: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return nil, err
	}
	if err := temporary.Close(); err != nil {
		return nil, err
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		if existing, readErr := os.ReadFile(path); readErr == nil && len(existing) == githubCredentialKeyBytes {
			return existing, nil
		}
		return nil, fmt.Errorf("install GitHub credential key: %w", err)
	}
	return value, nil
}

func (s *Store) SaveGitHubCredential(ctx context.Context, userID, accessToken string) error {
	userID, accessToken = strings.TrimSpace(userID), strings.TrimSpace(accessToken)
	if userID == "" || accessToken == "" {
		return errors.New("GitHub user ID and access token are required")
	}
	ciphertext, nonce, err := s.sealer.seal(userID, accessToken)
	if err != nil {
		return err
	}
	result, err := s.db.ExecContext(ctx, `INSERT INTO github_credentials
		(user_id, ciphertext, nonce, updated_at)
		VALUES (`+s.ph(1)+`, `+s.ph(2)+`, `+s.ph(3)+`, `+s.ph(4)+`)
		ON CONFLICT(user_id) DO UPDATE SET ciphertext = excluded.ciphertext,
		nonce = excluded.nonce, updated_at = excluded.updated_at`,
		userID, ciphertext, nonce, formatTime(time.Now().UTC()))
	if err != nil {
		return fmt.Errorf("save GitHub credential: %w", err)
	}
	return requireOneCredentialRow(result)
}

func (s *Store) GitHubCredential(ctx context.Context, userID string) (string, error) {
	row := s.db.QueryRowContext(ctx, `SELECT ciphertext, nonce
		FROM github_credentials WHERE user_id = `+s.ph(1), strings.TrimSpace(userID))
	var ciphertext, nonce []byte
	if err := row.Scan(&ciphertext, &nonce); err != nil {
		return "", err
	}
	return s.sealer.open(userID, ciphertext, nonce)
}

// TokenForUser implements the workspace package's GitHub token provider.
func (s *Store) TokenForUser(ctx context.Context, userID string) (string, error) {
	return s.GitHubCredential(ctx, userID)
}

func (s *Store) RemoveGitHubCredential(ctx context.Context, userID string) error {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil
	}
	if _, err := s.db.ExecContext(ctx, `DELETE FROM github_credentials WHERE user_id = `+s.ph(1), userID); err != nil {
		return fmt.Errorf("remove GitHub credential: %w", err)
	}
	return nil
}

func (s *credentialSealer) seal(userID, token string) ([]byte, []byte, error) {
	nonce := make([]byte, s.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, nil, fmt.Errorf("generate GitHub credential nonce: %w", err)
	}
	context := []byte("github:" + userID)
	return s.aead.Seal(nil, nonce, []byte(token), context), nonce, nil
}

func (s *credentialSealer) open(userID string, ciphertext, nonce []byte) (string, error) {
	token, err := s.aead.Open(nil, nonce, ciphertext, []byte("github:"+userID))
	if err != nil {
		return "", fmt.Errorf("decrypt GitHub credential: %w", err)
	}
	return string(token), nil
}

func requireOneCredentialRow(result sql.Result) error {
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected != 1 {
		return errors.New("GitHub credential was not saved")
	}
	return nil
}
