package provider

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/Saieshwar5/ayati-code/internal/agent"
)

type Definition struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Protocol   string `json:"protocol"`
	Configured bool   `json:"configured"`
}

type Registration struct {
	Definition   Definition
	Client       agent.Provider
	DefaultModel string
}

type Registry struct {
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
	_, exists := r.registrations[strings.TrimSpace(id)]
	return exists
}

func (r *Registry) List() []Definition {
	if r == nil {
		return []Definition{}
	}
	values := make([]Definition, 0, len(r.registrations))
	for _, registration := range r.registrations {
		definition := registration.Definition
		definition.Configured = registration.Client != nil && registration.DefaultModel != ""
		values = append(values, definition)
	}
	sort.Slice(values, func(left, right int) bool {
		return values[left].Name < values[right].Name
	})
	return values
}

func (r *Registry) Resolve(id string) (agent.Provider, string, error) {
	id = strings.TrimSpace(id)
	registration, exists := r.registrations[id]
	if !exists {
		return nil, "", fmt.Errorf("provider %q is not available", id)
	}
	if registration.Client == nil || registration.DefaultModel == "" {
		return nil, "", fmt.Errorf("provider %q is not configured", id)
	}
	return registration.Client, registration.DefaultModel, nil
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
