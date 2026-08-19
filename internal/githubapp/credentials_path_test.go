package githubapp

import (
	"path/filepath"
	"testing"
)

func TestDefaultCredentialsPathUsesPerpetualConfigDirectory(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", root)

	path, err := DefaultCredentialsPath()
	if err != nil {
		t.Fatalf("DefaultCredentialsPath: %v", err)
	}
	if path != filepath.Join(root, "perpetual", "github.json") {
		t.Fatalf("path = %q", path)
	}
}
