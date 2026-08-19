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
	ID        int64     `json:"id"`
	CreatedAt time.Time `json:"created_at"`
}

func (s *Store) AppendMessage(ctx context.Context, sessionID string, message agent.Message) error {
	payload, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("encode session message: %w", err)
	}
	now := time.Now().UTC()
	_, err = s.db.ExecContext(ctx, `INSERT INTO messages (session_id, payload, created_at)
		VALUES (?, ?, ?)`, strings.TrimSpace(sessionID), string(payload), formatTime(now))
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
	rows, err := s.db.QueryContext(ctx, `SELECT id, payload, created_at FROM messages
		WHERE session_id = ? ORDER BY id`, strings.TrimSpace(sessionID))
	if err != nil {
		return nil, fmt.Errorf("load conversation messages: %w", err)
	}
	defer rows.Close()
	var messages []ConversationMessage
	for rows.Next() {
		var payload, createdAt string
		var id int64
		if err := rows.Scan(&id, &payload, &createdAt); err != nil {
			return nil, fmt.Errorf("scan conversation message: %w", err)
		}
		var message agent.Message
		if err := json.Unmarshal([]byte(payload), &message); err != nil {
			return nil, fmt.Errorf("decode conversation message: %w", err)
		}
		created, err := parseStoredTime(createdAt)
		if err != nil {
			return nil, err
		}
		messages = append(messages, ConversationMessage{ID: id, Message: message, CreatedAt: created})
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
