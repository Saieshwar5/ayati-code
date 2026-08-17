package environment

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

func (s *Store) Acquire(ctx context.Context, workspaceID, preferredEnvironmentID string) (Lease, error) {
	workspaceID = strings.TrimSpace(workspaceID)
	preferredEnvironmentID = strings.TrimSpace(preferredEnvironmentID)
	if workspaceID == "" {
		return Lease{}, errors.New("workspace is required")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Lease{}, fmt.Errorf("begin environment acquisition: %w", err)
	}
	defer tx.Rollback()
	var active int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM environment_leases WHERE workspace_id = ?
		AND state IN ('acquiring', 'active', 'releasing')`, workspaceID).Scan(&active); err != nil {
		return Lease{}, fmt.Errorf("inspect workspace lease: %w", err)
	}
	if active != 0 {
		return Lease{}, ErrWorkspaceLeased
	}
	query := `SELECT e.id FROM environments e WHERE e.provisioning_state = 'ready'
		AND NOT EXISTS (SELECT 1 FROM environment_leases l WHERE l.environment_id = e.id
		AND l.state IN ('acquiring', 'active', 'releasing'))`
	arguments := []any{}
	if preferredEnvironmentID != "" {
		query += ` AND e.id = ?`
		arguments = append(arguments, preferredEnvironmentID)
	}
	query += ` ORDER BY e.created_at, e.id LIMIT 1`
	var environmentID string
	if err := tx.QueryRowContext(ctx, query, arguments...).Scan(&environmentID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Lease{}, ErrNoEnvironmentAvailable
		}
		return Lease{}, fmt.Errorf("select environment: %w", err)
	}
	result, err := tx.ExecContext(ctx, `UPDATE environments SET generation = generation + 1,
		updated_at = ? WHERE id = ?`, formatTime(time.Now().UTC()), environmentID)
	if err != nil {
		return Lease{}, fmt.Errorf("advance environment generation: %w", err)
	}
	if err := requireOneRow(result); err != nil {
		return Lease{}, err
	}
	var generation int64
	if err := tx.QueryRowContext(ctx, `SELECT generation FROM environments WHERE id = ?`,
		environmentID).Scan(&generation); err != nil {
		return Lease{}, fmt.Errorf("load environment generation: %w", err)
	}
	id, err := newID()
	if err != nil {
		return Lease{}, err
	}
	now := time.Now().UTC()
	value := Lease{
		ID: id, EnvironmentID: environmentID, WorkspaceID: workspaceID,
		Generation: generation, State: LeaseAcquiring, AcquiredAt: now,
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO environment_leases (
		id, environment_id, workspace_id, generation, state, runtime_id, error,
		acquired_at, activated_at, released_at
	) VALUES (?, ?, ?, ?, ?, '', '', ?, '', '')`, id, environmentID, workspaceID,
		generation, LeaseAcquiring, formatTime(now))
	if err != nil {
		return Lease{}, classifyLeaseConstraint(err)
	}
	if err := tx.Commit(); err != nil {
		return Lease{}, fmt.Errorf("commit environment acquisition: %w", err)
	}
	return value, nil
}

func (s *Store) Activate(ctx context.Context, leaseID string, generation int64, runtimeID string) error {
	runtimeID = strings.TrimSpace(runtimeID)
	if runtimeID == "" {
		return errors.New("runtime identity is required")
	}
	now := time.Now().UTC()
	result, err := s.db.ExecContext(ctx, `UPDATE environment_leases SET state = ?, runtime_id = ?,
		activated_at = ?, error = '' WHERE id = ? AND generation = ? AND state = ?`, LeaseActive,
		runtimeID, formatTime(now), strings.TrimSpace(leaseID), generation, LeaseAcquiring)
	if err != nil {
		return fmt.Errorf("activate environment lease: %w", err)
	}
	return requireLeaseTransition(result)
}

func (s *Store) BeginRelease(ctx context.Context, leaseID string, generation int64) error {
	result, err := s.db.ExecContext(ctx, `UPDATE environment_leases SET state = ?
		WHERE id = ? AND generation = ? AND state IN ('acquiring', 'active')`, LeaseReleasing,
		strings.TrimSpace(leaseID), generation)
	if err != nil {
		return fmt.Errorf("begin environment release: %w", err)
	}
	return requireLeaseTransition(result)
}

func (s *Store) ReplaceRuntime(ctx context.Context, leaseID string, generation int64, runtimeID string) error {
	runtimeID = strings.TrimSpace(runtimeID)
	if runtimeID == "" {
		return errors.New("runtime identity is required")
	}
	result, err := s.db.ExecContext(ctx, `UPDATE environment_leases SET runtime_id = ?, error = ''
		WHERE id = ? AND generation = ? AND state = ?`, runtimeID,
		strings.TrimSpace(leaseID), generation, LeaseActive)
	if err != nil {
		return fmt.Errorf("replace environment runtime: %w", err)
	}
	return requireLeaseTransition(result)
}

