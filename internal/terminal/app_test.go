package terminal

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sai-eshwar/no-nonsense-coding-ai/internal/config"
)

func TestSetupSavesKeyAndDefaultModel(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	t.Setenv(config.EnvPath, path)
	var output, errors bytes.Buffer
	app := App{Input: strings.NewReader("fw_test_secret\n\n"), Output: &output, Error: &errors}
	if err := app.Run(context.Background(), []string{"setup"}); err != nil {
		t.Fatalf("setup: %v", err)
	}
	values, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if values["FIREWORKS_API_KEY"] != "fw_test_secret" {
		t.Fatal("API key was not saved")
	}
	if values["NCA_MODEL"] != defaultModel {
		t.Fatalf("model = %q, want %q", values["NCA_MODEL"], defaultModel)
	}
	if strings.Contains(output.String(), "fw_test_secret") {
		t.Fatal("setup printed the API key")
	}
}

func TestModelCommandPersistsModel(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	t.Setenv(config.EnvPath, path)
	app := App{Input: strings.NewReader(""), Output: &bytes.Buffer{}, Error: &bytes.Buffer{}}
	if err := app.Run(context.Background(), []string{"model", "accounts/fireworks/models/test"}); err != nil {
		t.Fatalf("model command: %v", err)
	}
	values, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if values["NCA_MODEL"] != "accounts/fireworks/models/test" {
		t.Fatalf("model was not persisted: %#v", values)
	}
}

func TestConfigShowMasksKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(path, []byte("FIREWORKS_API_KEY=fw_super_secret_value\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(config.EnvPath, path)
	var output bytes.Buffer
	app := App{Input: strings.NewReader(""), Output: &output, Error: &bytes.Buffer{}}
	if err := app.Run(context.Background(), []string{"config", "show"}); err != nil {
		t.Fatalf("config show: %v", err)
	}
	if strings.Contains(output.String(), "fw_super_secret_value") {
		t.Fatal("config show exposed the API key")
	}
}
