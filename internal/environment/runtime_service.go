package environment

import (
	"context"
	"database/sql"
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
		CachePath: input.CachePath,
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
	_, lease, spec, err := s.currentSpec(ctx, input)
	if err != nil {
		return err
	}
	return s.release(ctx, lease, spec)
}

func (s *RuntimeService) release(ctx context.Context, lease Lease, spec RuntimeSpec) error {
	if lease.State != LeaseReleasing {
		if err := s.store.BeginRelease(ctx, lease.ID, lease.Generation); err != nil {
			return err
		}
		lease.State = LeaseReleasing
		spec.Lease = lease
	}
	if err := s.driver.Destroy(ctx, spec, lease.RuntimeID); err != nil {
		return s.failLease(ctx, lease, fmt.Errorf("destroy environment runtime: %w", err))
	}
	if err := s.store.CompleteRelease(ctx, lease.ID, lease.Generation); err != nil {
		return s.failLease(ctx, lease, fmt.Errorf("complete environment release: %w", err))
	}
	return nil
}

func (s *RuntimeService) Restore(ctx context.Context, input StopInput) (Assignment, error) {
	value, lease, spec, err := s.currentSpec(ctx, input)
	if err != nil {
		return Assignment{}, err
	}
	if lease.State != LeaseActive {
		return Assignment{}, ErrLeaseState
	}
	runtime, err := s.driver.Create(ctx, spec)
	if err != nil {
		return Assignment{}, s.failLease(ctx, lease, fmt.Errorf("restore environment runtime: %w", err))
	}
	if lease.RuntimeID != runtime.ID {
		if err := s.store.ReplaceRuntime(ctx, lease.ID, lease.Generation, runtime.ID); err != nil {
			cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			destroyErr := s.driver.Destroy(cleanup, spec, runtime.ID)
			cancel()
			return Assignment{}, s.failLease(ctx, lease,
				errors.Join(fmt.Errorf("record restored runtime: %w", err), contextualError("destroy unowned runtime", destroyErr)))
		}
	}
	lease.RuntimeID = runtime.ID
	return Assignment{Environment: value, Lease: lease, Runtime: runtime}, nil
}

func (s *RuntimeService) Current(ctx context.Context, input StopInput) (Assignment, error) {
	value, lease, _, err := s.currentSpec(ctx, input)
	if err != nil {
		return Assignment{}, err
	}
	return Assignment{Environment: value, Lease: lease, Runtime: Runtime{
		ID: lease.RuntimeID, EnvironmentID: value.ID, WorkspaceID: lease.WorkspaceID,
		LeaseID: lease.ID, Generation: lease.Generation,
	}}, nil
}

func (s *RuntimeService) Cleanup(ctx context.Context, input StopInput) error {
	lease, err := s.store.ActiveForWorkspace(ctx, strings.TrimSpace(input.WorkspaceID))
	if err == nil {
		value, loadErr := s.store.Get(ctx, lease.EnvironmentID)
		if loadErr != nil {
			return fmt.Errorf("load leased environment: %w", loadErr)
		}
		spec := RuntimeSpec{
			Environment: value, Lease: lease, WorkspacePath: input.WorkspacePath,
			CachePath: input.CachePath,
		}
		return s.release(ctx, lease, spec)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("load active workspace environment lease: %w", err)
	}
	lease, err = s.store.LatestForWorkspace(ctx, input.WorkspaceID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("load workspace environment history: %w", err)
	}
	if lease.State != LeaseFailed {
		return nil
	}
	value, err := s.store.Get(ctx, lease.EnvironmentID)
	if err != nil {
		return fmt.Errorf("load failed environment: %w", err)
	}
	spec := RuntimeSpec{
		Environment: value, Lease: lease, WorkspacePath: input.WorkspacePath,
		CachePath: input.CachePath,
	}
	if err := s.driver.Destroy(ctx, spec, lease.RuntimeID); err != nil {
		return fmt.Errorf("clean failed environment runtime: %w", err)
	}
	return nil
}

func (s *RuntimeService) currentSpec(
	ctx context.Context, input StopInput,
) (Environment, Lease, RuntimeSpec, error) {
	input.WorkspaceID = strings.TrimSpace(input.WorkspaceID)
	if input.WorkspaceID == "" {
		return Environment{}, Lease{}, RuntimeSpec{}, errors.New("workspace is required")
	}
	lease, err := s.store.ActiveForWorkspace(ctx, input.WorkspaceID)
	if err != nil {
		return Environment{}, Lease{}, RuntimeSpec{}, fmt.Errorf("load workspace environment lease: %w", err)
	}
	value, err := s.store.Get(ctx, lease.EnvironmentID)
	if err != nil {
		return Environment{}, Lease{}, RuntimeSpec{}, fmt.Errorf("load leased environment: %w", err)
	}
	spec := RuntimeSpec{
		Environment: value, Lease: lease, WorkspacePath: input.WorkspacePath,
		CachePath: input.CachePath,
	}
	return value, lease, spec, nil
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
