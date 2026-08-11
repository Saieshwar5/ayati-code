package app

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Saieshwar5/ayati-code/internal/config"
)

func TestVersionNeedsNoConfiguration(t *testing.T) {
	var output bytes.Buffer
	var errorOutput bytes.Buffer
	code := Run(context.Background(), []string{"--version"}, strings.NewReader(""), &output, &errorOutput)
	if code != 0 || strings.TrimSpace(output.String()) != "ayati dev" {
		t.Fatalf("code = %d, output = %q, error = %q", code, output.String(), errorOutput.String())
	}
}

func TestRunCreatesConfigurationOnFirstUse(t *testing.T) {
	configRoot := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configRoot)
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	var output bytes.Buffer
	var errorOutput bytes.Buffer
	code := Run(context.Background(), []string{"--workspace", t.TempDir()}, strings.NewReader("secret\ntest-model\n/quit\n"), &output, &errorOutput)
	if code != 0 {
		t.Fatalf("code = %d, error = %q", code, errorOutput.String())
	}
	values, err := config.Load(filepath.Join(configRoot, "ayati", "config.json"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if values.FireworksAPIKey != "secret" || values.Model != "test-model" {
		t.Fatalf("configuration = %#v", values)
	}
	if strings.Contains(output.String(), "secret") {
		t.Fatal("terminal output exposed the API key")
	}
}

func TestConfigCommandUpdatesModelAndKeepsKey(t *testing.T) {
	configRoot := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configRoot)
	path := filepath.Join(configRoot, "ayati", "config.json")
	if err := config.Save(path, config.Values{FireworksAPIKey: "saved-secret", Model: "old-model"}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	var output bytes.Buffer
	var errorOutput bytes.Buffer
	code := Run(context.Background(), []string{"config"}, strings.NewReader("\nnew-model\n"), &output, &errorOutput)
	if code != 0 {
		t.Fatalf("code = %d, error = %q", code, errorOutput.String())
	}
	values, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if values.FireworksAPIKey != "saved-secret" || values.Model != "new-model" {
		t.Fatalf("configuration = %#v", values)
	}
}

func TestQuitCommandExits(t *testing.T) {
	handled, exit := handleCommand("/quit", nil, "", nil, nil)
	if !handled || !exit {
		t.Fatalf("handled = %t, exit = %t", handled, exit)
	}
}
