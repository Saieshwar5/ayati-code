package webapp

import (
	"context"
	"os"
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

func TestPreferredWorkspaceRootUsesPerpetualForNewInstall(t *testing.T) {
	home := t.TempDir()
	want := filepath.Join(home, ".local", "share", "perpetual", "workspaces")
	if got := preferredWorkspaceRoot(home); got != want {
		t.Fatalf("root = %q", got)
	}
}

func TestPreferredWorkspaceRootReusesLegacyWorkspaces(t *testing.T) {
	home := t.TempDir()
	legacy := filepath.Join(home, ".local", "share", "ayati", "workspaces")
	if err := os.MkdirAll(legacy, 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if got := preferredWorkspaceRoot(home); got != legacy {
		t.Fatalf("root = %q", got)
	}
}

func TestEnvironmentValuePrefersPerpetualAndFallsBackToLegacy(t *testing.T) {
	t.Setenv("PERPETUAL_ADDRESS", "new-address")
	t.Setenv("AYATI_ADDRESS", "legacy-address")
	if value, legacy := envOrLegacy("PERPETUAL_ADDRESS", "AYATI_ADDRESS", "fallback"); value != "new-address" || legacy {
		t.Fatalf("value = %q, legacy = %v", value, legacy)
	}
	t.Setenv("PERPETUAL_ADDRESS", "")
	if value, legacy := envOrLegacy("PERPETUAL_ADDRESS", "AYATI_ADDRESS", "fallback"); value != "legacy-address" || !legacy {
		t.Fatalf("value = %q, legacy = %v", value, legacy)
	}
}

func (f *fakeImageResolver) ResolveImage(_ context.Context, image string) (string, error) {
	f.calls = append(f.calls, image)
	return f.digest, nil
}

func TestEnsureLocalEnvironmentCreatesOnlyFirstComputeSlot(t *testing.T) {
	database, err := appdatabase.Open(filepath.Join(t.TempDir(), "ayati.db"))
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
