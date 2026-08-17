package workspace

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Saieshwar5/ayati-code/internal/agent"
	"github.com/Saieshwar5/ayati-code/internal/sandbox"
)

type environment interface {
	Ensure(context.Context, sandbox.Spec) (sandbox.MountMode, error)
	Open(string, map[string]string) (agent.Shell, error)
	Remove(context.Context, string) error
}

type gitClient interface {
	Run(context.Context, ...string) error
	AuthenticatedRun(context.Context, ...string) error
	Output(context.Context, ...string) (string, error)
}

type Service struct {
	store       *Store
	environment environment
	git         gitClient
	root        string
}

func NewService(store *Store, environment environment, token func() (string, error), root string) (*Service, error) {
	if store == nil || environment == nil {
		return nil, errors.New("workspace store and sandbox environment are required")
	}
	root = strings.TrimSpace(root)
	if root == "" {
		return nil, errors.New("workspace root is required")
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve workspace root: %w", err)
	}
	git, err := newGitClient(token)
	if err != nil {
		return nil, err
	}
	return &Service{store: store, environment: environment, git: git, root: filepath.Clean(root)}, nil
}

func (s *Service) Stop(ctx context.Context, id string) error {
	value, err := s.store.Get(ctx, id)
	if err != nil {
		return err
	}
	if err := requireActiveWorkspace(value); err != nil {
		return err
	}
	if value.Status != StatusReady {
		return fmt.Errorf("workspace is %s, not ready", value.Status)
	}
	if err := s.environment.Remove(ctx, value.SandboxName); err != nil {
		return fmt.Errorf("remove sandbox: %w", err)
	}
	return s.store.UpdateStatus(ctx, id, StatusStopped, "")
}

func (s *Service) Resume(ctx context.Context, id string) error {
	value, err := s.store.Get(ctx, id)
	if err != nil {
		return err
	}
	if err := requireActiveWorkspace(value); err != nil {
		return err
	}
	if value.Status != StatusStopped {
		return fmt.Errorf("workspace is %s, not stopped", value.Status)
	}
	if value.PreparationStage != PreparationReady {
		return fmt.Errorf("workspace preparation is %s and cannot be resumed", value.PreparationStage)
	}
	mode, err := s.environment.Ensure(ctx, s.sandboxSpec(value, value.Authority.MountMode()))
	if err != nil {
		return fmt.Errorf("resume sandbox: %w", err)
	}
	if mode != value.Authority.MountMode() {
		return fmt.Errorf("resumed sandbox mount is %s, expected %s", mode, value.Authority.MountMode())
	}
	if err := s.store.UpdateEffectiveMountMode(ctx, id, string(mode)); err != nil {
		return err
	}
	return s.store.UpdateStatus(ctx, id, StatusReady, "")
}

func (s *Service) Delete(ctx context.Context, id string) error {
	value, err := s.store.Get(ctx, id)
	if err != nil {
		return err
	}
	workspaceDirectory := filepath.Join(s.root, value.ID)
	expectedRepository := filepath.Join(workspaceDirectory, "repo")
	if filepath.Clean(value.Path) != filepath.Clean(expectedRepository) {
		return errors.New("workspace path is outside the managed data root")
	}
	if value.Status == StatusCreating || value.Status == StatusInitializing {
		return errors.New("workspace initialization is still running; wait before deleting it")
	}
	working, err := s.store.HasWorkingSession(ctx, id)
	if err != nil {
		return fmt.Errorf("inspect running sessions: %w", err)
	}
	if working {
		return errors.New("a session is still running; stop it before deleting the workspace")
	}
	if err := s.environment.Remove(ctx, value.SandboxName); err != nil {
		return fmt.Errorf("remove sandbox: %w", err)
	}
	if err := os.RemoveAll(workspaceDirectory); err != nil {
		return fmt.Errorf("remove workspace files: %w", err)
	}
	if err := s.store.Delete(ctx, id); err != nil {
		return err
	}
	return nil
}

func (s *Service) Shell(ctx context.Context, id string) (agent.Shell, Workspace, error) {
	value, err := s.store.Get(ctx, id)
	if err != nil {
		return nil, Workspace{}, err
	}
	if err := requireActiveWorkspace(value); err != nil {
		return nil, Workspace{}, err
	}
	if value.Status != StatusReady {
		return nil, Workspace{}, fmt.Errorf("workspace is %s, not ready", value.Status)
	}
	mode, err := s.environment.Ensure(ctx, sandbox.Spec{
		Name: value.SandboxName, Path: value.Path, CachePath: workspaceCachePath(value.Path),
		MountMode: value.Authority.MountMode(),
	})
	if err != nil {
		return nil, Workspace{}, fmt.Errorf("restore sandbox: %w", err)
	}
	if value.EffectiveMountMode != string(mode) {
		if err := s.store.UpdateEffectiveMountMode(ctx, id, string(mode)); err != nil {
			return nil, Workspace{}, err
		}
		value.EffectiveMountMode = string(mode)
	}
	variables, err := s.store.EnvironmentValues(ctx, id, false)
	if err != nil {
		return nil, Workspace{}, err
	}
	shell, err := s.environment.Open(value.SandboxName, runtimeEnvironment(variables))
	return shell, value, err
}

func (s *Service) prepareRepository(ctx context.Context, value Workspace) error {
	gitDirectory := filepath.Join(value.Path, ".git")
	if info, err := os.Stat(gitDirectory); err == nil && info.IsDir() {
		return nil
	}
	entries, err := os.ReadDir(value.Path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect repository directory: %w", err)
	}
	if len(entries) != 0 {
		return errors.New("workspace directory exists but is not a Git repository")
	}
	if err := os.MkdirAll(filepath.Dir(value.Path), 0o700); err != nil {
		return fmt.Errorf("create workspace root: %w", err)
	}
	cloneBranch := value.Branch
	if value.CreateBranch {
		cloneBranch = value.BaseBranch
	}
	if err := s.git.AuthenticatedRun(ctx, "clone", "--branch", cloneBranch, "--single-branch", "--", value.CloneURL, value.Path); err != nil {
		return fmt.Errorf("clone repository: %w", err)
	}
	if value.CreateBranch {
		if err := s.git.Run(ctx, "-C", value.Path, "switch", "-c", value.Branch); err != nil {
			return fmt.Errorf("create branch: %w", err)
		}
	}
	return nil
}

func (s *Service) fail(ctx context.Context, id string, cause error) error {
	if err := s.store.FailPreparation(ctx, id, boundedMessage(cause.Error())); err != nil {
		return fmt.Errorf("%v; record failure: %w", cause, err)
	}
	return cause
}

func boundedMessage(value string) string {
	value = strings.TrimSpace(value)
	if len(value) <= 4096 {
		return value
	}
	return value[:4096] + "…"
}

func shellError(result agent.ShellResult) string {
	return boundedMessage(strings.TrimSpace(strings.Join([]string{result.Error, result.Stderr, result.Stdout}, "\n")))
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'"'"'`) + "'"
}
