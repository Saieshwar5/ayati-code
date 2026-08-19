package webapp

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	appdatabase "github.com/Saieshwar5/perpetual/internal/database"
	compute "github.com/Saieshwar5/perpetual/internal/environment"
	"github.com/Saieshwar5/perpetual/internal/workspace"
)

type fakeImageResolver struct {
	digest string
	calls  []string
}

func (f *fakeImageResolver) ResolveImage(_ context.Context, image string) (string, error) {
	f.calls = append(f.calls, image)
	return f.digest, nil
}

func TestResolvePathsUsesPerpetualDirectories(t *testing.T) {
	configRoot := t.TempDir()
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configRoot)
	t.Setenv("HOME", home)

	paths, err := resolvePaths("", "")
	if err != nil {
		t.Fatalf("resolvePaths: %v", err)
	}
	if paths.database != filepath.Join(configRoot, "perpetual", "perpetual.db") {
		t.Fatalf("database = %q", paths.database)
	}
	if paths.workspaces != filepath.Join(home, ".local", "share", "perpetual", "workspaces") {
		t.Fatalf("workspaces = %q", paths.workspaces)
	}
}

func TestEnsureLocalEnvironmentCreatesOnlyFirstComputeSlot(t *testing.T) {
	database, err := appdatabase.Open(filepath.Join(t.TempDir(), "perpetual.db"))
	if err != nil {
		t.Fatalf("database.Open: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if _, err := workspace.NewStore(database); err != nil {
		t.Fatalf("workspace.NewStore: %v", err)
	}
	store, err := compute.NewStore(database)
	if err != nil {
		t.Fatalf("environment.NewStore: %v", err)
	}
	resolver := &fakeImageResolver{digest: "sha256:" + strings.Repeat("a", 64)}
	for range 2 {
		if err := ensureLocalEnvironment(context.Background(), store, resolver, "perpetual-sandbox:dev"); err != nil {
			t.Fatalf("ensureLocalEnvironment: %v", err)
		}
	}
	values, err := store.List(context.Background())
	if err != nil || len(values) != 1 || values[0].Name != "Local Docker" ||
		values[0].State != compute.StateAvailable || values[0].ImageDigest != resolver.digest ||
		len(resolver.calls) != 1 {
		t.Fatalf("environments = %#v, calls = %#v, error = %v", values, resolver.calls, err)
	}
}
