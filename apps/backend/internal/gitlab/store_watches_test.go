package gitlab

import (
	"context"
	"testing"
)

func TestStore_CreateMRWatch_RoundTrip(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	w := &MRWatch{
		SessionID:    "sess-1",
		TaskID:       "task-1",
		RepositoryID: "",
		ProjectPath:  "group/project",
		MRIID:        42,
		Branch:       "feature/x",
	}
	if err := store.CreateMRWatch(ctx, w); err != nil {
		t.Fatalf("CreateMRWatch: %v", err)
	}
	if w.ID == "" {
		t.Fatalf("CreateMRWatch did not assign an ID")
	}
	got, err := store.GetMRWatchBySession(ctx, "sess-1")
	if err != nil {
		t.Fatalf("GetMRWatchBySession: %v", err)
	}
	if got == nil || got.ID != w.ID || got.ProjectPath != "group/project" || got.MRIID != 42 {
		t.Fatalf("unexpected watch: %+v", got)
	}
}

func TestStore_ReviewWatch_CRUD(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	rw := &ReviewWatch{
		WorkspaceID:       "ws-1",
		WorkflowID:        "wf-1",
		WorkflowStepID:    "wfs-1",
		Projects:          []ProjectFilter{{Path: "group/project"}},
		AgentProfileID:    "ag-1",
		ExecutorProfileID: "ex-1",
		Prompt:            "Review",
		ReviewScope:       ReviewScopeUserAndTeams,
		Enabled:           true,
		CleanupPolicy:     CleanupPolicyAuto,
	}
	if err := store.CreateReviewWatch(ctx, rw); err != nil {
		t.Fatalf("CreateReviewWatch: %v", err)
	}
	got, err := store.GetReviewWatch(ctx, rw.ID)
	if err != nil {
		t.Fatalf("GetReviewWatch: %v", err)
	}
	if got == nil || got.WorkspaceID != "ws-1" || len(got.Projects) != 1 || got.Projects[0].Path != "group/project" {
		t.Fatalf("unexpected review watch: %+v", got)
	}
	listed, err := store.ListReviewWatches(ctx, "ws-1")
	if err != nil {
		t.Fatalf("ListReviewWatches: %v", err)
	}
	if len(listed) != 1 || listed[0].ID != rw.ID {
		t.Fatalf("expected 1 watch in workspace, got %d", len(listed))
	}
	rw.Prompt = "Review carefully"
	rw.CleanupPolicy = CleanupPolicyAlways
	if err := store.UpdateReviewWatch(ctx, rw); err != nil {
		t.Fatalf("UpdateReviewWatch: %v", err)
	}
	got2, _ := store.GetReviewWatch(ctx, rw.ID)
	if got2 == nil || got2.Prompt != "Review carefully" || got2.CleanupPolicy != CleanupPolicyAlways {
		t.Fatalf("update did not stick: %+v", got2)
	}
	if err := store.DeleteReviewWatch(ctx, rw.ID); err != nil {
		t.Fatalf("DeleteReviewWatch: %v", err)
	}
	got3, _ := store.GetReviewWatch(ctx, rw.ID)
	if got3 != nil {
		t.Fatalf("expected nil after delete, got %+v", got3)
	}
}

func TestStore_ReviewMRTask_ReserveAndAssign(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	ok, err := store.ReserveReviewMRTask(ctx, "watch-1", "group/project", 42, "https://x")
	if err != nil || !ok {
		t.Fatalf("first reserve: ok=%v err=%v", ok, err)
	}
	ok2, _ := store.ReserveReviewMRTask(ctx, "watch-1", "group/project", 42, "https://x")
	if ok2 {
		t.Fatalf("second reserve should fail, got ok=true")
	}
	if err := store.AssignReviewMRTaskID(ctx, "watch-1", "group/project", 42, "task-99"); err != nil {
		t.Fatalf("AssignReviewMRTaskID: %v", err)
	}
	has, err := store.HasReviewMRTask(ctx, "watch-1", "group/project", 42)
	if err != nil || !has {
		t.Fatalf("HasReviewMRTask: has=%v err=%v", has, err)
	}
}

func TestStore_IssueWatch_CRUD(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	iw := &IssueWatch{
		WorkspaceID:       "ws-1",
		WorkflowID:        "wf-1",
		WorkflowStepID:    "wfs-1",
		Projects:          []ProjectFilter{{Path: "group/project"}},
		AgentProfileID:    "ag-1",
		ExecutorProfileID: "ex-1",
		Prompt:            "Fix",
		Labels:            []string{"bug", "p1"},
		Enabled:           true,
		CleanupPolicy:     CleanupPolicyAuto,
	}
	if err := store.CreateIssueWatch(ctx, iw); err != nil {
		t.Fatalf("CreateIssueWatch: %v", err)
	}
	got, err := store.GetIssueWatch(ctx, iw.ID)
	if err != nil || got == nil {
		t.Fatalf("GetIssueWatch: got=%v err=%v", got, err)
	}
	if len(got.Labels) != 2 || got.Labels[0] != "bug" {
		t.Fatalf("labels round-trip broken: %+v", got.Labels)
	}
	if len(got.Projects) != 1 || got.Projects[0].Path != "group/project" {
		t.Fatalf("projects round-trip broken: %+v", got.Projects)
	}
}

func TestStore_ActionPresets_RoundTrip(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	presets := &ActionPresets{
		WorkspaceID: "ws-1",
		MR:          DefaultMRActionPresets(),
		Issue:       DefaultIssueActionPresets(),
	}
	if err := store.UpsertActionPresets(ctx, presets); err != nil {
		t.Fatalf("UpsertActionPresets: %v", err)
	}
	got, err := store.GetActionPresets(ctx, "ws-1")
	if err != nil {
		t.Fatalf("GetActionPresets: %v", err)
	}
	if len(got.MR) != len(presets.MR) || got.MR[0].ID != presets.MR[0].ID {
		t.Fatalf("MR presets round-trip broken: %+v", got.MR)
	}
}
