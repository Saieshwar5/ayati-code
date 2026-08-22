package workspace

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	appdatabase "github.com/Saieshwar5/perpetual/internal/database"
)

// q converts SQLite-style "?" placeholders to Postgres "$N" placeholders when
// the active provider is Postgres. SQLite queries are returned unchanged.
func (s *Store) q(query string) string {
	if s.dialect != appdatabase.ProviderPostgres {
		return query
	}
	var builder strings.Builder
	position := 0
	for index := 0; index < len(query); index++ {
		if query[index] == '?' {
			position++
			fmt.Fprintf(&builder, "$%d", position)
			continue
		}
		builder.WriteByte(query[index])
	}
	return builder.String()
}

func (s *Store) execContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return s.db.ExecContext(ctx, s.q(query), args...)
}

func (s *Store) queryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	return s.db.QueryContext(ctx, s.q(query), args...)
}

func (s *Store) queryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	return s.db.QueryRowContext(ctx, s.q(query), args...)
}

func (s *Store) execTx(ctx context.Context, tx *sql.Tx, query string, args ...any) (sql.Result, error) {
	return tx.ExecContext(ctx, s.q(query), args...)
}

func (s *Store) queryTx(ctx context.Context, tx *sql.Tx, query string, args ...any) (*sql.Rows, error) {
	return tx.QueryContext(ctx, s.q(query), args...)
}

func (s *Store) queryRowTx(ctx context.Context, tx *sql.Tx, query string, args ...any) *sql.Row {
	return tx.QueryRowContext(ctx, s.q(query), args...)
}

// setSchemaVersion records the applied schema version on the active provider.
// SQLite uses PRAGMA user_version; Postgres appends to schema_migrations.
func (s *Store) setSchemaVersion(ctx context.Context, version int) error {
	if s.dialect == appdatabase.ProviderPostgres {
		if _, err := s.db.ExecContext(ctx,
			`INSERT INTO schema_migrations (version) VALUES ($1) ON CONFLICT (version) DO NOTHING`,
			version); err != nil {
			return fmt.Errorf("record postgres schema version %d: %w", version, err)
		}
		return nil
	}
	if _, err := s.db.ExecContext(ctx, fmt.Sprintf(`PRAGMA user_version = %d`, version)); err != nil {
		return fmt.Errorf("record sqlite schema version %d: %w", version, err)
	}
	return nil
}

// schemaFor returns provider-specific DDL for one table. Schema creators that
// differ between SQLite and Postgres are implemented here.
func schemaFor(dialect appdatabase.Provider, sqliteSchema, postgresSchema string) string {
	if dialect == appdatabase.ProviderPostgres {
		return postgresSchema
	}
	return sqliteSchema
}

const workspaceSchema = `CREATE TABLE IF NOT EXISTS workspaces (
	id TEXT PRIMARY KEY,
	user_id TEXT NOT NULL DEFAULT '',
	runtime_provider TEXT NOT NULL DEFAULT 'local',
	runtime_ref TEXT NOT NULL DEFAULT '',
	runtime_state TEXT NOT NULL DEFAULT 'not_created',
	runtime_updated_at TEXT NOT NULL DEFAULT '',
	repository TEXT NOT NULL,
	clone_url TEXT NOT NULL,
	base_branch TEXT NOT NULL,
	branch TEXT NOT NULL,
	create_branch INTEGER NOT NULL,
	environment_version_id TEXT NOT NULL DEFAULT '',
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

func messageSchema(dialect appdatabase.Provider) string {
	return schemaFor(dialect,
		`CREATE TABLE IF NOT EXISTS messages (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
			payload TEXT NOT NULL,
			created_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS messages (
			id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
			session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
			payload TEXT NOT NULL,
			created_at TEXT NOT NULL
		)`,
	)
}

func migratedMessagesSchema(dialect appdatabase.Provider) string {
	return schemaFor(dialect,
		`CREATE TABLE messages_v1 (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
			payload TEXT NOT NULL,
			created_at TEXT NOT NULL
		)`,
		`CREATE TABLE messages_v1 (
			id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
			session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
			payload TEXT NOT NULL,
			created_at TEXT NOT NULL
		)`,
	)
}

func environmentSchema(dialect appdatabase.Provider) string {
	return schemaFor(dialect,
		`CREATE TABLE IF NOT EXISTS workspace_environment (
			workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
			name TEXT NOT NULL,
			ciphertext BLOB NOT NULL,
			nonce BLOB NOT NULL,
			expose_during_setup INTEGER NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			PRIMARY KEY (workspace_id, name)
		)`,
		`CREATE TABLE IF NOT EXISTS workspace_environment (
			workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
			name TEXT NOT NULL,
			ciphertext BYTEA NOT NULL,
			nonce BYTEA NOT NULL,
			expose_during_setup INTEGER NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			PRIMARY KEY (workspace_id, name)
		)`,
	)
}

// setSchemaVersionTx records the schema version inside an active transaction.
// SQLite writes PRAGMA on the transaction connection; Postgres inserts into
// schema_migrations in the same transaction so the single SQLite connection
// limit is never deadlocked.
func (s *Store) setSchemaVersionTx(ctx context.Context, tx *sql.Tx, version int) error {
	if s.dialect == appdatabase.ProviderPostgres {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO schema_migrations (version) VALUES ($1) ON CONFLICT (version) DO NOTHING`,
			version); err != nil {
			return fmt.Errorf("record postgres schema version %d: %w", version, err)
		}
		return nil
	}
	if _, err := tx.ExecContext(ctx, fmt.Sprintf(`PRAGMA user_version = %d`, version)); err != nil {
		return fmt.Errorf("record sqlite schema version %d: %w", version, err)
	}
	return nil
}
