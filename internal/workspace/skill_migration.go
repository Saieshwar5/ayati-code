package workspace

import (
	"context"
	"fmt"
)

const skillSchema = `CREATE TABLE IF NOT EXISTS skills (
	id TEXT PRIMARY KEY,
	name TEXT NOT NULL,
	description TEXT NOT NULL DEFAULT '',
	markdown TEXT NOT NULL,
	revision INTEGER NOT NULL DEFAULT 1,
	archived_at TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL
)`

const agentSkillSchema = `CREATE TABLE IF NOT EXISTS agent_skills (
	agent_id TEXT NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
	skill_id TEXT NOT NULL REFERENCES skills(id) ON DELETE RESTRICT,
	position INTEGER NOT NULL,
	PRIMARY KEY (agent_id, skill_id),
	UNIQUE (agent_id, position)
)`

func (s *Store) migrateSkillCatalog(ctx context.Context) error {
	for _, statement := range []string{
		skillSchema,
		agentSkillSchema,
		`CREATE INDEX IF NOT EXISTS agent_skills_skill ON agent_skills(skill_id)`,
		`PRAGMA user_version = 8`,
	} {
		if _, err := s.db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("create skill catalog: %w", err)
		}
	}
	return nil
}
