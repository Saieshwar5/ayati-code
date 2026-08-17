package environment

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

type ImageResolver interface {
	ResolveImage(context.Context, string) (string, error)
}

type ManagementService struct {
	store  *Store
	images ImageResolver
}

func NewManagementService(store *Store, images ImageResolver) (*ManagementService, error) {
	if store == nil || images == nil {
		return nil, errors.New("environment store and image resolver are required")
	}
	return &ManagementService{store: store, images: images}, nil
}

func (s *ManagementService) List(ctx context.Context) ([]Environment, error) {
	return s.store.List(ctx)
}

func (s *ManagementService) Create(ctx context.Context, input CreateInput) (Environment, error) {
	value, err := s.store.Create(ctx, input)
	if err != nil {
		return Environment{}, err
	}
	return s.provision(ctx, value)
}

func (s *ManagementService) Repair(ctx context.Context, id string) (Environment, error) {
	value, err := s.store.Get(ctx, id)
	if err != nil {
		return Environment{}, err
	}
	if value.ActiveLease != nil {
		return value, ErrEnvironmentOccupied
	}
	if err := s.requireNoFailedLease(ctx, value.ID); err != nil {
		return value, err
	}
	if value.ProvisioningState != ProvisioningFailed && value.ProvisioningState != Provisioning {
		return value, ErrEnvironmentReady
	}
	return s.provision(ctx, value)
}

func (s *ManagementService) Delete(ctx context.Context, id string) error {
	if err := s.requireNoFailedLease(ctx, id); err != nil {
		return err
	}
	return s.store.Delete(ctx, id)
}

func (s *ManagementService) requireNoFailedLease(ctx context.Context, id string) error {
	lease, err := s.store.LatestForEnvironment(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect environment lease history: %w", err)
	}
	if lease.State == LeaseFailed {
		return ErrEnvironmentQuarantined
	}
	return nil
}

func (s *ManagementService) provision(ctx context.Context, value Environment) (Environment, error) {
	digest, err := s.images.ResolveImage(ctx, value.ImageRef)
	if err == nil {
		err = s.store.MarkReady(ctx, value.ID, digest)
	}
	if err != nil {
		cause := fmt.Errorf("provision environment: %w", err)
		if recordErr := s.store.MarkFailed(ctx, value.ID, cause); recordErr != nil {
			return value, errors.Join(cause, recordErr)
		}
		failed, loadErr := s.store.Get(ctx, value.ID)
		if loadErr != nil {
			return value, errors.Join(cause, loadErr)
		}
		return failed, cause
	}
	return s.store.Get(ctx, value.ID)
}
