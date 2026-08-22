package workspace

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Saieshwar5/perpetual/internal/environments"
	"github.com/Saieshwar5/perpetual/internal/exec"
	"github.com/Saieshwar5/perpetual/internal/workspaceruntime"
)

func (s *Service) Initialize(ctx context.Context, id string) error {
	value, err := s.store.Get(ctx, id)
	if err != nil {
		return err
	}
	if err := requireActiveWorkspace(value); err != nil {
		return err
	}
	if value.Status != StatusCreating && value.Status != StatusInitializationFailed {
		return fmt.Errorf("workspace is %s and cannot be initialized", value.Status)
	}
	if err := s.store.UpdateStatus(ctx, id, StatusInitializing, ""); err != nil {
		return err
	}
	if err := s.store.UpdateRuntimeState(ctx, id, workspaceruntime.RuntimeStateCreating); err != nil {
		return s.fail(ctx, id, err)
	}
	runtime, err := s.runtimeFor(value)
	if err != nil {
		return s.fail(ctx, id, err)
	}
	if err := runtime.Start(ctx, runtimeRef(value)); err != nil {
		return s.fail(ctx, id, fmt.Errorf("start workspace runtime: %w", err))
	}
	if value.RuntimeProvider == "lambda" && s.reposyncer != nil {
		if err := s.reposyncer.Push(ctx, value.ID, value.Path); err != nil {
			return s.fail(ctx, id, fmt.Errorf("sync repository to runtime: %w", err))
		}
	}
	if err := s.store.UpdatePreparation(ctx, id, PreparationCloning,
		value.Repository+" · "+value.Branch); err != nil {
		return s.fail(ctx, id, err)
	}
	if err := s.prepareRepository(ctx, value); err != nil {
		return s.fail(ctx, id, err)
	}
	if err := s.store.UpdatePreparation(ctx, id, PreparationAnalyzing,
		"Inspecting project metadata"); err != nil {
		return s.fail(ctx, id, err)
	}
	var profile ProjectProfile
	if value.SelectedProjectRoot == "" {
		profile, err = AnalyzeProject(value.Path)
	} else {
		profile, err = AnalyzeProjectAt(value.Path, value.SelectedProjectRoot)
	}
	if err != nil {
		var selection ProjectSelectionRequiredError
		if errors.As(err, &selection) {
			if recordErr := s.store.RequireProjectSelection(ctx, id, selection.Candidates); recordErr != nil {
				return s.fail(ctx, id, recordErr)
			}
			return err
		}
		return s.fail(ctx, id, fmt.Errorf("understand project: %w", err))
	}
	if value.Setup != "" {
		profile.SetupCommand = value.Setup
	}
	profile.CachePath = workspaceCachePath(value.Path)
	spec, err := BuildEnvironmentSpecAt(value.Path, profile.ProjectRoot)
	if err != nil {
		return s.fail(ctx, id, fmt.Errorf("build environment spec: %w", err))
	}
	profile.EnvironmentSpec = &spec
	if err := s.store.UpdateSetup(ctx, id, profile.SetupCommand); err != nil {
		return s.fail(ctx, id, err)
	}
	profile.BaselineCommit, err = s.gitOutput(ctx, value.Path, "rev-parse", "HEAD")
	if err != nil {
		return s.fail(ctx, id, fmt.Errorf("record baseline commit: %w", err))
	}
	before, err := s.gitOutput(ctx, value.Path, "status", "--porcelain=v1", "--untracked-files=all")
	if err != nil {
		return s.fail(ctx, id, fmt.Errorf("record baseline status: %w", err))
	}
	if before != "" {
		profile.BaselineResult = "dirty"
		_ = s.store.SaveProfile(ctx, id, profile)
		return s.fail(ctx, id, errors.New("repository is not clean before dependency setup: "+boundedMessage(before)))
	}
	environment, err := s.store.FindOrCreateEnvironment(ctx, value.UserID, value.Repository, profile.ProjectRoot)
	if err != nil {
		return s.fail(ctx, id, fmt.Errorf("record project environment: %w", err))
	}
	var environmentVersionID string
	createdEnvironment := false
	existing, found, err := s.store.FindReadyEnvironmentVersion(ctx, value.UserID, environment.ID, spec.Fingerprint)
	if err != nil {
		return s.fail(ctx, id, fmt.Errorf("find reusable environment: %w", err))
	}
	if found {
		environmentVersionID = existing.ID
	} else {
		version, createErr := s.store.CreateEnvironmentVersion(ctx, environment.ID,
			spec.Fingerprint, spec, workspaceCachePath(value.Path))
		if createErr != nil {
			return s.fail(ctx, id, fmt.Errorf("create environment version: %w", createErr))
		}
		environmentVersionID = version.ID
		createdEnvironment = true
	}
	failEnvironment := func(cause error) error {
		if createdEnvironment {
			_ = s.store.SetEnvironmentVersionState(ctx, environmentVersionID,
				EnvironmentVersionFailed, boundedMessage(cause.Error()))
		}
		return s.fail(ctx, id, cause)
	}
	if err := s.store.BindWorkspaceEnvironment(ctx, id, environmentVersionID); err != nil {
		return failEnvironment(fmt.Errorf("bind workspace environment: %w", err))
	}
	if err := s.store.SaveProfile(ctx, id, profile); err != nil {
		return failEnvironment(err)
	}
	if createdEnvironment {
		if err := s.store.UpdatePreparation(ctx, id, PreparationInstalling,
			"Waiting for environment build"); err != nil {
			return failEnvironment(err)
		}
		if err := s.store.UpdateStatus(ctx, id, StatusWaitingEnvironment, ""); err != nil {
			return failEnvironment(err)
		}
		if err := s.StartEnvironmentBuild(ctx, id); err != nil {
			return failEnvironment(fmt.Errorf("enqueue environment build: %w", err))
		}
		return nil
	}
	if err := s.store.UpdatePreparation(ctx, id, PreparationInstalling,
		profilePreparationDetail(profile)); err != nil {
		return s.fail(ctx, id, err)
	}
	restored := false
	if usableEnvironmentSnapshot(existing) {
		if err := s.restoreWorkspaceSnapshot(ctx, value, existing); err == nil {
			profile.SetupResult = "restored"
			if err := s.store.SaveProfile(ctx, id, profile); err == nil {
				restored = true
			}
		}
	}
	if !restored {
		if err := s.runSetup(ctx, value, &profile); err != nil {
			_ = s.store.SaveProfile(ctx, id, profile)
			return s.fail(ctx, id, err)
		}
	}
	if err := s.finalizePreparedWorkspace(ctx, value, &profile, before); err != nil {
		return s.fail(ctx, id, err)
	}
	if err := s.store.UpdateRuntimeState(ctx, id, workspaceruntime.RuntimeStateRunning); err != nil {
		return s.fail(ctx, id, err)
	}
	return nil
}

