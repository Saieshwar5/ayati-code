package sandbox

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Saieshwar5/ayati-code/internal/agent"
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

func TestManagerCreatesOnePersistentSandbox(t *testing.T) {
	path := filepath.Join(t.TempDir(), "repo")
	cache := filepath.Join(filepath.Dir(path), "cache")
	runner := &fakeRunner{
		results: []commandResult{
			{stderr: "No such container"}, {}, {},
			{stdout: "true|1234|ayati:test|" + path + "|false|" + cache + "\n"},
		},
		errors: []error{errors.New("missing")},
	}
	manager := &Manager{docker: "docker", image: "ayati:test", runner: runner}
	mode, err := manager.Ensure(context.Background(), Spec{
		Name: "ayati-workspace-1234", Path: path, MountMode: MountReadOnly,
	})
	if err != nil || mode != MountReadOnly {
		t.Fatalf("Ensure: %v", err)
	}
	if len(runner.calls) != 4 {
		t.Fatalf("calls = %#v", runner.calls)
	}
	create := runner.calls[1]
	if create[0] != "create" || create[len(create)-1] != "ayati:test" {
		t.Fatalf("create call = %#v", create)
	}
	joined := strings.Join(create, " ")
	for _, expected := range []string{
		"--read-only", "--cap-drop ALL", "dst=/workspace,readonly", "--pids-limit 256", "dst=/cache",
	} {
		if !strings.Contains(joined, expected) {
			t.Fatalf("create call %q does not contain %q", joined, expected)
		}
	}
}

func TestManagerReusesRunningSandbox(t *testing.T) {
	path := t.TempDir()
	cache := filepath.Join(filepath.Dir(path), "cache")
	runner := &fakeRunner{results: []commandResult{{
		stdout: "true|1234|ayati:test|" + path + "|false|" + cache + "\n",
	}}}
	manager := &Manager{docker: "docker", image: "ayati:test", runner: runner}
	if _, err := manager.Ensure(context.Background(), Spec{
		Name: "ayati-workspace-1234", Path: path, MountMode: MountReadOnly,
	}); err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	if len(runner.calls) != 1 || !reflect.DeepEqual(runner.calls[0][:3], []string{"container", "inspect", "--format"}) {
		t.Fatalf("calls = %#v", runner.calls)
	}
}

func TestManagerRecreatesSandboxWhenMountModeDiffers(t *testing.T) {
	path := t.TempDir()
	cache := filepath.Join(filepath.Dir(path), "cache")
	runner := &fakeRunner{results: []commandResult{
		{stdout: "true|1234|ayati:test|" + path + "|true|" + cache + "\n"},
		{}, {}, {},
		{stdout: "true|1234|ayati:test|" + path + "|false|" + cache + "\n"},
	}}
	manager := &Manager{docker: "docker", image: "ayati:test", runner: runner}
	mode, err := manager.Ensure(context.Background(), Spec{
		Name: "ayati-workspace-1234", Path: path, MountMode: MountReadOnly,
	})
	if err != nil || mode != MountReadOnly {
		t.Fatalf("Ensure mode = %q, error = %v", mode, err)
	}
	if len(runner.calls) != 5 || runner.calls[1][0] != "rm" || runner.calls[2][0] != "create" ||
		!strings.Contains(strings.Join(runner.calls[2], " "), "dst=/workspace,readonly") {
		t.Fatalf("calls = %#v", runner.calls)
	}
}

func TestManagerRecreatesSandboxWhenCachePathDiffers(t *testing.T) {
	path := t.TempDir()
	cache := filepath.Join(filepath.Dir(path), "cache")
	runner := &fakeRunner{results: []commandResult{
		{stdout: "true|1234|ayati:test|" + path + "|false|/tmp/old-cache\n"},
		{}, {}, {},
		{stdout: "true|1234|ayati:test|" + path + "|false|" + cache + "\n"},
	}}
	manager := &Manager{docker: "docker", image: "ayati:test", runner: runner}
	if _, err := manager.Ensure(context.Background(), Spec{
		Name: "ayati-workspace-1234", Path: path, MountMode: MountReadOnly,
	}); err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	if len(runner.calls) != 5 || runner.calls[1][0] != "rm" || runner.calls[2][0] != "create" {
		t.Fatalf("calls = %#v", runner.calls)
	}
}

