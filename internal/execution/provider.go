package execution

import (
	"context"
	"errors"
	"time"

	"github.com/Saieshwar5/perpetual/internal/workspace"
)

// StubProvider completes every request with a stop, proving the loop works
// end-to-end without external credentials.
type StubProvider struct{}

// Complete returns a finished response without tool calls.
func (StubProvider) Complete(_ context.Context, _ ModelRequest) (ModelResponse, error) {
	return ModelResponse{Content: "stub provider completed the execution room.", StopReason: "stop"}, nil
}

// RunWorker repeatedly claims and executes runs until ctx is canceled. It
// treats empty queues and quota blocks as idle waits instead of failures.
func RunWorker(ctx context.Context, worker *Worker, wait time.Duration) {
	if wait <= 0 {
		wait = 500 * time.Millisecond
	}
	for {
		err := worker.WorkOnce(ctx)
		if ctx.Err() != nil {
			return
		}
		if err == nil {
			continue
		}
		if errors.Is(err, errNoRuns) || errors.Is(err, workspace.ErrQuotaReached) {
			select {
			case <-ctx.Done():
				return
			case <-time.After(wait):
			}
			continue
		}
		// Per-run failures are already recorded durably by WorkOnce; keep the
		// worker alive and back off briefly so a poisoned run cannot spin loop.
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
	}
}
