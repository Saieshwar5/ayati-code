package environments

import (
	"context"
	"testing"
)

type fakeAPI struct {
	calls  int
	token  string
	active map[string]bool
}

func (f *fakeAPI) RunMicrovm(_ context.Context, input RunMicrovmInput) (Instance, error) {
	f.calls++
	if f.active == nil {
		f.active = make(map[string]bool)
	}
	f.active["vm-1"] = true
	return Instance{MicrovmID: "vm-1", Endpoint: "example.test", State: "RUNNING", ImageARN: input.ImageARN}, nil
}

func (f *fakeAPI) AuthToken(_ context.Context, id string) (string, error) {
	f.calls++
	return f.token, nil
}

func (f *fakeAPI) SuspendMicrovm(_ context.Context, id string) error {
	f.active[id] = false
	return nil
}

func (f *fakeAPI) ResumeMicrovm(_ context.Context, id string) error {
	f.active[id] = true
	return nil
}

func (f *fakeAPI) TerminateMicrovm(_ context.Context, id string) error {
	f.active[id] = false
	return nil
}

func (f *fakeAPI) GetMicrovm(_ context.Context, id string) (Instance, error) {
	return Instance{MicrovmID: id, Endpoint: "example.test", State: "RUNNING"}, nil
}

func TestManagerCreateAndShell(t *testing.T) {
	api := &fakeAPI{token: "jwe-token"}
	manager, err := NewManager(Config{Region: "us-east-1", ImageARN: "arn:image", ImageVersion: "1.0"}, api)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	instance, err := manager.Create(context.Background())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if instance.MicrovmID != "vm-1" || instance.Endpoint != "example.test" {
		t.Fatalf("instance = %#v", instance)
	}
	shell, err := manager.Shell(context.Background(), instance)
	if err != nil {
		t.Fatalf("Shell: %v", err)
	}
	if shell == nil {
		t.Fatal("nil shell client")
	}
	if api.calls < 2 {
		t.Fatalf("expected run + token calls, got %d", api.calls)
	}
}

func TestManagerValidatesConfig(t *testing.T) {
	if _, err := NewManager(Config{Region: "us-east-1"}, &fakeAPI{}); err == nil {
		t.Fatal("expected missing image arn error")
	}
	if _, err := NewManager(Config{Region: "us-east-1", ImageARN: "arn:image"}, nil); err == nil {
		t.Fatal("expected nil api error")
	}
}
