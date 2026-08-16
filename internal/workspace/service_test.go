package workspace

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Saieshwar5/ayati-code/internal/agent"
	"github.com/Saieshwar5/ayati-code/internal/sandbox"
)

type fakeEnvironment struct {
	ensured   []sandbox.Spec
	removed   []string
	variables []map[string]string
	shell     agent.Shell
	err       error
}

func (f *fakeEnvironment) Ensure(_ context.Context, spec sandbox.Spec) error {
	f.ensured = append(f.ensured, spec)
	return f.err
}

func (f *fakeEnvironment) Open(_ string, variables map[string]string) (agent.Shell, error) {
	f.variables = append(f.variables, variables)
	return f.shell, f.err
}

func (f *fakeEnvironment) Remove(_ context.Context, name string) error {
	f.removed = append(f.removed, name)
	return f.err
}

type recordingShell struct {
	commands []string
	result   agent.ShellResult
}

func (s *recordingShell) Execute(_ context.Context, request agent.ShellRequest) agent.ShellResult {
	s.commands = append(s.commands, request.Command)
	return s.result
}

type recordingGit struct {
	calls [][]string
}

func (g *recordingGit) Run(_ context.Context, arguments ...string) error {
	g.calls = append(g.calls, append([]string(nil), arguments...))
	if len(arguments) > 0 && arguments[0] == "clone" {
		path := arguments[len(arguments)-1]
		return os.MkdirAll(filepath.Join(path, ".git"), 0o700)
	}
	return nil
}

func (g *recordingGit) AuthenticatedRun(ctx context.Context, arguments ...string) error {
	return g.Run(ctx, arguments...)
}

func TestServiceInitializesBranchSandboxAndDependencies(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "ayati.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	path := filepath.Join(t.TempDir(), "repo")
	value, err := store.Create(context.Background(), Create{
		Repository: "owner/project", CloneURL: "https://github.com/owner/project.git",
		BaseBranch: "main", Branch: "ayati/change", CreateBranch: true,
		Setup: "go mod download", Path: path,
		Environment: []EnvironmentInput{
			{Name: "SETUP_TOKEN", Value: "setup-secret", ExposeDuringSetup: true},
			{Name: "RUNTIME_TOKEN", Value: "runtime-secret"},
		},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	shell := &recordingShell{result: agent.ShellResult{ExitCode: 0}}
	environment := &fakeEnvironment{shell: shell}
	git := &recordingGit{}
	service := &Service{store: store, environment: environment, git: git}
	if err := service.Initialize(context.Background(), value.ID); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	wantGit := [][]string{
		{"clone", "--branch", "main", "--single-branch", "--", value.CloneURL, path},
		{"-C", path, "switch", "-c", "ayati/change"},
	}
	if !reflect.DeepEqual(git.calls, wantGit) {
		t.Fatalf("git calls = %#v", git.calls)
	}
	if len(environment.ensured) != 1 || shell.commands[0] != "go mod download" {
		t.Fatalf("sandbox = %#v, commands = %#v", environment.ensured, shell.commands)
	}
	if !reflect.DeepEqual(environment.variables, []map[string]string{{"SETUP_TOKEN": "setup-secret"}}) {
		t.Fatalf("setup environment = %#v", environment.variables)
	}
	loaded, _ := store.Get(context.Background(), value.ID)
	if loaded.Status != StatusReady {
		t.Fatalf("workspace = %#v", loaded)
	}
}

func TestServiceRecordsInitializationFailure(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "ayati.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	path := filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(filepath.Join(path, ".git"), 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	value, _ := store.Create(context.Background(), Create{
		Repository: "owner/project", CloneURL: "https://github.com/owner/project.git",
		BaseBranch: "main", Branch: "main", Setup: "npm ci", Path: path,
	})
	environment := &fakeEnvironment{err: errors.New("docker unavailable")}
	service := &Service{store: store, environment: environment, git: &recordingGit{}}
	if err := service.Initialize(context.Background(), value.ID); err == nil {
		t.Fatal("Initialize succeeded")
	}
	loaded, _ := store.Get(context.Background(), value.ID)
	if loaded.Status != StatusInitializationFailed || loaded.Error == "" {
		t.Fatalf("workspace = %#v", loaded)
	}
}

