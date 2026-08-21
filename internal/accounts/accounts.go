package accounts

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	appdatabase "github.com/Saieshwar5/perpetual/internal/database"
)

const userSchema = `CREATE TABLE IF NOT EXISTS users (
	id TEXT PRIMARY KEY,
	github_id INTEGER NOT NULL UNIQUE,
	login TEXT NOT NULL,
	name TEXT NOT NULL DEFAULT '',
	avatar_url TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL
)`

const authSessionSchema = `CREATE TABLE IF NOT EXISTS auth_sessions (
	id TEXT PRIMARY KEY,
	user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	token_hash TEXT NOT NULL UNIQUE,
	created_at TEXT NOT NULL,
	expires_at TEXT NOT NULL,
	revoked_at TEXT NOT NULL DEFAULT ''
)`

// User is the application identity tied to one GitHub account.
type User struct {
	ID        string    `json:"id"`
	GitHubID  int64     `json:"github_id"`
	Login     string    `json:"login"`
	Name      string    `json:"name"`
	AvatarURL string    `json:"avatar_url"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// AuthSession is a server-side authenticated login.
type AuthSession struct {
	ID        string
	UserID    string
	CreatedAt time.Time
	ExpiresAt time.Time
}

type Store struct {
	db *sql.DB
}

func NewStore(database *appdatabase.Database) (*Store, error) {
	if database == nil || database.SQL() == nil {
		return nil, errors.New("database is required")
	}
	store := &Store{db: database.SQL()}
	if err := store.configure(); err != nil {
		return nil, err
	}
	return store, nil
}

func (s *Store) configure() error {
	for _, statement := range []string{
		userSchema,
		authSessionSchema,
		`CREATE INDEX IF NOT EXISTS auth_sessions_user ON auth_sessions(user_id)`,
		`CREATE INDEX IF NOT EXISTS auth_sessions_hash ON auth_sessions(token_hash)`,
	} {
		if _, err := s.db.Exec(statement); err != nil {
			return fmt.Errorf("initialize account schema: %w", err)
		}
	}
	return nil
}

// UpsertGitHubUser creates the identifiable user if needed and refreshes the
// profile fields on subsequent logins.
func (s *Store) UpsertGitHubUser(
	ctx context.Context, githubID int64, login, name, avatarURL string,
) (User, error) {
	login = strings.TrimSpace(login)
	if githubID <= 0 || login == "" {
		return User{}, errors.New("GitHub user ID and login are required")
	}
	now := time.Now().UTC()
	if _, err := s.db.ExecContext(ctx, `INSERT INTO users (
		id, github_id, login, name, avatar_url, created_at, updated_at
	) VALUES (?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(github_id) DO UPDATE SET
		login = excluded.login,
		name = excluded.name,
		avatar_url = excluded.avatar_url,
		updated_at = excluded.updated_at`,
		newID(), githubID, login, strings.TrimSpace(name), strings.TrimSpace(avatarURL),
		formatTime(now), formatTime(now)); err != nil {
		return User{}, fmt.Errorf("upsert GitHub user: %w", err)
	}
	return s.UserByGitHubID(ctx, githubID)
}

func (s *Store) UserByGitHubID(ctx context.Context, githubID int64) (User, error) {
	row := s.db.QueryRowContext(ctx, `SELECT id, github_id, login, name, avatar_url,
		created_at, updated_at FROM users WHERE github_id = ?`, githubID)
	user, err := scanUser(row)
	if err != nil {
		return User{}, err
	}
	return user, nil
}

// CreateSession stores a hash of token and returns the session record. The raw
// token is only returned to the caller in the opaque session cookie.
func (s *Store) CreateSession(ctx context.Context, userID, token string, ttl time.Duration) (AuthSession, error) {
	userID, token = strings.TrimSpace(userID), strings.TrimSpace(token)
	if userID == "" || token == "" {
		return AuthSession{}, errors.New("session user and token are required")
	}
	if ttl <= 0 {
		return AuthSession{}, errors.New("session lifetime must be positive")
	}
	now := time.Now().UTC()
	session := AuthSession{
		ID: newID(), UserID: userID, CreatedAt: now, ExpiresAt: now.Add(ttl),
	}
	if _, err := s.db.ExecContext(ctx, `INSERT INTO auth_sessions (
		id, user_id, token_hash, created_at, expires_at
	) VALUES (?, ?, ?, ?, ?)`,
		session.ID, session.UserID, hashToken(token), formatTime(session.CreatedAt),
		formatTime(session.ExpiresAt)); err != nil {
		return AuthSession{}, fmt.Errorf("create auth session: %w", err)
	}
	return session, nil
}

// UserBySessionToken returns the user if token identifies a valid, unexpired,
// non-revoked session.
func (s *Store) UserBySessionToken(ctx context.Context, token string) (User, bool, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return User{}, false, nil
	}
	row := s.db.QueryRowContext(ctx, `SELECT u.id, u.github_id, u.login, u.name, u.avatar_url,
		u.created_at, u.updated_at
		FROM auth_sessions AS s
		JOIN users AS u ON u.id = s.user_id
		WHERE s.token_hash = ? AND s.revoked_at = '' AND s.expires_at > ?
		LIMIT 1`, hashToken(token), formatTime(time.Now().UTC()))
	user, err := scanUser(row)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, false, nil
	}
	if err != nil {
		return User{}, false, fmt.Errorf("load auth session: %w", err)
	}
	return user, true, nil
}

// RevokeSession invalidates the session identified by token. Revocation is
// idempotent so logout remains safe.
func (s *Store) RevokeSession(ctx context.Context, token string) error {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE auth_sessions SET revoked_at = ?
		WHERE token_hash = ? AND revoked_at = ''`, formatTime(time.Now().UTC()), hashToken(token)); err != nil {
		return fmt.Errorf("revoke auth session: %w", err)
	}
	return nil
}

func (s *Store) DeleteExpiredSessions(ctx context.Context, olderThan time.Time) (int64, error) {
	result, err := s.db.ExecContext(ctx, `DELETE FROM auth_sessions WHERE expires_at <= ?`,
		formatTime(olderThan.UTC()))
	if err != nil {
		return 0, fmt.Errorf("delete expired auth sessions: %w", err)
	}
	deleted, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}
	return deleted, nil
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func newID() string {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		panic("crypto/rand unavailable: " + err.Error())
	}
	return hex.EncodeToString(value[:])
}

func formatTime(value time.Time) string { return value.Format(time.RFC3339Nano) }

func scanUser(row *sql.Row) (User, error) {
	var value User
	var createdAt, updatedAt string
	err := row.Scan(&value.ID, &value.GitHubID, &value.Login, &value.Name, &value.AvatarURL,
		&createdAt, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, sql.ErrNoRows
	}
	if err != nil {
		return User{}, err
	}
	value.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return User{}, fmt.Errorf("decode user creation time: %w", err)
	}
	value.UpdatedAt, err = time.Parse(time.RFC3339Nano, updatedAt)
	if err != nil {
		return User{}, fmt.Errorf("decode user update time: %w", err)
	}
	return value, nil
}
