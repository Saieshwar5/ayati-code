package workspaceruntime

import (
	"context"
	"strings"
	"testing"
)

func TestNewCloudRejectsMissingConfiguration(t *testing.T) {
	if _, err := NewCloud(CloudConfig{}); err == nil ||
		!strings.Contains(err.Error(), "not configured") {
		t.Fatalf("NewCloud error = %v", err)
	}
}

func TestCloudStubReportsNotImplemented(t *testing.T) {
	runtime, err := NewCloud(CloudConfig{Endpoint: "https://runtime.test", Token: "secret"})
	if err != nil {
		t.Fatalf("NewCloud: %v", err)
	}
	ref := Ref{ID: "workspace-1", RuntimeID: "runtime-1", Directory: "/workspace"}
	ctx := context.Background()
	for name, call := range map[string]func() error{
		"start":   func() error { return runtime.Start(ctx, ref) },
		"stop":    func() error { return runtime.Stop(ctx, ref) },
		"destroy": func() error { return runtime.Destroy(ctx, ref) },
	} {
		if err := call(); err == nil || !strings.Contains(err.Error(), "not implemented") {
			t.Fatalf("%s error = %v", name, err)
		}
	}
	if _, err := runtime.OpenShell(ctx, ref, nil); err == nil ||
		!strings.Contains(err.Error(), "not implemented") {
		t.Fatalf("OpenShell error = %v", err)
	}
}
