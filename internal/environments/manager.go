package environments

import (
	"context"
	"sync"
	"time"

	"github.com/Saieshwar5/perpetual/internal/vmagent"
)

// Manager owns Lambda microVM lifecycle and short-lived data-plane tokens.
type Manager struct {
	config Config
	api    API

	mu     sync.Mutex
	tokens map[string]cachedToken
}

type cachedToken struct {
	value     string
	expiresAt time.Time
}

// NewManager builds a Lambda environment manager.
func NewManager(config Config, api API) (*Manager, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	if api == nil {
		return nil, errNilAPI
	}
	return &Manager{config: config, api: api, tokens: make(map[string]cachedToken)}, nil
}

// Create launches a new microVM instance from the configured image.
func (m *Manager) Create(ctx context.Context) (Instance, error) {
	instance, err := m.api.RunMicrovm(ctx, RunMicrovmInput{
		ImageARN:         m.config.ImageARN,
		ImageVersion:     m.config.ImageVersion,
		ExecutionRoleARN: m.config.ExecutionRoleARN,
	})
	if err != nil {
		return Instance{}, err
	}
	return instance, nil
}

// Shell builds a data-plane client for an instance. Auth tokens are cached
// until shortly before expiry (AWS maximum TTL is 60 minutes).
func (m *Manager) Shell(ctx context.Context, instance Instance) (*vmagent.Client, error) {
	token, err := m.authToken(ctx, instance.MicrovmID)
	if err != nil {
		return nil, err
	}
	return vmagent.NewClient(instance.Endpoint, token)
}

// Suspend stops compute while preserving microVM state.
func (m *Manager) Suspend(ctx context.Context, microvmID string) error {
	return m.api.SuspendMicrovm(ctx, microvmID)
}

// Resume returns a suspended microVM to running state.
func (m *Manager) Resume(ctx context.Context, microvmID string) error {
	return m.api.ResumeMicrovm(ctx, microvmID)
}

// Terminate releases all microVM resources.
func (m *Manager) Terminate(ctx context.Context, microvmID string) error {
	m.mu.Lock()
	delete(m.tokens, microvmID)
	m.mu.Unlock()
	return m.api.TerminateMicrovm(ctx, microvmID)
}

func (m *Manager) authToken(ctx context.Context, microvmID string) (string, error) {
	m.mu.Lock()
	if cached, ok := m.tokens[microvmID]; ok && time.Now().Before(cached.expiresAt.Add(-2*time.Minute)) {
		value := cached.value
		m.mu.Unlock()
		return value, nil
	}
	m.mu.Unlock()

	token, err := m.api.AuthToken(ctx, microvmID)
	if err != nil {
		return "", err
	}
	m.mu.Lock()
	m.tokens[microvmID] = cachedToken{value: token, expiresAt: time.Now().Add(30 * time.Minute)}
	m.mu.Unlock()
	return token, nil
}
