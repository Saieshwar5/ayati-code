package environments

import (
	"context"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/Saieshwar5/perpetual/internal/vmagent"
)

func TestSyncerRoundTrip(t *testing.T) {
	vmRoot := t.TempDir()
	handler := &vmagent.Handler{Root: vmRoot, Env: map[string]string{"PATH": os.Getenv("PATH")}}
	server := httptest.NewServer(handler.DataHandler())
	defer server.Close()

	api := &fakeAPI{token: "token", active: map[string]bool{"vm-1": true}}
	manager, err := NewManager(Config{Region: "us-east-1", ImageARN: "arn:image", ImageVersion: "1.0"}, api)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	// Override the endpoint used by Shell: the fake API returns the test server.
	api.endpoint = server.URL
	instance := Instance{MicrovmID: "vm-1", Endpoint: server.URL, State: "RUNNING"}

	syncer, err := NewSyncer(manager)
	if err != nil {
		t.Fatalf("NewSyncer: %v", err)
	}
	source := t.TempDir()
	if err := os.WriteFile(filepath.Join(source, "README.md"), []byte("# hello"), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}
	if err := syncer.Push(context.Background(), instance, source); err != nil {
		t.Fatalf("Push: %v", err)
	}

	scratch := t.TempDir()
	if err := syncer.Pull(context.Background(), instance, scratch); err != nil {
		t.Fatalf("Pull: %v", err)
	}
	value, err := os.ReadFile(filepath.Join(scratch, "README.md"))
	if err != nil || string(value) != "# hello" {
		t.Fatalf("pulled README = %q, %v", value, err)
	}
}
