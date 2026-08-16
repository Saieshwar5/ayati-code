package sandbox

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/Saieshwar5/ayati-code/internal/agent"
)

const (
	DefaultImage   = "ayati-sandbox:dev"
	commandLimit   = 64 << 10
	outputLimit    = 32 << 10
	commandTimeout = 2 * time.Minute
)

type commandResult struct {
	stdout, stderr string
	exitCode       int
	truncated      bool
}

type runner interface {
	Run(context.Context, ...string) (commandResult, error)
	RunInput(context.Context, string, ...string) (commandResult, error)
}

type Manager struct {
	docker string
	image  string
	runner runner
}

type Spec struct {
	Name string
	Path string
}

type containerInfo struct {
	running bool
	path    string
}

func New(image string) (*Manager, error) {
	docker, err := exec.LookPath("docker")
	if err != nil {
		return nil, fmt.Errorf("find docker: %w", err)
	}
	if strings.TrimSpace(image) == "" {
		image = DefaultImage
	}
	return &Manager{docker: docker, image: image, runner: osRunner{docker: docker}}, nil
}

func (m *Manager) Ensure(ctx context.Context, spec Spec) error {
	path, err := validateSpec(spec)
	if err != nil {
		return err
	}
	container, exists, err := m.inspect(ctx, spec.Name)
	if err != nil {
		return err
	}
	if exists {
		if filepath.Clean(container.path) != filepath.Clean(path) {
			return fmt.Errorf("sandbox %s is mounted from a different workspace", spec.Name)
		}
		if container.running {
			return nil
		}
		_, err := m.run(ctx, "start", spec.Name)
		return err
	}
	if err := os.MkdirAll(path, 0o700); err != nil {
		return fmt.Errorf("create workspace directory: %w", err)
	}
	volume := path + ":/workspace:rw"
	_, err = m.run(ctx,
		"create", "--name", spec.Name,
		"--label", "ayati.workspace="+strings.TrimPrefix(spec.Name, "ayati-workspace-"),
		"--read-only", "--cap-drop", "ALL", "--security-opt", "no-new-privileges",
		"--pids-limit", "256", "--memory", "2g", "--cpus", "2",
		"--tmpfs", "/tmp:rw,nosuid,nodev,size=256m",
		"--tmpfs", "/home/ayati:rw,nosuid,nodev,size=512m,uid=1000,gid=1000",
		"--mount", "type=bind,src="+path+",dst=/workspace",
		"--workdir", "/workspace", m.image,
	)
	if err != nil {
		return fmt.Errorf("create sandbox with %s: %w", volume, err)
	}
	if _, err := m.run(ctx, "start", spec.Name); err != nil {
		return fmt.Errorf("start sandbox: %w", err)
	}
	return nil
}

func (m *Manager) Open(name string, variables map[string]string) (agent.Shell, error) {
	if err := validateName(name); err != nil {
		return nil, err
	}
	if err := validateVariables(variables); err != nil {
		return nil, err
	}
	return &Shell{manager: m, name: name, timeout: commandTimeout, variables: copyVariables(variables)}, nil
}

func (m *Manager) Remove(ctx context.Context, name string) error {
	if err := validateName(name); err != nil {
		return err
	}
	_, exists, err := m.inspect(ctx, name)
	if err != nil || !exists {
		return err
	}
	_, err = m.run(ctx, "rm", "--force", "--volumes", name)
	return err
}

func (m *Manager) stop(ctx context.Context, name string) error {
	if err := validateName(name); err != nil {
		return err
	}
	_, exists, err := m.inspect(ctx, name)
	if err != nil || !exists {
		return err
	}
	_, err = m.run(ctx, "stop", "--time", "1", name)
	return err
}

