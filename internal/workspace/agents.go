package workspace

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Saieshwar5/ayati-code/internal/agent"
)

const selectAgent = `SELECT agents.id, agents.name, agents.emoji, agents.description,
	agents.provider_id, agents.model, agents.max_steps, agents.shell_enabled,
	agents.instructions, agents.revision, agents.built_in, agents.archived_at,
	agents.created_at, agents.updated_at,
	CASE WHEN agents.id = application_settings.default_agent_id THEN 1 ELSE 0 END
	FROM agents CROSS JOIN application_settings WHERE application_settings.id = 1`

func (s *Store) CreateAgent(ctx context.Context, raw agent.DefinitionInput) (agent.Definition, error) {
	input, err := agent.NormalizeDefinition(raw)
	if err != nil {
		return agent.Definition{}, err
	}
	id, err := newID()
	if err != nil {
		return agent.Definition{}, err
	}
	now := time.Now().UTC()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return agent.Definition{}, fmt.Errorf("begin agent creation: %w", err)
	}
	defer tx.Rollback()
	skillIDs, err := validateSkillIDs(ctx, tx, input.SkillIDs)
	if err != nil {
		return agent.Definition{}, err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO agents (
		id, name, emoji, description, provider_id, model, max_steps, shell_enabled,
		instructions, revision, built_in, archived_at, created_at, updated_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 1, 0, '', ?, ?)`,
		id, input.Name, input.Emoji, input.Description, input.ProviderID, input.Model,
		input.MaxSteps, input.ShellEnabled, input.Instructions, formatTime(now), formatTime(now))
	if err != nil {
		return agent.Definition{}, fmt.Errorf("create agent: %w", err)
	}
	if err := replaceAgentSkills(ctx, tx, id, skillIDs); err != nil {
		return agent.Definition{}, err
	}
	if err := tx.Commit(); err != nil {
		return agent.Definition{}, fmt.Errorf("commit agent creation: %w", err)
	}
	return s.GetAgent(ctx, id)
}

func (s *Store) ListAgents(ctx context.Context, archived bool) ([]agent.Definition, error) {
	condition := "agents.archived_at = ''"
	if archived {
		condition = "agents.archived_at != ''"
	}
	rows, err := s.db.QueryContext(ctx, selectAgent+` AND `+condition+`
		ORDER BY CASE WHEN agents.id = application_settings.default_agent_id THEN 0 ELSE 1 END,
		agents.built_in DESC, agents.updated_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("list agents: %w", err)
	}
	var values []agent.Definition
	for rows.Next() {
		value, scanErr := scanAgent(rows)
		if scanErr != nil {
			rows.Close()
			return nil, scanErr
		}
		values = append(values, value)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	for index := range values {
		values[index].SkillIDs, err = s.loadAgentSkillIDs(ctx, values[index].ID)
		if err != nil {
			return nil, err
		}
	}
	return values, nil
}

func (s *Store) GetAgent(ctx context.Context, id string) (agent.Definition, error) {
	value, err := scanAgent(s.db.QueryRowContext(ctx, selectAgent+` AND agents.id = ?`, strings.TrimSpace(id)))
	if err != nil {
		return agent.Definition{}, err
	}
	value.SkillIDs, err = s.loadAgentSkillIDs(ctx, value.ID)
	return value, err
}

func (s *Store) DefaultAgent(ctx context.Context) (agent.Definition, error) {
	return scanAgent(s.db.QueryRowContext(ctx,
		selectAgent+` AND agents.id = application_settings.default_agent_id`))
}

func (s *Store) UpdateAgent(
	ctx context.Context, id string, raw agent.DefinitionInput,
) (agent.Definition, error) {
	current, err := s.GetAgent(ctx, id)
	if err != nil {
		return agent.Definition{}, err
	}
	if current.BuiltIn {
		return agent.Definition{}, errors.New("the built-in Ayati agent cannot be edited")
	}
	if current.ArchivedAt != nil {
		return agent.Definition{}, errors.New("restore the agent before editing it")
	}
	input, err := agent.NormalizeDefinition(raw)
	if err != nil {
		return agent.Definition{}, err
	}
	now := time.Now().UTC()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return agent.Definition{}, fmt.Errorf("begin agent update: %w", err)
	}
	defer tx.Rollback()
	skillIDs, err := validateSkillIDs(ctx, tx, input.SkillIDs)
	if err != nil {
		return agent.Definition{}, err
	}
	result, err := tx.ExecContext(ctx, `UPDATE agents SET name = ?, emoji = ?, description = ?,
		provider_id = ?, model = ?, max_steps = ?, shell_enabled = ?, instructions = ?,
		revision = revision + 1, updated_at = ? WHERE id = ?`,
		input.Name, input.Emoji, input.Description, input.ProviderID, input.Model,
		input.MaxSteps, input.ShellEnabled, input.Instructions, formatTime(now), current.ID)
	if err != nil {
		return agent.Definition{}, fmt.Errorf("update agent: %w", err)
	}
	if err := requireOneRow(result); err != nil {
		return agent.Definition{}, err
	}
	if err := replaceAgentSkills(ctx, tx, current.ID, skillIDs); err != nil {
		return agent.Definition{}, err
	}
	if err := tx.Commit(); err != nil {
		return agent.Definition{}, fmt.Errorf("commit agent update: %w", err)
	}
	return s.GetAgent(ctx, current.ID)
}

