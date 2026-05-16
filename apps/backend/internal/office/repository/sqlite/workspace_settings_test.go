package sqlite_test

import (
	"context"
	"testing"
)

func TestWorkspaceGovernance_Defaults(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	got, err := repo.GetRequireApprovalForNewAgents(ctx, "ws-1")
	if err != nil {
		t.Fatalf("GetRequireApprovalForNewAgents: %v", err)
	}
	if got {
		t.Error("expected false (default), got true")
	}
}

func TestWorkspaceGovernance_SetAndGet(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	if err := repo.SetRequireApprovalForNewAgents(ctx, "ws-1", true); err != nil {
		t.Fatalf("SetRequireApprovalForNewAgents: %v", err)
	}
	got, err := repo.GetRequireApprovalForNewAgents(ctx, "ws-1")
	if err != nil {
		t.Fatalf("GetRequireApprovalForNewAgents: %v", err)
	}
	if !got {
		t.Error("expected true after set, got false")
	}

	// Toggle back to false.
	if err := repo.SetRequireApprovalForNewAgents(ctx, "ws-1", false); err != nil {
		t.Fatalf("set false: %v", err)
	}
	got, _ = repo.GetRequireApprovalForNewAgents(ctx, "ws-1")
	if got {
		t.Error("expected false after toggle, got true")
	}
}

func TestWorkspaceGovernance_AllThreeFlags(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	for _, flag := range []struct {
		set func(context.Context, string, bool) error
		get func(context.Context, string) (bool, error)
	}{
		{repo.SetRequireApprovalForNewAgents, repo.GetRequireApprovalForNewAgents},
		{repo.SetRequireApprovalForTaskCompletion, repo.GetRequireApprovalForTaskCompletion},
		{repo.SetRequireApprovalForSkillChanges, repo.GetRequireApprovalForSkillChanges},
	} {
		if err := flag.set(ctx, "ws-2", true); err != nil {
			t.Fatalf("set: %v", err)
		}
		v, err := flag.get(ctx, "ws-2")
		if err != nil {
			t.Fatalf("get: %v", err)
		}
		if !v {
			t.Error("expected true after set")
		}
	}
}
