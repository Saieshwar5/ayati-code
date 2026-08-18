package workspace

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/Saieshwar5/perpetual/internal/agent"
	compute "github.com/Saieshwar5/perpetual/internal/environment"
)

type environment interface {
	Cleanup(context.Context, compute.StopInput) error
	Ensure(context.Context, compute.StopInput) (compute.Assignment, error)
	Start(context.Context, compute.StartInput) (compute.Assignment, error)
	Replace(context.Context, compute.ReplaceInput) (compute.Assignment, error)
	Stop(context.Context, compute.StopInput) error
	Open(context.Context, compute.StopInput, map[string]string) (agent.Shell, error)
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
	deleteMu    sync.Mutex
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
	if err := s.environment.Stop(ctx, runtimeInput(value, value.Authority == AuthorityDevelop)); err != nil {
		return fmt.Errorf("release environment: %w", err)
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
	_, err = s.environment.Start(ctx, compute.StartInput{
		WorkspaceID: value.ID, WorkspacePath: value.Path,
		CachePath: workspaceCachePath(value.Path), WorkspaceWritable: value.Authority == AuthorityDevelop,
	})
	if err != nil {
		return fmt.Errorf("resume environment: %w", err)
	}
	if err := s.store.UpdateEffectiveMountMode(ctx, id, effectiveMountMode(value.Authority)); err != nil {
		return s.releaseAfterResumeFailure(ctx, value, err)
	}
	if err := s.store.UpdateStatus(ctx, id, StatusReady, ""); err != nil {
		return s.releaseAfterResumeFailure(ctx, value, err)
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
	writable := value.Authority == AuthorityDevelop
	mode := effectiveMountMode(value.Authority)
	if value.EffectiveMountMode != mode {
		if err := s.store.UpdateEffectiveMountMode(ctx, id, mode); err != nil {
			return nil, Workspace{}, err
		}
		value.EffectiveMountMode = mode
	}
	variables, err := s.store.EnvironmentValues(ctx, id, false)
	if err != nil {
		return nil, Workspace{}, err
	}
	shell, err := s.environment.Open(ctx, runtimeInput(value, writable), runtimeEnvironment(variables))
	return shell, value, err
}

func runtimeInput(value Workspace, writable bool) compute.StopInput {
	return compute.StopInput{
		WorkspaceID: value.ID, WorkspacePath: value.Path,
		CachePath: workspaceCachePath(value.Path), WorkspaceWritable: writable,
	}
}

func effectiveMountMode(authority Authority) string {
	if authority == AuthorityDevelop {
		return "rw"
	}
	return "ro"
}

func (s *Service) releaseAfterResumeFailure(ctx context.Context, value Workspace, cause error) error {
	if err := s.environment.Stop(ctx,
		runtimeInput(value, value.Authority == AuthorityDevelop)); err != nil {
		return errors.Join(cause, fmt.Errorf("release resumed environment: %w", err))
	}
	return cause
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