func (m *Manager) inspect(ctx context.Context, name string) (containerInfo, bool, error) {
	if err := validateName(name); err != nil {
		return containerInfo{}, false, err
	}
	format := `{{.State.Running}}|{{index .Config.Labels "ayati.workspace"}}|{{range .Mounts}}{{if eq .Destination "/workspace"}}{{.Source}}{{end}}{{end}}`
	result, err := m.runner.Run(ctx, "container", "inspect", "--format", format, name)
	if err == nil {
		parts := strings.SplitN(strings.TrimSpace(result.stdout), "|", 3)
		if len(parts) != 3 {
			return containerInfo{}, false, errors.New("inspect sandbox: invalid Docker metadata")
		}
		wantLabel := strings.TrimPrefix(name, "ayati-workspace-")
		if parts[1] != wantLabel || strings.TrimSpace(parts[2]) == "" {
			return containerInfo{}, false, fmt.Errorf("container %s is not owned by Ayati", name)
		}
		return containerInfo{running: parts[0] == "true", path: parts[2]}, true, nil
	}
	if strings.Contains(result.stderr, "No such container") || strings.Contains(result.stderr, "No such object") {
		return containerInfo{}, false, nil
	}
	return containerInfo{}, false, fmt.Errorf("inspect sandbox: %w: %s", err, strings.TrimSpace(result.stderr))
}

func (m *Manager) run(ctx context.Context, arguments ...string) (commandResult, error) {
	result, err := m.runner.Run(ctx, arguments...)
	if err != nil {
		message := strings.TrimSpace(result.stderr)
		if message == "" {
			message = err.Error()
		}
		return result, errors.New(message)
	}
	return result, nil
}

type Shell struct {
	manager   *Manager
	name      string
	timeout   time.Duration
	variables map[string]string
}

func (s *Shell) Execute(ctx context.Context, request agent.ShellRequest) agent.ShellResult {
	started := time.Now()
	result := agent.ShellResult{Command: request.Command, ExitCode: -1}
	command := strings.TrimSpace(request.Command)
	if command == "" {
		result.Error = "shell command is empty"
		return result
	}
	if len(command) > commandLimit {
		result.Error = "shell command exceeds 64 KiB"
		return result
	}
	callContext, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	seconds := strconv.Itoa(int(s.timeout.Seconds()))
	input, arguments := environmentCommand(s.name, seconds, command, s.variables)
	output, err := s.manager.runner.RunInput(callContext, input, arguments...)
	contextErr := callContext.Err()
	var stopErr error
	if contextErr != nil {
		stopContext, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
		stopErr = s.manager.stop(stopContext, s.name)
		stopCancel()
	}
	result.Stdout = redactEnvironment(output.stdout, s.variables, output.truncated)
	result.Stderr = redactEnvironment(output.stderr, s.variables, output.truncated)
	result.ExitCode = output.exitCode
	result.Truncated = output.truncated
	result.Duration = time.Since(started)
	result.TimedOut = errors.Is(contextErr, context.DeadlineExceeded) || output.exitCode == 124
	if result.TimedOut {
		result.Error = "shell command timed out"
	} else if errors.Is(contextErr, context.Canceled) {
		result.Error = "shell command canceled"
	} else if err != nil && output.exitCode == -1 {
		result.Error = err.Error()
	}
	if stopErr != nil {
		result.Error = strings.TrimSpace(result.Error + "; stop sandbox: " + stopErr.Error())
	}
	return result
}

func validateSpec(spec Spec) (string, error) {
	if err := validateName(spec.Name); err != nil {
		return "", err
	}
	path, err := filepath.Abs(strings.TrimSpace(spec.Path))
	if err != nil || strings.TrimSpace(spec.Path) == "" {
		return "", errors.New("workspace path is required")
	}
	return path, nil
}

func validateName(name string) error {
	if !strings.HasPrefix(name, "ayati-workspace-") || len(name) > 80 {
		return errors.New("invalid Ayati sandbox name")
	}
	for _, character := range name {
		if (character < 'a' || character > 'z') && (character < '0' || character > '9') && character != '-' {
			return errors.New("invalid Ayati sandbox name")
		}
	}
	return nil
}
