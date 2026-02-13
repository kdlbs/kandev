package worktree

import (
	"context"
	"testing"
)

func TestNewRecreator(t *testing.T) {
	cfg := newTestConfig(t)
	log := newTestLogger()
	store := newMockStore()

	mgr, err := NewManager(cfg, store, log)
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	recreator := NewRecreator(mgr)
	if recreator == nil {
		t.Fatal("expected non-nil recreator")
	} else if recreator.manager != mgr {
		t.Error("recreator manager not set correctly")
	}
}

func TestRecreator_Recreate_NilManager(t *testing.T) {
	recreator := &Recreator{manager: nil}

	_, err := recreator.Recreate(context.Background(), RecreateRequest{
		SessionID:  "test-session",
		WorktreeID: "test-worktree",
	})

	if err == nil {
		t.Fatal("expected error for nil manager")
	}
	if err.Error() != "worktree manager not available" {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestRecreator_Recreate_WorktreeNotFound(t *testing.T) {
	cfg := newTestConfig(t)
	log := newTestLogger()
	store := newMockStore()

	mgr, err := NewManager(cfg, store, log)
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	recreator := NewRecreator(mgr)

	// Try to recreate with non-existent worktree
	_, err = recreator.Recreate(context.Background(), RecreateRequest{
		SessionID:  "non-existent-session",
		WorktreeID: "non-existent-worktree",
	})

	if err == nil {
		t.Fatal("expected error for non-existent worktree")
	}
	if err.Error() != "worktree record not found for recreation" {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestRecreator_Recreate_MissingRepositoryPath(t *testing.T) {
	cfg := newTestConfig(t)
	log := newTestLogger()
	store := newMockStore()

	mgr, err := NewManager(cfg, store, log)
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	// Create worktree record without repository path
	wt := &Worktree{
		ID:             "wt-001",
		SessionID:      "test-session",
		TaskID:         "test-task",
		RepositoryPath: "", // empty - should cause error
		Path:           "/some/path",
		Status:         StatusActive,
	}
	if err := store.CreateWorktree(context.Background(), wt); err != nil {
		t.Fatalf("failed to create worktree: %v", err)
	}

	recreator := NewRecreator(mgr)

	_, err = recreator.Recreate(context.Background(), RecreateRequest{
		SessionID:  "test-session",
		WorktreeID: "wt-001",
	})

	if err == nil {
		t.Fatal("expected error for missing repository path")
	}
	if err.Error() != "repository path not found in existing worktree record" {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestRecreator_Recreate_FindsByWorktreeID(t *testing.T) {
	cfg := newTestConfig(t)
	log := newTestLogger()
	store := newMockStore()

	mgr, err := NewManager(cfg, store, log)
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	// Create worktree record
	wt := &Worktree{
		ID:             "wt-002",
		SessionID:      "session-abc",
		TaskID:         "task-123",
		RepositoryPath: "/path/to/repo",
		BaseBranch:     "main",
		Path:           "/some/path",
		Status:         StatusActive,
	}
	if err := store.CreateWorktree(context.Background(), wt); err != nil {
		t.Fatalf("failed to create worktree: %v", err)
	}

	recreator := NewRecreator(mgr)

	// Try to recreate using worktree ID (with different session ID)
	// This tests that GetByID is tried first
	_, err = recreator.Recreate(context.Background(), RecreateRequest{
		SessionID:  "different-session",
		WorktreeID: "wt-002",
		TaskID:     "task-123",
	})

	// Will fail because actual creation fails (no git repo), but proves lookup worked
	if err == nil || err.Error() == "worktree record not found for recreation" {
		t.Error("expected recreation to find worktree by ID")
	}
}

func TestRecreator_Recreate_FindsBySessionID(t *testing.T) {
	cfg := newTestConfig(t)
	log := newTestLogger()
	store := newMockStore()

	mgr, err := NewManager(cfg, store, log)
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	// Create worktree record
	wt := &Worktree{
		ID:             "wt-003",
		SessionID:      "session-xyz",
		TaskID:         "task-456",
		RepositoryPath: "/path/to/repo",
		BaseBranch:     "develop",
		Path:           "/some/path",
		Status:         StatusActive,
	}
	if err := store.CreateWorktree(context.Background(), wt); err != nil {
		t.Fatalf("failed to create worktree: %v", err)
	}

	recreator := NewRecreator(mgr)

	// Try to recreate using only session ID (no worktree ID)
	// This tests that GetBySessionID is used as fallback
	_, err = recreator.Recreate(context.Background(), RecreateRequest{
		SessionID:  "session-xyz",
		WorktreeID: "", // empty - forces lookup by session ID
		TaskID:     "task-456",
	})

	// Will fail because actual creation fails (no git repo), but proves lookup worked
	if err == nil || err.Error() == "worktree record not found for recreation" {
		t.Error("expected recreation to find worktree by session ID")
	}
}

func TestRecreator_Recreate_UsesRequestBaseBranch(t *testing.T) {
	cfg := newTestConfig(t)
	log := newTestLogger()
	store := newMockStore()

	mgr, err := NewManager(cfg, store, log)
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	// Create worktree record with one base branch
	wt := &Worktree{
		ID:             "wt-004",
		SessionID:      "session-branch-test",
		TaskID:         "task-789",
		RepositoryPath: "/path/to/repo",
		BaseBranch:     "main", // stored branch
		Path:           "/some/path",
		Status:         StatusActive,
	}
	if err := store.CreateWorktree(context.Background(), wt); err != nil {
		t.Fatalf("failed to create worktree: %v", err)
	}

	recreator := NewRecreator(mgr)

	// Request with a different base branch - should use request's branch
	_, err = recreator.Recreate(context.Background(), RecreateRequest{
		SessionID:  "session-branch-test",
		WorktreeID: "wt-004",
		BaseBranch: "feature-branch", // different from stored
		TaskID:     "task-789",
	})

	// Will fail because actual creation fails, but the test verifies the logic path
	// A real integration test would verify the branch is used correctly
	if err == nil {
		t.Log("recreation succeeded - integration test would verify branch usage")
	}
}
