package config

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"reflect"
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
	want := Values{Version: CurrentVersion, Providers: map[string]ProviderValues{
		"fireworks": {APIKey: "secret", DefaultModel: "test-model"},
	}}
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
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if strings.Contains(string(data), "fireworks_api_key") ||
		!strings.Contains(string(data), `"version":1`) || !strings.Contains(string(data), `"providers"`) {
		t.Fatalf("persisted configuration = %s", data)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("values = %#v", got)
	}
}

func TestLoadMigratesLegacyFireworksConfiguration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	legacy := `{"fireworks_api_key":"secret","model":"test-model"}`
	if err := os.WriteFile(path, []byte(legacy), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	values, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	fireworks, exists := values.Provider("fireworks")
	if values.Version != CurrentVersion || !exists || fireworks.APIKey != "secret" ||
		fireworks.DefaultModel != "test-model" {
		t.Fatalf("values = %#v", values)
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

func TestLoadRejectsUnknownConfigurationVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	value := `{"version":2,"providers":{"fireworks":{"api_key":"secret","default_model":"test"}}}`
	if err := os.WriteFile(path, []byte(value), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if _, err := Load(path); err == nil || !strings.Contains(err.Error(), "unsupported configuration version") {
		t.Fatalf("Load error = %v", err)
	}
}

func TestSaveAllowsAnEmptyProviderConfiguration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := Save(path, Values{Version: CurrentVersion, Providers: map[string]ProviderValues{}}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	values, err := Load(path)
	if err != nil || len(values.Providers) != 0 {
		t.Fatalf("values = %#v, error = %v", values, err)
	}
}

func TestConfigureUpdatesModelWithoutExposingSavedKey(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	path, err := DefaultPath()
	if err != nil {
		t.Fatalf("DefaultPath: %v", err)
	}
	values := Values{}
	values.SetProvider("fireworks", ProviderValues{
		APIKey: "saved-secret", DefaultModel: "old-model",
	})
	if err := Save(path, values); err != nil {
		t.Fatalf("Save: %v", err)
	}
	var output, errorOutput bytes.Buffer
	code := Configure(context.Background(), strings.NewReader("\nnew-model\n"), &output, &errorOutput)
	if code != 0 || errorOutput.Len() != 0 {
		t.Fatalf("code = %d, error = %q", code, errorOutput.String())
	}
	values, err = Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	fireworks, exists := values.Provider("fireworks")
	if !exists || fireworks.APIKey != "saved-secret" || fireworks.DefaultModel != "new-model" {
		t.Fatalf("values = %#v", values)
	}
	if strings.Contains(output.String(), "saved-secret") {
		t.Fatal("output exposed the API key")
	}
}
