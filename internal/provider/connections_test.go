package provider

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Saieshwar5/perpetual/internal/agent"
	"github.com/Saieshwar5/perpetual/internal/config"
)

func TestConnectionsConfigureTestReloadAndRemove(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	var testedKey, testedModel, listedKey string
	specification := Specification{
		Definition: Definition{ID: "other", Name: "Other", Protocol: "test"},
		Factory: func(key string) (agent.Provider, error) {
			if key == "bad" {
				return nil, os.ErrPermission
			}
			return fakeClient{}, nil
		},
		Verifier: func(_ context.Context, key, model string) error {
			testedKey, testedModel = key, model
			return nil
		},
		Models: func(_ context.Context, key string) ([]string, error) {
			listedKey = key
			return []string{" model-b ", "model-a", "model-a", ""}, nil
		},
	}
	connections, err := NewConnections(path, specification)
	if err != nil {
		t.Fatalf("NewConnections: %v", err)
	}
	definition, err := connections.Configure("other", ConnectionInput{
		APIKey: "private-key", DefaultModel: "test-model",
	})
	if err != nil || !definition.Configured || definition.DefaultModel != "test-model" {
		t.Fatalf("definition = %#v, error = %v", definition, err)
	}
	if err := connections.Test(context.Background(), "other", ConnectionInput{}); err != nil {
		t.Fatalf("Test: %v", err)
	}
	if testedKey != "private-key" || testedModel != "test-model" {
		t.Fatalf("tested key = %q, model = %q", testedKey, testedModel)
	}
	models, err := connections.Models(context.Background(), "other")
	if err != nil || listedKey != "private-key" || len(models) != 2 ||
		models[0].ID != "model-a" || models[1].ID != "model-b" {
		t.Fatalf("models = %#v, listed key = %q, error = %v", models, listedKey, err)
	}
	reloaded, err := NewConnections(path, specification)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if _, model, err := reloaded.Registry().Resolve("other"); err != nil || model != "test-model" {
		t.Fatalf("model = %q, error = %v", model, err)
	}
	if err := reloaded.Remove("other"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	values, err := config.Load(path)
	if err != nil || len(values.Providers) != 0 {
		t.Fatalf("values = %#v, error = %v", values, err)
	}
	if _, _, err := reloaded.Registry().Resolve("other"); err == nil {
		t.Fatal("removed provider remains configured")
	}
	if _, err := reloaded.Models(context.Background(), "other"); err == nil {
		t.Fatal("Models accepted a removed provider connection")
	}
	data, err := os.ReadFile(path)
	if err != nil || strings.Contains(string(data), "private-key") {
		t.Fatalf("removed configuration = %q, error = %v", data, err)
	}
}

func TestConnectionsRejectUnsupportedAndExcessiveModels(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := config.Save(path, config.Values{Version: config.CurrentVersion,
		Providers: map[string]config.ProviderValues{
			"many": {APIKey: "key", DefaultModel: "model"},
			"none": {APIKey: "key", DefaultModel: "model"},
		}}); err != nil {
		t.Fatalf("config.Save: %v", err)
	}
	tooMany := make([]string, 10_001)
	connections, err := NewConnections(path,
		Specification{
			Definition: Definition{ID: "many", Name: "Many", Protocol: "test"},
			Factory:    func(string) (agent.Provider, error) { return fakeClient{}, nil },
			Models:     func(context.Context, string) ([]string, error) { return tooMany, nil },
		},
		Specification{
			Definition: Definition{ID: "none", Name: "None", Protocol: "test"},
			Factory:    func(string) (agent.Provider, error) { return fakeClient{}, nil },
		},
	)
	if err != nil {
		t.Fatalf("NewConnections: %v", err)
	}
	if _, err := connections.Models(context.Background(), "many"); err == nil {
		t.Fatal("Models accepted more than 10000 models")
	}
	if _, err := connections.Models(context.Background(), "none"); err == nil ||
		!strings.Contains(err.Error(), "does not support") {
		t.Fatalf("unsupported error = %v", err)
	}
}

func TestNormalizeModelsRejectsInvalidIDs(t *testing.T) {
	for _, id := range []string{strings.Repeat("x", 201), "model\nname"} {
		if _, err := normalizeModels([]string{id}); err == nil {
			t.Fatalf("normalizeModels accepted %q", id)
		}
	}
}

func TestConnectionsRejectIncompleteInputWithoutOverwritingConfiguration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	connections, err := NewConnections(path, Specification{
		Definition: Definition{ID: "other", Name: "Other", Protocol: "test"},
		Factory:    func(string) (agent.Provider, error) { return fakeClient{}, nil },
	})
	if err != nil {
		t.Fatalf("NewConnections: %v", err)
	}
	if _, err := connections.Configure("other", ConnectionInput{DefaultModel: "test"}); err == nil || !strings.Contains(err.Error(), "API key") {
		t.Fatalf("Configure error = %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("configuration file error = %v", err)
	}
}
