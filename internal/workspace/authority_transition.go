package workspace

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

type AuthorityChange struct {
	Authority    Authority `json:"authority"`
	Branch       string    `json:"branch"`
	CreateBranch bool      `json:"create_branch"`
}

func (s *Service) ChangeAuthority(
	ctx context.Context, id string, input AuthorityChange,
) (Workspace, error) {
	if strings.TrimSpace(string(input.Authority)) == "" {
		return Workspace{}, errors.New("workspace authority is required")
	}
	target, err := ParseAuthority(string(input.Authority))
	if err != nil {
		return Workspace{}, err
	}
	value, err := s.store.Get(ctx, id)
	if err != nil {
		return Workspace{}, err
	}
	if err := requireActiveWorkspace(value); err != nil {
		return Workspace{}, err
	}
	if value.Status != StatusReady {
		return Workspace{}, fmt.Errorf("workspace is %s, not ready", value.Status)
	}
	working, err := s.store.HasWorkingSession(ctx, id)
	if err != nil {
		return Workspace{}, fmt.Errorf("inspect running sessions: %w", err)
	}
	if working {
		return Workspace{}, errors.New("an agent is working in this workspace")
	}
	if target == value.Authority {
		return value, nil
	}

	branch, createBranch, err := s.resolveAuthorityBranch(ctx, value, target, input)
	if err != nil {
		return Workspace{}, err
	}
	if err := s.store.UpdateStatus(ctx, id, StatusInitializing, ""); err != nil {
		return Workspace{}, err
	}
	if err := s.store.UpdatePreparation(ctx, id, PreparationSealing,
		"Applying "+string(target)+" authority"); err != nil {
		_ = s.store.RestoreAuthorityAfterFailure(ctx, value.ID, value.EffectiveMountMode)
		return Workspace{}, err
	}
	if err := s.environment.Remove(ctx, value.SandboxName); err != nil {
		return Workspace{}, s.restoreAuthorityFailure(ctx, value, false, input, err)
	}

	branchChanged := branch != value.Branch
	if branchChanged {
		arguments := []string{"-C", value.Path, "switch"}
		if createBranch {
			arguments = append(arguments, "-c")
		}
		arguments = append(arguments, branch)
		if err := s.git.Run(ctx, arguments...); err != nil {
			return Workspace{}, s.restoreAuthorityFailure(ctx, value, false, input,
				fmt.Errorf("switch working branch: %w", err))
		}
	}

	mode, err := s.environment.Ensure(ctx, s.sandboxSpec(value, target.MountMode()))
	if err != nil {
		return Workspace{}, s.restoreAuthorityFailure(ctx, value, branchChanged, input,
			fmt.Errorf("apply %s authority: %w", target, err))
	}
	if mode != target.MountMode() {
		return Workspace{}, s.restoreAuthorityFailure(ctx, value, branchChanged, input,
			fmt.Errorf("sandbox mount is %s, expected %s", mode, target.MountMode()))
	}
	if err := s.store.CompleteAuthorityChange(ctx, id, target, branch, createBranch, string(mode)); err != nil {
		return Workspace{}, s.restoreAuthorityFailure(ctx, value, branchChanged, input, err)
	}
	return s.store.Get(ctx, id)
}

func (s *Service) resolveAuthorityBranch(
	ctx context.Context, value Workspace, target Authority, input AuthorityChange,
) (string, bool, error) {
	if target == AuthorityExplore {
		return value.Branch, value.CreateBranch, nil
	}
	branch := strings.TrimSpace(input.Branch)
	if branch == "" && value.Branch != value.BaseBranch {
		branch = value.Branch
	}
	if branch == "" || branch == value.BaseBranch {
		return "", false, errors.New("Develop authority requires a working branch different from the starting branch")
	}
	if err := s.git.Run(ctx, "check-ref-format", "--branch", branch); err != nil {
		return "", false, fmt.Errorf("invalid working branch %q", branch)
	}
	if branch != value.Branch && !input.CreateBranch {
		return "", false, errors.New("authority change requires creating a new local working branch")
	}
	createBranch := input.CreateBranch
	if branch == value.Branch {
		createBranch = value.CreateBranch
	}
	return branch, createBranch, nil
}

func (s *Service) restoreAuthorityFailure(
	ctx context.Context, value Workspace, branchChanged bool, input AuthorityChange, cause error,
) error {
	var recoveryErrors []string
	_ = s.environment.Remove(ctx, value.SandboxName)
	if branchChanged {
		if err := s.git.Run(ctx, "-C", value.Path, "switch", value.Branch); err != nil {
			recoveryErrors = append(recoveryErrors, "restore branch: "+err.Error())
		} else if input.CreateBranch {
			if err := s.git.Run(ctx, "-C", value.Path, "branch", "-D", strings.TrimSpace(input.Branch)); err != nil {
				recoveryErrors = append(recoveryErrors, "remove new branch: "+err.Error())
			}
		}
	}
	mode, err := s.environment.Ensure(ctx, s.sandboxSpec(value, value.Authority.MountMode()))
	if err != nil {
		recoveryErrors = append(recoveryErrors, "restore sandbox: "+err.Error())
	} else if mode != value.Authority.MountMode() {
		recoveryErrors = append(recoveryErrors,
			fmt.Sprintf("restored sandbox mount is %s, expected %s", mode, value.Authority.MountMode()))
	}
	if len(recoveryErrors) == 0 {
		if err := s.store.RestoreAuthorityAfterFailure(ctx, value.ID, string(mode)); err != nil {
			return fmt.Errorf("%w; record restored authority: %v", cause, err)
		}
		return cause
	}
	recovery := strings.Join(recoveryErrors, "; ")
	failed := fmt.Errorf("%w; authority recovery failed: %s", cause, recovery)
	if err := s.store.FailPreparation(ctx, value.ID, boundedMessage(failed.Error())); err != nil {
		return fmt.Errorf("%w; record authority failure: %v", failed, err)
	}
	return failed
}
