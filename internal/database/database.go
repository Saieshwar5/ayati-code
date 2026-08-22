package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	_ "github.com/jackc/pgx/v5/stdlib"
	_ "modernc.org/sqlite"
)

// Provider identifies the concrete SQL dialect and driver Perpetual uses.
type Provider string

// Supported database providers. SQLite remains the default and the development
// adapter; Postgres is the production multi-tenant system of record.
const (
	ProviderSQLite   Provider = "sqlite"
	ProviderPostgres Provider = "postgres"

	postgresDriverName = "pgx"
)

// Config selects the database backend and its connection source.
type Config struct {
	// Provider is either "sqlite" or "postgres". Empty means sqlite.
	Provider Provider
	// URL/Database carries the SQLite file path or the Postgres connection
	// string. For SQLite, keep passing the file path.
	URL string
	// Path is used by SQLite for file layout; Postgres ignores it.
	Path string
}

// Database owns the shared connection used by Perpetual's domain stores. The
// underlying driver is either SQLite (local/development) or Postgres
// (production multi-tenant).
type Database struct {
	provider Provider
	path     string
	sql      *sql.DB
}

// Open opens the SQLite database at path. It remains the shortcut used by
// existing call sites and tests; production callers should prefer
// OpenConfigured.
func Open(path string) (*Database, error) {
	return OpenSQLite(path)
}

// OpenSQLite opens a SQLite database file and applies the connection-level
// safety configuration used by Perpetual.
func OpenSQLite(path string) (*Database, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, errors.New("database path is required")
	}
	if path != ":memory:" {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return nil, fmt.Errorf("create database directory: %w", err)
		}
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite database: %w", err)
	}
	db.SetMaxOpenConns(1)
	for _, statement := range []string{
		`PRAGMA journal_mode = WAL`,
		`PRAGMA foreign_keys = ON`,
		`PRAGMA busy_timeout = 5000`,
	} {
		if _, err := db.Exec(statement); err != nil {
			db.Close()
			return nil, fmt.Errorf("configure sqlite database: %w", err)
		}
	}
	if path != ":memory:" {
		if err := os.Chmod(path, 0o600); err != nil {
			db.Close()
			return nil, fmt.Errorf("secure database: %w", err)
		}
	}
	return &Database{provider: ProviderSQLite, path: path, sql: db}, nil
}

// OpenPostgres opens a Postgres database from a connection string using the
// pgx/v5 stdlib driver.
func OpenPostgres(ctx context.Context, dsn string) (*Database, error) {
	dsn = strings.TrimSpace(dsn)
	if dsn == "" {
		return nil, errors.New("postgres connection string is required")
	}
	db, err := sql.Open(postgresDriverName, dsn)
	if err != nil {
		return nil, fmt.Errorf("open postgres database: %w", err)
	}
	// Postgres supports concurrent multi-worker operations; use a pool instead
	// of the SQLite single-connection limit.
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping postgres database: %w", err)
	}
	return &Database{provider: ProviderPostgres, path: dsn, sql: db}, nil
}

// OpenConfigured opens the provider selected by config. Empty provider or an
// empty URL falls back to SQLite for compatibility.
func OpenConfigured(ctx context.Context, config Config) (*Database, error) {
	provider := Provider(strings.ToLower(strings.TrimSpace(string(config.Provider))))
	if provider == "" {
		provider = ProviderSQLite
	}
	switch provider {
	case ProviderSQLite:
		path := strings.TrimSpace(config.URL)
		if path == "" {
			path = strings.TrimSpace(config.Path)
		}
		if path == "" {
			return nil, errors.New("sqlite database path is required")
		}
		return OpenSQLite(path)
	case ProviderPostgres:
		dsn := strings.TrimSpace(config.URL)
		if dsn == "" {
			return nil, errors.New("postgres connection string is required")
		}
		return OpenPostgres(ctx, dsn)
	default:
		return nil, fmt.Errorf("unsupported database provider %q", provider)
	}
}

// Provider returns the active database provider.
func (d *Database) Provider() Provider { return d.provider }

// Dialect returns the provider so domain packages can branch on SQL syntax
// when they must (column introspection, schema versioning).
func (d *Database) Dialect() Provider { return d.provider }

// Path returns the SQLite file path or the Postgres connection string used to
// open this database.
func (d *Database) Path() string { return d.path }

// SQL exposes the shared *sql.DB for domain stores.
func (d *Database) SQL() *sql.DB { return d.sql }

// Close closes the shared connection.
func (d *Database) Close() error { return d.sql.Close() }

// SchemaVersion returns the applied schema version. SQLite uses PRAGMA
// user_version; Postgres reads the schema_migrations table.
func (d *Database) SchemaVersion(ctx context.Context) (int, error) {
	if d.provider == ProviderPostgres {
		if _, err := d.sql.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
			version BIGINT PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)`); err != nil {
			return 0, fmt.Errorf("ensure postgres schema migrations: %w", err)
		}
		var version int64
		if err := d.sql.QueryRowContext(ctx,
			`SELECT COALESCE(MAX(version), 0) FROM schema_migrations`).Scan(&version); err != nil {
			return 0, fmt.Errorf("inspect postgres schema version: %w", err)
		}
		return int(version), nil
	}
	var version int
	if err := d.sql.QueryRowContext(ctx, `PRAGMA user_version`).Scan(&version); err != nil {
		return 0, fmt.Errorf("inspect sqlite schema version: %w", err)
	}
	return version, nil
}
