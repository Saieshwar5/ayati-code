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

	appdatabase "github.com/Saieshwar5/perpetual/internal/database"
)

const (
	StatusCreating             = "creating"
	StatusInitializing         = "initializing"
	StatusInitializationFailed = "initialization_failed"
	StatusNeedsConfiguration   = "needs_configuration"
	StatusReady                = "ready"
	StatusStopped              = "stopped"
	StatusDeleting             = "deleting"
	StatusDeletionFailed       = "deletion_failed"
)

var statuses = map[string]bool{
	StatusCreating: true, StatusInitializing: true, StatusInitializationFailed: true,
	StatusNeedsConfiguration: true, StatusReady: true, StatusStopped: true,
	StatusDeleting: true, StatusDeletionFailed: true,
}

type Workspace struct {
	ID                      string             `json:"id"`
	Repository              string             `json:"repository"`
	CloneURL                string             `json:"clone_url"`
	BaseBranch              string             `json:"base_branch"`
	Branch                  string             `json:"branch"`
	CreateBranch            bool               `json:"create_branch"`
	PreparationStage        string             `json:"preparation_stage"`
	PreparationDetail       string             `json:"preparation_detail,omitempty"`
	PreparationFailedStage  string             `json:"preparation_failed_stage,omitempty"`
	SelectedProjectRoot     string             `json:"selected_project_root,omitempty"`
	ConfigurationCandidates []ProjectCandidate `json:"configuration_candidates"`
	Profile                 *ProjectProfile    `json:"project_profile,omitempty"`
	Setup                   string             `json:"setup_command"`
	Path                    string             `json:"path"`
	Status                  string             `json:"status"`
	Error                   string             `json:"error,omitempty"`
	PullRequestNumber       int                `json:"pull_request_number,omitempty"`
	PullRequestURL          string             `json:"pull_request_url,omitempty"`
	ArchivedAt              *time.Time         `json:"archived_at,omitempty"`
	CreatedAt               time.Time          `json:"created_at"`
	UpdatedAt               time.Time          `json:"updated_at"`
}

type Create struct {
	Repository   string
	CloneURL     string
	BaseBranch   string
	Branch       string
	CreateBranch bool
	Setup        string
	Path         string
	Root         string
	Environment  []EnvironmentInput
}

type Store struct {
	db       *sql.DB
	sealer   *environmentSealer
	database *appdatabase.Database
}

func DefaultPath() (string, error) {
	root, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve data directory: %w", err)
	}
	return filepath.Join(root, "perpetual", "perpetual.db"), nil
}

func Open(path string) (*Store, error) {
	database, err := appdatabase.Open(path)
	if err != nil {
		return nil, err
	}
	store, err := NewStore(database)
	if err != nil {
		database.Close()
		return nil, err
	}
	store.database = database
	return store, nil
}

func NewStore(database *appdatabase.Database) (*Store, error) {
	if database == nil || database.SQL() == nil {
		return nil, errors.New("database is required")
	}
	db := database.SQL()
	var schemaVersion int
	if err := db.QueryRow(`PRAGMA user_version`).Scan(&schemaVersion); err != nil {
		return nil, fmt.Errorf("inspect database version: %w", err)
	}
	sealer, err := newEnvironmentSealer(database.Path(), schemaVersion >= 2)
	if err != nil {
		return nil, err
	}
	store := &Store{db: db, sealer: sealer}
	if err := store.configure(); err != nil {
		return nil, err
	}
	return store, nil
}

func (s *Store) Close() error {
	if s.database == nil {
		return nil
	}
	return s.database.Close()
}

