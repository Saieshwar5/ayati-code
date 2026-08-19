package provider

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/Saieshwar5/perpetual/internal/agent"
)

type Definition struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Protocol       string `json:"protocol"`
	Configured     bool   `json:"configured"`
	Configurable   bool   `json:"configurable"`
	SupportsTest   bool   `json:"supports_test"`
	SupportsModels bool   `json:"supports_models"`
	DefaultModel   string `json:"default_model,omitempty"`
}

type Registration struct {
	Definition   Definition
	Client       agent.Provider
	DefaultModel string
}

type Registry struct {
	mu            sync.RWMutex
	registrations map[string]Registration
}

func New(registrations ...Registration) (*Registry, error) {
	registry := &Registry{registrations: make(map[string]Registration, len(registrations))}
	for _, registration := range registrations {
		definition := registration.Definition
		definition.ID = strings.TrimSpace(definition.ID)
		definition.Name = strings.TrimSpace(definition.Name)
		definition.Protocol = strings.TrimSpace(definition.Protocol)
		if definition.ID == "" || definition.Name == "" || definition.Protocol == "" {
			return nil, errors.New("provider id, name, and protocol are required")
		}
		if !validID(definition.ID) {
			return nil, fmt.Errorf("provider id %q is invalid", definition.ID)
		}
		if _, exists := registry.registrations[definition.ID]; exists {
			return nil, fmt.Errorf("provider %q is registered more than once", definition.ID)
		}
		registration.Definition = definition
		registration.DefaultModel = strings.TrimSpace(registration.DefaultModel)
		registry.registrations[definition.ID] = registration
	}
	return registry, nil
}

func (r *Registry) Has(id string) bool {
	if r == nil {
		return false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, exists := r.registrations[strings.TrimSpace(id)]
	return exists
}

func (r *Registry) List() []Definition {
	if r == nil {
		return []Definition{}
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	values := make([]Definition, 0, len(r.registrations))
	for _, registration := range r.registrations {
		definition := registration.Definition
		definition.Configured = registration.Client != nil && registration.DefaultModel != ""
		if definition.Configured {
			definition.DefaultModel = registration.DefaultModel
		}
		values = append(values, definition)
	}
	sort.Slice(values, func(left, right int) bool {
		return values[left].Name < values[right].Name
	})
	return values
}

func (r *Registry) Resolve(id string) (agent.Provider, string, error) {
	id = strings.TrimSpace(id)
	r.mu.RLock()
	defer r.mu.RUnlock()
	registration, exists := r.registrations[id]
	if !exists {
		return nil, "", fmt.Errorf("provider %q is not available", id)
	}
	if registration.Client == nil || registration.DefaultModel == "" {
		return nil, "", fmt.Errorf("provider %q is not configured", id)
	}
	return registration.Client, registration.DefaultModel, nil
}

func (r *Registry) Configure(id string, client agent.Provider, defaultModel string) error {
	id, defaultModel = strings.TrimSpace(id), strings.TrimSpace(defaultModel)
	if client == nil || defaultModel == "" {
		return errors.New("provider client and default model are required")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	registration, exists := r.registrations[id]
	if !exists {
		return fmt.Errorf("provider %q is not available", id)
	}
	registration.Client = client
	registration.DefaultModel = defaultModel
	r.registrations[id] = registration
	return nil
}

func (r *Registry) Clear(id string) error {
	id = strings.TrimSpace(id)
	r.mu.Lock()
	defer r.mu.Unlock()
	registration, exists := r.registrations[id]
	if !exists {
		return fmt.Errorf("provider %q is not available", id)
	}
	registration.Client = nil
	registration.DefaultModel = ""
	r.registrations[id] = registration
	return nil
}

func validID(value string) bool {
	if len(value) > 64 {
		return false
	}
	for index, character := range value {
		if character >= 'a' && character <= 'z' || character >= '0' && character <= '9' ||
			index > 0 && (character == '-' || character == '_' || character == '.') {
			continue
		}
		return false
	}
	return true
}
