package environment

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

type RuntimeService struct {
	store  *Store
	driver RuntimeDriver
}

func NewRuntimeService(store *Store, driver RuntimeDriver) (*RuntimeService, error) {
	if store == nil || driver == nil {
		return nil, errors.New("environment store and runtime driver are required")
	}
	return &RuntimeService{store: store, driver: driver}, nil
}

func (s *RuntimeService) Start(ctx context.Context, input StartInput) (Assignment, error) {
	input.WorkspaceID = strings.TrimSpace(input.WorkspaceID)
	if input.WorkspaceID == "" {
		return Assignment{}, errors.New("workspace is required")
	}
	lease, err := s.store.Acquire(ctx, input.WorkspaceID, input.PreferredEnvironmentID)
	if err != nil {
		return Assignment{}, err
	}
	value, err := s.store.Get(ctx, lease.EnvironmentID)
	if err != nil {
		return Assignment{}, s.failStart(ctx, lease, "load leased environment", err)
	}
	spec := RuntimeSpec{
		Environment: value, Lease: lease, WorkspacePath: input.WorkspacePath,
		CachePath: input.CachePath, WorkspaceWritable: input.WorkspaceWritable,
	}
	runtime, err := s.driver.Create(ctx, spec)
	if err != nil {
		return Assignment{}, s.failStart(ctx, lease, "create environment runtime", err)
	}
	if err := s.store.Activate(ctx, lease.ID, lease.Generation, runtime.ID); err != nil {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		destroyErr := s.driver.Destroy(cleanup, spec, runtime.ID)
		cancel()
		return Assignment{}, s.failStart(ctx, lease, "activate environment lease",
			errors.Join(err, contextualError("destroy unowned runtime", destroyErr)))
	}
	lease.State = LeaseActive
	lease.RuntimeID = runtime.ID
	return Assignment{Environment: value, Lease: lease, Runtime: runtime}, nil
}

func (s *RuntimeService) Stop(ctx context.Context, input StopInput) error {
	input.WorkspaceID = strings.TrimSpace(input.WorkspaceID)
	if input.WorkspaceID == "" {
		return errors.New("workspace is required")
	}
	lease, err := s.store.ActiveForWorkspace(ctx, input.WorkspaceID)
	if err != nil {
		return fmt.Errorf("load workspace environment lease: %w", err)
	}
	if lease.State != LeaseActive {
		return ErrLeaseState
	}
	value, err := s.store.Get(ctx, lease.EnvironmentID)
	if err != nil {
		return fmt.Errorf("load leased environment: %w", err)
	}
	spec := RuntimeSpec{
		Environment: value, Lease: lease, WorkspacePath: input.WorkspacePath,
		CachePath: input.CachePath, WorkspaceWritable: input.WorkspaceWritable,
	}
	if err := s.store.BeginRelease(ctx, lease.ID, lease.Generation); err != nil {
		return err
	}
	lease.State = LeaseReleasing
	spec.Lease = lease
	if err := s.driver.Destroy(ctx, spec, lease.RuntimeID); err != nil {
		return s.failLease(ctx, lease, fmt.Errorf("destroy environment runtime: %w", err))
	}
	if err := s.store.CompleteRelease(ctx, lease.ID, lease.Generation); err != nil {
		return s.failLease(ctx, lease, fmt.Errorf("complete environment release: %w", err))
	}
	return nil
}

func (s *RuntimeService) failStart(
	ctx context.Context, lease Lease, action string, cause error,
) error {
	return s.failLease(ctx, lease, fmt.Errorf("%s: %w", action, cause))
}

func (s *RuntimeService) failLease(_ context.Context, lease Lease, cause error) error {
	cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.store.Fail(cleanup, lease.ID, lease.Generation, cause); err != nil {
		return errors.Join(cause, fmt.Errorf("record environment failure: %w", err))
	}
	return cause
}

func contextualError(action string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", action, err)
}