func (s *Store) Create(ctx context.Context, input Create) (Workspace, error) {
	input.Repository = strings.TrimSpace(input.Repository)
	input.CloneURL = strings.TrimSpace(input.CloneURL)
	input.BaseBranch = strings.TrimSpace(input.BaseBranch)
	input.Branch = strings.TrimSpace(input.Branch)
	input.Setup = strings.TrimSpace(input.Setup)
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
	defaultAgent, err := s.DefaultAgent(ctx)
	if err != nil {
		return Workspace{}, fmt.Errorf("load default agent: %w", err)
	}
	value := Workspace{
		ID: id, Repository: input.Repository, CloneURL: input.CloneURL,
		BaseBranch: input.BaseBranch, Branch: input.Branch, Setup: input.Setup,
		CreateBranch:     input.CreateBranch,
		PreparationStage: PreparationPending, ConfigurationCandidates: []ProjectCandidate{},
		Path: path, Status: StatusCreating,
		CreatedAt: now, UpdatedAt: now,
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Workspace{}, fmt.Errorf("begin workspace creation: %w", err)
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `INSERT INTO workspaces (
		id, repository, clone_url, base_branch, branch, create_branch, setup_command, path,
		sandbox_name, status, error, pull_request_number, pull_request_url, created_at, updated_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, '', 0, '', ?, ?)`,
		value.ID, value.Repository, value.CloneURL, value.BaseBranch, value.Branch,
		value.CreateBranch, value.Setup, value.Path, value.ID, value.Status,
		formatTime(value.CreatedAt), formatTime(value.UpdatedAt),
	)
	if err != nil {
		return Workspace{}, fmt.Errorf("create workspace: %w", err)
	}
	if _, err := createSession(ctx, tx, value.ID, "Original session", defaultAgent.ID, now); err != nil {
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
	value, err := scanWorkspace(row)
	if err == nil {
		err = s.attachProfile(ctx, &value)
	}
	return value, err
}

func (s *Store) List(ctx context.Context) ([]Workspace, error) {
	rows, err := s.db.QueryContext(ctx, selectWorkspace+` WHERE archived_at = '' ORDER BY updated_at DESC`)
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
	if err := rows.Close(); err != nil {
		return nil, err
	}
	for index := range values {
		if err := s.attachProfile(ctx, &values[index]); err != nil {
			return nil, err
		}
	}
	return values, nil
}

func (s *Store) ListArchived(ctx context.Context) ([]Workspace, error) {
	rows, err := s.db.QueryContext(ctx, selectWorkspace+` WHERE archived_at != '' ORDER BY archived_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("list archived workspaces: %w", err)
	}
	defer rows.Close()
	var values []Workspace
	for rows.Next() {
		value, scanErr := scanWorkspace(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		values = append(values, value)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list archived workspaces: %w", err)
	}
	for index := range values {
		if err := s.attachProfile(ctx, &values[index]); err != nil {
			return nil, err
		}
	}
	return values, nil
}

const selectWorkspace = `SELECT id, repository, clone_url, base_branch, branch,
	create_branch, preparation_stage, preparation_detail, preparation_failed_stage,
	selected_project_root, configuration_candidates, setup_command, path, sandbox_name, status, error,
	pull_request_number, pull_request_url, archived_at, created_at, updated_at FROM workspaces`

type scanner interface{ Scan(...any) error }

func scanWorkspace(row scanner) (Workspace, error) {
	var value Workspace
	var archivedAt, createdAt, updatedAt, candidates, legacySandboxName string
	err := row.Scan(
		&value.ID, &value.Repository, &value.CloneURL, &value.BaseBranch, &value.Branch,
		&value.CreateBranch,
		&value.PreparationStage, &value.PreparationDetail, &value.PreparationFailedStage,
		&value.SelectedProjectRoot, &candidates, &value.Setup,
		&value.Path, &legacySandboxName, &value.Status, &value.Error,
		&value.PullRequestNumber, &value.PullRequestURL,
		&archivedAt, &createdAt, &updatedAt,
	)
	if err != nil {
		return Workspace{}, err
	}
	value.ConfigurationCandidates, err = decodeProjectCandidates(candidates)
	if err != nil {
		return Workspace{}, err
	}
	if archivedAt != "" {
		archived, parseErr := time.Parse(time.RFC3339Nano, archivedAt)
		if parseErr != nil {
			return Workspace{}, fmt.Errorf("decode workspace archive time: %w", parseErr)
		}
		value.ArchivedAt = &archived
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
