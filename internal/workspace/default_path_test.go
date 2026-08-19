package workspace

import (
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
