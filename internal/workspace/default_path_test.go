package workspace

import (
	"path/filepath"
	"testing"
)

func TestDefaultPathUsesPerpetualConfigDirectory(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", root)

	path, err := DefaultPath()
	if err != nil {
		t.Fatalf("DefaultPath: %v", err)
	}
	if path != filepath.Join(root, "perpetual", "perpetual.db") {
		t.Fatalf("path = %q", path)
	}
}
