package workspace

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Saieshwar5/ayati-code/internal/agent"
)

const selectSkill = `SELECT skills.id, skills.name, skills.description, skills.markdown,
	skills.revision, skills.archived_at, skills.created_at, skills.updated_at,
	(SELECT COUNT(*) FROM agent_skills WHERE agent_skills.skill_id = skills.id)
	FROM skills`

func (s *Store) CreateSkill(ctx context.Context, raw agent.SkillInput) (agent.Skill, error) {
	input, err := agent.NormalizeSkill(raw)
	if err != nil {
		return agent.Skill{}, err
	}
	id, err := newID()
	if err != nil {
		return agent.Skill{}, err
	}
	now := formatTime(time.Now().UTC())
	_, err = s.db.ExecContext(ctx, `INSERT INTO skills (
		id, name, description, markdown, revision, archived_at, created_at, updated_at
	) VALUES (?, ?, ?, ?, 1, '', ?, ?)`, id, input.Name, input.Description, input.Markdown, now, now)
	if err != nil {
		return agent.Skill{}, fmt.Errorf("create skill: %w", err)
	}
	return s.GetSkill(ctx, id)
}

func (s *Store) ListSkills(ctx context.Context, archived bool) ([]agent.Skill, error) {
	operator := "="
	if archived {
		operator = "!="
	}
	rows, err := s.db.QueryContext(ctx, selectSkill+` WHERE skills.archived_at `+operator+` ''
		ORDER BY skills.updated_at DESC, skills.name`)
	if err != nil {
		return nil, fmt.Errorf("list skills: %w", err)
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

func (s *Store) GetSkill(ctx context.Context, id string) (agent.Skill, error) {
	return scanSkill(s.db.QueryRowContext(ctx, selectSkill+` WHERE skills.id = ?`, strings.TrimSpace(id)))
}

func (s *Store) UpdateSkill(ctx context.Context, id string, raw agent.SkillInput) (agent.Skill, error) {
	input, err := agent.NormalizeSkill(raw)
	if err != nil {
		return agent.Skill{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return agent.Skill{}, fmt.Errorf("begin skill update: %w", err)
	}
	defer tx.Rollback()
	current, err := scanSkill(tx.QueryRowContext(ctx, selectSkill+` WHERE skills.id = ?`, strings.TrimSpace(id)))
	if err != nil {
		return agent.Skill{}, err
	}
	if current.ArchivedAt != nil {
		return agent.Skill{}, errors.New("restore the skill before editing it")
	}
	if err := validateUpdatedSkillSize(ctx, tx, current.ID, len(input.Markdown)); err != nil {
		return agent.Skill{}, err
	}
	result, err := tx.ExecContext(ctx, `UPDATE skills SET name = ?, description = ?, markdown = ?,
		revision = revision + 1, updated_at = ? WHERE id = ?`, input.Name, input.Description,
		input.Markdown, formatTime(time.Now().UTC()), current.ID)
	if err != nil {
		return agent.Skill{}, fmt.Errorf("update skill: %w", err)
	}
	if err := requireOneRow(result); err != nil {
		return agent.Skill{}, err
	}
	if err := tx.Commit(); err != nil {
		return agent.Skill{}, fmt.Errorf("commit skill update: %w", err)
	}
	return s.GetSkill(ctx, current.ID)
}

func (s *Store) ArchiveSkill(ctx context.Context, id string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin skill archive: %w", err)
	}
	defer tx.Rollback()
	value, err := scanSkill(tx.QueryRowContext(ctx, selectSkill+` WHERE skills.id = ?`, strings.TrimSpace(id)))
	if err != nil {
		return err
	}
	if value.ArchivedAt != nil {
		return nil
	}
	if value.AttachedAgents > 0 {
		return fmt.Errorf("detach this skill from %d agent(s) before archiving it", value.AttachedAgents)
	}
	now := formatTime(time.Now().UTC())
	result, err := tx.ExecContext(ctx,
		`UPDATE skills SET archived_at = ?, updated_at = ? WHERE id = ?`, now, now, value.ID)
	if err != nil {
		return fmt.Errorf("archive skill: %w", err)
	}
	if err := requireOneRow(result); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) RestoreSkill(ctx context.Context, id string) (agent.Skill, error) {
	result, err := s.db.ExecContext(ctx, `UPDATE skills SET archived_at = '', updated_at = ? WHERE id = ?`,
		formatTime(time.Now().UTC()), strings.TrimSpace(id))
	if err != nil {
		return agent.Skill{}, fmt.Errorf("restore skill: %w", err)
	}
	if err := requireOneRow(result); err != nil {
		return agent.Skill{}, err
	}
	return s.GetSkill(ctx, id)
}

func scanSkill(row scanner) (agent.Skill, error) {
	var value agent.Skill
	var archivedAt, createdAt, updatedAt string
	if err := row.Scan(&value.ID, &value.Name, &value.Description, &value.Markdown,
		&value.Revision, &archivedAt, &createdAt, &updatedAt, &value.AttachedAgents); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return agent.Skill{}, err
		}
		return agent.Skill{}, fmt.Errorf("scan skill: %w", err)
	}
	var err error
	if value.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt); err != nil {
		return agent.Skill{}, fmt.Errorf("decode skill creation time: %w", err)
	}
	if value.UpdatedAt, err = time.Parse(time.RFC3339Nano, updatedAt); err != nil {
		return agent.Skill{}, fmt.Errorf("decode skill update time: %w", err)
	}
	if archivedAt != "" {
		archived, parseErr := time.Parse(time.RFC3339Nano, archivedAt)
		if parseErr != nil {
			return agent.Skill{}, fmt.Errorf("decode skill archive time: %w", parseErr)
		}
		value.ArchivedAt = &archived
	}
	return value, nil
}
