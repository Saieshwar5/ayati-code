package githubapp

import (
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