// finalizePreparedWorkspace records the post-setup baseline, seals the
// profile, and marks the workspace ready. It is shared by the inline
// materialization path and by build_environment jobs that finish a waiting
// workspace.
func (s *Service) finalizePreparedWorkspace(
	ctx context.Context, value Workspace, profile *ProjectProfile, before string,
) error {
	if err := s.store.UpdatePreparation(ctx, value.ID, PreparationVerifying,
		"Checking the Git baseline"); err != nil {
		return err
	}
	after, err := s.gitOutput(ctx, value.Path, "status", "--porcelain=v1", "--untracked-files=all")
	if err != nil {
		return fmt.Errorf("verify setup baseline: %w", err)
	}
	profile.BaselineResult = "clean"
	if after != before {
		profile.BaselineResult = "changed"
	}
	if err := s.store.SaveProfile(ctx, value.ID, *profile); err != nil {
		return err
	}
	if err := s.store.UpdatePreparation(ctx, value.ID, PreparationSealing,
		"Finalizing workspace"); err != nil {
		return err
	}
	now := time.Now().UTC()
	profile.PreparedAt = &now
	if err := s.store.SaveProfile(ctx, value.ID, *profile); err != nil {
		return err
	}
	return s.store.CompletePreparation(ctx, value.ID)
}

func (s *Service) runSetup(ctx context.Context, value Workspace, profile *ProjectProfile) error {
	if profile.SetupCommand == "" {
		profile.SetupResult = "skipped"
		return nil
	}
	shell, err := s.openShell(ctx, value, true)
	if err != nil {
		return err
	}
	result := shell.Execute(ctx, exec.ShellRequest{Command: profile.SetupCommand})
	if result.ExitCode != 0 || result.Error != "" {
		profile.SetupResult = "failed"
		return fmt.Errorf("setup failed: %s", shellError(result))
	}
	profile.SetupResult = "passed"
	return nil
}

