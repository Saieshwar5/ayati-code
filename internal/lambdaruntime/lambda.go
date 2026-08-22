// Package lambdaruntime implements workspaceruntime.Runtime with durable
// AWS Lambda MicroVM instances backed by the workspace store.
package lambdaruntime

import (
	"bytes"
	"context"
	"errors"

	"github.com/Saieshwar5/perpetual/internal/environments"
	"github.com/Saieshwar5/perpetual/internal/exec"
	"github.com/Saieshwar5/perpetual/internal/vmagent"
	"github.com/Saieshwar5/perpetual/internal/workspace"
	"github.com/Saieshwar5/perpetual/internal/workspaceruntime"
)

// LambdaRuntime executes shells inside Lambda MicroVMs. Instance refs live in
// SQLite so controller restarts can suspend/resume/terminate/reconcile.
type LambdaRuntime struct {
	manager *environments.Manager
	store   *workspace.Store
}

// NewLambda builds a Lambda runtime with durable instance storage.
func NewLambda(manager *environments.Manager, store *workspace.Store) (*LambdaRuntime, error) {
	if manager == nil {
		return nil, errors.New("lambda runtime manager is required")
	}
	if store == nil {
		return nil, errors.New("lambda runtime store is required")
	}
	return &LambdaRuntime{manager: manager, store: store}, nil
}

func (r *LambdaRuntime) Start(ctx context.Context, ref workspaceruntime.Ref) error {
	if _, err := r.store.RuntimeInstance(ctx, ref.ID); err == nil {
		return nil
	}
	instance, err := r.manager.Create(ctx)
	if err != nil {
		return err
	}
	return r.store.SaveRuntimeInstance(ctx, toRuntimeInstance(ref.ID, instance))
}

func (r *LambdaRuntime) Stop(ctx context.Context, ref workspaceruntime.Ref) error {
	stored, err := r.requireInstance(ctx, ref.ID)
	if err != nil {
		return err
	}
	if err := r.manager.Suspend(ctx, stored.InstanceID); err != nil {
		return err
	}
	stored.State = "suspended"
	return r.store.SaveRuntimeInstance(ctx, stored)
}

func (r *LambdaRuntime) Resume(ctx context.Context, ref workspaceruntime.Ref) error {
	stored, err := r.requireInstance(ctx, ref.ID)
	if err != nil {
		return err
	}
	if err := r.manager.Resume(ctx, stored.InstanceID); err != nil {
		return err
	}
	stored.State = "running"
	return r.store.SaveRuntimeInstance(ctx, stored)
}

func (r *LambdaRuntime) Destroy(ctx context.Context, ref workspaceruntime.Ref) error {
	stored, err := r.requireInstance(ctx, ref.ID)
	if err != nil {
		return err
	}
	if err := r.manager.Terminate(ctx, stored.InstanceID); err != nil {
		return err
	}
	return r.store.DeleteRuntimeInstance(ctx, ref.ID)
}

func (r *LambdaRuntime) OpenShell(ctx context.Context, ref workspaceruntime.Ref, _ map[string]string) (exec.Shell, error) {
	stored, err := r.requireInstance(ctx, ref.ID)
	if err != nil {
		return nil, err
	}
	client, err := r.manager.Shell(ctx, environments.Instance{
		MicrovmID: stored.InstanceID,
		Endpoint:  stored.Endpoint,
	})
	if err != nil {
		return nil, err
	}
	return workspaceruntime.NewRemoteShell(client)
}

func (r *LambdaRuntime) requireInstance(ctx context.Context, workspaceID string) (workspace.RuntimeInstance, error) {
	stored, err := r.store.RuntimeInstance(ctx, workspaceID)
	if err != nil {
		return workspace.RuntimeInstance{}, errors.New("lambda runtime instance not found for " + workspaceID)
	}
	return stored, nil
}

func toRuntimeInstance(workspaceID string, instance environments.Instance) workspace.RuntimeInstance {
	return workspace.RuntimeInstance{
		WorkspaceID: workspaceID,
		Provider:    "lambda",
		InstanceID:  instance.MicrovmID,
		Endpoint:    instance.Endpoint,
		ImageARN:    instance.ImageARN,
		State:       instance.State,
	}
}

// Reconcile syncs persisted Lambda instance records with AWS state. Records
// whose microVM is terminated/failed or no longer visible are removed; live
// ones are refreshed to the provider-reported state.
func (r *LambdaRuntime) Reconcile(ctx context.Context) error {
	records, err := r.store.ListRuntimeInstances(ctx)
	if err != nil {
		return err
	}
	for _, record := range records {
		if record.Provider != "lambda" {
			continue
		}
		instance, err := r.manager.Get(ctx, record.InstanceID)
		if err != nil {
			_ = r.store.DeleteRuntimeInstance(ctx, record.WorkspaceID)
			continue
		}
		switch instance.State {
		case "TERMINATED", "FAILED":
			_ = r.store.DeleteRuntimeInstance(ctx, record.WorkspaceID)
		default:
			record.State = instance.State
			if err := r.store.SaveRuntimeInstance(ctx, record); err != nil {
				return err
			}
		}
	}
	return nil
}

// PushRepo serializes the controller working tree and uploads it into the
// workspace microVM working root through the authenticated data plane.
func (r *LambdaRuntime) PushRepo(ctx context.Context, workspaceID, tree string) error {
	stored, err := r.store.RuntimeInstance(ctx, workspaceID)
	if err != nil {
		return err
	}
	client, err := r.manager.Shell(ctx, environments.Instance{
		MicrovmID: stored.InstanceID,
		Endpoint:  stored.Endpoint,
	})
	if err != nil {
		return err
	}
	data, err := vmagent.TarTree(tree)
	if err != nil {
		return err
	}
	return client.Bootstrap(ctx, bytes.NewReader(data))
}
