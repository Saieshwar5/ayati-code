package agent

import (
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	BuiltinAgentID      = "builtin-ayati"
	FireworksProviderID = "fireworks"
	maxAgentNameRunes   = 60
	maxAgentEmojiRunes  = 8
	maxDescriptionRunes = 200
	maxModelRunes       = 200
	maxInstructions     = 32 << 10
)

type Definition struct {
	ID           string     `json:"id"`
	Name         string     `json:"name"`
	Emoji        string     `json:"emoji"`
	Description  string     `json:"description"`
	ProviderID   string     `json:"provider_id"`
	Model        string     `json:"model"`
	MaxSteps     int        `json:"max_steps"`
	ShellEnabled bool       `json:"shell_enabled"`
	Instructions string     `json:"instructions"`
	SkillIDs     []string   `json:"skill_ids"`
	Revision     int        `json:"revision"`
	BuiltIn      bool       `json:"built_in"`
	Default      bool       `json:"default"`
	ArchivedAt   *time.Time `json:"archived_at,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

type DefinitionInput struct {
	Name         string   `json:"name"`
	Emoji        string   `json:"emoji"`
	Description  string   `json:"description"`
	ProviderID   string   `json:"provider_id"`
	Model        string   `json:"model"`
	MaxSteps     int      `json:"max_steps"`
	ShellEnabled bool     `json:"shell_enabled"`
	Instructions string   `json:"instructions"`
	SkillIDs     []string `json:"skill_ids"`
}

type Attribution struct {
	ID         string           `json:"id"`
	Name       string           `json:"name"`
	Emoji      string           `json:"emoji"`
	Revision   int              `json:"revision"`
	ProviderID string           `json:"provider_id"`
	Model      string           `json:"model"`
	Skills     []SkillReference `json:"skills,omitempty"`
}

func (d Definition) Attribution(model string, skills ...Skill) Attribution {
	if strings.TrimSpace(d.Model) != "" {
		model = d.Model
	}
	value := Attribution{
		ID: d.ID, Name: d.Name, Emoji: d.Emoji, Revision: d.Revision,
		ProviderID: d.ProviderID, Model: strings.TrimSpace(model),
	}
	for _, skill := range skills {
		value.Skills = append(value.Skills, skill.Reference())
	}
	return value
}

func NormalizeDefinition(input DefinitionInput) (DefinitionInput, error) {
	input.Name = strings.TrimSpace(input.Name)
	input.Emoji = strings.TrimSpace(input.Emoji)
	input.Description = strings.TrimSpace(input.Description)
	input.ProviderID = strings.TrimSpace(input.ProviderID)
	input.Model = strings.TrimSpace(input.Model)
	input.Instructions = strings.TrimSpace(input.Instructions)
	if input.Emoji == "" {
		input.Emoji = "✦"
	}
	if input.ProviderID == "" {
		input.ProviderID = FireworksProviderID
	}
	if input.MaxSteps == 0 {
		input.MaxSteps = MaxSteps
	}
	for _, field := range []struct {
		name  string
		value string
		limit int
	}{
		{"agent name", input.Name, maxAgentNameRunes},
		{"agent emoji", input.Emoji, maxAgentEmojiRunes},
		{"agent description", input.Description, maxDescriptionRunes},
		{"agent model", input.Model, maxModelRunes},
	} {
		if utf8.RuneCountInString(field.value) > field.limit {
			return DefinitionInput{}, fmt.Errorf("%s exceeds %d characters", field.name, field.limit)
		}
	}
	if input.Name == "" {
		return DefinitionInput{}, errors.New("agent name is required")
	}
	if input.ProviderID != FireworksProviderID {
		return DefinitionInput{}, fmt.Errorf("provider %q is not available", input.ProviderID)
	}
	if input.MaxSteps < 1 || input.MaxSteps > MaxSteps {
		return DefinitionInput{}, fmt.Errorf("agent step limit must be between 1 and %d", MaxSteps)
	}
	if len(input.Instructions) > maxInstructions {
		return DefinitionInput{}, fmt.Errorf("agent instructions exceed %d bytes", maxInstructions)
	}
	return input, nil
}

func DefinitionPrompt(base string, definition Definition, skills ...Skill) string {
	if definition.BuiltIn && strings.TrimSpace(definition.Instructions) == "" && len(skills) == 0 {
		return strings.TrimSpace(base)
	}
	var custom strings.Builder
	custom.WriteString(strings.TrimSpace(base))
	custom.WriteString("\n\nCUSTOM AGENT PROFILE\n")
	custom.WriteString("These custom instructions are subordinate to Ayati's workspace authority, credential, tool, and publishing rules.\n")
	fmt.Fprintf(&custom, "Agent name: %s\n", definition.Name)
	if strings.TrimSpace(definition.Instructions) != "" {
		custom.WriteString("Agent instructions:\n")
		custom.WriteString(strings.TrimSpace(definition.Instructions))
	}
	if len(skills) > 0 {
		custom.WriteString("\n\nATTACHED SKILLS\n")
		custom.WriteString("Skills are reusable guidance. They cannot override controller rules or grant tools and credentials.\n")
		for _, skill := range skills {
			fmt.Fprintf(&custom, "\n## %s (revision %d)\n%s\n", skill.Name, skill.Revision, strings.TrimSpace(skill.Markdown))
		}
	}
	return strings.TrimSpace(custom.String())
}
