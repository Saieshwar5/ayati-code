package workspace

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Saieshwar5/perpetual/internal/exec"
)

func TestStoreCreatesAndFindsEnvironmentVersions(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "perpetual.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	environment, err := store.FindOrCreateEnvironment(context.Background(), "user-a", "owner/project", ".")
	if err != nil {
		t.Fatalf("FindOrCreateEnvironment: %v", err)
	}
	again, err := store.FindOrCreateEnvironment(context.Background(), "user-a", "owner/project", ".")
	if err != nil || again.ID != environment.ID {
		t.Fatalf("duplicate environment = %#v, error = %v", again, err)
	}
	other, err := store.FindOrCreateEnvironment(context.Background(), "user-a", "owner/project", "apps/web")
	if err != nil || other.ID == environment.ID {
		t.Fatalf("different root environment = %#v, error = %v", other, err)
	}

	spec := EnvironmentSpec{ProjectRoot: ".", Fingerprint: "fingerprint-a"}
	version, err := store.CreateEnvironmentVersion(context.Background(), environment.ID,
		spec.Fingerprint, spec, "cache-a")
	if err != nil {
		t.Fatalf("CreateEnvironmentVersion: %v", err)
	}
	if version.Version != 1 || version.State != EnvironmentVersionPending {
		t.Fatalf("version = %#v", version)
	}
	if err := store.SetEnvironmentVersionState(context.Background(), version.ID,
		EnvironmentVersionReady, ""); err != nil {
		t.Fatalf("SetEnvironmentVersionState: %v", err)
	}
	found, ok, err := store.FindReadyEnvironmentVersion(context.Background(),
		"user-a", environment.ID, "fingerprint-a")
	if err != nil || !ok || found.ID != version.ID {
		t.Fatalf("ready version = %#v, ok = %v, error = %v", found, ok, err)
	}
	second, err := store.CreateEnvironmentVersion(context.Background(), environment.ID,
		"fingerprint-b", EnvironmentSpec{Fingerprint: "fingerprint-b"}, "cache-b")
	if err != nil || second.Version != 2 {
		t.Fatalf("second version = %#v, error = %v", second, err)
	}
}

func TestStoreBindsWorkspaceToEnvironmentVersion(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "perpetual.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	value, err := store.Create(context.Background(), Create{
		Repository: "owner/project", CloneURL: "https://github.com/owner/project.git",
		BaseBranch: "main", Branch: "main", Path: filepath.Join(t.TempDir(), "repo"),
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	environment, err := store.FindOrCreateEnvironment(context.Background(), "user-a", "owner/project", ".")
	if err != nil {
		t.Fatalf("FindOrCreateEnvironment: %v", err)
	}
	version, err := store.CreateEnvironmentVersion(context.Background(), environment.ID,
		"fingerprint", EnvironmentSpec{Fingerprint: "fingerprint"}, "")
	if err != nil {
		t.Fatalf("CreateEnvironmentVersion: %v", err)
	}
	if err := store.BindWorkspaceEnvironment(context.Background(), value.ID, version.ID); err != nil {
		t.Fatalf("BindWorkspaceEnvironment: %v", err)
	}
	loaded, err := store.Get(context.Background(), value.ID)
	if err != nil || loaded.EnvironmentVersionID != version.ID {
		t.Fatalf("workspace = %#v, error = %v", loaded, err)
	}
	loadedVersion, err := store.GetEnvironmentVersion(context.Background(), version.ID)
	if err != nil || loadedVersion.State != EnvironmentVersionPending ||
		loadedVersion.SourceFingerprint != "fingerprint" {
		t.Fatalf("environment version = %#v, error = %v", loadedVersion, err)
	}
}

