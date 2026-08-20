package workspace

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

const workspaceSchema = `CREATE TABLE IF NOT EXISTS workspaces (
	id TEXT PRIMARY KEY,
	repository TEXT NOT NULL,
	clone_url TEXT NOT NULL,
	base_branch TEXT NOT NULL,
	branch TEXT NOT NULL,
	create_branch INTEGER NOT NULL,
	preparation_stage TEXT NOT NULL DEFAULT 'pending',
	preparation_detail TEXT NOT NULL DEFAULT '',
	preparation_failed_stage TEXT NOT NULL DEFAULT '',
	selected_project_root TEXT NOT NULL DEFAULT '',
	configuration_candidates TEXT NOT NULL DEFAULT '[]',
	setup_command TEXT NOT NULL,
	path TEXT NOT NULL UNIQUE,
	status TEXT NOT NULL,
	error TEXT NOT NULL DEFAULT '',
	pull_request_number INTEGER NOT NULL DEFAULT 0,
	pull_request_url TEXT NOT NULL DEFAULT '',
	archived_at TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL
)`

const sessionSchema = `CREATE TABLE IF NOT EXISTS sessions (
	id TEXT PRIMARY KEY,
	workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
	title TEXT NOT NULL,
	status TEXT NOT NULL,
	error TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL
)`

const messageSchema = `CREATE TABLE IF NOT EXISTS messages (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
	payload TEXT NOT NULL,
	created_at TEXT NOT NULL
)`

const environmentSchema = `CREATE TABLE IF NOT EXISTS workspace_environment (
	workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
	name TEXT NOT NULL,
	ciphertext BLOB NOT NULL,
	nonce BLOB NOT NULL,
	expose_during_setup INTEGER NOT NULL DEFAULT 0,
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL,
	PRIMARY KEY (workspace_id, name)
)`

func (s *Store) configure() error {
	for _, statement := range []string{
		workspaceSchema, environmentSchema,
	} {
		if _, err := s.db.Exec(statement); err != nil {
			return fmt.Errorf("initialize database: %w", err)
		}
	}
	if err := s.migrateSessions(context.Background()); err != nil {
		return err
	}
	if err := s.migrateProjectProfiles(context.Background()); err != nil {
		return err
	}
	if err := s.migrateWorkspaceReadiness(context.Background()); err != nil {
		return err
	}
	if err := s.migrateWorkspaceArchive(context.Background()); err != nil {
		return err
	}
	if err := s.removeAgentCustomization(context.Background()); err != nil {
		return err
	}
	if err := s.migrateSingleWorkspaceMode(context.Background()); err != nil {
		return err
	}
	if err := s.migrateRemoveComputeSandbox(context.Background()); err != nil {
		return err
	}
	if err := s.migrateRemoveAgentRuns(context.Background()); err != nil {
		return err
	}
	return s.recoverInterruptedWork(context.Background())
}

func (s *Store) migrateSessions(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin session migration: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, sessionSchema); err != nil {
		return fmt.Errorf("create sessions table: %w", err)
	}
	if err := seedWorkspaceSessions(ctx, tx); err != nil {
		return err
	}
	columns, err := tableColumns(ctx, tx, "messages")
	if err != nil {
		return err
	}
	switch {
	case len(columns) == 0:
		if _, err := tx.ExecContext(ctx, messageSchema); err != nil {
			return fmt.Errorf("create messages table: %w", err)
		}
	case columns["workspace_id"]:
		if err := migrateWorkspaceMessages(ctx, tx); err != nil {
			return err
		}
	case !columns["session_id"]:
		return fmt.Errorf("messages table has an unsupported schema")
	}
	for _, statement := range []string{
		`CREATE INDEX IF NOT EXISTS sessions_workspace_updated ON sessions(workspace_id, updated_at DESC)`,
		`CREATE INDEX IF NOT EXISTS messages_session ON messages(session_id, id)`,
	} {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("create session index: %w", err)
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE sessions SET status = ?,
		error = 'Agent run interrupted when Perpetual restarted', updated_at = ? WHERE status = ?`,
		SessionStatusFailed, formatTime(time.Now().UTC()), SessionStatusWorking); err != nil {
		return fmt.Errorf("recover interrupted sessions: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE workspaces SET status = ?, error = ''
		WHERE status IN ('working', 'agent_failed', 'review', 'pull_request_open', 'done')`, StatusReady); err != nil {
		return fmt.Errorf("normalize workspace status: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `PRAGMA user_version = 2`); err != nil {
		return fmt.Errorf("record schema version: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit session migration: %w", err)
	}
	return nil
}