func (s *Store) CompleteRelease(ctx context.Context, leaseID string, generation int64) error {
	now := time.Now().UTC()
	result, err := s.db.ExecContext(ctx, `UPDATE environment_leases SET state = ?, released_at = ?
		WHERE id = ? AND generation = ? AND state = ?`, LeaseReleased, formatTime(now),
		strings.TrimSpace(leaseID), generation, LeaseReleasing)
	if err != nil {
		return fmt.Errorf("complete environment release: %w", err)
	}
	return requireLeaseTransition(result)
}

func (s *Store) Fail(ctx context.Context, leaseID string, generation int64, cause error) error {
	message := "environment lease failed"
	if cause != nil && strings.TrimSpace(cause.Error()) != "" {
		message = strings.TrimSpace(cause.Error())
	}
	now := time.Now().UTC()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin failed environment lease: %w", err)
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `UPDATE environment_leases SET state = ?, error = ?, released_at = ?
		WHERE id = ? AND generation = ? AND state IN ('acquiring', 'active', 'releasing')`,
		LeaseFailed, message, formatTime(now), strings.TrimSpace(leaseID), generation)
	if err != nil {
		return fmt.Errorf("fail environment lease: %w", err)
	}
	if err := requireLeaseTransition(result); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE environments SET provisioning_state = ?, error = ?,
		updated_at = ? WHERE id = (SELECT environment_id FROM environment_leases WHERE id = ?)`,
		ProvisioningFailed, message, formatTime(now), strings.TrimSpace(leaseID)); err != nil {
		return fmt.Errorf("quarantine failed environment: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit failed environment lease: %w", err)
	}
	return nil
}

func (s *Store) ActiveForWorkspace(ctx context.Context, workspaceID string) (Lease, error) {
	row := s.db.QueryRowContext(ctx, leaseSelect+` WHERE workspace_id = ?
		AND state IN ('acquiring', 'active', 'releasing')`, strings.TrimSpace(workspaceID))
	return scanLease(row)
}

func (s *Store) ActiveForEnvironment(ctx context.Context, environmentID string) (Lease, error) {
	row := s.db.QueryRowContext(ctx, leaseSelect+` WHERE environment_id = ?
		AND state IN ('acquiring', 'active', 'releasing')`, strings.TrimSpace(environmentID))
	return scanLease(row)
}

func (s *Store) LatestForWorkspace(ctx context.Context, workspaceID string) (Lease, error) {
	row := s.db.QueryRowContext(ctx, leaseSelect+` WHERE workspace_id = ?
		ORDER BY acquired_at DESC, id DESC LIMIT 1`, strings.TrimSpace(workspaceID))
	return scanLease(row)
}

const leaseSelect = `SELECT id, environment_id, workspace_id, generation, state, runtime_id,
	error, acquired_at, activated_at, released_at FROM environment_leases`

func scanLease(row scanner) (Lease, error) {
	var value Lease
	var acquiredAt, activatedAt, releasedAt string
	if err := row.Scan(&value.ID, &value.EnvironmentID, &value.WorkspaceID, &value.Generation,
		&value.State, &value.RuntimeID, &value.Error, &acquiredAt, &activatedAt, &releasedAt); err != nil {
		return Lease{}, err
	}
	var err error
	value.AcquiredAt, value.ActivatedAt, value.ReleasedAt, err = parseLeaseTimes(
		acquiredAt, activatedAt, releasedAt,
	)
	return value, err
}

func parseLeaseTimes(acquiredAt, activatedAt, releasedAt string) (time.Time, *time.Time, *time.Time, error) {
	acquired, err := time.Parse(time.RFC3339Nano, acquiredAt)
	if err != nil {
		return time.Time{}, nil, nil, fmt.Errorf("parse lease acquisition time: %w", err)
	}
	activated, err := parseOptionalTime(activatedAt)
	if err != nil {
		return time.Time{}, nil, nil, err
	}
	released, err := parseOptionalTime(releasedAt)
	return acquired, activated, released, err
}

func parseOptionalTime(value string) (*time.Time, error) {
	if value == "" {
		return nil, nil
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	return &parsed, err
}

func requireLeaseTransition(result sql.Result) error {
	if err := requireOneRow(result); errors.Is(err, sql.ErrNoRows) {
		return ErrLeaseState
	} else {
		return err
	}
}

func classifyLeaseConstraint(err error) error {
	message := err.Error()
	if strings.Contains(message, "environment_leases.workspace_id") {
		return ErrWorkspaceLeased
	}
	if strings.Contains(message, "environment_leases.environment_id") {
		return ErrNoEnvironmentAvailable
	}
	return fmt.Errorf("create environment lease: %w", err)
}
