package workspace

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/Saieshwar5/perpetual/internal/environments"
	"github.com/Saieshwar5/perpetual/internal/exec"
	"github.com/Saieshwar5/perpetual/internal/workspaceruntime"
)

type gitClient interface {
	Run(context.Context, ...string) error
	AuthenticatedRun(context.Context, string, ...string) error
	Output(context.Context, ...string) (string, error)
}

// GitHubTokenProvider keeps workspace preparation and publishing independent
// from the account store while still using the owning user's GitHub token.
type GitHubTokenProvider interface {
	TokenForUser(context.Context, string) (string, error)
}

// RuntimeProvider maps a workspace's persisted runtime provider name to the
// runtime implementation that should execute its commands.
type RuntimeProvider interface {
	RuntimeFor(provider string) (workspaceruntime.Runtime, error)
}

// ImageBuilder builds and waits for a Lambda MicroVM image. Implied by
// environments.ImageBuilder; injected only for lambda workspaces.
type ImageBuilder interface {
	Build(context.Context) (environments.ImageRef, error)
}

// RepoSyncer pushes the controller working tree into a workspace runtime (used
// for lambda microVM instances). Nil for local-only controllers.
type RepoSyncer interface {
	Push(ctx context.Context, workspaceID, tree string) error
	Pull(ctx context.Context, workspaceID, scratch string) error
}

// Service coordinates workspace lifecycle with the controller's Git and the
// workspace runtime that executes setup and agent commands. The runtime is the
// only execution seam; the service never opens a shell directly on the host.
type Service struct {
	store        *Store
	runtime      workspaceruntime.Runtime
	provider     RuntimeProvider
	imageBuilder ImageBuilder
	reposyncer   RepoSyncer
	git          gitClient
	credentials  GitHubTokenProvider
	root         string
	deleteMu     sync.Mutex
}

func NewService(store *Store, credentials GitHubTokenProvider, provider RuntimeProvider, root string) (*Service, error) {
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
	git, err := newGitClient()
	if err != nil {
		return nil, err
	}
	return &Service{
		store:       store,
		git:         git,
		provider:    provider,
		credentials: credentials,
		root:        filepath.Clean(root),
		runtime:     workspaceruntime.NewLocal(),
	}, nil
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
	runtime, err := s.runtimeFor(value)
	if err != nil {
		return err
	}
	if err := runtime.Stop(ctx, runtimeRef(value)); err != nil {
		return fmt.Errorf("stop workspace runtime: %w", err)
	}
	if err := s.store.UpdateRuntimeState(ctx, id, workspaceruntime.RuntimeStateStopped); err != nil {
		return fmt.Errorf("record stopped runtime: %w", err)
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
	runtime, runtimeErr := s.runtimeFor(value)
	if runtimeErr != nil {
		return runtimeErr
	}
	if err := runtime.Start(ctx, runtimeRef(value)); err != nil {
		return fmt.Errorf("start workspace runtime: %w", err)
	}
	if err := s.store.UpdateRuntimeState(ctx, id, workspaceruntime.RuntimeStateRunning); err != nil {
		return fmt.Errorf("record running runtime: %w", err)
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
	runtime, err := s.runtimeFor(value)
	if err != nil {
		return nil, err
	}
	return runtime.OpenShell(ctx, runtimeRef(value), environment)
}

// SetImageBuilder injects the Lambda image builder used by lambda workspaces.
func (s *Service) SetImageBuilder(builder ImageBuilder) {
	s.imageBuilder = builder
}

// SetRepoSyncer injects the lambda repo syncer used during preparation.
func (s *Service) SetRepoSyncer(syncer RepoSyncer) {
	s.reposyncer = syncer
}

// runtimeFor returns the runtime selected for value's provider, falling back
// to the directly configured runtime and then the local compatibility runtime
// so a zero-value Service remains usable in tests. Provider resolution errors
// are propagated instead of silently falling back, so misconfiguration is
// visible inside workspace jobs.
func (s *Service) runtimeFor(value Workspace) (workspaceruntime.Runtime, error) {
	provider := strings.TrimSpace(value.RuntimeProvider)
	if provider == "" {
		provider = "local"
	}
	if s.provider != nil {
		runtime, err := s.provider.RuntimeFor(provider)
		if err != nil {
			return nil, fmt.Errorf("resolve workspace runtime %q: %w", provider, err)
		}
		return runtime, nil
	}
	if s.runtime != nil {
		return s.runtime, nil
	}
	return workspaceruntime.NewLocal(), nil
}

func runtimeRef(value Workspace) workspaceruntime.Ref {
	return workspaceruntime.Ref{
		ID:        value.ID,
		RuntimeID: value.RuntimeRef,
		Directory: value.Path,
		CacheDir:  workspaceCachePath(value.Path),
	}
}

func (s *Service) tokenForUser(ctx context.Context, userID string) (string, error) {
	if s.credentials == nil {
		return "", nil
	}
	token, err := s.credentials.TokenForUser(ctx, userID)
	if err != nil {
		return "", fmt.Errorf("load GitHub credential for workspace owner: %w", err)
	}
	return token, nil
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
	token, err := s.tokenForUser(ctx, value.UserID)
	if err != nil {
		return err
	}
	if err := s.git.AuthenticatedRun(ctx, token, "clone", "--branch", cloneBranch, "--single-branch", "--", value.CloneURL, value.Path); err != nil {
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