type legacyWorkspace struct {
	id, status, message, createdAt, updatedAt string
}

func seedWorkspaceSessions(ctx context.Context, tx *sql.Tx) error {
	rows, err := tx.QueryContext(ctx, `SELECT id, status, error, created_at, updated_at FROM workspaces
		WHERE NOT EXISTS (SELECT 1 FROM sessions WHERE sessions.workspace_id = workspaces.id)`)
	if err != nil {
		return fmt.Errorf("find workspaces without sessions: %w", err)
	}
	var values []legacyWorkspace
	for rows.Next() {
		var value legacyWorkspace
		if err := rows.Scan(&value.id, &value.status, &value.message, &value.createdAt, &value.updatedAt); err != nil {
			rows.Close()
			return fmt.Errorf("scan workspace for session migration: %w", err)
		}
		values = append(values, value)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, value := range values {
		createdAt, err := parseStoredTime(value.createdAt)
		if err != nil {
			return err
		}
		session, err := createSession(ctx, tx, value.id, "Original session", createdAt)
		if err != nil {
			return err
		}
		status, message := migratedSessionStatus(value.status, value.message)
		if status != SessionStatusIdle {
			if _, err := tx.ExecContext(ctx, `UPDATE sessions SET status = ?, error = ?, updated_at = ? WHERE id = ?`,
				status, message, value.updatedAt, session.ID); err != nil {
				return fmt.Errorf("migrate session status: %w", err)
			}
		}
	}
	return nil
}

func migratedSessionStatus(status, message string) (string, string) {
	switch status {
	case "working":
		return SessionStatusFailed, "Agent run interrupted when Perpetual restarted"
	case "agent_failed":
		return SessionStatusFailed, message
	case "review", "pull_request_open", "done":
		return SessionStatusReview, ""
	default:
		return SessionStatusIdle, ""
	}
}

func migrateWorkspaceMessages(ctx context.Context, tx *sql.Tx) error {
	var before int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM messages`).Scan(&before); err != nil {
		return fmt.Errorf("count legacy messages: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `CREATE TABLE messages_v1 (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
		payload TEXT NOT NULL,
		created_at TEXT NOT NULL
	)`); err != nil {
		return fmt.Errorf("create migrated messages table: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO messages_v1 (id, session_id, payload, created_at)
		SELECT messages.id, sessions.id, messages.payload, messages.created_at
		FROM messages JOIN sessions ON sessions.workspace_id = messages.workspace_id`); err != nil {
		return fmt.Errorf("migrate workspace messages: %w", err)
	}
	var after int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM messages_v1`).Scan(&after); err != nil {
		return fmt.Errorf("count migrated messages: %w", err)
	}
	if before != after {
		return fmt.Errorf("migrate workspace messages: copied %d of %d", after, before)
	}
	if _, err := tx.ExecContext(ctx, `DROP TABLE messages`); err != nil {
		return fmt.Errorf("remove legacy messages table: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `ALTER TABLE messages_v1 RENAME TO messages`); err != nil {
		return fmt.Errorf("install migrated messages table: %w", err)
	}
	return nil
}

func tableColumns(ctx context.Context, tx *sql.Tx, table string) (map[string]bool, error) {
	rows, err := tx.QueryContext(ctx, `PRAGMA table_info(`+table+`)`)
	if err != nil {
		return nil, fmt.Errorf("inspect %s schema: %w", table, err)
	}
	defer rows.Close()
	columns := make(map[string]bool)
	for rows.Next() {
		var position, notNull, primaryKey int
		var name, kind string
		var defaultValue any
		if err := rows.Scan(&position, &name, &kind, &notNull, &defaultValue, &primaryKey); err != nil {
			return nil, err
		}
		columns[name] = true
	}
	return columns, rows.Err()
}
