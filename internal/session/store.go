package session

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/sai-eshwar/no-nonsense-coding-ai/internal/chat"
)

type Header struct {
	Type      string    `json:"type"`
	ID        string    `json:"id"`
	CWD       string    `json:"cwd"`
	CreatedAt time.Time `json:"created_at"`
}

type Entry struct {
	Type      string        `json:"type"`
	Timestamp time.Time     `json:"timestamp"`
	Message   *chat.Message `json:"message,omitempty"`
	Summary   *Summary      `json:"summary,omitempty"`
}

type Summary struct {
	Content         string `json:"content"`
	CoveredMessages int    `json:"covered_messages"`
}

type Session struct {
	Header   Header
	Path     string
	Messages []chat.Message
	Summary  *Summary
}

type Info struct {
	ID        string
	CWD       string
	Path      string
	CreatedAt time.Time
	UpdatedAt time.Time
}

type Store struct {
	Dir string
}

func DefaultDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("find home directory: %w", err)
	}
	return filepath.Join(home, ".nca", "sessions"), nil
}

func (s Store) Create(cwd string) (*Session, error) {
	absCWD, err := filepath.Abs(cwd)
	if err != nil {
		return nil, fmt.Errorf("resolve working directory: %w", err)
	}
	if err := os.MkdirAll(s.Dir, 0o700); err != nil {
		return nil, fmt.Errorf("create session directory: %w", err)
	}
	id, err := newID()
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	header := Header{Type: "session", ID: id, CWD: absCWD, CreatedAt: now}
	path := filepath.Join(s.Dir, now.Format("20060102-150405")+"_"+id+".jsonl")
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return nil, fmt.Errorf("create session: %w", err)
	}
	defer file.Close()
	if err := writeJSONLine(file, header); err != nil {
		return nil, fmt.Errorf("write session header: %w", err)
	}
	if err := file.Sync(); err != nil {
		return nil, fmt.Errorf("sync session header: %w", err)
	}
	return &Session{Header: header, Path: path}, nil
}

func (s Store) Append(current *Session, message chat.Message) error {
	return s.appendEntry(current, Entry{Type: "message", Timestamp: time.Now().UTC(), Message: &message})
}

func (s Store) AppendSummary(current *Session, summary Summary) error {
	if summary.CoveredMessages < 0 || summary.CoveredMessages > len(current.Messages) {
		return fmt.Errorf("summary covers invalid message count %d", summary.CoveredMessages)
	}
	if strings.TrimSpace(summary.Content) == "" {
		return fmt.Errorf("summary content is empty")
	}
	if err := s.appendEntry(current, Entry{Type: "summary", Timestamp: time.Now().UTC(), Summary: &summary}); err != nil {
		return err
	}
	copy := summary
	current.Summary = &copy
	return nil
}

func (s Store) appendEntry(current *Session, entry Entry) error {
	file, err := os.OpenFile(current.Path, os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return fmt.Errorf("open session: %w", err)
	}
	defer file.Close()
	if err := writeJSONLine(file, entry); err != nil {
		return fmt.Errorf("append session: %w", err)
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("sync session: %w", err)
	}
	if entry.Type == "message" && entry.Message != nil {
		current.Messages = append(current.Messages, *entry.Message)
	}
	return nil
}

func (s Store) Open(idOrPath string) (*Session, error) {
	path := idOrPath
	if !strings.ContainsRune(idOrPath, os.PathSeparator) {
		infos, err := s.List()
		if err != nil {
			return nil, err
		}
		var matches []Info
		for _, info := range infos {
			if info.ID == idOrPath || strings.HasPrefix(info.ID, idOrPath) {
				matches = append(matches, info)
			}
		}
		if len(matches) == 0 {
			return nil, fmt.Errorf("session %q not found", idOrPath)
		}
		if len(matches) > 1 {
			return nil, fmt.Errorf("session prefix %q is ambiguous", idOrPath)
		}
		path = matches[0].Path
	}
	return load(path)
}

func (s Store) ContinueRecent(cwd string) (*Session, error) {
	absCWD, err := filepath.Abs(cwd)
	if err != nil {
		return nil, fmt.Errorf("resolve working directory: %w", err)
	}
	infos, err := s.List()
	if err != nil {
		return nil, err
	}
	for _, info := range infos {
		if info.CWD == absCWD {
			return load(info.Path)
		}
	}
	return s.Create(absCWD)
}

func (s Store) List() ([]Info, error) {
	entries, err := os.ReadDir(s.Dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read session directory: %w", err)
	}
	infos := make([]Info, 0, len(entries))
	for _, directoryEntry := range entries {
		if directoryEntry.IsDir() || filepath.Ext(directoryEntry.Name()) != ".jsonl" {
			continue
		}
		path := filepath.Join(s.Dir, directoryEntry.Name())
		header, err := readHeader(path)
		if err != nil {
			continue
		}
		fileInfo, err := directoryEntry.Info()
		if err != nil {
			continue
		}
		infos = append(infos, Info{ID: header.ID, CWD: header.CWD, Path: path, CreatedAt: header.CreatedAt, UpdatedAt: fileInfo.ModTime()})
	}
	sort.Slice(infos, func(i, j int) bool { return infos[i].UpdatedAt.After(infos[j].UpdatedAt) })
	return infos, nil
}

func load(path string) (*Session, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open session: %w", err)
	}
	defer file.Close()

	reader := bufio.NewReader(file)
	first, err := reader.ReadBytes('\n')
	if err != nil && err != io.EOF {
		return nil, fmt.Errorf("read session header: %w", err)
	}
	var header Header
	if err := json.Unmarshal(first, &header); err != nil || header.Type != "session" {
		return nil, fmt.Errorf("invalid session header in %s", path)
	}
	current := &Session{Header: header, Path: path}
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64<<10), 8<<20)
	for scanner.Scan() {
		var entry Entry
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			return nil, fmt.Errorf("decode session entry: %w", err)
		}
		if entry.Type == "message" && entry.Message != nil {
			current.Messages = append(current.Messages, *entry.Message)
		} else if entry.Type == "summary" && entry.Summary != nil {
			if entry.Summary.CoveredMessages < 0 || entry.Summary.CoveredMessages > len(current.Messages) {
				return nil, fmt.Errorf("invalid summary coverage in %s", path)
			}
			copy := *entry.Summary
			current.Summary = &copy
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read session entries: %w", err)
	}
	return current, nil
}

func readHeader(path string) (Header, error) {
	file, err := os.Open(path)
	if err != nil {
		return Header{}, err
	}
	defer file.Close()
	var header Header
	err = json.NewDecoder(file).Decode(&header)
	return header, err
}

func writeJSONLine(writer io.Writer, value any) error {
	encoded, err := json.Marshal(value)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(writer, "%s\n", encoded)
	return err
}

func newID() (string, error) {
	bytes := make([]byte, 6)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("generate session id: %w", err)
	}
	return hex.EncodeToString(bytes), nil
}
