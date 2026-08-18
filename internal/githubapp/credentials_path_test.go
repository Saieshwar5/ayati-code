package githubapp

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultCredentialsPathUsesPerpetualDirectory(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", root)
	path, err := DefaultCredentialsPath()
	if err != nil || path != filepath.Join(root, "perpetual", "github.json") {
		t.Fatalf("path = %q, error = %v", path, err)
	}
}

func TestDefaultCredentialsPathReusesLegacyFile(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", root)
	legacy := filepath.Join(root, "ayati", "github.json")
	if err := os.MkdirAll(filepath.Dir(legacy), 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(legacy, []byte(`{}`), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	path, err := DefaultCredentialsPath()
	if err != nil || path != legacy {
		t.Fatalf("path = %q, error = %v", path, err)
	}
}
