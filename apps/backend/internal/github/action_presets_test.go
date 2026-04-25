package github

import (
	"context"
	"testing"
)

// TestGetActionPresets_EmptyReturnsDefaults verifies that a workspace with no
// stored presets gets the hard-coded defaults — the /github page should never
// see an empty dropdown for a fresh workspace.
func TestGetActionPresets_EmptyReturnsDefaults(t *testing.T) {
	svc, _, _ := setupSyncTest(t)
	ctx := context.Background()

	got, err := svc.GetActionPresets(ctx, "ws-1")
	if err != nil {
		t.Fatalf("GetActionPresets: %v", err)
	}
	if got.WorkspaceID != "ws-1" {
		t.Errorf("WorkspaceID = %q, want %q", got.WorkspaceID, "ws-1")
	}
	if len(got.PR) != len(DefaultPRActionPresets()) {
		t.Errorf("PR preset count = %d, want %d", len(got.PR), len(DefaultPRActionPresets()))
	}
	if len(got.Issue) != len(DefaultIssueActionPresets()) {
		t.Errorf("Issue preset count = %d, want %d", len(got.Issue), len(DefaultIssueActionPresets()))
	}
}

// TestUpdateActionPresets_PartialUpdate verifies that updating only the PR
// list leaves the Issue list at defaults, and vice versa.
func TestUpdateActionPresets_PartialUpdate(t *testing.T) {
	svc, _, _ := setupSyncTest(t)
	ctx := context.Background()

	custom := []ActionPreset{
		{ID: "custom", Label: "Custom review", Hint: "Deep review", Icon: "eye", PromptTemplate: "Review {url}"},
	}
	got, err := svc.UpdateActionPresets(ctx, &UpdateActionPresetsRequest{
		WorkspaceID: "ws-1",
		PR:          &custom,
	})
	if err != nil {
		t.Fatalf("UpdateActionPresets: %v", err)
	}
	if len(got.PR) != 1 || got.PR[0].Label != "Custom review" {
		t.Errorf("unexpected PR presets: %+v", got.PR)
	}
	if len(got.Issue) != len(DefaultIssueActionPresets()) {
		t.Errorf("Issue presets not preserved at defaults: %+v", got.Issue)
	}

	// Re-read to make sure it persisted.
	reread, err := svc.GetActionPresets(ctx, "ws-1")
	if err != nil {
		t.Fatalf("GetActionPresets after update: %v", err)
	}
	if len(reread.PR) != 1 || reread.PR[0].Label != "Custom review" {
		t.Errorf("stored PR presets lost: %+v", reread.PR)
	}
}

// TestUpdateActionPresets_NormalisesInput verifies that empty-label presets
// are dropped and missing IDs get auto-assigned.
func TestUpdateActionPresets_NormalisesInput(t *testing.T) {
	svc, _, _ := setupSyncTest(t)
	ctx := context.Background()

	input := []ActionPreset{
		{ID: "", Label: "  Has label  ", PromptTemplate: "go"},
		{ID: "skip", Label: "", PromptTemplate: "drop me"},
	}
	got, err := svc.UpdateActionPresets(ctx, &UpdateActionPresetsRequest{
		WorkspaceID: "ws-1",
		PR:          &input,
	})
	if err != nil {
		t.Fatalf("UpdateActionPresets: %v", err)
	}
	if len(got.PR) != 1 {
		t.Fatalf("empty-label preset not dropped: %+v", got.PR)
	}
	if got.PR[0].Label != "Has label" {
		t.Errorf("label not trimmed: %q", got.PR[0].Label)
	}
	if got.PR[0].ID == "" {
		t.Errorf("missing ID not auto-assigned")
	}
}

// TestResetActionPresets_RestoresDefaults verifies that reset removes stored
// customisations — the next Get returns defaults again.
func TestResetActionPresets_RestoresDefaults(t *testing.T) {
	svc, _, _ := setupSyncTest(t)
	ctx := context.Background()

	custom := []ActionPreset{
		{ID: "custom", Label: "Custom", PromptTemplate: "x"},
	}
	if _, err := svc.UpdateActionPresets(ctx, &UpdateActionPresetsRequest{
		WorkspaceID: "ws-1",
		PR:          &custom,
	}); err != nil {
		t.Fatalf("UpdateActionPresets: %v", err)
	}

	got, err := svc.ResetActionPresets(ctx, "ws-1")
	if err != nil {
		t.Fatalf("ResetActionPresets: %v", err)
	}
	if len(got.PR) != len(DefaultPRActionPresets()) {
		t.Errorf("reset did not restore PR defaults: got %d", len(got.PR))
	}
}

// TestUpdateActionPresets_RequiresWorkspaceID guards the controller-layer
// invariant: the service refuses an empty workspace rather than writing
// rows keyed on "".
func TestUpdateActionPresets_RequiresWorkspaceID(t *testing.T) {
	svc, _, _ := setupSyncTest(t)
	ctx := context.Background()

	empty := []ActionPreset{{ID: "x", Label: "x"}}
	if _, err := svc.UpdateActionPresets(ctx, &UpdateActionPresetsRequest{PR: &empty}); err == nil {
		t.Fatal("expected error on empty workspace_id, got nil")
	}
}

// TestGetActionPresets_FallsBackPerKind verifies the half-populated case: if
// PR was customised but Issue wasn't, Issue falls back to defaults.
func TestGetActionPresets_FallsBackPerKind(t *testing.T) {
	svc, _, _ := setupSyncTest(t)
	ctx := context.Background()

	custom := []ActionPreset{
		{ID: "only-pr", Label: "Only PR", PromptTemplate: "pr"},
	}
	if _, err := svc.UpdateActionPresets(ctx, &UpdateActionPresetsRequest{
		WorkspaceID: "ws-1",
		PR:          &custom,
	}); err != nil {
		t.Fatalf("UpdateActionPresets: %v", err)
	}

	got, err := svc.GetActionPresets(ctx, "ws-1")
	if err != nil {
		t.Fatalf("GetActionPresets: %v", err)
	}
	if len(got.PR) != 1 {
		t.Errorf("PR not the customised list: %+v", got.PR)
	}
	if len(got.Issue) == 0 {
		t.Errorf("Issue list empty — expected fallback to defaults")
	}
}
