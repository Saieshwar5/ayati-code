package chat

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Saieshwar5/ayati-code/internal/agent"
	"github.com/Saieshwar5/ayati-code/internal/workspace"
)

func (s *Service) execute(
	runCtx context.Context, run workspace.AgentRun, active *activeRun,
) (agent.Completion, error) {
	defer active.cancel()
	if err := s.store.MarkAgentRunRunning(context.Background(), run.ID); err != nil {
		s.finishRun(run.WorkspaceID, active)
		finishErr := s.store.FinishAgentRun(context.Background(), run.ID,
			workspace.AgentRunStatusFailed, workspace.SessionStatusFailed, err.Error())
		s.notify(run.WorkspaceID, run.SessionID, run.ID)
		if finishErr != nil {
			return agent.Completion{}, fmt.Errorf("start agent run: %v; record failure: %w", err, finishErr)
		}
		return agent.Completion{}, err
	}
	s.notify(run.WorkspaceID, run.SessionID, run.ID)
	completion, shellCalls, runErr := s.runLoop(runCtx, run)
	canceled := s.finishRun(run.WorkspaceID, active)
	runStatus, sessionStatus, message := finalRunStatus(runErr, canceled, shellCalls)
	if canceled && runErr == nil {
		runErr = context.Canceled
	}
	if err := s.store.FinishAgentRun(context.Background(), run.ID, runStatus, sessionStatus, message); err != nil {
		if runErr != nil {
			return completion, fmt.Errorf("agent run failed: %v; record completion: %w", runErr, err)
		}
		return completion, fmt.Errorf("record agent run completion: %w", err)
	}
	s.notify(run.WorkspaceID, run.SessionID, run.ID)
	return completion, runErr
}

func (s *Service) runLoop(
	ctx context.Context, run workspace.AgentRun,
) (agent.Completion, int, error) {
	session, err := s.store.GetSession(ctx, run.WorkspaceID, run.SessionID)
	if err != nil {
		return agent.Completion{}, 0, fmt.Errorf("load session: %w", err)
	}
	definition, err := s.store.GetAgent(ctx, session.SelectedAgentID)
	if err != nil {
		return agent.Completion{}, 0, fmt.Errorf("load selected agent: %w", err)
	}
	if definition.ArchivedAt != nil {
		return agent.Completion{}, 0, errors.New("the selected agent is archived; choose another agent")
	}
	selectedProvider, defaultModel, err := s.providers.Resolve(definition.ProviderID)
	if err != nil {
		return agent.Completion{}, 0, err
	}
	skills, err := s.store.AgentSkills(ctx, definition.ID)
	if err != nil {
		return agent.Completion{}, 0, fmt.Errorf("load selected agent skills: %w", err)
	}
	shell, currentWorkspace, err := s.runtime.Shell(ctx, run.WorkspaceID)
	if err != nil {
		return agent.Completion{}, 0, err
	}
	history, err := s.store.Messages(ctx, run.SessionID)
	if err != nil {
		return agent.Completion{}, 0, err
	}
	workspaceContext := workspacePromptContext(currentWorkspace)
	model := strings.TrimSpace(definition.Model)
	if model == "" {
		model = defaultModel
	}
	attribution := definition.Attribution(model, skills...)
	if !definition.ShellEnabled {
		shell = nil
	}
	observer := &observer{}
	loop := agent.Loop{
		Provider: selectedProvider, Shell: shell,
		Recorder: recorder{
			ctx: ctx, store: s.store, sessionID: run.SessionID, attribution: &attribution,
			notifier: s.notifier, workspaceID: run.WorkspaceID, runID: run.ID,
		},
		Observer: observer, Model: model, StepLimit: definition.MaxSteps,
		Prompt: agent.DefinitionPrompt(agent.WorkspacePrompt(workspaceContext), definition, skills...),
	}
	completion, err := loop.Continue(ctx, &history)
	return completion, observer.shellCalls, err
}

func workspacePromptContext(value workspace.Workspace) agent.WorkspaceContext {
	result := agent.WorkspaceContext{
		Repository: value.Repository, Branch: value.Branch, Authority: string(value.Authority),
	}
	if profile := value.Profile; profile != nil {
		result.ProjectRoot = profile.ProjectRoot
		result.Languages = profile.Languages
		result.RuntimeVersions = profile.RuntimeVersions
		result.PackageManagers = profile.PackageManagers
		result.SetupResult = profile.SetupResult
		result.BaselineCommit = profile.BaselineCommit
		result.TestCommand = profile.TestCommand
		result.LintCommand = profile.LintCommand
		result.TypecheckCommand = profile.TypecheckCommand
		result.BuildCommand = profile.BuildCommand
	}
	return result
}

func finalRunStatus(runErr error, canceled bool, shellCalls int) (string, string, string) {
	if canceled || errors.Is(runErr, context.Canceled) {
		return workspace.AgentRunStatusCanceled, workspace.SessionStatusCanceled, ""
	}
	if runErr != nil {
		return workspace.AgentRunStatusFailed, workspace.SessionStatusFailed, runErr.Error()
	}
	if shellCalls > 0 {
		return workspace.AgentRunStatusCompleted, workspace.SessionStatusReview, ""
	}
	return workspace.AgentRunStatusCompleted, workspace.SessionStatusIdle, ""
}
