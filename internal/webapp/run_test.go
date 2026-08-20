package webapp

import (
	"path/filepath"
	"testing"
)

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
