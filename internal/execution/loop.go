package execution

import (
	"context"
	"errors"
	"fmt"

	"github.com/Saieshwar5/perpetual/internal/workspace"
)

// Worker claims durable runs and drives the execution-room loop for one run at
// a time. Multiple Worker instances can share one store; SKIP LOCKED claims
// keep them from executing the same run.
type Worker struct {
	store    *workspace.Store
	provider ModelProvider
	shell    ShellRunner
}

// NewWorker builds a run worker.
func NewWorker(store *workspace.Store, provider ModelProvider, shell ShellRunner) (*Worker, error) {
	if store == nil {
		return nil, errors.New("execution worker store is required")
	}
	if provider == nil {
		return nil, errors.New("execution worker model provider is required")
	}
	if shell == nil {
		return nil, errors.New("execution worker shell is required")
	}
	return &Worker{store: store, provider: provider, shell: shell}, nil
}

// WorkOnce claims and executes one queued execution room. It returns ErrNoRuns
// when the queue is empty and records run failures durably before returning
// per-run errors.
func (w *Worker) WorkOnce(ctx context.Context) error {
	run, err := w.store.ClaimNextRun(ctx)
	if err != nil {
		if errors.Is(err, workspace.ErrNoRuns) {
			return errNoRuns
		}
		return err
	}
	return w.execute(ctx, run)
}

// execute drives one run's loop with a fresh model/provider view. Completed
// steps are skipped by idempotent key so crashed workers resume safely.
func (w *Worker) execute(ctx context.Context, run workspace.Run) error {
	steps, err := w.store.RunSteps(ctx, run.ID)
	if err != nil {
		return err
	}
	done := make(map[string]bool, len(steps))
	for _, step := range steps {
		if step.Status == "done" {
			done[step.StepKey] = true
		}
	}
	version := int64(0)
	for step := run.StepCursor; step < run.MaxSteps; step++ {
		select {
		case <-ctx.Done():
			_ = w.store.FailRun(ctx, run.ID, "execution room canceled")
			return ctx.Err()
		default:
		}
		if err := w.store.TouchRunLease(ctx, run.ID); err != nil {
			return err
		}
		modelKey := fmt.Sprintf("step-%d", step)
		if !done[modelKey] {
			request := ModelRequest{
				System: "You are Perpetual's coding agent. You have exactly one tool: shell(command). " +
					"Run commands to inspect the repository, install dependencies, and make changes.",
				Messages:  []string{fmt.Sprintf("Run %s for workspace %s: continue the task.", run.ID, run.WorkspaceID)},
				MaxTokens: 4096,
				Tools:     []string{"shell"},
			}
			response, err := w.provider.Complete(ctx, request)
			if err != nil {
				_ = w.store.FailRun(ctx, run.ID, "model call failed")
				return err
			}
			if err := w.store.AppendRunStep(ctx, run.ID, modelKey, workspace.StepModel, "done",
				map[string]any{"request": request},
				map[string]any{"content": response.Content, "stop_reason": response.StopReason, "usage": response.Usage}); err != nil {
				return err
			}
			if response.StopReason == "stop" {
				return w.store.CompleteRun(ctx, run.ID, response.Content)
			}
			for _, call := range response.ToolCalls {
				if call.Name != "shell" {
					continue
				}
				shellKey := fmt.Sprintf("shell-%d", step)
				command := commandFrom(call.Arguments)
				if done[shellKey] {
					continue
				}
				result, err := w.shell.Run(ctx, command)
				if err != nil {
					_ = w.store.FailRun(ctx, run.ID, "shell execution failed")
					return err
				}
				if err := w.store.AppendRunStep(ctx, run.ID, shellKey, workspace.StepShell, "done",
					map[string]any{"command": command, "tool_call": call},
					map[string]any{"stdout": result.Stdout, "stderr": result.Stderr,
						"exit_code": result.ExitCode, "timed_out": result.TimedOut,
						"truncated": result.Truncated, "duration": result.Duration}); err != nil {
					return err
				}
			}
		}
		// Work memory is advanced optimistically so later PRs can surface it.
		version++
		if err := w.store.SaveWorkMemory(ctx, run.ID, map[string]any{"steps_completed": step + 1}, version); err != nil {
			return err
		}
		if err := w.store.TouchRunLease(ctx, run.ID); err != nil {
			return err
		}
	}
	return w.store.FailRun(ctx, run.ID, "execution room reached max steps")
}

func commandFrom(arguments map[string]any) string {
	value, ok := arguments["command"]
	if !ok {
		return ""
	}
	text, _ := value.(string)
	return text
}
