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

type providerResolver interface {
	Resolve(string) (agent.Provider, string, error)
}

type Service struct {
	store     *workspace.Store
	runtime   workspaceRuntime
	providers providerResolver
	locksMu   sync.Mutex
	locks     map[string]*sync.Mutex
	runsMu    sync.Mutex
	runs      map[string]context.CancelFunc
}

func New(store *workspace.Store, runtime workspaceRuntime, providers providerResolver) (*Service, error) {
	if store == nil || runtime == nil || providers == nil {
		return nil, errors.New("chat store, workspace runtime, and provider registry are required")
	}
	return &Service{
		store: store, runtime: runtime, providers: providers,
		locks: make(map[string]*sync.Mutex), runs: make(map[string]context.CancelFunc),
	}, nil
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
	text = strings.TrimSpace(text)
	if text == "" {
		return agent.Completion{}, errors.New("message is required")
	}
	lock := s.lock(workspaceID)
	if !lock.TryLock() {
		return agent.Completion{}, errors.New("another session is already running in this workspace")
	}
	defer lock.Unlock()
	session, err := s.store.GetSession(ctx, workspaceID, sessionID)
	if err != nil {
		return agent.Completion{}, fmt.Errorf("load session: %w", err)
	}
	definition, err := s.store.GetAgent(ctx, session.SelectedAgentID)
	if err != nil {
		return agent.Completion{}, fmt.Errorf("load selected agent: %w", err)
	}
	if definition.ArchivedAt != nil {
		return agent.Completion{}, errors.New("the selected agent is archived; choose another agent")
	}
	selectedProvider, defaultModel, err := s.providers.Resolve(definition.ProviderID)
	if err != nil {
		return agent.Completion{}, err
	}
	runCtx, cancel := context.WithCancel(ctx)
	s.setRun(workspaceID, cancel)
	defer func() {
		cancel()
		s.clearRun(workspaceID)
	}()
	shell, currentWorkspace, err := s.runtime.Shell(runCtx, workspaceID)
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
	workspaceContext := agent.WorkspaceContext{
		Repository: currentWorkspace.Repository,
		Branch:     currentWorkspace.Branch, Authority: string(currentWorkspace.Authority),
	}
	if profile := currentWorkspace.Profile; profile != nil {
		workspaceContext.ProjectRoot = profile.ProjectRoot
		workspaceContext.Languages = profile.Languages
		workspaceContext.RuntimeVersions = profile.RuntimeVersions
		workspaceContext.PackageManagers = profile.PackageManagers
		workspaceContext.SetupResult = profile.SetupResult
		workspaceContext.BaselineCommit = profile.BaselineCommit
		workspaceContext.TestCommand = profile.TestCommand
		workspaceContext.LintCommand = profile.LintCommand
		workspaceContext.TypecheckCommand = profile.TypecheckCommand
		workspaceContext.BuildCommand = profile.BuildCommand
	}
	model := strings.TrimSpace(definition.Model)
	if model == "" {
		model = defaultModel
	}
	attribution := definition.Attribution(model)
	if !definition.ShellEnabled {
		shell = nil
	}
	loop := agent.Loop{
		Provider: selectedProvider, Shell: shell,
		Recorder: recorder{
			ctx: runCtx, store: s.store, sessionID: sessionID, attribution: &attribution,
		},
		Observer: observer, Model: model, StepLimit: definition.MaxSteps,
		Prompt: agent.DefinitionPrompt(agent.WorkspacePrompt(workspaceContext), definition),
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
	ctx         context.Context
	store       *workspace.Store
	sessionID   string
	attribution *agent.Attribution
}

func (r recorder) Append(message agent.Message) error {
	if err := r.store.AppendAttributedMessage(r.ctx, r.sessionID, message, r.attribution); err != nil {
		return fmt.Errorf("record conversation: %w", err)
	}
	return nil
}

type observer struct{ shellCalls int }

func (o *observer) Step(_, _ int)                {}
func (o *observer) ToolCall(agent.ShellRequest)  { o.shellCalls++ }
func (o *observer) ToolResult(agent.ShellResult) {}
func (o *observer) Assistant(string)             {}
