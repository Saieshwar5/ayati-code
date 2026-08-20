package workspace

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/Saieshwar5/perpetual/internal/exec"
)

type gitClient interface {
	Run(context.Context, ...string) error
	AuthenticatedRun(context.Context, ...string) error
	Output(context.Context, ...string) (string, error)
}

// shellFactory creates a bounded shell for a workspace command. The factory is
// injectable so tests can record variables without executing on the host.
type shellFactory func(variables map[string]string, dir string) (exec.Shell, error)

type Service struct {
	store    *Store
	shells   shellFactory
	git      gitClient
	root     string
	deleteMu sync.Mutex
}

func NewService(store *Store, token func() (string, error), root string) (*Service, error) {
	if store == nil {
		return nil, errors.New("workspace store is required")
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
	service := &Service{store: store, git: git, root: filepath.Clean(root)}
	service.shells = func(variables map[string]string, dir string) (exec.Shell, error) {
		return exec.New(variables, dir)
	}
	return service, nil
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
	return s.store.UpdateStatus(ctx, id, StatusReady, "")
}

func (s *Service) Shell(ctx context.Context, id string) (exec.Shell, Workspace, error) {
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
	shell, err := s.openShell(ctx, value, false)
	return shell, value, err
}

func (s *Service) openShell(ctx context.Context, value Workspace, setupOnly bool) (exec.Shell, error) {
	variables, err := s.store.EnvironmentValues(ctx, value.ID, setupOnly)
	if err != nil {
		return nil, err
	}
	environment := shellEnvironment(workspaceCachePath(value.Path), variables)
	if err := os.MkdirAll(environment["HOME"], 0o700); err != nil {
		return nil, fmt.Errorf("create shell home: %w", err)
	}
	return s.shells(environment, value.Path)
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

func shellError(result exec.ShellResult) string {
	return boundedMessage(strings.TrimSpace(strings.Join([]string{result.Error, result.Stderr, result.Stdout}, "\n")))
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'"'"'`) + "'"
}
