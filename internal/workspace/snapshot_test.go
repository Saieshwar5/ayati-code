package workspace

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Saieshwar5/perpetual/internal/exec"
)

func TestCaptureAndRestoreWorkspaceSnapshot(t *testing.T) {
	root := t.TempDir()
	git := newTestGit(t)
	source := filepath.Join(root, "source")
	initTestRepo(t, git, source, map[string]string{
		".gitignore":                "node_modules/\n",
		"package.json":              `{"name":"app"}`,
		"node_modules/dep/index.js": "export default 1;\n",
	})

	service := &Service{root: root, git: git}
	snapshot, err := service.captureWorkspaceSnapshot(context.Background(),
		Workspace{Path: source}, "version-1")
	if err != nil {
		t.Fatalf("captureWorkspaceSnapshot: %v", err)
	}
	if len(snapshot.Manifest) != 1 || snapshot.Manifest[0] != "node_modules/dep/index.js" ||
		snapshot.Bytes == 0 || snapshot.Ref == "" {
		t.Fatalf("snapshot = %#v", snapshot)
	}

	target := filepath.Join(root, "target")
	if err := os.MkdirAll(target, 0o700); err != nil {
		t.Fatalf("MkdirAll target: %v", err)
	}
	version := EnvironmentVersion{
		SnapshotType: snapshot.Type, SnapshotRef: snapshot.Ref,
		SnapshotManifest: snapshot.Manifest,
	}
	if err := service.restoreWorkspaceSnapshot(context.Background(),
		Workspace{Path: target}, version); err != nil {
		t.Fatalf("restoreWorkspaceSnapshot: %v", err)
	}
	content, err := os.ReadFile(filepath.Join(target, "node_modules", "dep", "index.js"))
	if err != nil || string(content) != "export default 1;\n" {
		t.Fatalf("restored file = %q, error = %v", content, err)
	}
}

func TestInitializeReusesSnapshotWithoutRunningSetup(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "perpetual.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	value := environmentProjectWorkspace(t, store, "reuse")
	if err := store.UpdateSetup(context.Background(), value.ID, "npm ci"); err != nil {
		t.Fatalf("UpdateSetup: %v", err)
	}
	root := t.TempDir()
	snapshotDir, snapshotBytes := writeSnapshotFixture(t, root, value.ID)
	environment, err := store.FindOrCreateEnvironment(context.Background(), "user-a", "owner/project", ".")
	if err != nil {
		t.Fatalf("FindOrCreateEnvironment: %v", err)
	}
	spec, err := BuildEnvironmentSpec(value.Path)
	if err != nil {
		t.Fatalf("BuildEnvironmentSpec: %v", err)
	}
	version, err := store.CreateEnvironmentVersion(context.Background(), environment.ID,
		spec.Fingerprint, spec, workspaceCachePath(value.Path))
	if err != nil {
		t.Fatalf("CreateEnvironmentVersion: %v", err)
	}
	if err := store.SetEnvironmentVersionState(context.Background(), version.ID,
		EnvironmentVersionReady, ""); err != nil {
		t.Fatalf("SetEnvironmentVersionState: %v", err)
	}
	if err := store.SetEnvironmentVersionSnapshot(context.Background(), version.ID,
		SnapshotTypeLocalCopy, snapshotDir, []string{"node_modules/dep/index.js"}, snapshotBytes); err != nil {
		t.Fatalf("SetEnvironmentVersionSnapshot: %v", err)
	}
	if err := store.BindWorkspaceEnvironment(context.Background(), value.ID, version.ID); err != nil {
		t.Fatalf("BindWorkspaceEnvironment: %v", err)
	}

	shell := &recordingShell{result: exec.ShellResult{ExitCode: 0}}
	service := &Service{
		store: store, runtime: &fakeRuntime{shell: shell},
		git: &recordingGit{statusResults: []string{"", "", ""}}, root: root,
	}
	if err := service.Initialize(context.Background(), value.ID); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	loaded, _ := store.Get(context.Background(), value.ID)
	if loaded.Status != StatusReady || loaded.Profile == nil ||
		loaded.Profile.SetupResult != "restored" {
		t.Fatalf("workspace = %#v", loaded)
	}
	if len(shell.commands) != 0 {
		t.Fatalf("setup ran when snapshot was restored: %#v", shell.commands)
	}
	content, err := os.ReadFile(filepath.Join(value.Path, "node_modules", "dep", "index.js"))
	if err != nil || !strings.Contains(string(content), "restored") {
		t.Fatalf("restored dependency = %q, error = %v", content, err)
	}
}

