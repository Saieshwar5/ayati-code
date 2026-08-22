package workspace

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

func (s *Store) configure() error {
	for _, statement := range []string{
		workspaceSchema, environmentSchema(s.dialect),
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
	if err := s.migrateWorkspaceUsers(context.Background()); err != nil {
		return err
	}
	if err := s.migrateWorkspaceRuntimeColumns(context.Background()); err != nil {
		return err
	}
	if err := s.migrateRemoveAgentRuns(context.Background()); err != nil {
		return err
	}
	if err := s.migrateWorkspaceJobs(context.Background()); err != nil {
		return err
	}
	if err := s.migrateEnvironmentVersions(context.Background()); err != nil {
		return err
	}
	if err := s.RecoverJobs(context.Background()); err != nil {
		return err
	}
	return s.recoverInterruptedWork(context.Background())
}

func (s *Store) migrateWorkspaceUsers(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin workspace owner migration: %w", err)
	}
	defer tx.Rollback()
	columns, err := tableColumns(ctx, tx, s.database.Dialect(), "workspaces")
	if err != nil {
		return err
	}
	if !columns["user_id"] {
		if _, err := s.execTx(ctx, tx, `ALTER TABLE workspaces ADD COLUMN
			user_id TEXT NOT NULL DEFAULT ''`); err != nil {
			return fmt.Errorf("migrate workspace owner: %w", err)
		}
	}
	if _, err := s.execTx(ctx, tx, `CREATE INDEX IF NOT EXISTS workspaces_user ON workspaces(user_id)`); err != nil {
		return fmt.Errorf("create workspace owner index: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit workspace owner migration: %w", err)
	}
	return nil
}

func (s *Store) migrateWorkspaceRuntimeColumns(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin workspace runtime migration: %w", err)
	}
	defer tx.Rollback()
	columns, err := tableColumns(ctx, tx, s.database.Dialect(), "workspaces")
	if err != nil {
		return err
	}
	for _, statement := range []string{
		`ALTER TABLE workspaces ADD COLUMN runtime_provider TEXT NOT NULL DEFAULT 'local'`,
		`ALTER TABLE workspaces ADD COLUMN runtime_ref TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE workspaces ADD COLUMN runtime_state TEXT NOT NULL DEFAULT 'not_created'`,
		`ALTER TABLE workspaces ADD COLUMN runtime_updated_at TEXT NOT NULL DEFAULT ''`,
	} {
		name := strings.Fields(strings.TrimPrefix(statement, "ALTER TABLE workspaces ADD COLUMN "))[0]
		if columns[name] {
			continue
		}
		if _, err := s.execTx(ctx, tx, statement); err != nil {
			return fmt.Errorf("migrate workspace runtime %s: %w", name, err)
		}
	}
	if _, err := s.execTx(ctx, tx, `UPDATE workspaces SET runtime_provider = 'local'
		WHERE runtime_provider = ''`); err != nil {
		return fmt.Errorf("default workspace runtime provider: %w", err)
	}
	if _, err := s.execTx(ctx, tx, `UPDATE workspaces SET runtime_state = 'not_created'
		WHERE runtime_state = ''`); err != nil {
		return fmt.Errorf("default workspace runtime state: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit workspace runtime migration: %w", err)
	}
	return nil
}

func (s *Store) migrateSessions(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin session migration: %w", err)
	}
	defer tx.Rollback()
	if _, err := s.execTx(ctx, tx, sessionSchema); err != nil {
		return fmt.Errorf("create sessions table: %w", err)
	}
	if err := s.seedWorkspaceSessions(ctx, tx); err != nil {
		return err
	}
	columns, err := tableColumns(ctx, tx, s.database.Dialect(), "messages")
	if err != nil {
		return err
	}
	switch {
	case len(columns) == 0:
		if _, err := s.execTx(ctx, tx, messageSchema(s.dialect)); err != nil {
			return fmt.Errorf("create messages table: %w", err)
		}
	case columns["workspace_id"]:
		if err := s.migrateWorkspaceMessages(ctx, tx); err != nil {
			return err
		}
	case !columns["session_id"]:
		return fmt.Errorf("messages table has an unsupported schema")
	}
	for _, statement := range []string{
		`CREATE INDEX IF NOT EXISTS sessions_workspace_updated ON sessions(workspace_id, updated_at DESC)`,
		`CREATE INDEX IF NOT EXISTS messages_session ON messages(session_id, id)`,
	} {
		if _, err := s.execTx(ctx, tx, statement); err != nil {
			return fmt.Errorf("create session index: %w", err)
		}
	}
	if _, err := s.execTx(ctx, tx, `UPDATE sessions SET status = ?,
		error = 'Agent run interrupted when Perpetual restarted', updated_at = ? WHERE status = ?`,
		SessionStatusFailed, formatTime(time.Now().UTC()), SessionStatusWorking); err != nil {
		return fmt.Errorf("recover interrupted sessions: %w", err)
	}
	if _, err := s.execTx(ctx, tx, `UPDATE workspaces SET status = ?, error = ''
		WHERE status IN ('working', 'agent_failed', 'review', 'pull_request_open', 'done')`, StatusReady); err != nil {
		return fmt.Errorf("normalize workspace status: %w", err)
	}
	if err := s.setSchemaVersionTx(ctx, tx, 2); err != nil {
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

func (s *Store) seedWorkspaceSessions(ctx context.Context, tx *sql.Tx) error {
	rows, err := s.queryTx(ctx, tx, `SELECT id, status, error, created_at, updated_at FROM workspaces
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
			if _, err := s.execTx(ctx, tx, `UPDATE sessions SET status = ?, error = ?, updated_at = ? WHERE id = ?`,
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

func (s *Store) migrateWorkspaceMessages(ctx context.Context, tx *sql.Tx) error {
	var before int
	if err := s.queryRowTx(ctx, tx, `SELECT COUNT(*) FROM messages`).Scan(&before); err != nil {
		return fmt.Errorf("count legacy messages: %w", err)
	}
	if _, err := s.execTx(ctx, tx, migratedMessagesSchema(s.dialect)); err != nil {
		return fmt.Errorf("create migrated messages table: %w", err)
	}
	if _, err := s.execTx(ctx, tx, `INSERT INTO messages_v1 (id, session_id, payload, created_at)
		SELECT messages.id, sessions.id, messages.payload, messages.created_at
		FROM messages JOIN sessions ON sessions.workspace_id = messages.workspace_id`); err != nil {
		return fmt.Errorf("migrate workspace messages: %w", err)
	}
	var after int
	if err := s.queryRowTx(ctx, tx, `SELECT COUNT(*) FROM messages_v1`).Scan(&after); err != nil {
		return fmt.Errorf("count migrated messages: %w", err)
	}
	if before != after {
		return fmt.Errorf("migrate workspace messages: copied %d of %d", after, before)
	}
	if _, err := s.execTx(ctx, tx, `DROP TABLE messages`); err != nil {
		return fmt.Errorf("remove legacy messages table: %w", err)
	}
	if _, err := s.execTx(ctx, tx, `ALTER TABLE messages_v1 RENAME TO messages`); err != nil {
		return fmt.Errorf("install migrated messages table: %w", err)
	}
	return nil
}