func TestServiceDeletesManagedWorkspaceAndHistory(t *testing.T) {
	root := t.TempDir()
	store, err := Open(filepath.Join(t.TempDir(), "ayati.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	value, err := store.Create(context.Background(), Create{
		Repository: "owner/project", CloneURL: "https://github.com/owner/project.git",
		BaseBranch: "main", Branch: "ayati/delete", Root: root,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := store.UpdateStatus(context.Background(), value.ID, StatusReady, ""); err != nil {
		t.Fatalf("UpdateStatus: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(value.Path, ".git"), 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	sessions, err := store.ListSessions(context.Background(), value.ID)
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if err := store.AppendMessage(context.Background(), sessions[0].ID,
		agent.Message{Role: "user", Content: "delete me"}); err != nil {
		t.Fatalf("AppendMessage: %v", err)
	}
	environment := &fakeEnvironment{}
	service := &Service{store: store, environment: environment, git: &recordingGit{}, root: root}
	if err := service.Delete(context.Background(), value.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if len(environment.removed) != 1 || environment.removed[0] != value.SandboxName {
		t.Fatalf("removed sandboxes = %#v", environment.removed)
	}
	if _, err := os.Stat(filepath.Join(root, value.ID)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("workspace directory still exists: %v", err)
	}
	if _, err := store.Get(context.Background(), value.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("deleted workspace error = %v", err)
	}
	remaining, err := store.ListSessions(context.Background(), value.ID)
	if err != nil || len(remaining) != 0 {
		t.Fatalf("remaining sessions = %#v, error = %v", remaining, err)
	}
	messages, err := store.Messages(context.Background(), sessions[0].ID)
	if err != nil || len(messages) != 0 {
		t.Fatalf("remaining messages = %#v, error = %v", messages, err)
	}
}

func TestServiceRefusesToDeleteWorkspaceOutsideManagedRoot(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "ayati.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	path := filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(path, 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	value, err := store.Create(context.Background(), Create{
		Repository: "owner/project", CloneURL: "https://github.com/owner/project.git",
		BaseBranch: "main", Branch: "ayati/delete", Path: path,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	environment := &fakeEnvironment{}
	service := &Service{store: store, environment: environment, git: &recordingGit{}, root: t.TempDir()}
	err = service.Delete(context.Background(), value.ID)
	if err == nil || !strings.Contains(err.Error(), "outside the managed data root") {
		t.Fatalf("Delete error = %v", err)
	}
	if len(environment.removed) != 0 {
		t.Fatalf("removed sandboxes = %#v", environment.removed)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("unmanaged path was removed: %v", err)
	}
}

func TestServiceRefusesDeletionDuringInitialization(t *testing.T) {
	root := t.TempDir()
	store, err := Open(filepath.Join(t.TempDir(), "ayati.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	value, err := store.Create(context.Background(), Create{
		Repository: "owner/project", CloneURL: "https://github.com/owner/project.git",
		BaseBranch: "main", Branch: "ayati/delete", Root: root,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	service := &Service{store: store, environment: &fakeEnvironment{}, git: &recordingGit{}, root: root}
	err = service.Delete(context.Background(), value.ID)
	if err == nil || !strings.Contains(err.Error(), "initialization is still running") {
		t.Fatalf("Delete error = %v", err)
	}
	if _, err := store.Get(context.Background(), value.ID); err != nil {
		t.Fatalf("workspace was deleted: %v", err)
	}
}

func TestDetectSetupUsesLockfiles(t *testing.T) {
	path := t.TempDir()
	for _, name := range []string{"go.mod", "package-lock.json", "requirements.txt"} {
		if err := os.WriteFile(filepath.Join(path, name), []byte("test"), 0o600); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
	}
	want := "go mod download && npm ci && python3 -m venv .venv && .venv/bin/pip install -r requirements.txt"
	if got := DetectSetup(path); got != want {
		t.Fatalf("DetectSetup = %q", got)
	}
}

func TestShellQuoteContainsUserInput(t *testing.T) {
	if got, want := shellQuote("it's safe"), `'it'"'"'s safe'`; got != want {
		t.Fatalf("shellQuote = %q, want %q", got, want)
	}
}

func TestServicePublishesWorkspaceChanges(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "ayati.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	value, err := store.Create(context.Background(), Create{
		Repository: "owner/project", CloneURL: "https://github.com/owner/project.git",
		BaseBranch: "main", Branch: "ayati/change", Path: filepath.Join(t.TempDir(), "repo"),
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := store.UpdateStatus(context.Background(), value.ID, StatusReady, ""); err != nil {
		t.Fatalf("UpdateStatus: %v", err)
	}
	shell := &recordingShell{result: agent.ShellResult{Stdout: " M app.go\n", ExitCode: 0}}
	git := &recordingGit{}
	service := &Service{store: store, environment: &fakeEnvironment{shell: shell}, git: git}
	if err := service.Publish(context.Background(), value.ID, "feat: change", "octocat", "octocat@example.com"); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	want := [][]string{
		{"-C", value.Path, "push", "--no-verify", "--", value.CloneURL, "refs/heads/ayati/change:refs/heads/ayati/change"},
	}
	if !reflect.DeepEqual(git.calls, want) {
		t.Fatalf("git calls = %#v", git.calls)
	}
	if len(shell.commands) != 2 || !strings.Contains(shell.commands[1], "commit --no-verify") {
		t.Fatalf("shell commands = %#v", shell.commands)
	}
}
