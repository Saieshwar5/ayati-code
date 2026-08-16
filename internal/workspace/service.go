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
	Ensure(context.Context, sandbox.Spec) error
	Open(string) (agent.Shell, error)
	Remove(context.Context, string) error
}

type gitClient interface {
	Run(context.Context, ...string) error
	AuthenticatedRun(context.Context, ...string) error
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

func (s *Service) Initialize(ctx context.Context, id string) error {
	value, err := s.store.Get(ctx, id)
	if err != nil {
		return err
	}
	if err := s.store.UpdateStatus(ctx, id, StatusInitializing, ""); err != nil {
		return err
	}
	if err := s.prepareRepository(ctx, value); err != nil {
		return s.fail(ctx, id, err)
	}
	if err := s.environment.Ensure(ctx, sandbox.Spec{Name: value.SandboxName, Path: value.Path}); err != nil {
		return s.fail(ctx, id, fmt.Errorf("start sandbox: %w", err))
	}
	setup := value.Setup
	if setup == "" {
		setup = DetectSetup(value.Path)
		if err := s.store.UpdateSetup(ctx, id, setup); err != nil {
			return s.fail(ctx, id, err)
		}
	}
	if setup != "" {
		shell, err := s.environment.Open(value.SandboxName)
		if err != nil {
			return s.fail(ctx, id, err)
		}
		result := shell.Execute(ctx, agent.ShellRequest{Command: setup})
		if result.ExitCode != 0 || result.Error != "" {
			message := strings.TrimSpace(strings.Join([]string{result.Error, result.Stderr, result.Stdout}, "\n"))
			return s.fail(ctx, id, fmt.Errorf("setup failed: %s", boundedMessage(message)))
		}
	}
	return s.store.UpdateStatus(ctx, id, StatusReady, "")
}

func (s *Service) Stop(ctx context.Context, id string) error {
	value, err := s.store.Get(ctx, id)
	if err != nil {
		return err
	}
	if err := s.environment.Remove(ctx, value.SandboxName); err != nil {
		return fmt.Errorf("remove sandbox: %w", err)
	}
	return s.store.UpdateStatus(ctx, id, StatusStopped, "")
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

type Changes struct {
	Status string `json:"status"`
	Diff   string `json:"diff"`
}

func (s *Service) Changes(ctx context.Context, id string) (Changes, error) {
	shell, _, err := s.Shell(ctx, id)
	if err != nil {
		return Changes{}, err
	}
	status := shell.Execute(ctx, agent.ShellRequest{Command: "git status --short"})
	if status.ExitCode != 0 || status.Error != "" {
		return Changes{}, fmt.Errorf("inspect Git status: %s", shellError(status))
	}
	diffCommand := `git diff --no-ext-diff --stat && git diff --no-ext-diff && ` +
		`git ls-files --others --exclude-standard -z | ` +
		`xargs -0 -r -n1 sh -c 'git diff --no-index -- /dev/null "$1"; code=$?; test "$code" -eq 0 -o "$code" -eq 1' sh`
	diff := shell.Execute(ctx, agent.ShellRequest{Command: diffCommand})
	if diff.ExitCode != 0 || diff.Error != "" {
		return Changes{}, fmt.Errorf("inspect Git diff: %s", shellError(diff))
	}
	return Changes{Status: status.Stdout, Diff: diff.Stdout}, nil
}

func (s *Service) Publish(ctx context.Context, id, message, authorName, authorEmail string) error {
	shell, value, err := s.Shell(ctx, id)
	if err != nil {
		return err
	}
	working, err := s.store.HasWorkingSession(ctx, id)
	if err != nil {
		return fmt.Errorf("inspect running sessions: %w", err)
	}
	if working {
		return errors.New("a session is still running; wait for the agent before publishing")
	}
	message = strings.TrimSpace(message)
	if message == "" {
		return errors.New("commit message is required")
	}
	status := shell.Execute(ctx, agent.ShellRequest{Command: "git status --porcelain"})
	if status.ExitCode != 0 || status.Error != "" {
		return fmt.Errorf("inspect changes: %s", shellError(status))
	}
	if strings.TrimSpace(status.Stdout) != "" {
		arguments := []string{"git", "-c", shellQuote("core.hooksPath=/dev/null")}
		arguments = append(arguments, "add", "--all", "&&", "git", "-c", shellQuote("core.hooksPath=/dev/null"))
		if strings.TrimSpace(authorName) != "" {
			arguments = append(arguments, "-c", shellQuote("user.name="+strings.TrimSpace(authorName)))
		}
		if strings.TrimSpace(authorEmail) != "" {
			arguments = append(arguments, "-c", shellQuote("user.email="+strings.TrimSpace(authorEmail)))
		}
		arguments = append(arguments, "commit", "--no-verify", "-m", shellQuote(message))
		commit := shell.Execute(ctx, agent.ShellRequest{Command: strings.Join(arguments, " ")})
		if commit.ExitCode != 0 || commit.Error != "" {
			return fmt.Errorf("commit changes: %s", shellError(commit))
		}
	}
	refspec := "refs/heads/" + value.Branch + ":refs/heads/" + value.Branch
	if err := s.git.AuthenticatedRun(ctx, "-C", value.Path, "push", "--no-verify", "--", value.CloneURL, refspec); err != nil {
		return fmt.Errorf("push branch: %w", err)
	}
	return nil
}

func (s *Service) Shell(ctx context.Context, id string) (agent.Shell, Workspace, error) {
	value, err := s.store.Get(ctx, id)
	if err != nil {
		return nil, Workspace{}, err
	}
	if value.Status != StatusReady {
		return nil, Workspace{}, fmt.Errorf("workspace is %s, not ready", value.Status)
	}
	if err := s.environment.Ensure(ctx, sandbox.Spec{Name: value.SandboxName, Path: value.Path}); err != nil {
		return nil, Workspace{}, fmt.Errorf("restore sandbox: %w", err)
	}
	shell, err := s.environment.Open(value.SandboxName)
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
	if err := s.store.UpdateStatus(ctx, id, StatusInitializationFailed, boundedMessage(cause.Error())); err != nil {
		return fmt.Errorf("%v; record failure: %w", cause, err)
	}
	return cause
}

func DetectSetup(path string) string {
	var commands []string
	if fileExists(path, "go.mod") {
		commands = append(commands, "go mod download")
	}
	switch {
	case fileExists(path, "pnpm-lock.yaml"):
		commands = append(commands, "corepack pnpm install --frozen-lockfile")
	case fileExists(path, "yarn.lock"):
		commands = append(commands, "corepack yarn install --immutable")
	case fileExists(path, "package-lock.json"):
		commands = append(commands, "npm ci")
	case fileExists(path, "package.json"):
		commands = append(commands, "npm install")
	}
	switch {
	case fileExists(path, "requirements.txt"):
		commands = append(commands, "python3 -m venv .venv && .venv/bin/pip install -r requirements.txt")
	case fileExists(path, "pyproject.toml"):
		commands = append(commands, "python3 -m venv .venv && .venv/bin/pip install .")
	}
	return strings.Join(commands, " && ")
}

func fileExists(root, name string) bool {
	info, err := os.Stat(filepath.Join(root, name))
	return err == nil && !info.IsDir()
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