func TestManagerRejectsWritableCacheInsideExploreProject(t *testing.T) {
	path := t.TempDir()
	manager := &Manager{runner: &fakeRunner{}}
	_, err := manager.Ensure(context.Background(), Spec{
		Name: "ayati-workspace-1234", Path: path,
		CachePath: filepath.Join(path, ".cache"), MountMode: MountReadOnly,
	})
	if err == nil || !strings.Contains(err.Error(), "outside the project") {
		t.Fatalf("Ensure error = %v", err)
	}
}

func TestShellExecutesOnlyCommandInsideWorkspaceSandbox(t *testing.T) {
	runner := &fakeRunner{results: []commandResult{{stdout: "ok\n", exitCode: 0}}}
	manager := &Manager{docker: "docker", image: "ayati:test", runner: runner}
	shell := &Shell{manager: manager, name: "ayati-workspace-1234", timeout: 2 * time.Minute}
	result := shell.Execute(context.Background(), agent.ShellRequest{Command: "go test ./..."})
	if result.ExitCode != 0 || result.Stdout != "ok\n" || result.Error != "" {
		t.Fatalf("result = %#v", result)
	}
	call := runner.calls[0]
	if call[0] != "exec" || call[1] != "-i" || call[2] != "ayati-workspace-1234" || call[len(call)-1] != "go test ./..." {
		t.Fatalf("call = %#v", call)
	}
}

func TestShellInjectsAndRedactsWorkspaceEnvironment(t *testing.T) {
	runner := &fakeRunner{results: []commandResult{{
		stdout: "token=very-secret-value\n", stderr: "very-secret-value failed\n", exitCode: 1,
	}}}
	manager := &Manager{docker: "docker", image: "ayati:test", runner: runner}
	shell, err := manager.Open("ayati-workspace-1234", map[string]string{"PROJECT_TOKEN": "very-secret-value"})
	if err != nil {
		t.Fatalf("Open: %v", err)
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

func TestRedactionCoversSecretAtTruncationBoundary(t *testing.T) {
	value := redactEnvironment("output very-secr", map[string]string{"TOKEN": "very-secret-value"}, true)
	if strings.Contains(value, "very-secr") || !strings.Contains(value, "[REDACTED:TOKEN]") {
		t.Fatalf("redacted output = %q", value)
	}
}

func TestManagerRejectsUnscopedContainerName(t *testing.T) {
	manager := &Manager{runner: &fakeRunner{}}
	if err := manager.Remove(context.Background(), "postgres"); err == nil {
		t.Fatal("Remove accepted an unscoped container")
	}
}

func TestManagerRejectsUnsafeEnvironmentName(t *testing.T) {
	manager := &Manager{runner: &fakeRunner{}}
	if _, err := manager.Open("ayati-workspace-1234", map[string]string{"BAD-NAME": "value"}); err == nil {
		t.Fatal("Open accepted an unsafe environment name")
	}
}

func TestManagerDoesNotRemoveContainerWithoutOwnershipLabel(t *testing.T) {
	runner := &fakeRunner{results: []commandResult{{
		stdout: "true|someone-else|ayati:test|/tmp/project|true|/tmp/cache\n",
	}}}
	manager := &Manager{runner: runner}
	if err := manager.Remove(context.Background(), "ayati-workspace-1234"); err == nil {
		t.Fatal("Remove accepted a container with another ownership label")
	}
	if len(runner.calls) != 1 {
		t.Fatalf("calls = %#v", runner.calls)
	}
}
