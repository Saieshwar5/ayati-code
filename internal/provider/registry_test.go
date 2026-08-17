package provider

import (
	"context"
	"strings"
	"testing"

	"github.com/Saieshwar5/ayati-code/internal/agent"
)

type fakeClient struct{}

func (fakeClient) Next(context.Context, agent.Request) (agent.Message, error) {
	return agent.Message{Role: "assistant", Content: "done"}, nil
}

func TestRegistryListsAndResolvesProviders(t *testing.T) {
	client := fakeClient{}
	registry, err := New(
		Registration{Definition: Definition{ID: "other", Name: "Other", Protocol: "test"}},
		Registration{
			Definition: Definition{ID: "configured", Name: "Configured", Protocol: "test"},
			Client:     client, DefaultModel: "test-model",
		},
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	values := registry.List()
	if len(values) != 2 || values[0].ID != "configured" || !values[0].Configured || values[1].Configured {
		t.Fatalf("providers = %#v", values)
	}
	resolved, model, err := registry.Resolve("configured")
	if err != nil || resolved == nil || model != "test-model" {
		t.Fatalf("resolved = %#v, model = %q, error = %v", resolved, model, err)
	}
	if _, _, err := registry.Resolve("other"); err == nil || !strings.Contains(err.Error(), "not configured") {
		t.Fatalf("unconfigured error = %v", err)
	}
}

func TestRegistryRejectsInvalidAndDuplicateProviders(t *testing.T) {
	for _, registrations := range [][]Registration{
		{{Definition: Definition{ID: "", Name: "Missing", Protocol: "test"}}},
		{{Definition: Definition{ID: "Not Valid", Name: "Invalid", Protocol: "test"}}},
		{
			{Definition: Definition{ID: "same", Name: "Same", Protocol: "test"}},
			{Definition: Definition{ID: "same", Name: "Duplicate", Protocol: "test"}},
		},
	} {
		if _, err := New(registrations...); err == nil {
			t.Fatalf("New accepted %#v", registrations)
		}
	}
}
