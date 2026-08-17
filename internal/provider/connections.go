package provider

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	"github.com/Saieshwar5/ayati-code/internal/agent"
	"github.com/Saieshwar5/ayati-code/internal/config"
)

type ConnectionInput struct {
	APIKey       string `json:"api_key"`
	DefaultModel string `json:"default_model"`
}

type Factory func(string) (agent.Provider, error)
type Verifier func(context.Context, string, string) error
type ModelLister func(context.Context, string) ([]string, error)

type Model struct {
	ID string `json:"id"`
}

type Specification struct {
	Definition Definition
	Factory    Factory
	Verifier   Verifier
	Models     ModelLister
}

type Connections struct {
	mu       sync.Mutex
	path     string
	registry *Registry
	specs    map[string]Specification
}

func NewConnections(path string, specifications ...Specification) (*Connections, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("provider configuration path is required")
	}
	values, err := loadValues(path)
	if err != nil {
		return nil, err
	}
	registrations := make([]Registration, 0, len(specifications))
	specs := make(map[string]Specification, len(specifications))
	for _, specification := range specifications {
		definition := specification.Definition
		definition.Configurable = specification.Factory != nil
		definition.SupportsTest = specification.Verifier != nil
		definition.SupportsModels = specification.Models != nil
		specification.Definition = definition
		registration := Registration{Definition: definition}
		if settings, exists := values.Provider(definition.ID); exists {
			if specification.Factory == nil {
				return nil, fmt.Errorf("provider %q cannot be configured", definition.ID)
			}
			client, clientErr := specification.Factory(settings.APIKey)
			if clientErr != nil {
				return nil, fmt.Errorf("configure provider %q: %w", definition.ID, clientErr)
			}
			registration.Client = client
			registration.DefaultModel = settings.DefaultModel
		}
		registrations = append(registrations, registration)
		specs[definition.ID] = specification
	}
	registry, err := New(registrations...)
	if err != nil {
		return nil, err
	}
	return &Connections{path: path, registry: registry, specs: specs}, nil
}

func (c *Connections) Registry() *Registry { return c.registry }

func (c *Connections) Configure(id string, input ConnectionInput) (Definition, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	specification, values, settings, err := c.resolve(id, input)
	if err != nil {
		return Definition{}, err
	}
	client, err := specification.Factory(settings.APIKey)
	if err != nil {
		return Definition{}, err
	}
	values.SetProvider(specification.Definition.ID, settings)
	if err := config.Save(c.path, values); err != nil {
		return Definition{}, err
	}
	if err := c.registry.Configure(specification.Definition.ID, client, settings.DefaultModel); err != nil {
		return Definition{}, err
	}
	return c.definition(specification.Definition.ID)
}

func (c *Connections) Test(ctx context.Context, id string, input ConnectionInput) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	specification, _, settings, err := c.resolve(id, input)
	if err != nil {
		return err
	}
	if specification.Verifier == nil {
		return fmt.Errorf("provider %q does not support connection tests", specification.Definition.ID)
	}
	return specification.Verifier(ctx, settings.APIKey, settings.DefaultModel)
}

func (c *Connections) Models(ctx context.Context, id string) ([]Model, error) {
	c.mu.Lock()
	id = strings.TrimSpace(id)
	specification, exists := c.specs[id]
	if !exists {
		c.mu.Unlock()
		return nil, fmt.Errorf("provider %q is not available", id)
	}
	if specification.Models == nil {
		c.mu.Unlock()
		return nil, fmt.Errorf("provider %q does not support model discovery", id)
	}
	values, err := loadValues(c.path)
	if err != nil {
		c.mu.Unlock()
		return nil, err
	}
	settings, configured := values.Provider(id)
	c.mu.Unlock()
	if !configured || strings.TrimSpace(settings.APIKey) == "" {
		return nil, fmt.Errorf("provider %q is not configured", id)
	}
	ids, err := specification.Models(ctx, settings.APIKey)
	if err != nil {
		return nil, err
	}
	return normalizeModels(ids)
}

func (c *Connections) Remove(id string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	id = strings.TrimSpace(id)
	if _, exists := c.specs[id]; !exists {
		return fmt.Errorf("provider %q is not available", id)
	}
	values, err := loadValues(c.path)
	if err != nil {
		return err
	}
	values.DeleteProvider(id)
	if err := config.Save(c.path, values); err != nil {
		return err
	}
	return c.registry.Clear(id)
}

func (c *Connections) resolve(
	id string, input ConnectionInput,
) (Specification, config.Values, config.ProviderValues, error) {
	id = strings.TrimSpace(id)
	specification, exists := c.specs[id]
	if !exists {
		return Specification{}, config.Values{}, config.ProviderValues{}, fmt.Errorf("provider %q is not available", id)
	}
	if specification.Factory == nil {
		return Specification{}, config.Values{}, config.ProviderValues{}, fmt.Errorf("provider %q cannot be configured", id)
	}
	values, err := loadValues(c.path)
	if err != nil {
		return Specification{}, config.Values{}, config.ProviderValues{}, err
	}
	settings, _ := values.Provider(id)
	if strings.TrimSpace(input.APIKey) != "" {
		settings.APIKey = strings.TrimSpace(input.APIKey)
	}
	if strings.TrimSpace(input.DefaultModel) != "" {
		settings.DefaultModel = strings.TrimSpace(input.DefaultModel)
	}
	if settings.APIKey == "" {
		return Specification{}, config.Values{}, config.ProviderValues{}, errors.New("API key is required")
	}
	if settings.DefaultModel == "" {
		return Specification{}, config.Values{}, config.ProviderValues{}, errors.New("default model is required")
	}
	return specification, values, settings, nil
}

func (c *Connections) definition(id string) (Definition, error) {
	for _, definition := range c.registry.List() {
		if definition.ID == id {
			return definition, nil
		}
	}
	return Definition{}, fmt.Errorf("provider %q is not available", id)
}

func loadValues(path string) (config.Values, error) {
	values, err := config.Load(path)
	if errors.Is(err, os.ErrNotExist) {
		return config.Values{Version: config.CurrentVersion, Providers: map[string]config.ProviderValues{}}, nil
	}
	return values, err
}

func normalizeModels(ids []string) ([]Model, error) {
	if len(ids) > 10_000 {
		return nil, errors.New("provider returned more than 10000 models")
	}
	unique := make(map[string]struct{}, len(ids))
	for _, raw := range ids {
		id := strings.TrimSpace(raw)
		if id == "" {
			continue
		}
		if !utf8.ValidString(id) || utf8.RuneCountInString(id) > 200 {
			return nil, errors.New("provider returned an invalid model ID")
		}
		for _, character := range id {
			if unicode.IsControl(character) {
				return nil, errors.New("provider returned an invalid model ID")
			}
		}
		unique[id] = struct{}{}
	}
	values := make([]Model, 0, len(unique))
	for id := range unique {
		values = append(values, Model{ID: id})
	}
	sort.Slice(values, func(left, right int) bool { return values[left].ID < values[right].ID })
	return values, nil
}
