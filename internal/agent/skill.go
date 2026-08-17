package agent

import (
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	MaxAgentSkills        = 12
	MaxCombinedSkillBytes = 64 << 10
	maxSkillNameRunes     = 80
	maxSkillDescription   = 240
	maxSkillMarkdown      = 32 << 10
)

type Skill struct {
	ID             string     `json:"id"`
	Name           string     `json:"name"`
	Description    string     `json:"description"`
	Markdown       string     `json:"markdown"`
	Revision       int        `json:"revision"`
	AttachedAgents int        `json:"attached_agents"`
	ArchivedAt     *time.Time `json:"archived_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

type SkillInput struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Markdown    string `json:"markdown"`
}

type SkillReference struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Revision int    `json:"revision"`
}

func (s Skill) Reference() SkillReference {
	return SkillReference{ID: s.ID, Name: s.Name, Revision: s.Revision}
}

func NormalizeSkill(input SkillInput) (SkillInput, error) {
	input.Name = strings.TrimSpace(input.Name)
	input.Description = strings.TrimSpace(input.Description)
	input.Markdown = strings.TrimSpace(input.Markdown)
	if input.Name == "" {
		return SkillInput{}, errors.New("skill name is required")
	}
	if utf8.RuneCountInString(input.Name) > maxSkillNameRunes {
		return SkillInput{}, fmt.Errorf("skill name exceeds %d characters", maxSkillNameRunes)
	}
	if utf8.RuneCountInString(input.Description) > maxSkillDescription {
		return SkillInput{}, fmt.Errorf("skill description exceeds %d characters", maxSkillDescription)
	}
	if input.Markdown == "" {
		return SkillInput{}, errors.New("skill Markdown is required")
	}
	if len(input.Markdown) > maxSkillMarkdown {
		return SkillInput{}, fmt.Errorf("skill Markdown exceeds %d bytes", maxSkillMarkdown)
	}
	return input, nil
}
