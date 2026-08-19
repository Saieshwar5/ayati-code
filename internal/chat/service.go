package chat

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/Saieshwar5/perpetual/internal/agent"
	"github.com/Saieshwar5/perpetual/internal/workspace"
)

type workspaceRuntime interface {
	Shell(context.Context, string) (agent.Shell, workspace.Workspace, error)
}

type Notifier interface {
	SessionChanged(workspaceID, sessionID, runID string)
}

type Service struct {
	store    *workspace.Store
	runtime  workspaceRuntime
	provider agent.Provider
	model    string
	notifier Notifier
	locksMu  sync.Mutex
	locks    map[string]*sync.Mutex
	runsMu   sync.Mutex
	runs     map[string]*activeRun
}

type activeRun struct {
	id        string
	sessionID string
	cancel    context.CancelFunc
	canceled  bool
}

type runResult struct {
	completion agent.Completion
	err        error
}

func New(
	store *workspace.Store, runtime workspaceRuntime, provider agent.Provider, model string,
	notifiers ...Notifier,
) (*Service, error) {
	if store == nil || runtime == nil || provider == nil || strings.TrimSpace(model) == "" {
		return nil, errors.New("chat store, workspace runtime, Fireworks client, and model are required")
	}
	service := &Service{
		store: store, runtime: runtime, provider: provider, model: strings.TrimSpace(model),
		locks: make(map[string]*sync.Mutex), runs: make(map[string]*activeRun),
	}
	if len(notifiers) > 0 {
		service.notifier = notifiers[0]
	}
	return service, nil
}

func (s *Service) Messages(
	ctx context.Context, workspaceID, sessionID string,
) ([]workspace.ConversationMessage, error) {
	if _, err := s.store.GetSession(ctx, workspaceID, sessionID); err != nil {
		return nil, fmt.Errorf("load session: %w", err)
	}
	return s.store.ConversationMessages(ctx, sessionID)
}

func (s *Service) Send(ctx context.Context, workspaceID, sessionID, text string) (agent.Completion, error) {
	_, result, err := s.start(ctx, workspaceID, sessionID, text)
	if err != nil {
		return agent.Completion{}, err
	}
	completed := <-result
	return completed.completion, completed.err
}

// Start accepts a durable run and executes it independently of the HTTP request.
func (s *Service) Start(
	ownerCtx context.Context, workspaceID, sessionID, text string,
) (workspace.AgentRun, error) {
	run, _, err := s.start(ownerCtx, workspaceID, sessionID, text)
	return run, err
}

func (s *Service) start(
	ownerCtx context.Context, workspaceID, sessionID, text string,
) (workspace.AgentRun, <-chan runResult, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return workspace.AgentRun{}, nil, errors.New("message is required")
	}
	if err := ownerCtx.Err(); err != nil {
		return workspace.AgentRun{}, nil, err
	}
	lock := s.lock(workspaceID)
	if !lock.TryLock() {
		return workspace.AgentRun{}, nil, workspace.ErrAgentRunActive
	}
	currentWorkspace, err := s.store.Get(ownerCtx, workspaceID)
	if err != nil {
		lock.Unlock()
		return workspace.AgentRun{}, nil, fmt.Errorf("load workspace: %w", err)
	}
	if currentWorkspace.Status != workspace.StatusReady {
		lock.Unlock()
		return workspace.AgentRun{}, nil, errors.New("workspace is not ready")
	}
	run, err := s.store.BeginAgentRun(ownerCtx, workspaceID, sessionID, text)
	if err != nil {
		lock.Unlock()
		return workspace.AgentRun{}, nil, err
	}
	runCtx, cancel := context.WithCancel(ownerCtx)
	active := s.setRun(workspaceID, run.ID, sessionID, cancel)
	result := make(chan runResult, 1)
	s.notify(workspaceID, sessionID, run.ID)
	go func() {
		defer lock.Unlock()
		var completed runResult
		defer func() {
			if recovered := recover(); recovered != nil {
				active.cancel()
				s.finishRun(run.WorkspaceID, active)
				message := "agent run stopped unexpectedly"
				_ = s.store.FinishAgentRun(context.Background(), run.ID,
					workspace.AgentRunStatusFailed, workspace.SessionStatusFailed, message)
				s.notify(run.WorkspaceID, run.SessionID, run.ID)
				completed.err = errors.New(message)
			}
			result <- completed
			close(result)
		}()
		completed.completion, completed.err = s.execute(runCtx, run, active)
	}()
	return run, result, nil
}

func (s *Service) Cancel(workspaceID string) {
	s.cancelRun(workspaceID, "", "")
}

func (s *Service) CancelSession(workspaceID, sessionID string) bool {
	return s.cancelRun(workspaceID, sessionID, "")
}

func (s *Service) CancelRun(workspaceID, sessionID, runID string) bool {
	return s.cancelRun(workspaceID, sessionID, runID)
}

func (s *Service) cancelRun(workspaceID, sessionID, runID string) bool {
	s.runsMu.Lock()
	run := s.runs[workspaceID]
	if run == nil || sessionID != "" && run.sessionID != sessionID || runID != "" && run.id != runID {
		s.runsMu.Unlock()
		return false
	}
	run.canceled = true
	s.runsMu.Unlock()
	run.cancel()
	return true
}

func (s *Service) CancelAndWait(workspaceID string) {
	s.Cancel(workspaceID)
	lock := s.lock(workspaceID)
	lock.Lock()
	lock.Unlock()
}

func (s *Service) WithWorkspaceIdle(workspaceID string, action func() error) error {
	if action == nil {
		return errors.New("workspace action is required")
	}
	lock := s.lock(workspaceID)
	if !lock.TryLock() {
		return errors.New("an agent is working in this workspace")
	}
	defer lock.Unlock()
	return action()
}

func (s *Service) setRun(workspaceID, runID, sessionID string, cancel context.CancelFunc) *activeRun {
	s.runsMu.Lock()
	defer s.runsMu.Unlock()
	run := &activeRun{id: runID, sessionID: sessionID, cancel: cancel}
	s.runs[workspaceID] = run
	return run
}

func (s *Service) notify(workspaceID, sessionID, runID string) {
	if s.notifier != nil {
		s.notifier.SessionChanged(workspaceID, sessionID, runID)
	}
}

func (s *Service) finishRun(workspaceID string, run *activeRun) bool {
	s.runsMu.Lock()
	defer s.runsMu.Unlock()
	if s.runs[workspaceID] == run {
		delete(s.runs, workspaceID)
	}
	return run.canceled
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
	ctx         context.Context
	store       *workspace.Store
	sessionID   string
	notifier    Notifier
	workspaceID string
	runID       string
}

func (r recorder) Append(message agent.Message) error {
	if err := r.store.AppendMessage(r.ctx, r.sessionID, message); err != nil {
		return fmt.Errorf("record conversation: %w", err)
	}
	if r.notifier != nil {
		r.notifier.SessionChanged(r.workspaceID, r.sessionID, r.runID)
	}
	return nil
}

type observer struct{ shellCalls int }

func (o *observer) Step(_, _ int)                {}
func (o *observer) ToolCall(agent.ShellRequest)  { o.shellCalls++ }
func (o *observer) ToolResult(agent.ShellResult) {}
func (o *observer) Assistant(string)             {}
