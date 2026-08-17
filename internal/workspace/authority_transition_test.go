package workspace

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

func TestChangeAuthorityEnablesDevelopOnLocalBranch(t *testing.T) {
	store, value := readyAuthorityWorkspace(t, AuthorityExplore, "main", false)
	environment := &fakeEnvironment{}
	git := &recordingGit{}
	service := &Service{store: store, environment: environment, git: git}

	updated, err := service.ChangeAuthority(context.Background(), value.ID, AuthorityChange{
		Authority: AuthorityDevelop, Branch: "ayati/change", CreateBranch: true,
	})
	if err != nil {
		t.Fatalf("ChangeAuthority: %v", err)
	}
	if updated.Authority != AuthorityDevelop || updated.Branch != "ayati/change" ||
		!updated.CreateBranch || updated.EffectiveMountMode != "rw" || updated.Status != StatusReady {
		t.Fatalf("workspace = %#v", updated)
	}
	if len(environment.removed) != 1 || len(environment.ensured) != 1 ||
		!environment.ensured[0].WorkspaceWritable {
		t.Fatalf("sandbox lifecycle = removed %#v, ensured %#v", environment.removed, environment.ensured)
	}
	wantCalls := [][]string{
		{"check-ref-format", "--branch", "ayati/change"},
		{"-C", value.Path, "switch", "-c", "ayati/change"},
	}
	assertGitCalls(t, git.calls, wantCalls)
}

func TestChangeAuthorityFreezesDevelopChangesWithoutSwitchingBranch(t *testing.T) {
	store, value := readyAuthorityWorkspace(t, AuthorityDevelop, "ayati/change", true)
	environment := &fakeEnvironment{}
	git := &recordingGit{}
	service := &Service{store: store, environment: environment, git: git}

	updated, err := service.ChangeAuthority(context.Background(), value.ID,
		AuthorityChange{Authority: AuthorityExplore})
	if err != nil {
		t.Fatalf("ChangeAuthority: %v", err)
	}
	if updated.Authority != AuthorityExplore || updated.Branch != "ayati/change" ||
		updated.EffectiveMountMode != "ro" || len(git.calls) != 0 ||
		environment.ensured[0].WorkspaceWritable {
		t.Fatalf("workspace = %#v, git = %#v, sandbox = %#v", updated, git.calls, environment.ensured)
	}
}

func TestChangeAuthorityRestoresExploreWhenDevelopMountFails(t *testing.T) {
	store, value := readyAuthorityWorkspace(t, AuthorityExplore, "main", false)
	environment := &fakeEnvironment{ensureErrs: []error{errors.New("docker unavailable"), nil}}
	git := &recordingGit{}
	service := &Service{store: store, environment: environment, git: git}

	_, err := service.ChangeAuthority(context.Background(), value.ID, AuthorityChange{
		Authority: AuthorityDevelop, Branch: "ayati/change", CreateBranch: true,
	})
	if err == nil || !strings.Contains(err.Error(), "docker unavailable") {
		t.Fatalf("ChangeAuthority error = %v", err)
	}
	loaded, loadErr := store.Get(context.Background(), value.ID)
	if loadErr != nil || loaded.Authority != AuthorityExplore || loaded.Branch != "main" ||
		loaded.Status != StatusReady || loaded.EffectiveMountMode != "ro" || loaded.Error != "" {
		t.Fatalf("workspace = %#v, error = %v", loaded, loadErr)
	}
	if len(environment.ensured) != 2 || !environment.ensured[0].WorkspaceWritable ||
		environment.ensured[1].WorkspaceWritable {
		t.Fatalf("sandbox lifecycle = %#v", environment.ensured)
	}
	wantCalls := [][]string{
		{"check-ref-format", "--branch", "ayati/change"},
		{"-C", value.Path, "switch", "-c", "ayati/change"},
		{"-C", value.Path, "switch", "main"},
		{"-C", value.Path, "branch", "-D", "ayati/change"},
	}
	assertGitCalls(t, git.calls, wantCalls)
}

func TestChangeAuthorityRejectsActiveAgentAndInvalidBranch(t *testing.T) {
	store, value := readyAuthorityWorkspace(t, AuthorityExplore, "main", false)
	sessions, _ := store.ListSessions(context.Background(), value.ID)
	if err := store.UpdateSessionStatus(context.Background(), sessions[0].ID,
		SessionStatusWorking, ""); err != nil {
		t.Fatalf("UpdateSessionStatus: %v", err)
	}
	service := &Service{store: store, environment: &fakeEnvironment{}, git: &recordingGit{}}
	_, err := service.ChangeAuthority(context.Background(), value.ID, AuthorityChange{
		Authority: AuthorityDevelop, Branch: "ayati/change", CreateBranch: true,
	})
	if err == nil || !strings.Contains(err.Error(), "agent is working") {
		t.Fatalf("active ChangeAuthority error = %v", err)
	}
	if err := store.UpdateSessionStatus(context.Background(), sessions[0].ID,
		SessionStatusIdle, ""); err != nil {
		t.Fatalf("UpdateSessionStatus: %v", err)
	}
	service.git = &recordingGit{runErrors: []error{errors.New("invalid")}}
	_, err = service.ChangeAuthority(context.Background(), value.ID, AuthorityChange{
		Authority: AuthorityDevelop, Branch: "bad branch", CreateBranch: true,
	})
	if err == nil || !strings.Contains(err.Error(), "invalid working branch") {
		t.Fatalf("invalid branch error = %v", err)
	}
	service.git = &recordingGit{}
	_, err = service.ChangeAuthority(context.Background(), value.ID, AuthorityChange{
		Authority: AuthorityDevelop, Branch: "existing/change", CreateBranch: false,
	})
	if err == nil || !strings.Contains(err.Error(), "requires creating a new local") {
		t.Fatalf("existing branch error = %v", err)
	}
}

func readyAuthorityWorkspace(
	t *testing.T, authority Authority, branch string, createBranch bool,
) (*Store, Workspace) {
	t.Helper()
	store, err := Open(filepath.Join(t.TempDir(), "ayati.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	value, err := store.Create(context.Background(), Create{
		Repository: "owner/project", CloneURL: "https://github.com/owner/project.git",
		BaseBranch: "main", Branch: branch, CreateBranch: createBranch,
		Authority: authority, Path: filepath.Join(t.TempDir(), "repo"),
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := store.CompletePreparation(context.Background(), value.ID); err != nil {
		t.Fatalf("CompletePreparation: %v", err)
	}
	if err := store.UpdateEffectiveMountMode(context.Background(), value.ID,
		string(authority.MountMode())); err != nil {
		t.Fatalf("UpdateEffectiveMountMode: %v", err)
	}
	return store, value
}

func assertGitCalls(t *testing.T, got, want [][]string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("git calls = %#v, want %#v", got, want)
	}
	for index := range want {
		if strings.Join(got[index], "\x00") != strings.Join(want[index], "\x00") {
			t.Fatalf("git calls = %#v, want %#v", got, want)
		}
	}
}
