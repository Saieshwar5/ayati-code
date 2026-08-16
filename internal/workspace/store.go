package workspace

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

const (
	StatusCreating             = "creating"
	StatusInitializing         = "initializing"
	StatusInitializationFailed = "initialization_failed"
	StatusReady                = "ready"
	StatusStopped              = "stopped"
)

var statuses = map[string]bool{
	StatusCreating: true, StatusInitializing: true, StatusInitializationFailed: true,
	StatusReady: true, StatusStopped: true,
}

type Workspace struct {
	ID                 string    `json:"id"`
	Repository         string    `json:"repository"`
	CloneURL           string    `json:"clone_url"`
	BaseBranch         string    `json:"base_branch"`
	Branch             string    `json:"branch"`
	CreateBranch       bool      `json:"create_branch"`
	Authority          Authority `json:"authority"`
	EffectiveMountMode string    `json:"effective_mount_mode,omitempty"`
	Setup              string    `json:"setup_command"`
	Path               string    `json:"path"`
	SandboxName        string    `json:"sandbox_name"`
	Status             string    `json:"status"`
	Error              string    `json:"error,omitempty"`
	PullRequestNumber  int       `json:"pull_request_number,omitempty"`
	PullRequestURL     string    `json:"pull_request_url,omitempty"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

type Create struct {
	Repository   string
	CloneURL     string
	BaseBranch   string
	Branch       string
	CreateBranch bool
	Authority    Authority
	Setup        string
	Path         string
	Root         string
	Environment  []EnvironmentInput
}

type Store struct {
	db     *sql.DB
	sealer *environmentSealer
}

func DefaultPath() (string, error) {
	root, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve data directory: %w", err)
	}
	return filepath.Join(root, "ayati", "ayati.db"), nil
}

func Open(path string) (*Store, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("database path is required")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create database directory: %w", err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	db.SetMaxOpenConns(1)
	var schemaVersion int
	if err := db.QueryRow(`PRAGMA user_version`).Scan(&schemaVersion); err != nil {
		db.Close()
		return nil, fmt.Errorf("inspect database version: %w", err)
	}
	sealer, err := newEnvironmentSealer(path, schemaVersion >= 2)
	if err != nil {
		db.Close()
		return nil, err
	}
	store := &Store{db: db, sealer: sealer}
	if err := store.configure(); err != nil {
		db.Close()
		return nil, err
	}
	if path != ":memory:" {
		if err := os.Chmod(path, 0o600); err != nil {
			db.Close()
			return nil, fmt.Errorf("secure database: %w", err)
		}
	}
	return store, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) Create(ctx context.Context, input Create) (Workspace, error) {
	input.Repository = strings.TrimSpace(input.Repository)
	input.CloneURL = strings.TrimSpace(input.CloneURL)
	input.BaseBranch = strings.TrimSpace(input.BaseBranch)
	input.Branch = strings.TrimSpace(input.Branch)
	input.Setup = strings.TrimSpace(input.Setup)
	authority, err := ParseAuthority(string(input.Authority))
	if err != nil {
		return Workspace{}, err
	}
	input.Authority = authority
	if input.Repository == "" || input.CloneURL == "" || input.BaseBranch == "" || input.Branch == "" {
		return Workspace{}, errors.New("repository, clone URL, base branch, and branch are required")
	}
	id, err := newID()
	if err != nil {
		return Workspace{}, err
	}
	pathValue := strings.TrimSpace(input.Path)
	if pathValue == "" && strings.TrimSpace(input.Root) != "" {
		pathValue = filepath.Join(input.Root, id, "repo")
	}
	if pathValue == "" {
		return Workspace{}, errors.New("workspace path or root is required")
	}
	path, err := filepath.Abs(pathValue)
	if err != nil {
		return Workspace{}, fmt.Errorf("resolve workspace path: %w", err)
	}
	now := time.Now().UTC()
	value := Workspace{
		ID: id, Repository: input.Repository, CloneURL: input.CloneURL,
		BaseBranch: input.BaseBranch, Branch: input.Branch, Setup: input.Setup,
		CreateBranch: input.CreateBranch,
		Authority:    input.Authority,
		Path:         path, SandboxName: "ayati-workspace-" + id, Status: StatusCreating,
		CreatedAt: now, UpdatedAt: now,
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Workspace{}, fmt.Errorf("begin workspace creation: %w", err)
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `INSERT INTO workspaces (
		id, repository, clone_url, base_branch, branch, create_branch, authority,
		effective_mount_mode, setup_command, path,
		sandbox_name, status, error, pull_request_number, pull_request_url, created_at, updated_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, '', ?, ?, ?, ?, '', 0, '', ?, ?)`,
		value.ID, value.Repository, value.CloneURL, value.BaseBranch, value.Branch,
		value.CreateBranch, value.Authority, value.Setup, value.Path, value.SandboxName, value.Status,
		formatTime(value.CreatedAt), formatTime(value.UpdatedAt),
	)
	if err != nil {
		return Workspace{}, fmt.Errorf("create workspace: %w", err)
	}
	if _, err := createSession(ctx, tx, value.ID, "Original session", now); err != nil {
		return Workspace{}, err
	}
	if err := s.insertEnvironment(ctx, tx, value.ID, input.Environment, now); err != nil {
		return Workspace{}, err
	}
	if err := tx.Commit(); err != nil {
		return Workspace{}, fmt.Errorf("commit workspace creation: %w", err)
	}
	return value, nil
}

func (s *Store) Get(ctx context.Context, id string) (Workspace, error) {
	row := s.db.QueryRowContext(ctx, selectWorkspace+` WHERE id = ?`, strings.TrimSpace(id))
	return scanWorkspace(row)
}

func (s *Store) List(ctx context.Context) ([]Workspace, error) {
	rows, err := s.db.QueryContext(ctx, selectWorkspace+` ORDER BY updated_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("list workspaces: %w", err)
	}
	defer rows.Close()
	var values []Workspace
	for rows.Next() {
		value, err := scanWorkspace(rows)
		if err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list workspaces: %w", err)
	}
	return values, nil
}

