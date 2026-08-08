package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSaveLoadAndPermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	want := Values{"FIREWORKS_API_KEY": "secret value", "NCA_MODEL": "model-one"}
	if err := Save(path, want); err != nil {
		t.Fatalf("Save: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("permissions are %o, want 600", info.Mode().Perm())
	}
	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got["FIREWORKS_API_KEY"] != want["FIREWORKS_API_KEY"] || got["NCA_MODEL"] != want["NCA_MODEL"] {
		t.Fatalf("loaded values differ: %#v", got)
	}
}

func TestEffectiveEnvironmentOverridesFile(t *testing.T) {
	t.Setenv("NCA_MODEL", "environment-model")
	got := Effective(Values{"NCA_MODEL": "file-model"}, "NCA_MODEL", "default-model")
	if got != "environment-model" {
		t.Fatalf("Effective = %q", got)
	}
}

func TestPathPrefersAyatiCodeEnvironment(t *testing.T) {
	want := filepath.Join(t.TempDir(), "ayati-code.env")
	t.Setenv(EnvPath, want)
	t.Setenv(LegacyEnvPath, filepath.Join(t.TempDir(), "ayati-micro.env"))

	got, err := Path()
	if err != nil {
		t.Fatalf("Path: %v", err)
	}
	if got != want {
		t.Fatalf("Path = %q, want %q", got, want)
	}
}

func TestPathFallsBackToAyatiMicroEnvironment(t *testing.T) {
	want := filepath.Join(t.TempDir(), "ayati-micro.env")
	t.Setenv(EnvPath, "")
	t.Setenv(LegacyEnvPath, want)

	got, err := Path()
	if err != nil {
		t.Fatalf("Path: %v", err)
	}
	if got != want {
		t.Fatalf("Path = %q, want %q", got, want)
	}
}

func TestLoadRejectsUnknownKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(path, []byte("UNKNOWN=value\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("expected unknown key error")
	}
}

func TestMaskDoesNotRevealSecret(t *testing.T) {
	masked := Mask("fw_1234567890")
	if masked == "fw_1234567890" || masked == "" {
		t.Fatalf("unsafe mask %q", masked)
	}
}
