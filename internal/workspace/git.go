package workspace

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type osGit struct {
	path  string
	token func() (string, error)
}

func newGitClient(token func() (string, error)) (gitClient, error) {
	path, err := exec.LookPath("git")
	if err != nil {
		return nil, fmt.Errorf("find git: %w", err)
	}
	return osGit{path: path, token: token}, nil
}

func (g osGit) Run(ctx context.Context, arguments ...string) error {
	_, err := g.execute(ctx, false, arguments...)
	return err
}

func (g osGit) AuthenticatedRun(ctx context.Context, arguments ...string) error {
	_, err := g.execute(ctx, true, arguments...)
	return err
}

func (g osGit) execute(ctx context.Context, authenticated bool, arguments ...string) (string, error) {
	settings := []string{
		"-c", "core.hooksPath=/dev/null",
		"-c", "credential.helper=",
		"-c", "protocol.ext.allow=never",
		"-c", "protocol.file.allow=never",
		"-c", "http.proxy=",
		"-c", "http.sslVerify=true",
	}
	command := exec.CommandContext(ctx, g.path, append(settings, arguments...)...)
	command.Env = append(os.Environ(),
		"GIT_TERMINAL_PROMPT=0", "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null",
	)
	cleanup := func() {}
	if authenticated {
		if g.token == nil {
			return "", fmt.Errorf("GitHub credential is unavailable")
		}
		token, err := g.token()
		if err != nil {
			return "", fmt.Errorf("load GitHub credential: %w", err)
		}
		askpass, remove, err := writeAskPass(token)
		if err != nil {
			return "", err
		}
		cleanup = remove
		command.Env = append(command.Env, "GIT_ASKPASS="+askpass, "AYATI_GITHUB_TOKEN="+token)
	}
	defer cleanup()
	output, err := command.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%w: %s", err, boundedMessage(string(output)))
	}
	return string(output), nil
}

func writeAskPass(token string) (string, func(), error) {
	if strings.TrimSpace(token) == "" {
		return "", nil, fmt.Errorf("GitHub credential is empty")
	}
	directory, err := os.MkdirTemp("", "ayati-git-*")
	if err != nil {
		return "", nil, fmt.Errorf("create Git credential helper: %w", err)
	}
	remove := func() { _ = os.RemoveAll(directory) }
	path := filepath.Join(directory, "askpass")
	script := `#!/bin/sh
case "$1" in
  *Username*) printf '%s' 'x-access-token' ;;
  *) printf '%s' "$AYATI_GITHUB_TOKEN" ;;
esac
`
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		remove()
		return "", nil, fmt.Errorf("write Git credential helper: %w", err)
	}
	return path, remove, nil
}
