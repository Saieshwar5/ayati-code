package workspace

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/Saieshwar5/ayati-code/internal/agent"
)

type skillQuerier interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func (s *Store) AgentSkills(ctx context.Context, agentID string) ([]agent.Skill, error) {
	return queryAgentSkills(ctx, s.db, agentID)
}

func queryAgentSkills(ctx context.Context, query skillQuerier, agentID string) ([]agent.Skill, error) {
	rows, err := query.QueryContext(ctx, selectSkill+`
		JOIN agent_skills ON agent_skills.skill_id = skills.id
		WHERE agent_skills.agent_id = ? ORDER BY agent_skills.position`, strings.TrimSpace(agentID))
	if err != nil {
		return nil, fmt.Errorf("load agent skills: %w", err)
	}
	defer rows.Close()
	var values []agent.Skill
	for rows.Next() {
		value, scanErr := scanSkill(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		values = append(values, value)
	}
	return values, rows.Err()
}

func validateSkillIDs(ctx context.Context, query skillQuerier, values []string) ([]string, error) {
	if len(values) > agent.MaxAgentSkills {
		return nil, fmt.Errorf("an agent can attach at most %d skills", agent.MaxAgentSkills)
	}
	ids := make([]string, 0, len(values))
	seen := make(map[string]bool, len(values))
	totalBytes := 0
	for _, raw := range values {
		id := strings.TrimSpace(raw)
		if id == "" {
			return nil, errors.New("skill ID is required")
		}
		if seen[id] {
			return nil, fmt.Errorf("skill %q is attached more than once", id)
		}
		value, err := scanSkill(query.QueryRowContext(ctx, selectSkill+` WHERE skills.id = ?`, id))
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("skill %q was not found", id)
		}
		if err != nil {
			return nil, err
		}
		if value.ArchivedAt != nil {
			return nil, fmt.Errorf("skill %q is archived", value.Name)
		}
		totalBytes += len(value.Markdown)
		if totalBytes > agent.MaxCombinedSkillBytes {
			return nil, fmt.Errorf("attached skill Markdown exceeds %d bytes", agent.MaxCombinedSkillBytes)
		}
		seen[id] = true
		ids = append(ids, id)
	}
	return ids, nil
}

func replaceAgentSkills(ctx context.Context, tx *sql.Tx, agentID string, skillIDs []string) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM agent_skills WHERE agent_id = ?`, agentID); err != nil {
		return fmt.Errorf("clear agent skills: %w", err)
	}
	for position, skillID := range skillIDs {
		if _, err := tx.ExecContext(ctx, `INSERT INTO agent_skills (agent_id, skill_id, position)
			VALUES (?, ?, ?)`, agentID, skillID, position); err != nil {
			return fmt.Errorf("attach agent skill: %w", err)
		}
	}
	return nil
}

func validateUpdatedSkillSize(ctx context.Context, tx *sql.Tx, skillID string, markdownBytes int) error {
	rows, err := tx.QueryContext(ctx, `SELECT agent_id FROM agent_skills WHERE skill_id = ?`, skillID)
	if err != nil {
		return fmt.Errorf("find agents using skill: %w", err)
	}
	var agentIDs []string
	for rows.Next() {
		var agentID string
		if err := rows.Scan(&agentID); err != nil {
			rows.Close()
			return err
		}
		agentIDs = append(agentIDs, agentID)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, agentID := range agentIDs {
		var otherBytes int
		if err := tx.QueryRowContext(ctx, `SELECT COALESCE(SUM(LENGTH(CAST(skills.markdown AS BLOB))), 0)
			FROM agent_skills JOIN skills ON skills.id = agent_skills.skill_id
			WHERE agent_skills.agent_id = ? AND skills.id != ?`, agentID, skillID).Scan(&otherBytes); err != nil {
			return fmt.Errorf("measure attached skill Markdown: %w", err)
		}
		if otherBytes+markdownBytes > agent.MaxCombinedSkillBytes {
			return fmt.Errorf("skill update would exceed %d attached Markdown bytes for an agent", agent.MaxCombinedSkillBytes)
		}
	}
	return nil
}

func (s *Store) loadAgentSkillIDs(ctx context.Context, id string) ([]string, error) {
	values, err := s.AgentSkills(ctx, id)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(values))
	for _, value := range values {
		ids = append(ids, value.ID)
	}
	return ids, nil
}
