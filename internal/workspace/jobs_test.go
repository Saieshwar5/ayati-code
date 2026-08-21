package workspace

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Saieshwar5/perpetual/internal/exec"
)

func TestStoreCreatesClaimsAndFinishesWorkspaceJob(t *testing.T) {
	store, value := jobWorkspace(t)
	if _, err := store.CreateJob(context.Background(), value.ID, JobKindPrepare); err != nil {
		t.Fatalf("CreateJob: %v", err)
	}
	active, err := store.HasActiveJob(context.Background(), value.ID, JobKindPrepare)
	if err != nil || !active {
		t.Fatalf("active = %v, error = %v", active, err)
	}
	job, err := store.ClaimNextJob(context.Background())
	if err != nil {
		t.Fatalf("ClaimNextJob: %v", err)
	}
	if job.State != JobStateRunning || job.Attempts != 1 || job.WorkspaceID != value.ID ||
		job.Kind != JobKindPrepare || job.LeaseOwner != jobLeaseOwner {
		t.Fatalf("claimed job = %#v", job)
	}
	if err := store.FinishJob(context.Background(), job.ID, JobStateSucceeded, ""); err != nil {
		t.Fatalf("FinishJob: %v", err)
	}
	jobs, err := store.Jobs(context.Background(), value.ID)
	if err != nil || len(jobs) != 1 || jobs[0].State != JobStateSucceeded {
		t.Fatalf("jobs = %#v, error = %v", jobs, err)
	}
}

func TestStartPreparationIsIdempotentWhileJobIsActive(t *testing.T) {
	store, value := jobWorkspace(t)
	service := &Service{store: store}
	if err := service.StartPreparation(context.Background(), value.ID); err != nil {
		t.Fatalf("StartPreparation: %v", err)
	}
	if err := service.StartPreparation(context.Background(), value.ID); err != nil {
		t.Fatalf("second StartPreparation: %v", err)
	}
	jobs, err := store.Jobs(context.Background(), value.ID)
	if err != nil || len(jobs) != 1 {
		t.Fatalf("jobs = %#v, error = %v", jobs, err)
	}
}

func TestStartPreparationRejectsReadyWorkspace(t *testing.T) {
	store, value := readyWorkspace(t, "main", false)
	service := &Service{store: store}
	err := service.StartPreparation(context.Background(), value.ID)
	if err == nil || !strings.Contains(err.Error(), "cannot be prepared") {
		t.Fatalf("StartPreparation error = %v", err)
	}
}

func TestRunNextJobPreparesWorkspaceAndRecordsSuccess(t *testing.T) {
	store, value := jobWorkspace(t)
	if err := store.UpdateSetup(context.Background(), value.ID, "go mod download"); err != nil {
		t.Fatalf("UpdateSetup: %v", err)
	}
	service := &Service{
		store:   store,
		runtime: &fakeRuntime{shell: &recordingShell{result: exec.ShellResult{ExitCode: 0}}},
		git:     &recordingGit{},
	}
	if err := service.StartPreparation(context.Background(), value.ID); err != nil {
		t.Fatalf("StartPreparation: %v", err)
	}
	if err := service.RunNextJob(context.Background()); err != nil {
		t.Fatalf("run prepare: %v", err)
	}
	if err := service.RunNextJob(context.Background()); err != nil {
		t.Fatalf("run environment build: %v", err)
	}
	loaded, err := store.Get(context.Background(), value.ID)
	if err != nil || loaded.Status != StatusReady || loaded.PreparationStage != PreparationReady {
		t.Fatalf("workspace = %#v, error = %v", loaded, err)
	}
	jobs, err := store.Jobs(context.Background(), value.ID)
	if err != nil || len(jobs) != 2 || jobs[0].State != JobStateSucceeded ||
		jobs[1].State != JobStateSucceeded {
		t.Fatalf("jobs = %#v, error = %v", jobs, err)
	}
}

func TestRunNextJobRecordsPreparationFailure(t *testing.T) {
	store, value := jobWorkspace(t)
	if err := store.UpdateSetup(context.Background(), value.ID, "npm ci"); err != nil {
		t.Fatalf("UpdateSetup: %v", err)
	}
	service := &Service{
		store:   store,
		runtime: &fakeRuntime{shell: &recordingShell{result: exec.ShellResult{ExitCode: 1, Stderr: "npm not found"}}},
		git:     &recordingGit{},
	}
	if err := service.StartPreparation(context.Background(), value.ID); err != nil {
		t.Fatalf("StartPreparation: %v", err)
	}
	if err := service.RunNextJob(context.Background()); err != nil {
		t.Fatalf("run prepare: %v", err)
	}
	if err := service.RunNextJob(context.Background()); err != nil {
		t.Fatalf("run environment build: %v", err)
	}
	loaded, err := store.Get(context.Background(), value.ID)
	if err != nil || loaded.Status != StatusInitializationFailed {
		t.Fatalf("workspace = %#v, error = %v", loaded, err)
	}
	jobs, err := store.Jobs(context.Background(), value.ID)
	if err != nil || len(jobs) != 2 || jobs[1].State != JobStateFailed ||
		!strings.Contains(jobs[1].Error, "npm not found") {
		t.Fatalf("jobs = %#v, error = %v", jobs, err)
	}
}

func TestRunNextJobReturnsErrNoJobs(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "perpetual.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	service := &Service{store: store}
	if err := service.RunNextJob(context.Background()); !errors.Is(err, ErrNoJobs) {
		t.Fatalf("RunNextJob error = %v", err)
	}
}

func TestRecoverJobsMarksActiveJobsInterrupted(t *testing.T) {
	store, value := jobWorkspace(t)
	if _, err := store.CreateJob(context.Background(), value.ID, JobKindPrepare); err != nil {
		t.Fatalf("CreateJob: %v", err)
	}
	if _, err := store.ClaimNextJob(context.Background()); err != nil {
		t.Fatalf("ClaimNextJob: %v", err)
	}
	if err := store.RecoverJobs(context.Background()); err != nil {
		t.Fatalf("RecoverJobs: %v", err)
	}
	jobs, err := store.Jobs(context.Background(), value.ID)
	if err != nil || len(jobs) != 1 || jobs[0].State != JobStateFailed ||
		!strings.Contains(jobs[0].Error, "interrupted") {
		t.Fatalf("jobs = %#v, error = %v", jobs, err)
	}
}

func jobWorkspace(t *testing.T) (*Store, Workspace) {
	t.Helper()
	store, err := Open(filepath.Join(t.TempDir(), "perpetual.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	value, err := store.Create(context.Background(), Create{
		Repository: "owner/project", CloneURL: "https://github.com/owner/project.git",
		BaseBranch: "main", Branch: "perpetual/job", Path: filepath.Join(t.TempDir(), "repo"),
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	return store, value
}