// executeEnvironmentBuild runs dependency setup for a workspace's bound
// environment version and records the version as ready or failed. It is the
// reusable body behind build_environment jobs.
func (s *Service) executeEnvironmentBuild(ctx context.Context, workspaceID string) error {
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
		return nil
	}
	if value.RuntimeProvider == "lambda" && s.imageBuilder != nil {
		return s.buildLambdaImage(ctx, value, version)
	}
	profile, err := s.store.ProjectProfile(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("load environment build profile: %w", err)
	}
	before, err := s.gitOutput(ctx, value.Path, "status", "--porcelain=v1", "--untracked-files=all")
	if err != nil {
		return fmt.Errorf("record environment build baseline: %w", err)
	}
	if err := s.runSetup(ctx, value, profile); err != nil {
		_ = s.store.SetEnvironmentVersionState(ctx, version.ID,
			EnvironmentVersionFailed, boundedMessage(err.Error()))
		_ = s.store.SaveProfile(ctx, workspaceID, *profile)
		if value.Status == StatusWaitingEnvironment {
			_ = s.fail(ctx, workspaceID, err)
		}
		return err
	}
	if err := s.store.SaveProfile(ctx, workspaceID, *profile); err != nil {
		return err
	}
	if snapshot, captureErr := s.captureWorkspaceSnapshot(ctx, value, version.ID); captureErr == nil &&
		len(snapshot.Manifest) > 0 {
		_ = s.store.SetEnvironmentVersionSnapshot(ctx, version.ID, snapshot.Type,
			snapshot.Ref, snapshot.Manifest, snapshot.Bytes)
	}
	if err := s.store.SetEnvironmentVersionState(ctx, version.ID,
		EnvironmentVersionReady, ""); err != nil {
		return err
	}
	if value.Status == StatusWaitingEnvironment {
		if err := s.finalizePreparedWorkspace(ctx, value, profile, before); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) gitOutput(ctx context.Context, path string, arguments ...string) (string, error) {
	value, err := s.git.Output(ctx, append([]string{"-C", path}, arguments...)...)
	return strings.TrimSpace(value), err
}

func workspaceCachePath(repositoryPath string) string {
	return filepath.Join(filepath.Dir(repositoryPath), "cache")
}

// shellEnvironment builds a safe local environment for setup and agent shell
// commands. Tool caches live inside the managed workspace cache so they survive
// Stop and Resume, and HOME is private to the workspace so host configuration
// and controller credentials are never visible to commands.
func shellEnvironment(cachePath string, values map[string]string) map[string]string {
	result := make(map[string]string, len(values)+12)
	for name, value := range values {
		result[name] = value
	}
	if path := os.Getenv("PATH"); path != "" {
		result["PATH"] = path
	}
	result["HOME"] = filepath.Join(cachePath, "home")
	result["TMPDIR"] = "/tmp"
	result["XDG_CACHE_HOME"] = filepath.Join(cachePath, "xdg")
	result["GOCACHE"] = filepath.Join(cachePath, "go-build")
	result["GOMODCACHE"] = filepath.Join(cachePath, "go-mod")
	result["COREPACK_HOME"] = filepath.Join(cachePath, "corepack")
	result["npm_config_cache"] = filepath.Join(cachePath, "npm")
	result["PIP_CACHE_DIR"] = filepath.Join(cachePath, "pip")
	result["CARGO_HOME"] = filepath.Join(cachePath, "cargo")
	result["CARGO_TARGET_DIR"] = filepath.Join(cachePath, "rust-target")
	result["PYTHONDONTWRITEBYTECODE"] = "1"
	return result
}

// buildLambdaImage builds/reuses a Lambda MicroVM image for the workspace and
// records the image reference on the bound environment version.
func (s *Service) buildLambdaImage(ctx context.Context, value Workspace, version EnvironmentVersion) error {
	// Reuse an already-built image recorded on this version.
	if image, versionString, ok := environments.ParseImageRef(version.ArtifactRef); ok &&
		image != "" && versionString != "" {
		return nil
	}
	image, err := s.imageBuilder.Build(ctx)
	if err != nil {
		_ = s.store.SetEnvironmentVersionState(ctx, version.ID,
			EnvironmentVersionFailed, boundedMessage(err.Error()))
		return err
	}
	artifact := "lambda:" + image.ImageARN + ":" + image.Version
	if err := s.store.SetEnvironmentVersionArtifact(ctx, version.ID, artifact); err != nil {
		return err
	}
	if err := s.store.SetEnvironmentVersionState(ctx, version.ID, EnvironmentVersionReady, ""); err != nil {
		return err
	}
	return nil
}
