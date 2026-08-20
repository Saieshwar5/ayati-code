package workspace

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Saieshwar5/perpetual/internal/agent"
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
	if err := s.store.SaveProfile(ctx, id, profile); err != nil {
		return s.fail(ctx, id, err)
	}
	if err := s.store.UpdatePreparation(ctx, id, PreparationInstalling,
		profilePreparationDetail(profile)); err != nil {
		return s.fail(ctx, id, err)
	}
	if err := s.runSetup(ctx, value, &profile); err != nil {
		_ = s.store.SaveProfile(ctx, id, profile)
		return s.fail(ctx, id, err)
	}
	if err := s.store.UpdatePreparation(ctx, id, PreparationVerifying,
		"Checking the Git baseline"); err != nil {
		return s.fail(ctx, id, err)
	}
	after, err := s.gitOutput(ctx, value.Path, "status", "--porcelain=v1", "--untracked-files=all")
	if err != nil {
		_ = s.store.SaveProfile(ctx, id, profile)
		return s.fail(ctx, id, fmt.Errorf("verify setup baseline: %w", err))
	}
	profile.BaselineResult = "clean"
	if after != before {
		profile.BaselineResult = "changed"
	}
	if err := s.store.SaveProfile(ctx, id, profile); err != nil {
		return s.fail(ctx, id, err)
	}
	if err := s.store.UpdatePreparation(ctx, id, PreparationSealing,
		"Finalizing workspace"); err != nil {
		return s.fail(ctx, id, err)
	}
	now := time.Now().UTC()
	profile.PreparedAt = &now
	if err := s.store.SaveProfile(ctx, id, profile); err != nil {
		return s.fail(ctx, id, err)
	}
	if err := s.store.CompletePreparation(ctx, id); err != nil {
		return s.fail(ctx, id, err)
	}
	return nil
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
	result := shell.Execute(ctx, agent.ShellRequest{Command: profile.SetupCommand})
	if result.ExitCode != 0 || result.Error != "" {
		profile.SetupResult = "failed"
		return fmt.Errorf("setup failed: %s", shellError(result))
	}
	profile.SetupResult = "passed"
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
