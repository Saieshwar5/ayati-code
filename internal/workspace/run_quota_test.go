package workspace

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
)

func TestClaimNextRunWithLimitsBlocksSecondWorkspaceRun(t *testing.T) {
	store, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer store.Close()
	ctx := context.Background()
	ws, err := store.Create(ctx, Create{
		UserID: "user-q", Repository: "owner/quota", CloneURL: "https://github.com/owner/quota.git",
		BaseBranch: "main", Branch: "main", Path: filepath.Join(t.TempDir(), "repo"),
	})
	if err != nil {
		t.Fatalf("Create workspace: %v", err)
	}
	session, err := store.CreateSession(ctx, ws.ID, "quota session")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	for i := 0; i < 2; i++ {
		if _, err := store.EnqueueRun(ctx, EnqueueRunInput{
			UserID: "user-q", WorkspaceID: ws.ID, SessionID: session.ID,
		}); err != nil {
			t.Fatalf("EnqueueRun %d: %v", i, err)
		}
	}
	limits := ClaimLimits{MaxPerUser: 10, MaxPerWorkspace: 1, MaxGlobal: 10}
	first, err := store.ClaimNextRunWithLimits(ctx, limits)
	if err != nil {
		t.Fatalf("first claim: %v", err)
	}
	if _, err := store.ClaimNextRunWithLimits(ctx, limits); !errors.Is(err, ErrQuotaReached) {
		t.Fatalf("second claim error = %v, want ErrQuotaReached", err)
	}
	if _, err := store.GetRun(ctx, first.ID); err != nil {
		t.Fatalf("GetRun claimed: %v", err)
	}
}
