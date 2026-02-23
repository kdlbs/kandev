package orchestrator

import (
	"context"
	"testing"

	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

func TestWorkflowStore_LoadState(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")

	agentMgr := &mockAgentManager{isPassthrough: true}
	store := newWorkflowStore(repo, newMockStepGetter(), agentMgr, nil, testLogger())

	state, err := store.LoadState(ctx, "t1", "s1")
	if err != nil {
		t.Fatalf("LoadState failed: %v", err)
	}

	if state.TaskID != "t1" {
		t.Errorf("expected TaskID %q, got %q", "t1", state.TaskID)
	}
	if state.SessionID != "s1" {
		t.Errorf("expected SessionID %q, got %q", "s1", state.SessionID)
	}
	if state.WorkflowID != "wf1" {
		t.Errorf("expected WorkflowID %q, got %q", "wf1", state.WorkflowID)
	}
	if state.CurrentStepID != "step1" {
		t.Errorf("expected CurrentStepID %q, got %q", "step1", state.CurrentStepID)
	}
	if state.TaskDescription != "Test" {
		t.Errorf("expected TaskDescription %q, got %q", "Test", state.TaskDescription)
	}
	if !state.IsPassthrough {
		t.Error("expected IsPassthrough to be true")
	}
}

func TestWorkflowStore_LoadStep(t *testing.T) {
	ctx := context.Background()

	stepGetter := newMockStepGetter()
	stepGetter.steps["step1"] = &wfmodels.WorkflowStep{
		ID:         "step1",
		WorkflowID: "wf1",
		Name:       "Planning",
		Position:   0,
	}

	store := newWorkflowStore(nil, stepGetter, nil, nil, testLogger())

	spec, err := store.LoadStep(ctx, "wf1", "step1")
	if err != nil {
		t.Fatalf("LoadStep failed: %v", err)
	}

	if spec.ID != "step1" {
		t.Errorf("expected ID %q, got %q", "step1", spec.ID)
	}
	if spec.Name != "Planning" {
		t.Errorf("expected Name %q, got %q", "Planning", spec.Name)
	}
	if spec.Position != 0 {
		t.Errorf("expected Position %d, got %d", 0, spec.Position)
	}
}

func TestWorkflowStore_LoadNextStep(t *testing.T) {
	ctx := context.Background()

	stepGetter := newMockStepGetter()
	stepGetter.steps["step1"] = &wfmodels.WorkflowStep{
		ID: "step1", WorkflowID: "wf1", Name: "Step 1", Position: 0,
	}
	stepGetter.steps["step2"] = &wfmodels.WorkflowStep{
		ID: "step2", WorkflowID: "wf1", Name: "Step 2", Position: 1,
	}
	stepGetter.steps["step3"] = &wfmodels.WorkflowStep{
		ID: "step3", WorkflowID: "wf1", Name: "Step 3", Position: 2,
	}

	store := newWorkflowStore(nil, stepGetter, nil, nil, testLogger())

	t.Run("returns next step by position", func(t *testing.T) {
		spec, err := store.LoadNextStep(ctx, "wf1", 0)
		if err != nil {
			t.Fatalf("LoadNextStep failed: %v", err)
		}
		if spec.ID != "step2" {
			t.Errorf("expected next step ID %q, got %q", "step2", spec.ID)
		}
		if spec.Position != 1 {
			t.Errorf("expected Position %d, got %d", 1, spec.Position)
		}
	})

	t.Run("returns error when no next step exists", func(t *testing.T) {
		_, err := store.LoadNextStep(ctx, "wf1", 2)
		if err == nil {
			t.Fatal("expected error when no next step exists, got nil")
		}
	})
}

func TestWorkflowStore_ApplyTransition(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")

	store := newWorkflowStore(repo, newMockStepGetter(), nil, nil, testLogger())

	err := store.ApplyTransition(ctx, "t1", "s1", "step1", "step2", "on_turn_complete")
	if err != nil {
		t.Fatalf("ApplyTransition failed: %v", err)
	}

	// Verify task's WorkflowStepID is updated
	task, err := repo.GetTask(ctx, "t1")
	if err != nil {
		t.Fatalf("failed to get task: %v", err)
	}
	if task.WorkflowStepID != "step2" {
		t.Errorf("expected task WorkflowStepID %q, got %q", "step2", task.WorkflowStepID)
	}

	// Verify session's workflow step is updated
	session, err := repo.GetTaskSession(ctx, "s1")
	if err != nil {
		t.Fatalf("failed to get session: %v", err)
	}
	if session.WorkflowStepID == nil || *session.WorkflowStepID != "step2" {
		t.Errorf("expected session WorkflowStepID %q, got %v", "step2", session.WorkflowStepID)
	}

	// Verify review status is cleared
	if session.ReviewStatus != nil && *session.ReviewStatus != "" {
		t.Errorf("expected review status to be cleared, got %q", *session.ReviewStatus)
	}
}

func TestWorkflowStore_PersistData(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")

	store := newWorkflowStore(repo, newMockStepGetter(), nil, nil, testLogger())

	// Persist initial data
	err := store.PersistData(ctx, "s1", map[string]any{"plan_mode": true})
	if err != nil {
		t.Fatalf("PersistData failed: %v", err)
	}

	// Verify data was stored
	session, err := repo.GetTaskSession(ctx, "s1")
	if err != nil {
		t.Fatalf("failed to get session: %v", err)
	}
	wd, ok := session.Metadata["workflow_data"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected workflow_data in metadata, got %v", session.Metadata)
	}
	if wd["plan_mode"] != true {
		t.Errorf("expected plan_mode=true, got %v", wd["plan_mode"])
	}

	// Persist more data and verify it merges (does not overwrite)
	err = store.PersistData(ctx, "s1", map[string]any{"auto_start": false})
	if err != nil {
		t.Fatalf("PersistData (merge) failed: %v", err)
	}

	session, err = repo.GetTaskSession(ctx, "s1")
	if err != nil {
		t.Fatalf("failed to get session after merge: %v", err)
	}
	wd, ok = session.Metadata["workflow_data"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected workflow_data after merge, got %v", session.Metadata)
	}
	if wd["plan_mode"] != true {
		t.Errorf("expected plan_mode to still be true after merge, got %v", wd["plan_mode"])
	}
	if wd["auto_start"] != false {
		t.Errorf("expected auto_start=false, got %v", wd["auto_start"])
	}
}

func TestWorkflowStore_OperationIdempotency(t *testing.T) {
	ctx := context.Background()
	store := newWorkflowStore(nil, newMockStepGetter(), nil, nil, testLogger())

	t.Run("empty operation ID returns false", func(t *testing.T) {
		applied, err := store.IsOperationApplied(ctx, "")
		if err != nil {
			t.Fatalf("IsOperationApplied failed: %v", err)
		}
		if applied {
			t.Error("expected empty operation ID to return false")
		}
	})

	t.Run("unknown operation returns false", func(t *testing.T) {
		applied, err := store.IsOperationApplied(ctx, "unknown-op")
		if err != nil {
			t.Fatalf("IsOperationApplied failed: %v", err)
		}
		if applied {
			t.Error("expected unknown operation to return false")
		}
	})

	t.Run("marked operation returns true", func(t *testing.T) {
		err := store.MarkOperationApplied(ctx, "op-1")
		if err != nil {
			t.Fatalf("MarkOperationApplied failed: %v", err)
		}

		applied, err := store.IsOperationApplied(ctx, "op-1")
		if err != nil {
			t.Fatalf("IsOperationApplied failed: %v", err)
		}
		if !applied {
			t.Error("expected marked operation to return true")
		}
	})
}
