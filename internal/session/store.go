package session

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Saieshwar5/ayati-code/internal/agent"
)

const maxRecordBytes = 1 << 20

type Info struct {
	ID        string    `json:"id"`
	Workspace string    `json:"workspace"`
	Model     string    `json:"model"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"-"`
}

type record struct {
	Type    string         `json:"type"`
	Session *Info          `json:"session,omitempty"`
	Message *agent.Message `json:"message,omitempty"`
}

type Store struct{ root string }

type Session struct {
	Info     Info
	Messages []agent.Message
	path     string
	mu       sync.Mutex
}

func DefaultRoot() (string, error) {
	if root := strings.TrimSpace(os.Getenv("XDG_STATE_HOME")); root != "" {
		return filepath.Join(root, "ayati", "sessions"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	return filepath.Join(home, ".local", "state", "ayati", "sessions"), nil
}

func Open(root string) (*Store, error) {
	if strings.TrimSpace(root) == "" {
		return nil, fmt.Errorf("session directory is required")
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, fmt.Errorf("create session directory: %w", err)
	}
	if err := os.Chmod(root, 0o700); err != nil {
		return nil, fmt.Errorf("secure session directory: %w", err)
	}
	return &Store{root: root}, nil
}

func (s *Store) New(workspace, model string) (*Session, error) {
	if strings.TrimSpace(workspace) == "" || strings.TrimSpace(model) == "" {
		return nil, fmt.Errorf("workspace and model are required")
	}
	id, err := newID()
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	info := Info{ID: id, Workspace: workspace, Model: model, CreatedAt: now, UpdatedAt: now}
	path := filepath.Join(s.root, id+".jsonl")
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return nil, fmt.Errorf("create session: %w", err)
	}
	if err := json.NewEncoder(file).Encode(record{Type: "session", Session: &info}); err != nil {
		file.Close()
		return nil, fmt.Errorf("write session metadata: %w", err)
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return nil, fmt.Errorf("sync session metadata: %w", err)
	}
	if err := file.Close(); err != nil {
		return nil, fmt.Errorf("close session: %w", err)
	}
	return &Session{Info: info, path: path}, nil
}

func (s *Store) Load(reference string) (*Session, error) {
	path, err := s.resolve(reference)
	if err != nil {
		return nil, err
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open session: %w", err)
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64<<10), maxRecordBytes)
	var info Info
	var messages []agent.Message
	line := 0
	for scanner.Scan() {
		line++
		var item record
		if err := json.Unmarshal(scanner.Bytes(), &item); err != nil {
			return nil, fmt.Errorf("decode session line %d: %w", line, err)
		}
		if line == 1 {
			if item.Type != "session" || item.Session == nil {
				return nil, fmt.Errorf("session metadata is missing")
			}
			info = *item.Session
			continue
		}
		if item.Type != "message" || item.Message == nil {
			return nil, fmt.Errorf("invalid session record on line %d", line)
		}
		messages = append(messages, *item.Message)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read session: %w", err)
	}
	if info.ID == "" {
		return nil, fmt.Errorf("session is empty")
	}
	if stat, err := os.Stat(path); err == nil {
		info.UpdatedAt = stat.ModTime()
	}
	return &Session{Info: info, Messages: messages, path: path}, nil
}

func (s *Store) List(workspace string) ([]Info, error) {
	entries, err := os.ReadDir(s.root)
	if err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}
	infos := make([]Info, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".jsonl" {
			continue
		}
		info, err := readInfo(filepath.Join(s.root, entry.Name()))
		if err != nil {
			return nil, err
		}
		if info.Workspace == workspace {
			infos = append(infos, info)
		}
	}
	sort.Slice(infos, func(i, j int) bool { return infos[i].UpdatedAt.After(infos[j].UpdatedAt) })
	return infos, nil
}

func (s *Session) Append(message agent.Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	file, err := os.OpenFile(s.path, os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return fmt.Errorf("open session for append: %w", err)
	}
	if err := json.NewEncoder(file).Encode(record{Type: "message", Message: &message}); err != nil {
		file.Close()
		return fmt.Errorf("append session message: %w", err)
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return fmt.Errorf("sync session message: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close session message: %w", err)
	}
	s.Info.UpdatedAt = time.Now()
	return nil
}

func (s *Store) resolve(reference string) (string, error) {
	reference = strings.TrimSpace(strings.TrimSuffix(reference, ".jsonl"))
	if reference == "" {
		return "", fmt.Errorf("session id is required")
	}
	exact := filepath.Join(s.root, reference+".jsonl")
	if info, err := os.Stat(exact); err == nil && info.Mode().IsRegular() {
		return exact, nil
	}
	entries, err := os.ReadDir(s.root)
	if err != nil {
		return "", fmt.Errorf("list sessions: %w", err)
	}
	var matches []string
	for _, entry := range entries {
		name := strings.TrimSuffix(entry.Name(), ".jsonl")
		if !entry.IsDir() && strings.HasPrefix(name, reference) {
			matches = append(matches, filepath.Join(s.root, entry.Name()))
		}
	}
	if len(matches) == 0 {
		return "", fmt.Errorf("session %q not found", reference)
	}
	if len(matches) > 1 {
		return "", fmt.Errorf("session prefix %q is ambiguous", reference)
	}
	return matches[0], nil
}

func readInfo(path string) (Info, error) {
	file, err := os.Open(path)
	if err != nil {
		return Info{}, fmt.Errorf("open session metadata: %w", err)
	}
	defer file.Close()
	var item record
	if err := json.NewDecoder(file).Decode(&item); err != nil || item.Type != "session" || item.Session == nil {
		return Info{}, fmt.Errorf("decode session metadata %s", filepath.Base(path))
	}
	info := *item.Session
	if stat, err := file.Stat(); err == nil {
		info.UpdatedAt = stat.ModTime()
	}
	return info, nil
}

func newID() (string, error) {
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		return "", fmt.Errorf("generate session id: %w", err)
	}
	return hex.EncodeToString(value), nil
}
