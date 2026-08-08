package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	agentruntime "github.com/Saieshwar5/ayati-code/internal/runtime"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func TestRunExecutesOneShotRequestAndEmitsTerminalEvent(t *testing.T) {
	workspace := t.TempDir()
	configPath := filepath.Join(t.TempDir(), "runtime.json")
	config := `{
  "provider":{"kind":"openai-chat","model":"test-model","endpoint":"https://example.test/chat","api_key_env":"AYATI_RUNTIME_TEST_KEY"},
  "limits":{"max_steps":3,"run_timeout_seconds":10,"model_timeout_seconds":5,"shell_timeout_seconds":5,"max_tool_output_bytes":1024}
}`
	if err := os.WriteFile(configPath, []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AYATI_RUNTIME_TEST_KEY", "parent-only-secret")

	responses := []string{
		`{"choices":[{"message":{"role":"assistant","tool_calls":[{"id":"call-1","type":"function","function":{"name":"shell","arguments":"{\"command\":\"test -z \\\"$AYATI_RUNTIME_TEST_KEY\\\" && printf runtime-ok > result.txt\"}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":10,"completion_tokens":2,"total_tokens":12}}`,
		`{"choices":[{"message":{"role":"assistant","content":"completed"},"finish_reason":"stop"}],"usage":{"prompt_tokens":14,"completion_tokens":1,"total_tokens":15}}`,
	}
	calls := 0
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		response := &http.Response{StatusCode: 200, Status: "200 OK", Header: make(http.Header), Body: io.NopCloser(strings.NewReader(responses[calls]))}
		calls++
		return response, nil
	})}
	request, err := json.Marshal(map[string]string{"run_id": "integration-run", "prompt": "create result", "workspace": workspace})
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	var errors bytes.Buffer
	exitCode := runWithClient([]string{"run", "--config", configPath}, bytes.NewReader(request), &output, &errors, client)
	if exitCode != 0 {
		t.Fatalf("exit code = %d, stderr=%s, stdout=%s", exitCode, errors.String(), output.String())
	}
	content, err := os.ReadFile(filepath.Join(workspace, "result.txt"))
	if err != nil {
		t.Fatalf("read result: %v", err)
	}
	if string(content) != "runtime-ok" || calls != 2 {
		t.Fatalf("result=%q provider calls=%d", content, calls)
	}

	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	if len(lines) != 6 {
		t.Fatalf("event lines = %d\n%s", len(lines), output.String())
	}
	var terminal agentruntime.Event
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &terminal); err != nil {
		t.Fatalf("decode terminal event: %v", err)
	}
	if terminal.Type != agentruntime.EventRunCompleted || terminal.Outcome == nil || terminal.Outcome.Status != agentruntime.StatusCompleted || terminal.Outcome.ToolCalls != 1 {
		t.Fatalf("unexpected terminal event: %+v", terminal)
	}
}