func (s *Store) UpdateStatus(ctx context.Context, id, status, message string) error {
	if !statuses[status] {
		return fmt.Errorf("invalid workspace status %q", status)
	}
	result, err := s.db.ExecContext(ctx,
		`UPDATE workspaces SET status = ?, error = ?, updated_at = ? WHERE id = ?`,
		status, strings.TrimSpace(message), formatTime(time.Now().UTC()), strings.TrimSpace(id),
	)
	if err != nil {
		return fmt.Errorf("update workspace: %w", err)
	}
	if count, err := result.RowsAffected(); err != nil || count != 1 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) UpdateSetup(ctx context.Context, id, command string) error {
	result, err := s.db.ExecContext(ctx,
		`UPDATE workspaces SET setup_command = ?, updated_at = ? WHERE id = ?`,
		strings.TrimSpace(command), formatTime(time.Now().UTC()), strings.TrimSpace(id),
	)
	if err != nil {
		return fmt.Errorf("update workspace setup: %w", err)
	}
	if count, err := result.RowsAffected(); err != nil || count != 1 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) UpdatePullRequest(ctx context.Context, id string, number int, pullURL string) error {
	if number < 1 || strings.TrimSpace(pullURL) == "" {
		return errors.New("pull request number and URL are required")
	}
	result, err := s.db.ExecContext(ctx, `UPDATE workspaces SET
		pull_request_number = ?, pull_request_url = ?, updated_at = ? WHERE id = ?`,
		number, strings.TrimSpace(pullURL),
		formatTime(time.Now().UTC()), strings.TrimSpace(id),
	)
	if err != nil {
		return fmt.Errorf("update pull request: %w", err)
	}
	if count, err := result.RowsAffected(); err != nil || count != 1 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) Delete(ctx context.Context, id string) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM workspaces WHERE id = ?`, strings.TrimSpace(id))
	if err != nil {
		return fmt.Errorf("delete workspace: %w", err)
	}
	if count, err := result.RowsAffected(); err != nil || count != 1 {
		return sql.ErrNoRows
	}
	return nil
}

const selectWorkspace = `SELECT id, repository, clone_url, base_branch, branch,
	create_branch, authority, effective_mount_mode, setup_command, path, sandbox_name, status, error,
	pull_request_number, pull_request_url, created_at, updated_at FROM workspaces`

type scanner interface{ Scan(...any) error }

func scanWorkspace(row scanner) (Workspace, error) {
	var value Workspace
	var createdAt, updatedAt string
	err := row.Scan(
		&value.ID, &value.Repository, &value.CloneURL, &value.BaseBranch, &value.Branch,
		&value.CreateBranch, &value.Authority, &value.EffectiveMountMode, &value.Setup,
		&value.Path, &value.SandboxName, &value.Status, &value.Error,
		&value.PullRequestNumber, &value.PullRequestURL,
		&createdAt, &updatedAt,
	)
	if err != nil {
		return Workspace{}, err
	}
	value.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return Workspace{}, fmt.Errorf("decode workspace creation time: %w", err)
	}
	value.UpdatedAt, err = time.Parse(time.RFC3339Nano, updatedAt)
	if err != nil {
		return Workspace{}, fmt.Errorf("decode workspace update time: %w", err)
	}
	return value, nil
}

func formatTime(value time.Time) string { return value.Format(time.RFC3339Nano) }