func TestInitializeFallsBackToSetupWithoutSnapshot(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "perpetual.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	value := environmentProjectWorkspace(t, store, "fallback")
	if err := store.UpdateSetup(context.Background(), value.ID, "npm ci"); err != nil {
		t.Fatalf("UpdateSetup: %v", err)
	}
	environment, err := store.FindOrCreateEnvironment(context.Background(), "user-a", "owner/project", ".")
	if err != nil {
		t.Fatalf("FindOrCreateEnvironment: %v", err)
	}
	spec, err := BuildEnvironmentSpec(value.Path)
	if err != nil {
		t.Fatalf("BuildEnvironmentSpec: %v", err)
	}
	version, err := store.CreateEnvironmentVersion(context.Background(), environment.ID,
		spec.Fingerprint, spec, workspaceCachePath(value.Path))
	if err != nil {
		t.Fatalf("CreateEnvironmentVersion: %v", err)
	}
	if err := store.SetEnvironmentVersionState(context.Background(), version.ID,
		EnvironmentVersionReady, ""); err != nil {
		t.Fatalf("SetEnvironmentVersionState: %v", err)
	}
	if err := store.BindWorkspaceEnvironment(context.Background(), value.ID, version.ID); err != nil {
		t.Fatalf("BindWorkspaceEnvironment: %v", err)
	}

	shell := &recordingShell{result: exec.ShellResult{ExitCode: 0}}
	service := &Service{
		store: store, runtime: &fakeRuntime{shell: shell},
		git: &recordingGit{statusResults: []string{"", "", ""}},
	}
	if err := service.Initialize(context.Background(), value.ID); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	if len(shell.commands) != 1 || shell.commands[0] != "npm ci" {
		t.Fatalf("fallback setup commands = %#v", shell.commands)
	}
	loaded, _ := store.Get(context.Background(), value.ID)
	if loaded.Status != StatusReady || loaded.Profile == nil ||
		loaded.Profile.SetupResult != "passed" {
		t.Fatalf("workspace = %#v", loaded)
	}
}

func TestBuildEnvironmentJobCapturesSnapshot(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "perpetual.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	root := t.TempDir()
	git := newTestGit(t)
	repo := filepath.Join(root, "repo")
	initTestRepo(t, git, repo, map[string]string{
		".gitignore":   "node_modules/\n",
		"package.json": `{"name":"app"}`,
	})
	writeEnvironmentFile(t, repo, "node_modules/dep/index.js", "ignored dependency\n")

	value, err := store.Create(context.Background(), Create{
		Repository: "owner/project", CloneURL: "https://github.com/owner/project.git",
		BaseBranch: "main", Branch: "main", Path: repo,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	environment, err := store.FindOrCreateEnvironment(context.Background(), "user-a", "owner/project", ".")
	if err != nil {
		t.Fatalf("FindOrCreateEnvironment: %v", err)
	}
	spec := EnvironmentSpec{ProjectRoot: ".", Fingerprint: "capture-fingerprint"}
	version, err := store.CreateEnvironmentVersion(context.Background(), environment.ID,
		spec.Fingerprint, spec, workspaceCachePath(repo))
	if err != nil {
		t.Fatalf("CreateEnvironmentVersion: %v", err)
	}
	if err := store.BindWorkspaceEnvironment(context.Background(), value.ID, version.ID); err != nil {
		t.Fatalf("BindWorkspaceEnvironment: %v", err)
	}
	if err := store.SaveProfile(context.Background(), value.ID, ProjectProfile{
		ProjectRoot: ".", SetupCommand: "npm ci", EnvironmentSpec: &spec,
	}); err != nil {
		t.Fatalf("SaveProfile: %v", err)
	}

	shell := &recordingShell{result: exec.ShellResult{ExitCode: 0}}
	service := &Service{
		store: store, root: root, git: git,
		runtime: &fakeRuntime{shell: shell},
	}
	if err := service.executeEnvironmentBuild(context.Background(), value.ID); err != nil {
		t.Fatalf("executeEnvironmentBuild: %v", err)
	}
	loaded, err := store.GetEnvironmentVersion(context.Background(), version.ID)
	if err != nil || loaded.State != EnvironmentVersionReady ||
		loaded.SnapshotType != SnapshotTypeLocalCopy || len(loaded.SnapshotManifest) == 0 {
		t.Fatalf("environment version = %#v, error = %v", loaded, err)
	}
	found := false
	for _, entry := range loaded.SnapshotManifest {
		if entry == "node_modules/dep/index.js" {
			found = true
		}
	}
	if !found {
		t.Fatalf("snapshot manifest = %#v", loaded.SnapshotManifest)
	}
}

func writeSnapshotFixture(t *testing.T, root, versionID string) (string, int64) {
	t.Helper()
	dir := filepath.Join(root, "environment-snapshots", versionID)
	path := filepath.Join(dir, "node_modules", "dep", "index.js")
	writeEnvironmentFile(t, root, filepath.Join("environment-snapshots", versionID,
		"node_modules", "dep", "index.js"), "restored dependency\n")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat snapshot: %v", err)
	}
	return dir, info.Size()
}

func newTestGit(t *testing.T) gitClient {
	t.Helper()
	git, err := newGitClient()
	if err != nil {
		t.Fatalf("newGitClient: %v", err)
	}
	return git
}

func initTestRepo(t *testing.T, git gitClient, path string, files map[string]string) {
	t.Helper()
	ctx := context.Background()
	if err := os.MkdirAll(path, 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := git.Run(ctx, "-C", path, "init", "-b", "main"); err != nil {
		t.Fatalf("git init: %v", err)
	}
	for name, content := range files {
		writeEnvironmentFile(t, path, name, content)
	}
	if err := git.Run(ctx, "-C", path, "add", "--all"); err != nil {
		t.Fatalf("git add: %v", err)
	}
	if err := git.Run(ctx, "-C", path, "-c", "user.name=test",
		"-c", "user.email=test@example.com", "commit", "-m", "init"); err != nil {
		t.Fatalf("git commit: %v", err)
	}
}
