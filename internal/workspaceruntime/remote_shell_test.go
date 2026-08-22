package workspaceruntime

import (
	"context"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/Saieshwar5/perpetual/internal/exec"
	"github.com/Saieshwar5/perpetual/internal/vmagent"
)

func TestRemoteShellExecutesInAgent(t *testing.T) {
	handler := &vmagent.Handler{Root: t.TempDir(), Env: map[string]string{"PATH": os.Getenv("PATH")}}
	server := httptest.NewServer(handler.DataHandler())
	defer server.Close()

	client, err := vmagent.NewClient(server.URL, "token")
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	shell, err := NewRemoteShell(client)
	if err != nil {
		t.Fatalf("NewRemoteShell: %v", err)
	}
	result := shell.Execute(context.Background(), exec.ShellRequest{Command: "printf remote-ok"})
	if result.ExitCode != 0 || result.Stdout != "remote-ok" {
		t.Fatalf("result = %#v", result)
	}
	if shell == nil {
		t.Fatal("shell should not be nil")
	}
}
