package install

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLinkCreatesAyatiMicroSymlink(t *testing.T) {
	root := t.TempDir()
	executable := filepath.Join(root, "source", "ayati-micro")
	if err := os.MkdirAll(filepath.Dir(executable), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(executable, []byte("binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	target, err := Link(executable, filepath.Join(root, "bin"))
	if err != nil {
		t.Fatalf("Link: %v", err)
	}
	linked, err := os.Readlink(target)
	if err != nil {
		t.Fatalf("Readlink: %v", err)
	}
	if linked != executable {
		t.Fatalf("link points to %q, want %q", linked, executable)
	}
}

func TestLinkRefusesToReplaceRegularFile(t *testing.T) {
	binDir := t.TempDir()
	target := filepath.Join(binDir, "ayati-micro")
	if err := os.WriteFile(target, []byte("keep"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := Link("/some/executable", binDir); err == nil {
		t.Fatal("expected refusal to replace regular file")
	}
}
