package environment

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	appdatabase "github.com/Saieshwar5/perpetual/internal/database"
)

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

func (s *Store) Create(ctx context.Context, input CreateInput) (Environment, error) {
	input, err := normalizeCreate(input)
	if err != nil {
		return Environment{}, err
	}
	id, err := newID()
	if err != nil {
		return Environment{}, err
	}
	now := time.Now().UTC()
	_, err = s.db.ExecContext(ctx, `INSERT INTO environments (
		id, name, driver, image_ref, image_digest, cpu_millis, memory_mb, pid_limit,
		network_policy, provisioning_state, generation, error, created_at, updated_at
	) VALUES (?, ?, ?, ?, '', ?, ?, ?, ?, ?, 0, '', ?, ?)`, id, input.Name, input.Driver,
		input.ImageRef, input.CPUMillis, input.MemoryMB, input.PIDLimit, input.NetworkPolicy,
		Provisioning, formatTime(now), formatTime(now))
	if err != nil {
		return Environment{}, fmt.Errorf("create environment: %w", err)
	}
	return s.Get(ctx, id)
}

func (s *Store) MarkReady(ctx context.Context, id, digest string) error {
	digest = strings.TrimSpace(digest)
	if digest == "" {
		return errors.New("environment image digest is required")
	}
	result, err := s.db.ExecContext(ctx, `UPDATE environments SET image_digest = ?,
		provisioning_state = ?, error = '', updated_at = ? WHERE id = ?
		AND provisioning_state IN ('provisioning', 'failed')`, digest, ProvisioningReady,
		formatTime(time.Now().UTC()), strings.TrimSpace(id))
	if err != nil {
		return fmt.Errorf("mark environment ready: %w", err)
	}
	return requireOneRow(result)
}

func (s *Store) MarkFailed(ctx context.Context, id string, cause error) error {
	message := "environment provisioning failed"
	if cause != nil && strings.TrimSpace(cause.Error()) != "" {
		message = strings.TrimSpace(cause.Error())
	}
	result, err := s.db.ExecContext(ctx, `UPDATE environments SET provisioning_state = ?,
		error = ?, updated_at = ? WHERE id = ?`, ProvisioningFailed, message,
		formatTime(time.Now().UTC()), strings.TrimSpace(id))
	if err != nil {
		return fmt.Errorf("mark environment failed: %w", err)
	}
	return requireOneRow(result)
}

func (s *Store) Get(ctx context.Context, id string) (Environment, error) {
	row := s.db.QueryRowContext(ctx, environmentSelect+` WHERE e.id = ?`, strings.TrimSpace(id))
	return scanEnvironment(row)
}

func (s *Store) List(ctx context.Context) ([]Environment, error) {
	rows, err := s.db.QueryContext(ctx, environmentSelect+` ORDER BY e.created_at, e.id`)
	if err != nil {
		return nil, fmt.Errorf("list environments: %w", err)
	}
	defer rows.Close()
	values := []Environment{}
	for rows.Next() {
		value, err := scanEnvironment(rows)
		if err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, rows.Err()
}

func (s *Store) Delete(ctx context.Context, id string) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM environments WHERE id = ? AND NOT EXISTS (
		SELECT 1 FROM environment_leases WHERE environment_id = environments.id
		AND state IN ('acquiring', 'active', 'releasing'))`, strings.TrimSpace(id))
	if err != nil {
		return fmt.Errorf("delete environment: %w", err)
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		if _, getErr := s.Get(ctx, id); getErr == nil {
			return ErrEnvironmentOccupied
		}
		return sql.ErrNoRows
	}
	return nil
}

const environmentSelect = `SELECT e.id, e.name, e.driver, e.image_ref, e.image_digest,
	e.cpu_millis, e.memory_mb, e.pid_limit, e.network_policy, e.provisioning_state,
	e.generation, e.error, e.created_at, e.updated_at,
	EXISTS (SELECT 1 FROM environment_leases failed WHERE failed.environment_id = e.id
		AND failed.state = 'failed'),
	COALESCE(l.id, ''), COALESCE(l.workspace_id, ''), COALESCE(l.generation, 0),
	COALESCE(l.state, ''), COALESCE(l.runtime_id, ''), COALESCE(l.error, ''),
	COALESCE(l.acquired_at, ''), COALESCE(l.activated_at, ''), COALESCE(l.released_at, '')
	FROM environments e LEFT JOIN environment_leases l ON l.environment_id = e.id
	AND l.state IN ('acquiring', 'active', 'releasing')`

type scanner interface {
	Scan(...any) error
}

func scanEnvironment(row scanner) (Environment, error) {
	var value Environment
	var createdAt, updatedAt string
	var lease Lease
	var acquiredAt, activatedAt, releasedAt string
	err := row.Scan(&value.ID, &value.Name, &value.Driver, &value.ImageRef, &value.ImageDigest,
		&value.CPUMillis, &value.MemoryMB, &value.PIDLimit, &value.NetworkPolicy,
		&value.ProvisioningState, &value.Generation, &value.Error, &createdAt, &updatedAt, &value.Quarantined,
		&lease.ID, &lease.WorkspaceID, &lease.Generation, &lease.State, &lease.RuntimeID,
		&lease.Error, &acquiredAt, &activatedAt, &releasedAt)
	if err != nil {
		return Environment{}, err
	}
	value.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return Environment{}, fmt.Errorf("parse environment creation time: %w", err)
	}
	value.UpdatedAt, err = time.Parse(time.RFC3339Nano, updatedAt)
	if err != nil {
		return Environment{}, fmt.Errorf("parse environment update time: %w", err)
	}
	value.State = stateFor(value.ProvisioningState, lease.State)
	if lease.ID != "" {
		lease.EnvironmentID = value.ID
		lease.AcquiredAt, lease.ActivatedAt, lease.ReleasedAt, err = parseLeaseTimes(acquiredAt, activatedAt, releasedAt)
		if err != nil {
			return Environment{}, err
		}
		value.ActiveLease = &lease
	}
	return value, nil
}

func newID() (string, error) {
	var data [12]byte
	if _, err := rand.Read(data[:]); err != nil {
		return "", fmt.Errorf("create environment identity: %w", err)
	}
	return hex.EncodeToString(data[:]), nil
}

func formatTime(value time.Time) string { return value.Format(time.RFC3339Nano) }

func requireOneRow(result sql.Result) error {
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return sql.ErrNoRows
	}
	return nil
}
