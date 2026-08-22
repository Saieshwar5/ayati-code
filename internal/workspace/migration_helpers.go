package workspace

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	appdatabase "github.com/Saieshwar5/perpetual/internal/database"
)

// databaseColumns inspects the columns of a table using the active database
// dialect. SQLite uses PRAGMA; Postgres uses information_schema.
func databaseColumns(ctx context.Context, db *sql.DB, dialect appdatabase.Provider, table string) (map[string]bool, error) {
	if dialect == appdatabase.ProviderPostgres {
		return postgresColumns(ctx, db, table)
	}
	return sqliteColumns(ctx, db, table)
}

// tableColumns mirrors databaseColumns for a transaction handle.
func tableColumns(ctx context.Context, tx *sql.Tx, dialect appdatabase.Provider, table string) (map[string]bool, error) {
	if dialect == appdatabase.ProviderPostgres {
		return postgresColumnsTx(ctx, tx, table)
	}
	return sqliteColumnsTx(ctx, tx, table)
}

func sqliteColumns(ctx context.Context, db *sql.DB, table string) (map[string]bool, error) {
	rows, err := db.QueryContext(ctx, `PRAGMA table_info(`+table+`)`)
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

func sqliteColumnsTx(ctx context.Context, tx *sql.Tx, table string) (map[string]bool, error) {
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

func postgresColumns(ctx context.Context, db *sql.DB, table string) (map[string]bool, error) {
	rows, err := db.QueryContext(ctx, `SELECT column_name FROM information_schema.columns
		WHERE table_name = $1`, strings.ToLower(strings.TrimSpace(table)))
	if err != nil {
		return nil, fmt.Errorf("inspect %s schema: %w", table, err)
	}
	defer rows.Close()
	columns := make(map[string]bool)
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		columns[name] = true
	}
	return columns, rows.Err()
}

func postgresColumnsTx(ctx context.Context, tx *sql.Tx, table string) (map[string]bool, error) {
	rows, err := tx.QueryContext(ctx, `SELECT column_name FROM information_schema.columns
		WHERE table_name = $1`, strings.ToLower(strings.TrimSpace(table)))
	if err != nil {
		return nil, fmt.Errorf("inspect %s schema: %w", table, err)
	}
	defer rows.Close()
	columns := make(map[string]bool)
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		columns[name] = true
	}
	return columns, rows.Err()
}