func (s *Store) SetDefaultAgent(ctx context.Context, id string) (agent.Definition, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return agent.Definition{}, fmt.Errorf("begin default agent change: %w", err)
	}
	defer tx.Rollback()
	value, err := scanAgent(tx.QueryRowContext(ctx,
		selectAgent+` AND agents.id = ?`, strings.TrimSpace(id)))
	if err != nil {
		return agent.Definition{}, err
	}
	if value.ArchivedAt != nil {
		return agent.Definition{}, errors.New("an archived agent cannot be the default")
	}
	result, err := tx.ExecContext(ctx,
		`UPDATE application_settings SET default_agent_id = ? WHERE id = 1`, value.ID)
	if err != nil {
		return agent.Definition{}, fmt.Errorf("set default agent: %w", err)
	}
	if err := requireOneRow(result); err != nil {
		return agent.Definition{}, err
	}
	if err := tx.Commit(); err != nil {
		return agent.Definition{}, fmt.Errorf("commit default agent change: %w", err)
	}
	return s.GetAgent(ctx, value.ID)
}

func (s *Store) DuplicateAgent(ctx context.Context, id string) (agent.Definition, error) {
	value, err := s.GetAgent(ctx, id)
	if err != nil {
		return agent.Definition{}, err
	}
	return s.CreateAgent(ctx, agent.DefinitionInput{
		Name: value.Name + " copy", Emoji: value.Emoji, Description: value.Description,
		ProviderID: value.ProviderID, Model: value.Model, MaxSteps: value.MaxSteps,
		ShellEnabled: value.ShellEnabled, Instructions: value.Instructions, SkillIDs: value.SkillIDs,
	})
}

func (s *Store) ArchiveAgent(ctx context.Context, id string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin agent archive: %w", err)
	}
	defer tx.Rollback()
	value, err := scanAgent(tx.QueryRowContext(ctx,
		selectAgent+` AND agents.id = ?`, strings.TrimSpace(id)))
	if err != nil {
		return err
	}
	if value.BuiltIn {
		return errors.New("the built-in Ayati agent cannot be archived")
	}
	if value.Default {
		return errors.New("choose another default agent before archiving this one")
	}
	if value.ArchivedAt != nil {
		return nil
	}
	defaultAgent, err := scanAgent(tx.QueryRowContext(ctx,
		selectAgent+` AND agents.id = application_settings.default_agent_id`))
	if err != nil {
		return err
	}
	now := formatTime(time.Now().UTC())
	if _, err := tx.ExecContext(ctx, `UPDATE sessions SET selected_agent_id = ?
		WHERE selected_agent_id = ?`, defaultAgent.ID, value.ID); err != nil {
		return fmt.Errorf("reassign archived agent sessions: %w", err)
	}
	result, err := tx.ExecContext(ctx,
		`UPDATE agents SET archived_at = ?, updated_at = ? WHERE id = ?`, now, now, value.ID)
	if err != nil {
		return fmt.Errorf("archive agent: %w", err)
	}
	if err := requireOneRow(result); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) RestoreAgent(ctx context.Context, id string) (agent.Definition, error) {
	result, err := s.db.ExecContext(ctx, `UPDATE agents SET archived_at = '', updated_at = ? WHERE id = ?`,
		formatTime(time.Now().UTC()), strings.TrimSpace(id))
	if err != nil {
		return agent.Definition{}, fmt.Errorf("restore agent: %w", err)
	}
	if err := requireOneRow(result); err != nil {
		return agent.Definition{}, err
	}
	return s.GetAgent(ctx, id)
}

func scanAgent(row scanner) (agent.Definition, error) {
	var value agent.Definition
	var archivedAt, createdAt, updatedAt string
	if err := row.Scan(&value.ID, &value.Name, &value.Emoji, &value.Description,
		&value.ProviderID, &value.Model, &value.MaxSteps, &value.ShellEnabled,
		&value.Instructions, &value.Revision, &value.BuiltIn, &archivedAt,
		&createdAt, &updatedAt, &value.Default); err != nil {
		return agent.Definition{}, err
	}
	var err error
	if value.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt); err != nil {
		return agent.Definition{}, fmt.Errorf("decode agent creation time: %w", err)
	}
	if value.UpdatedAt, err = time.Parse(time.RFC3339Nano, updatedAt); err != nil {
		return agent.Definition{}, fmt.Errorf("decode agent update time: %w", err)
	}
	if archivedAt != "" {
		archived, parseErr := time.Parse(time.RFC3339Nano, archivedAt)
		if parseErr != nil {
			return agent.Definition{}, fmt.Errorf("decode agent archive time: %w", parseErr)
		}
		value.ArchivedAt = &archived
	}
	return value, nil
}
