package workspace

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// ErrNoJobs is returned when no queued workspace job is available to claim.
var ErrNoJobs = errors.New("no queued workspace jobs")

// StartPreparation enqueues a durable prepare_workspace job for the workspace.
// If an active preparation job already exists the request is idempotent.
func (s *Service) StartPreparation(ctx context.Context, id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return errors.New("workspace ID is required")
	}
	active, err := s.store.HasActiveJob(ctx, id, JobKindPrepare)
	if err != nil {
		return err
	}
	if active {
		return nil
	}
	value, err := s.store.Get(ctx, id)
	if err != nil {
		return err
	}
	if value.Status != StatusCreating && value.Status != StatusInitializationFailed {
		return fmt.Errorf("workspace is %s and cannot be prepared", value.Status)
	}
	_, err = s.store.CreateJob(ctx, id, JobKindPrepare)
	return err
}

// StartEnvironmentBuild enqueues a durable build_environment job for the
// workspace's bound environment version. It is idempotent while a build job is
// already active and refuses to rebuild an already ready version.
func (s *Service) StartEnvironmentBuild(ctx context.Context, workspaceID string) error {
	workspaceID = strings.TrimSpace(workspaceID)
	if workspaceID == "" {
		return errors.New("workspace ID is required")
	}
	value, err := s.store.Get(ctx, workspaceID)
	if err != nil {
		return err
	}
	if err := requireActiveWorkspace(value); err != nil {
		return err
	}
	if value.EnvironmentVersionID == "" {
		return errors.New("workspace is not bound to an environment version")
	}
	version, err := s.store.GetEnvironmentVersion(ctx, value.EnvironmentVersionID)
	if err != nil {
		return err
	}
	if version.State == EnvironmentVersionReady {
		return errors.New("environment version is already ready")
	}
	active, err := s.store.HasActiveJob(ctx, workspaceID, JobKindBuildEnvironment)
	if err != nil {
		return err
	}
	if active {
		return nil
	}
	_, err = s.store.CreateJob(ctx, workspaceID, JobKindBuildEnvironment)
	return err
}

// RebuildEnvironment exposes the durable build_environment enqueue path to
// the web layer.
func (s *Service) RebuildEnvironment(ctx context.Context, id string) error {
	return s.StartEnvironmentBuild(ctx, id)
}

// RunNextJob claims and executes one queued workspace job. It returns
// ErrNoJobs when the queue is empty and nil after a job completes, because job
// execution failures are recorded on the job for the user to inspect and retry.
func (s *Service) RunNextJob(ctx context.Context) error {
	job, err := s.store.ClaimNextJob(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNoJobs
	}
	if err != nil {
		return err
	}
	return s.executeJob(ctx, job)
}

// RunWorker processes queued workspace jobs until the context is canceled.
func (s *Service) RunWorker(ctx context.Context) {
	const idleWait = 250 * time.Millisecond
	const backoffWait = time.Second
	for {
		err := s.RunNextJob(ctx)
		if ctx.Err() != nil {
			return
		}
		wait := idleWait
		if err != nil && !errors.Is(err, ErrNoJobs) {
			wait = backoffWait
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
	}
}

func (s *Service) executeJob(ctx context.Context, job Job) error {
	var runErr error
	switch job.Kind {
	case JobKindPrepare:
		runErr = s.Initialize(ctx, job.WorkspaceID)
	case JobKindBuildEnvironment:
		runErr = s.executeEnvironmentBuild(ctx, job.WorkspaceID)
	default:
		runErr = fmt.Errorf("unknown job kind %q", job.Kind)
	}
	state, message := JobStateSucceeded, ""
	if runErr != nil {
		state, message = JobStateFailed, boundedMessage(runErr.Error())
	}
	if err := s.store.FinishJob(ctx, job.ID, state, message); err != nil {
		return fmt.Errorf("record job %s: %w", job.ID, err)
	}
	return nil
}
