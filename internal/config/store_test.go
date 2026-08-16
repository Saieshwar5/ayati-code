package config

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultPathUsesXDGConfigHome(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", root)
	path, err := DefaultPath()
	if err != nil {
		t.Fatalf("DefaultPath: %v", err)
	}
	if path != filepath.Join(root, "ayati", "config.json") {
		t.Fatalf("path = %q", path)
	}
}

func TestSaveAndLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "config.json")
	want := Values{FireworksAPIKey: "secret", Model: "test-model"}
	if err := Save(path, want); err != nil {
		t.Fatalf("Save: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("permissions = %o", info.Mode().Perm())
	}
	directoryInfo, err := os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatalf("Stat directory: %v", err)
	}
	if directoryInfo.Mode().Perm() != 0o700 {
		t.Fatalf("directory permissions = %o", directoryInfo.Mode().Perm())
	}
	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got != want {
		t.Fatalf("values = %#v", got)
	}
}

func TestLoadRejectsIncompleteConfiguration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"model":"test"}`), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("Load accepted configuration without an API key")
	}
}

func TestConfigureUpdatesModelWithoutExposingSavedKey(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	path, err := DefaultPath()
	if err != nil {
		t.Fatalf("DefaultPath: %v", err)
	}
	if err := Save(path, Values{FireworksAPIKey: "saved-secret", Model: "old-model"}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	var output, errorOutput bytes.Buffer
	code := Configure(context.Background(), strings.NewReader("\nnew-model\n"), &output, &errorOutput)
	if code != 0 || errorOutput.Len() != 0 {
		t.Fatalf("code = %d, error = %q", code, errorOutput.String())
	}
	values, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if values.FireworksAPIKey != "saved-secret" || values.Model != "new-model" {
		t.Fatalf("values = %#v", values)
	}
	if strings.Contains(output.String(), "saved-secret") {
		t.Fatal("output exposed the API key")
	}
}