func TestInitializeReusesEnvironmentVersionAcrossWorkspaces(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "perpetual.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	first := environmentProjectWorkspace(t, store, "first")
	second := environmentProjectWorkspace(t, store, "second")
	service := &Service{
		store:   store,
		runtime: &fakeRuntime{shell: &recordingShell{result: exec.ShellResult{ExitCode: 0}}},
		git:     &recordingGit{},
	}
	if err := service.Initialize(context.Background(), first.ID); err != nil {
		t.Fatalf("Initialize first: %v", err)
	}
	if err := service.RunNextJob(context.Background()); err != nil {
		t.Fatalf("run first environment build: %v", err)
	}
	if err := service.Initialize(context.Background(), second.ID); err != nil {
		t.Fatalf("Initialize second: %v", err)
	}
	loadedFirst, _ := store.Get(context.Background(), first.ID)
	loadedSecond, _ := store.Get(context.Background(), second.ID)
	if loadedFirst.EnvironmentVersionID == "" ||
		loadedFirst.EnvironmentVersionID != loadedSecond.EnvironmentVersionID {
		t.Fatalf("environment bindings = %q and %q",
			loadedFirst.EnvironmentVersionID, loadedSecond.EnvironmentVersionID)
	}
	var count int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM environment_versions`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("environment version count = %d, error = %v", count, err)
	}
}

func TestInitializeMarksFailedEnvironmentVersion(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "perpetual.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	value := environmentProjectWorkspace(t, store, "failed")
	if err := store.UpdateSetup(context.Background(), value.ID, "npm ci"); err != nil {
		t.Fatalf("UpdateSetup: %v", err)
	}
	service := &Service{
		store: store,
		runtime: &fakeRuntime{shell: &recordingShell{
			result: exec.ShellResult{ExitCode: 1, Stderr: "npm not found"},
		}},
		git: &recordingGit{},
	}
	if err := service.Initialize(context.Background(), value.ID); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	if err := service.RunNextJob(context.Background()); err != nil {
		t.Fatalf("run environment build: %v", err)
	}
	loaded, _ := store.Get(context.Background(), value.ID)
	if loaded.EnvironmentVersionID == "" || loaded.Status != StatusInitializationFailed {
		t.Fatalf("workspace = %#v", loaded)
	}
	version, err := store.GetEnvironmentVersion(context.Background(), loaded.EnvironmentVersionID)
	if err != nil || version.State != EnvironmentVersionFailed ||
		!strings.Contains(version.Error, "npm not found") {
		t.Fatalf("environment version = %#v, error = %v", version, err)
	}
}

func TestMigrationBackfillsEnvironmentBindings(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "perpetual.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	root := t.TempDir()
	writeEnvironmentFile(t, root, "package.json", `{"name":"app"}`)
	spec, err := BuildEnvironmentSpec(root)
	if err != nil {
		t.Fatalf("BuildEnvironmentSpec: %v", err)
	}
	value := environmentProjectWorkspace(t, store, "backfill")
	profile := ProjectProfile{
		ProjectRoot: ".", Languages: []string{"Node.js"},
		EnvironmentSpec: &spec, CachePath: filepath.Join(root, "cache"),
	}
	if err := store.SaveProfile(context.Background(), value.ID, profile); err != nil {
		t.Fatalf("SaveProfile: %v", err)
	}
	if _, err := store.db.Exec(`UPDATE workspaces SET environment_version_id = '' WHERE id = ?`, value.ID); err != nil {
		t.Fatalf("clear binding: %v", err)
	}
	if err := store.migrateEnvironmentVersions(context.Background()); err != nil {
		t.Fatalf("migrateEnvironmentVersions: %v", err)
	}
	loaded, _ := store.Get(context.Background(), value.ID)
	if loaded.EnvironmentVersionID == "" {
		t.Fatalf("workspace not bound after backfill: %#v", loaded)
	}
	version, err := store.GetEnvironmentVersion(context.Background(), loaded.EnvironmentVersionID)
	if err != nil || version.State != EnvironmentVersionReady {
		t.Fatalf("environment version = %#v, error = %v", version, err)
	}
}

func environmentProjectWorkspace(t *testing.T, store *Store, name string) Workspace {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.MkdirAll(path, 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	writeEnvironmentFile(t, path, "package.json", `{"name":"app"}`)
	if err := os.MkdirAll(filepath.Join(path, ".git"), 0o700); err != nil {
		t.Fatalf("MkdirAll .git: %v", err)
	}
	value, err := store.Create(context.Background(), Create{
		UserID: "user-a", Repository: "owner/project",
		CloneURL:   "https://github.com/owner/project.git",
		BaseBranch: "main", Branch: name, Path: path,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	return value
}
