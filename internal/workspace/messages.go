package workspace

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Saieshwar5/perpetual/internal/agent"
)

type ConversationMessage struct {
	agent.Message
	Agent *agent.Attribution `json:"agent,omitempty"`
}

func (s *Store) AppendMessage(ctx context.Context, sessionID string, message agent.Message) error {
	return s.AppendAttributedMessage(ctx, sessionID, message, nil)
}

func (s *Store) AppendAttributedMessage(
	ctx context.Context, sessionID string, message agent.Message, attribution *agent.Attribution,
) error {
	payload, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("encode session message: %w", err)
	}
	now := time.Now().UTC()
	var agentID, name, emoji, providerID, model string
	skills := "[]"
	var revision int
	if attribution != nil && message.Role == "assistant" {
		agentID, name, emoji = attribution.ID, attribution.Name, attribution.Emoji
		revision, providerID, model = attribution.Revision, attribution.ProviderID, attribution.Model
		if len(attribution.Skills) > 0 {
			encoded, encodeErr := json.Marshal(attribution.Skills)
			if encodeErr != nil {
				return fmt.Errorf("encode agent skill attribution: %w", encodeErr)
			}
			skills = string(encoded)
		}
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO messages (
		session_id, payload, agent_id, agent_name, agent_emoji, agent_revision,
		agent_provider_id, agent_model, agent_skills, created_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, strings.TrimSpace(sessionID), string(payload),
		agentID, name, emoji, revision, providerID, model, skills, formatTime(now))
	if err != nil {
		return fmt.Errorf("append session message: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE sessions SET updated_at = ? WHERE id = ?`,
		formatTime(now), strings.TrimSpace(sessionID)); err != nil {
		return fmt.Errorf("touch session: %w", err)
	}
	return s.touchWorkspaceForSession(ctx, sessionID, now)
}

func (s *Store) ConversationMessages(
	ctx context.Context, sessionID string,
) ([]ConversationMessage, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT payload, agent_id, agent_name, agent_emoji,
		agent_revision, agent_provider_id, agent_model, agent_skills FROM messages
		WHERE session_id = ? ORDER BY id`, strings.TrimSpace(sessionID))
	if err != nil {
		return nil, fmt.Errorf("load conversation messages: %w", err)
	}
	defer rows.Close()
	var messages []ConversationMessage
	for rows.Next() {
		var payload, agentID, name, emoji, providerID, model, skills string
		var revision int
		if err := rows.Scan(&payload, &agentID, &name, &emoji, &revision, &providerID, &model, &skills); err != nil {
			return nil, fmt.Errorf("scan conversation message: %w", err)
		}
		var message agent.Message
		if err := json.Unmarshal([]byte(payload), &message); err != nil {
			return nil, fmt.Errorf("decode conversation message: %w", err)
		}
		value := ConversationMessage{Message: message}
		if message.Role == "assistant" {
			if agentID == "" {
				agentID, name, emoji, revision, providerID = agent.BuiltinAgentID, "Perpetual", "✦", 1, agent.FireworksProviderID
			}
			value.Agent = &agent.Attribution{
				ID: agentID, Name: name, Emoji: emoji, Revision: revision,
				ProviderID: providerID, Model: model,
			}
			if err := json.Unmarshal([]byte(skills), &value.Agent.Skills); err != nil {
				return nil, fmt.Errorf("decode agent skill attribution: %w", err)
			}
		}
		messages = append(messages, value)
	}
	return messages, rows.Err()
}

func (s *Store) Messages(ctx context.Context, sessionID string) ([]agent.Message, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT payload FROM messages WHERE session_id = ? ORDER BY id`, strings.TrimSpace(sessionID))
	if err != nil {
		return nil, fmt.Errorf("load session messages: %w", err)
	}
	defer rows.Close()
	var messages []agent.Message
	for rows.Next() {
		var payload string
		if err := rows.Scan(&payload); err != nil {
			return nil, fmt.Errorf("scan session message: %w", err)
		}
		var message agent.Message
		if err := json.Unmarshal([]byte(payload), &message); err != nil {
			return nil, fmt.Errorf("decode session message: %w", err)
		}
		messages = append(messages, message)
	}
	return messages, rows.Err()
}

func newID() (string, error) {
	var data [8]byte
	if _, err := rand.Read(data[:]); err != nil {
		return "", fmt.Errorf("create workspace id: %w", err)
	}
	return hex.EncodeToString(data[:]), nil
}
