package sandbox

import (
	"context"
	"database/sql"
	"errors"

	"github.com/Saieshwar5/perpetual/internal/agent"
	"github.com/Saieshwar5/perpetual/internal/environment"
)

type RuntimeManager struct {
	service *environment.RuntimeService
	driver  *DockerDriver
}

func (m *RuntimeManager) Ensure(
	ctx context.Context, input environment.StopInput,
) (environment.Assignment, error) {
	if _, err := m.service.Current(ctx, input); err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			return environment.Assignment{}, err
		}
		return m.service.Start(ctx, environment.StartInput{
			WorkspaceID: input.WorkspaceID, WorkspacePath: input.WorkspacePath,
			CachePath: input.CachePath,
		})
	}
	return m.service.Restore(ctx, input)
}

func NewRuntimeManager(store *environment.Store, driver *DockerDriver) (*RuntimeManager, error) {
	if driver == nil {
		return nil, errors.New("Docker runtime driver is required")
	}
	service, err := environment.NewRuntimeService(store, driver)
	if err != nil {
		return nil, err
	}
	return &RuntimeManager{service: service, driver: driver}, nil
}

func (m *RuntimeManager) Start(
	ctx context.Context, input environment.StartInput,
) (environment.Assignment, error) {
	return m.service.Start(ctx, input)
}

func (m *RuntimeManager) Stop(ctx context.Context, input environment.StopInput) error {
	return m.service.Stop(ctx, input)
}

func (m *RuntimeManager) Cleanup(ctx context.Context, input environment.StopInput) error {
	return m.service.Cleanup(ctx, input)
}

func (m *RuntimeManager) Open(
	ctx context.Context, input environment.StopInput, variables map[string]string,
) (agent.Shell, error) {
	assignment, err := m.service.Restore(ctx, input)
	if err != nil {
		return nil, err
	}
	return m.driver.OpenRuntime(assignment.Runtime.ID, variables)
}
