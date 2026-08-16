package chat

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/Saieshwar5/ayati-code/internal/agent"
	"github.com/Saieshwar5/ayati-code/internal/workspace"
)

type workspaceRuntime interface {
	Shell(context.Context, string) (agent.Shell, workspace.Workspace, error)
}

type Service struct {
	store    *workspace.Store
	runtime  workspaceRuntime
	provider agent.Provider
	model    string
	locksMu  sync.Mutex
	locks    map[string]*sync.Mutex
	runsMu   sync.Mutex
	runs     map[string]context.CancelFunc
}

func New(store *workspace.Store, runtime workspaceRuntime, provider agent.Provider, model string) (*Service, error) {
	if store == nil || runtime == nil || provider == nil || strings.TrimSpace(model) == "" {
		return nil, errors.New("chat store, workspace runtime, provider, and model are required")
	}
	return &Service{
		store: store, runtime: runtime, provider: provider, model: strings.TrimSpace(model),
		locks: make(map[string]*sync.Mutex), runs: make(map[string]context.CancelFunc),
	}, nil
}

func (s *Service) Messages(ctx context.Context, workspaceID, sessionID string) ([]agent.Message, error) {
	if _, err := s.store.GetSession(ctx, workspaceID, sessionID); err != nil {
		return nil, fmt.Errorf("load session: %w", err)
	}
	return s.store.Messages(ctx, sessionID)
}

func (s *Service) Send(ctx context.Context, workspaceID, sessionID, text string) (agent.Completion, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return agent.Completion{}, errors.New("message is required")
	}
	if _, err := s.store.GetSession(ctx, workspaceID, sessionID); err != nil {
		return agent.Completion{}, fmt.Errorf("load session: %w", err)
	}
	lock := s.lock(workspaceID)
	if !lock.TryLock() {
		return agent.Completion{}, errors.New("another session is already running in this workspace")
	}
	defer lock.Unlock()
	runCtx, cancel := context.WithCancel(ctx)
	s.setRun(workspaceID, cancel)
	defer func() {
		cancel()
		s.clearRun(workspaceID)
	}()
	shell, _, err := s.runtime.Shell(runCtx, workspaceID)
	if err != nil {
		return agent.Completion{}, err
	}
	history, err := s.store.Messages(runCtx, sessionID)
	if err != nil {
		return agent.Completion{}, err
	}
	if err := s.store.TitleSessionFromMessage(runCtx, sessionID, text); err != nil {
		return agent.Completion{}, fmt.Errorf("title session: %w", err)
	}
	if err := s.store.UpdateSessionStatus(runCtx, sessionID, workspace.SessionStatusWorking, ""); err != nil {
		return agent.Completion{}, err
	}
	observer := &observer{}
	loop := agent.Loop{
		Provider: s.provider, Shell: shell,
		Recorder: recorder{ctx: runCtx, store: s.store, sessionID: sessionID},
		Observer: observer, Model: s.model,
	}
	completion, err := loop.Run(runCtx, &history, text)
	if err != nil {
		message := err.Error()
		if errors.Is(err, context.Canceled) {
			message = "agent run canceled"
		}
		_ = s.store.UpdateSessionStatus(context.Background(), sessionID, workspace.SessionStatusFailed, message)
		return completion, err
	}
	status := workspace.SessionStatusIdle
	if observer.shellCalls > 0 {
		status = workspace.SessionStatusReview
	}
	if err := s.store.UpdateSessionStatus(runCtx, sessionID, status, ""); err != nil {
		return completion, err
	}
	return completion, nil
}

func (s *Service) Cancel(workspaceID string) {
	s.runsMu.Lock()
	cancel := s.runs[workspaceID]
	s.runsMu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func (s *Service) CancelAndWait(workspaceID string) {
	s.Cancel(workspaceID)
	lock := s.lock(workspaceID)
	lock.Lock()
	lock.Unlock()
}

func (s *Service) setRun(workspaceID string, cancel context.CancelFunc) {
	s.runsMu.Lock()
	defer s.runsMu.Unlock()
	s.runs[workspaceID] = cancel
}

func (s *Service) clearRun(workspaceID string) {
	s.runsMu.Lock()
	defer s.runsMu.Unlock()
	delete(s.runs, workspaceID)
}

func (s *Service) lock(id string) *sync.Mutex {
	s.locksMu.Lock()
	defer s.locksMu.Unlock()
	if s.locks[id] == nil {
		s.locks[id] = &sync.Mutex{}
	}
	return s.locks[id]
}

type recorder struct {
	ctx       context.Context
	store     *workspace.Store
	sessionID string
}

func (r recorder) Append(message agent.Message) error {
	if err := r.store.AppendMessage(r.ctx, r.sessionID, message); err != nil {
		return fmt.Errorf("record conversation: %w", err)
	}
	return nil
}

type observer struct{ shellCalls int }

func (o *observer) Step(_, _ int)                {}
func (o *observer) ToolCall(agent.ShellRequest)  { o.shellCalls++ }
func (o *observer) ToolResult(agent.ShellResult) {}
func (o *observer) Assistant(string)             {}
