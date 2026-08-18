package repository

import (
	"context"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/workflow/models"
)

func createTestStep(t *testing.T, repo *Repository, workflowID, name string) *models.WorkflowStep {
	t.Helper()
	step := &models.WorkflowStep{
		WorkflowID: workflowID,
		Name:       name,
		Position:   0,
		Color:      "#000000",
	}
	if err := repo.CreateStep(context.Background(), step); err != nil {
		t.Fatalf("failed to create step %q: %v", name, err)
	}
	return step
}

func TestCoordinatorMonitoring_SaveThenLoadRoundTrip(t *testing.T) {
	repo := setupTestRepo(t)
	ctx := context.Background()
	step1 := createTestStep(t, repo, "wf-test", "Step 1")
	step2 := createTestStep(t, repo, "wf-test", "Step 2")

	// Fresh workflow: no saved rows.
	entries, err := repo.GetCoordinatorMonitoring(ctx, "wf-test")
	if err != nil {
		t.Fatalf("GetCoordinatorMonitoring: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected no entries, got %d", len(entries))
	}

	want := []models.CoordinatorStepMonitor{
		{WorkflowStepID: step1.ID, Selected: true, Prompt: "watch step 1"},
		{WorkflowStepID: step2.ID, Selected: false, Prompt: "notes only"},
	}
	if err := repo.ReplaceCoordinatorMonitoring(ctx, "ws-test", "wf-test", want); err != nil {
		t.Fatalf("ReplaceCoordinatorMonitoring: %v", err)
	}

	got, err := repo.GetCoordinatorMonitoring(ctx, "wf-test")
	if err != nil {
		t.Fatalf("GetCoordinatorMonitoring: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(got))
	}
	byStep := map[string]models.CoordinatorStepMonitor{}
	for _, e := range got {
		byStep[e.WorkflowStepID] = e
	}
	if e := byStep[step1.ID]; !e.Selected || e.Prompt != "watch step 1" {
		t.Errorf("step1 mismatch: %+v", e)
	}
	if e := byStep[step2.ID]; e.Selected || e.Prompt != "notes only" {
		t.Errorf("step2 mismatch: %+v", e)
	}
	if byStep[step1.ID].UpdatedAt.IsZero() {
		t.Error("expected UpdatedAt to be set")
	}
}

func TestCoordinatorMonitoring_ReplaceSemanticsRemovesDeselectedEmptyRows(t *testing.T) {
	repo := setupTestRepo(t)
	ctx := context.Background()
	step1 := createTestStep(t, repo, "wf-test", "Step 1")
	step2 := createTestStep(t, repo, "wf-test", "Step 2")

	first := []models.CoordinatorStepMonitor{
		{WorkflowStepID: step1.ID, Selected: true, Prompt: "first prompt"},
		{WorkflowStepID: step2.ID, Selected: true, Prompt: "second prompt"},
	}
	if err := repo.ReplaceCoordinatorMonitoring(ctx, "ws-test", "wf-test", first); err != nil {
		t.Fatalf("first ReplaceCoordinatorMonitoring: %v", err)
	}

	// Second save: step1 unchecked with no prompt (should disappear), step2 kept.
	second := []models.CoordinatorStepMonitor{
		{WorkflowStepID: step1.ID, Selected: false, Prompt: ""},
		{WorkflowStepID: step2.ID, Selected: true, Prompt: "still watching"},
	}
	if err := repo.ReplaceCoordinatorMonitoring(ctx, "ws-test", "wf-test", second); err != nil {
		t.Fatalf("second ReplaceCoordinatorMonitoring: %v", err)
	}

	got, err := repo.GetCoordinatorMonitoring(ctx, "wf-test")
	if err != nil {
		t.Fatalf("GetCoordinatorMonitoring: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected exactly 1 remaining entry, got %d: %+v", len(got), got)
	}
	if got[0].WorkflowStepID != step2.ID || got[0].Prompt != "still watching" {
		t.Errorf("unexpected surviving entry: %+v", got[0])
	}
}

func TestCoordinatorMonitoring_RejectsForeignStepID_NoPartialWrite(t *testing.T) {
	repo := setupTestRepo(t)
	ctx := context.Background()
	step1 := createTestStep(t, repo, "wf-test", "Step 1")

	// Seed a second workflow with its own step.
	if _, err := repo.db.Exec(`INSERT INTO workflows (id, workspace_id, name, created_at, updated_at)
		VALUES ('wf-other', '', 'Other', datetime('now'), datetime('now'))`); err != nil {
		t.Fatalf("failed to insert other workflow: %v", err)
	}
	otherStep := createTestStep(t, repo, "wf-other", "Other Step")

	// Prime wf-test with a valid saved row.
	valid := []models.CoordinatorStepMonitor{
		{WorkflowStepID: step1.ID, Selected: true, Prompt: "keep me"},
	}
	if err := repo.ReplaceCoordinatorMonitoring(ctx, "ws-test", "wf-test", valid); err != nil {
		t.Fatalf("priming ReplaceCoordinatorMonitoring: %v", err)
	}

	bad := []models.CoordinatorStepMonitor{
		{WorkflowStepID: step1.ID, Selected: true, Prompt: "should not apply"},
		{WorkflowStepID: otherStep.ID, Selected: true, Prompt: "foreign step"},
	}
	err := repo.ReplaceCoordinatorMonitoring(ctx, "ws-test", "wf-test", bad)
	if err == nil {
		t.Fatal("expected error for foreign workflow_step_id, got nil")
	}
	if !strings.Contains(err.Error(), "does not belong to workflow") {
		t.Errorf("unexpected error message: %v", err)
	}

	// Nothing should have changed: the original primed row must survive untouched.
	got, err := repo.GetCoordinatorMonitoring(ctx, "wf-test")
	if err != nil {
		t.Fatalf("GetCoordinatorMonitoring: %v", err)
	}
	if len(got) != 1 || got[0].Prompt != "keep me" {
		t.Fatalf("expected no partial write, got: %+v", got)
	}
}

func TestCoordinatorMonitoring_FKCascadeOnStepDelete(t *testing.T) {
	repo := setupTestRepo(t)
	ctx := context.Background()
	step1 := createTestStep(t, repo, "wf-test", "Step 1")

	if err := repo.ReplaceCoordinatorMonitoring(ctx, "ws-test", "wf-test", []models.CoordinatorStepMonitor{
		{WorkflowStepID: step1.ID, Selected: true, Prompt: "watch"},
	}); err != nil {
		t.Fatalf("ReplaceCoordinatorMonitoring: %v", err)
	}

	if err := repo.DeleteStep(ctx, step1.ID); err != nil {
		t.Fatalf("DeleteStep: %v", err)
	}

	got, err := repo.GetCoordinatorMonitoring(ctx, "wf-test")
	if err != nil {
		t.Fatalf("GetCoordinatorMonitoring: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected cascade delete to remove monitoring row, got: %+v", got)
	}
}

func TestCoordinatorMonitoring_FKCascadeOnWorkflowDelete(t *testing.T) {
	repo := setupTestRepo(t)
	ctx := context.Background()
	step1 := createTestStep(t, repo, "wf-test", "Step 1")

	if err := repo.ReplaceCoordinatorMonitoring(ctx, "ws-test", "wf-test", []models.CoordinatorStepMonitor{
		{WorkflowStepID: step1.ID, Selected: true, Prompt: "watch"},
	}); err != nil {
		t.Fatalf("ReplaceCoordinatorMonitoring: %v", err)
	}

	if _, err := repo.db.Exec(`DELETE FROM workflows WHERE id = 'wf-test'`); err != nil {
		t.Fatalf("failed to delete workflow: %v", err)
	}

	got, err := repo.GetCoordinatorMonitoring(ctx, "wf-test")
	if err != nil {
		t.Fatalf("GetCoordinatorMonitoring: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected cascade delete to remove monitoring row, got: %+v", got)
	}
}
