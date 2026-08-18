package workspace

import (
	"context"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Saieshwar5/perpetual/internal/agent"
)

func TestServicePublishesWorkspaceChanges(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "ayati.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	value, err := store.Create(context.Background(), Create{
		Repository: "owner/project", CloneURL: "https://github.com/owner/project.git",
		BaseBranch: "main", Branch: "perpetual/change",
		Path: filepath.Join(t.TempDir(), "repo"),
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
		{"-C", value.Path, "push", "--no-verify", "--", value.CloneURL, "refs/heads/perpetual/change:refs/heads/perpetual/change"},
	}
	if !reflect.DeepEqual(git.calls, want) {
		t.Fatalf("git calls = %#v", git.calls)
	}
	if len(shell.commands) != 2 || !strings.Contains(shell.commands[1], "commit --no-verify") {
		t.Fatalf("shell commands = %#v", shell.commands)
	}
}

func TestServiceRefusesDirectBranchPublishing(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "ayati.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	value, err := store.Create(context.Background(), Create{
		Repository: "owner/project", CloneURL: "https://github.com/owner/project.git",
		BaseBranch: "main", Branch: "main",
		Path: filepath.Join(t.TempDir(), "repo"),
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	shell, git := &recordingShell{}, &recordingGit{}
	service := &Service{store: store, environment: &fakeEnvironment{shell: shell}, git: git}
	err = service.Publish(context.Background(), value.ID, "feat: direct", "octocat", "octocat@example.com")
	if err == nil || !strings.Contains(err.Error(), "working branch") {
		t.Fatalf("Publish error = %v", err)
	}
	if len(shell.commands) != 0 || len(git.calls) != 0 {
		t.Fatalf("publishing reached shell=%#v git=%#v", shell.commands, git.calls)
	}
}
