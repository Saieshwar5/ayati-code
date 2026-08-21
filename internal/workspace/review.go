package workspace

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Saieshwar5/perpetual/internal/exec"
)

type Changes struct {
	Status string `json:"status"`
	Diff   string `json:"diff"`
}

func (s *Service) Changes(ctx context.Context, id string) (Changes, error) {
	shell, _, err := s.Shell(ctx, id)
	if err != nil {
		return Changes{}, err
	}
	status := shell.Execute(ctx, exec.ShellRequest{Command: "git status --short"})
	if status.ExitCode != 0 || status.Error != "" {
		return Changes{}, fmt.Errorf("inspect Git status: %s", shellError(status))
	}
	diffCommand := `git diff --no-ext-diff --stat && git diff --no-ext-diff && ` +
		`git ls-files --others --exclude-standard -z | ` +
		`xargs -0 -r -n1 sh -c 'git diff --no-index -- /dev/null "$1"; code=$?; test "$code" -eq 0 -o "$code" -eq 1' sh`
	diff := shell.Execute(ctx, exec.ShellRequest{Command: diffCommand})
	if diff.ExitCode != 0 || diff.Error != "" {
		return Changes{}, fmt.Errorf("inspect Git diff: %s", shellError(diff))
	}
	return Changes{Status: status.Stdout, Diff: diff.Stdout}, nil
}

func (s *Service) Publish(ctx context.Context, id, message, authorName, authorEmail string) error {
	stored, err := s.store.Get(ctx, id)
	if err != nil {
		return err
	}
	if stored.Branch == stored.BaseBranch {
		return errors.New("publishing requires a working branch different from the pull request base")
	}
	shell, value, err := s.Shell(ctx, id)
	if err != nil {
		return err
	}
	working, err := s.store.HasWorkingSession(ctx, id)
	if err != nil {
		return fmt.Errorf("inspect running sessions: %w", err)
	}
	if working {
		return errors.New("a session is still running; wait for the agent before publishing")
	}
	message = strings.TrimSpace(message)
	if message == "" {
		return errors.New("commit message is required")
	}
	status := shell.Execute(ctx, exec.ShellRequest{Command: "git status --porcelain"})
	if status.ExitCode != 0 || status.Error != "" {
		return fmt.Errorf("inspect changes: %s", shellError(status))
	}
	if strings.TrimSpace(status.Stdout) != "" {
		arguments := []string{"git", "-c", shellQuote("core.hooksPath=/dev/null")}
		arguments = append(arguments, "add", "--all", "&&", "git", "-c", shellQuote("core.hooksPath=/dev/null"))
		if strings.TrimSpace(authorName) != "" {
			arguments = append(arguments, "-c", shellQuote("user.name="+strings.TrimSpace(authorName)))
		}
		if strings.TrimSpace(authorEmail) != "" {
			arguments = append(arguments, "-c", shellQuote("user.email="+strings.TrimSpace(authorEmail)))
		}
		arguments = append(arguments, "commit", "--no-verify", "-m", shellQuote(message))
		commit := shell.Execute(ctx, exec.ShellRequest{Command: strings.Join(arguments, " ")})
		if commit.ExitCode != 0 || commit.Error != "" {
			return fmt.Errorf("commit changes: %s", shellError(commit))
		}
	}
	token, err := s.tokenForUser(ctx, stored.UserID)
	if err != nil {
		return err
	}
	refspec := "refs/heads/" + value.Branch + ":refs/heads/" + value.Branch
	if err := s.git.AuthenticatedRun(ctx, token, "-C", value.Path, "push", "--no-verify", "--", value.CloneURL, refspec); err != nil {
		return fmt.Errorf("push branch: %w", err)
	}
	return nil
}
