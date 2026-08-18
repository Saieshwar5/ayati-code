package sandbox

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Saieshwar5/perpetual/internal/agent"
)

type fakeRunner struct {
	results []commandResult
	errors  []error
	calls   [][]string
	inputs  []string
}

func (f *fakeRunner) Run(_ context.Context, arguments ...string) (commandResult, error) {
	return f.RunInput(context.Background(), "", arguments...)
}

func (f *fakeRunner) RunInput(_ context.Context, input string, arguments ...string) (commandResult, error) {
	f.calls = append(f.calls, append([]string(nil), arguments...))
	f.inputs = append(f.inputs, input)
	result, err := commandResult{}, error(nil)
	if len(f.results) > 0 {
		result, f.results = f.results[0], f.results[1:]
	}
	if len(f.errors) > 0 {
		err, f.errors = f.errors[0], f.errors[1:]
	}
	return result, err
}

func TestShellExecutesOnlyCommandInsideLeasedRuntime(t *testing.T) {
	runner := &fakeRunner{results: []commandResult{{stdout: "ok\n", exitCode: 0}}}
	shell := &Shell{
		runner: runner, stop: func(context.Context, string) error { return nil },
		name: strings.Repeat("a", 64), timeout: 2 * time.Minute,
	}
	result := shell.Execute(context.Background(), agent.ShellRequest{Command: "go test ./..."})
	if result.ExitCode != 0 || result.Stdout != "ok\n" || result.Error != "" {
		t.Fatalf("result = %#v", result)
	}
	call := runner.calls[0]
	if call[0] != "exec" || call[1] != "-i" || call[2] != shell.name || call[len(call)-1] != "go test ./..." {
		t.Fatalf("call = %#v", call)
	}
}

func TestShellInjectsAndRedactsWorkspaceEnvironment(t *testing.T) {
	runner := &fakeRunner{results: []commandResult{{
		stdout: "token=very-secret-value\n", stderr: "very-secret-value failed\n", exitCode: 1,
	}}}
	driver := &DockerDriver{runner: runner}
	shell, err := driver.OpenRuntime(strings.Repeat("b", 64), map[string]string{"PROJECT_TOKEN": "very-secret-value"})
	if err != nil {
		t.Fatalf("OpenRuntime: %v", err)
	}
	result := shell.Execute(context.Background(), agent.ShellRequest{Command: "printenv PROJECT_TOKEN"})
	arguments := strings.Join(runner.calls[0], " ")
	if strings.Contains(arguments, "very-secret-value") || !strings.Contains(runner.inputs[0], "very-secret-value") {
		t.Fatalf("arguments = %q, input = %q", arguments, runner.inputs[0])
	}
	if strings.Contains(result.Stdout+result.Stderr, "very-secret-value") ||
		!strings.Contains(result.Stdout, "[REDACTED:PROJECT_TOKEN]") {
		t.Fatalf("result = %#v", result)
	}
}

func TestShellCancellationStopsExactRuntime(t *testing.T) {
	runner := &fakeRunner{results: []commandResult{{exitCode: -1}}}
	stopped := ""
	shell := &Shell{
		runner: runner, stop: func(_ context.Context, target string) error { stopped = target; return nil },
		name: strings.Repeat("c", 64), timeout: time.Minute,
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result := shell.Execute(ctx, agent.ShellRequest{Command: "sleep 30"})
	if result.Error != "shell command canceled" || stopped != shell.name {
		t.Fatalf("result = %#v, stopped = %q", result, stopped)
	}
}

func TestRedactionCoversSecretAtTruncationBoundary(t *testing.T) {
	value := redactEnvironment("output very-secr", map[string]string{"TOKEN": "very-secret-value"}, true)
	if strings.Contains(value, "very-secr") || !strings.Contains(value, "[REDACTED:TOKEN]") {
		t.Fatalf("redacted output = %q", value)
	}
}

func TestDockerDriverRejectsUnsafeRuntimeInput(t *testing.T) {
	driver := &DockerDriver{runner: &fakeRunner{}}
	if _, err := driver.OpenRuntime("--privileged", nil); err == nil {
		t.Fatal("OpenRuntime accepted an unsafe runtime identity")
	}
	if _, err := driver.OpenRuntime(strings.Repeat("d", 64), map[string]string{"BAD-NAME": "value"}); err == nil {
		t.Fatal("OpenRuntime accepted an unsafe environment name")
	}
	if _, err := driver.ResolveImage(context.Background(), "--privileged"); err == nil {
		t.Fatalf("ResolveImage error = %v", err)
	}
}
