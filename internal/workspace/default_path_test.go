package workspace

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultPathUsesPerpetualDatabase(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", root)
	path, err := DefaultPath()
	if err != nil || path != filepath.Join(root, "perpetual", "perpetual.db") {
		t.Fatalf("path = %q, error = %v", path, err)
	}
}

func TestDefaultPathReusesLegacyDatabase(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", root)
	legacy := filepath.Join(root, "ayati", "ayati.db")
	if err := os.MkdirAll(filepath.Dir(legacy), 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(legacy, nil, 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	path, err := DefaultPath()
	if err != nil || path != legacy {
		t.Fatalf("path = %q, error = %v", path, err)
	}
}
