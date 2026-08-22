package workspaceruntime

import (
	"context"
	"errors"
	"sync"

	"github.com/Saieshwar5/perpetual/internal/environments"
	"github.com/Saieshwar5/perpetual/internal/exec"
)

// LambdaRuntime executes workspace shells inside AWS Lambda MicroVMs. It is a
// Runtime implementation; the controller remains the owner of Git and secrets.
type LambdaRuntime struct {
	manager   *environments.Manager
	mu        sync.Mutex
	instances map[string]environments.Instance
}

// NewLambda builds a Lambda runtime from an environment manager.
func NewLambda(manager *environments.Manager) (*LambdaRuntime, error) {
	if manager == nil {
		return nil, errors.New("lambda runtime manager is required")
	}
	return &LambdaRuntime{manager: manager, instances: make(map[string]environments.Instance)}, nil
}

func (r *LambdaRuntime) Start(ctx context.Context, ref Ref) error {
	instance, err := r.manager.Create(ctx)
	if err != nil {
		return err
	}
	r.mu.Lock()
	r.instances[ref.ID] = instance
	r.mu.Unlock()
	return nil
}

func (r *LambdaRuntime) Stop(ctx context.Context, ref Ref) error {
	id, ok := r.instanceID(ref.ID)
	if !ok {
		return nil // idempotent stop for unknown instances
	}
	return r.manager.Suspend(ctx, id)
}

func (r *LambdaRuntime) Resume(ctx context.Context, ref Ref) error {
	id, ok := r.instanceID(ref.ID)
	if !ok {
		return nil
	}
	return r.manager.Resume(ctx, id)
}

func (r *LambdaRuntime) Destroy(ctx context.Context, ref Ref) error {
	id, ok := r.instanceID(ref.ID)
	if !ok {
		return nil
	}
	err := r.manager.Terminate(ctx, id)
	if err == nil {
		r.mu.Lock()
		delete(r.instances, ref.ID)
		r.mu.Unlock()
	}
	return err
}

func (r *LambdaRuntime) OpenShell(ctx context.Context, ref Ref, _ map[string]string) (exec.Shell, error) {
	instance, ok := r.instance(ref.ID)
	if !ok {
		return nil, errors.New("lambda runtime instance is not running for " + ref.ID)
	}
	client, err := r.manager.Shell(ctx, instance)
	if err != nil {
		return nil, err
	}
	return NewRemoteShell(client)
}

func (r *LambdaRuntime) instanceID(id string) (string, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	instance, ok := r.instances[id]
	if !ok {
		return "", false
	}
	return instance.MicrovmID, true
}

func (r *LambdaRuntime) instance(id string) (environments.Instance, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	instance, ok := r.instances[id]
	return instance, ok
}
