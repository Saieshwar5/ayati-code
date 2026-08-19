package workspace

import (
	"context"
	"fmt"
	"time"

	"github.com/Saieshwar5/perpetual/internal/agent"
)

const agentSchema = `CREATE TABLE IF NOT EXISTS agents (
	id TEXT PRIMARY KEY,
	name TEXT NOT NULL,
	emoji TEXT NOT NULL,
	description TEXT NOT NULL DEFAULT '',
	provider_id TEXT NOT NULL,
	model TEXT NOT NULL DEFAULT '',
	max_steps INTEGER NOT NULL CHECK (max_steps BETWEEN 1 AND 20),
	shell_enabled INTEGER NOT NULL DEFAULT 1,
	instructions TEXT NOT NULL DEFAULT '',
	revision INTEGER NOT NULL DEFAULT 1,
	built_in INTEGER NOT NULL DEFAULT 0,
	archived_at TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL
)`

const applicationSettingsSchema = `CREATE TABLE IF NOT EXISTS application_settings (
	id INTEGER PRIMARY KEY CHECK (id = 1),
	default_agent_id TEXT NOT NULL REFERENCES agents(id)
)`

func (s *Store) migrateAgentCatalog(ctx context.Context) error {
	for _, statement := range []string{agentSchema, applicationSettingsSchema} {
		if _, err := s.db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("create agent catalog: %w", err)
		}
	}
	now := formatTime(time.Now().UTC())
	if _, err := s.db.ExecContext(ctx, `INSERT OR IGNORE INTO agents (
		id, name, emoji, description, provider_id, model, max_steps, shell_enabled,
		instructions, revision, built_in, archived_at, created_at, updated_at
	) VALUES (?, 'Perpetual', '✦', 'General coding agent', ?, '', ?, 1, '', 1, 1, '', ?, ?)`,
		agent.BuiltinAgentID, agent.FireworksProviderID, agent.MaxSteps, now, now); err != nil {
		return fmt.Errorf("seed built-in agent: %w", err)
	}
	if _, err := s.db.ExecContext(ctx,
		`INSERT OR IGNORE INTO application_settings (id, default_agent_id) VALUES (1, ?)`,
		agent.BuiltinAgentID); err != nil {
		return fmt.Errorf("seed agent settings: %w", err)
	}
	if err := s.addAgentSessionColumns(ctx); err != nil {
		return err
	}
	if err := s.addMessageAttributionColumns(ctx); err != nil {
		return err
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE sessions SET selected_agent_id = ?
		WHERE selected_agent_id = ''`, agent.BuiltinAgentID); err != nil {
		return fmt.Errorf("assign existing sessions to built-in agent: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS sessions_agent
		ON sessions(selected_agent_id)`); err != nil {
		return fmt.Errorf("create session agent index: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, `PRAGMA user_version = 7`); err != nil {
		return fmt.Errorf("record agent catalog migration: %w", err)
	}
	return nil
}

func (s *Store) addAgentSessionColumns(ctx context.Context) error {
	columns, err := databaseColumns(ctx, s.db, "sessions")
	if err != nil {
		return err
	}
	if columns["selected_agent_id"] {
		return nil
	}
	_, err = s.db.ExecContext(ctx, `ALTER TABLE sessions ADD COLUMN selected_agent_id TEXT NOT NULL DEFAULT 'builtin-perpetual'`)
	if err != nil {
		return fmt.Errorf("add selected session agent: %w", err)
	}
	return nil
}

func (s *Store) addMessageAttributionColumns(ctx context.Context) error {
	columns, err := databaseColumns(ctx, s.db, "messages")
	if err != nil {
		return err
	}
	for _, column := range []string{
		"agent_id", "agent_name", "agent_emoji", "agent_revision", "agent_provider_id", "agent_model",
		"agent_skills",
	} {
		if columns[column] {
			continue
		}
		kind := "TEXT NOT NULL DEFAULT ''"
		if column == "agent_revision" {
			kind = "INTEGER NOT NULL DEFAULT 0"
		} else if column == "agent_skills" {
			kind = "TEXT NOT NULL DEFAULT '[]'"
		}
		if _, err := s.db.ExecContext(ctx, `ALTER TABLE messages ADD COLUMN `+column+` `+kind); err != nil {
			return fmt.Errorf("add message attribution column %s: %w", column, err)
		}
	}
	return nil
}
