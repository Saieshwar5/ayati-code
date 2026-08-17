package provider

import (
	"path/filepath"
	"testing"
)

func TestBuiltinSpecificationsRegisterSixProviders(t *testing.T) {
	connections, err := NewConnections(filepath.Join(t.TempDir(), "config.json"), BuiltinSpecifications()...)
	if err != nil {
		t.Fatalf("NewConnections: %v", err)
	}
	values := connections.Registry().List()
	if len(values) != 6 {
		t.Fatalf("providers = %#v", values)
	}
	want := map[string]bool{
		"deepseek": true, "fireworks": true, "groq": true,
		"openai": true, "openrouter": true, "together": true,
	}
	for _, value := range values {
		if !want[value.ID] || !value.Configurable || value.Configured ||
			(value.ID == "fireworks" && value.SupportsModels) ||
			(value.ID != "fireworks" && !value.SupportsModels) {
			t.Errorf("provider = %#v", value)
		}
		delete(want, value.ID)
	}
	if len(want) != 0 {
		t.Fatalf("missing providers = %#v", want)
	}
}

func TestCompatibleProviderFactoriesRequireKeys(t *testing.T) {
	for _, specification := range BuiltinSpecifications()[1:] {
		if _, err := specification.Factory(""); err == nil {
			t.Errorf("%s accepted an empty key", specification.Definition.ID)
		}
	}
}
