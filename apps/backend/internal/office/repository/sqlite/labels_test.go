package sqlite_test

import (
	"context"
	"testing"
)

func TestGetOrCreateLabel_CreatesNew(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	lbl, err := repo.GetOrCreateLabel(ctx, "ws-1", "bug")
	if err != nil {
		t.Fatalf("GetOrCreateLabel: %v", err)
	}
	if lbl.ID == "" {
		t.Error("expected non-empty label ID")
	}
	if lbl.Name != "bug" {
		t.Errorf("name = %q, want %q", lbl.Name, "bug")
	}
	if lbl.WorkspaceID != "ws-1" {
		t.Errorf("workspace_id = %q, want %q", lbl.WorkspaceID, "ws-1")
	}
	if lbl.Color == "" {
		t.Error("expected non-empty color")
	}
}

func TestGetOrCreateLabel_ReturnsExisting(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	first, err := repo.GetOrCreateLabel(ctx, "ws-1", "bug")
	if err != nil {
		t.Fatalf("first GetOrCreateLabel: %v", err)
	}
	second, err := repo.GetOrCreateLabel(ctx, "ws-1", "bug")
	if err != nil {
		t.Fatalf("second GetOrCreateLabel: %v", err)
	}
	if first.ID != second.ID {
		t.Errorf("expected same label ID; got %q and %q", first.ID, second.ID)
	}
	if first.Color != second.Color {
		t.Errorf("color changed on re-fetch: %q vs %q", first.Color, second.Color)
	}
}

func TestAddRemoveLabelToTask(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	lbl, err := repo.GetOrCreateLabel(ctx, "ws-1", "urgent")
	if err != nil {
		t.Fatalf("GetOrCreateLabel: %v", err)
	}

	if err := repo.AddLabelToTask(ctx, "task-1", lbl.ID); err != nil {
		t.Fatalf("AddLabelToTask: %v", err)
	}

	// Idempotent — adding again should not error.
	if err := repo.AddLabelToTask(ctx, "task-1", lbl.ID); err != nil {
		t.Fatalf("AddLabelToTask (idempotent): %v", err)
	}

	labels, err := repo.ListLabelsForTask(ctx, "task-1")
	if err != nil {
		t.Fatalf("ListLabelsForTask: %v", err)
	}
	if len(labels) != 1 {
		t.Fatalf("count = %d, want 1", len(labels))
	}
	if labels[0].Name != "urgent" {
		t.Errorf("label name = %q, want %q", labels[0].Name, "urgent")
	}

	if err := repo.RemoveLabelFromTask(ctx, "task-1", lbl.ID); err != nil {
		t.Fatalf("RemoveLabelFromTask: %v", err)
	}

	labels, err = repo.ListLabelsForTask(ctx, "task-1")
	if err != nil {
		t.Fatalf("ListLabelsForTask after remove: %v", err)
	}
	if len(labels) != 0 {
		t.Errorf("count after remove = %d, want 0", len(labels))
	}
}

func TestListLabelsForTask(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	names := []string{"alpha", "beta", "gamma"}
	for _, n := range names {
		lbl, err := repo.GetOrCreateLabel(ctx, "ws-2", n)
		if err != nil {
			t.Fatalf("GetOrCreateLabel %q: %v", n, err)
		}
		if err := repo.AddLabelToTask(ctx, "task-2", lbl.ID); err != nil {
			t.Fatalf("AddLabelToTask %q: %v", n, err)
		}
	}

	labels, err := repo.ListLabelsForTask(ctx, "task-2")
	if err != nil {
		t.Fatalf("ListLabelsForTask: %v", err)
	}
	if len(labels) != 3 {
		t.Fatalf("count = %d, want 3", len(labels))
	}
	// Results are ordered by name.
	for i, want := range names {
		if labels[i].Name != want {
			t.Errorf("label[%d].Name = %q, want %q", i, labels[i].Name, want)
		}
	}
}

func TestListLabelsForTasks_Batch(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	lbl1, _ := repo.GetOrCreateLabel(ctx, "ws-3", "backend")
	lbl2, _ := repo.GetOrCreateLabel(ctx, "ws-3", "frontend")

	_ = repo.AddLabelToTask(ctx, "t-a", lbl1.ID)
	_ = repo.AddLabelToTask(ctx, "t-b", lbl2.ID)
	_ = repo.AddLabelToTask(ctx, "t-b", lbl1.ID)

	result, err := repo.ListLabelsForTasks(ctx, []string{"t-a", "t-b", "t-c"})
	if err != nil {
		t.Fatalf("ListLabelsForTasks: %v", err)
	}

	if len(result["t-a"]) != 1 {
		t.Errorf("t-a label count = %d, want 1", len(result["t-a"]))
	}
	if len(result["t-b"]) != 2 {
		t.Errorf("t-b label count = %d, want 2", len(result["t-b"]))
	}
	if len(result["t-c"]) != 0 {
		t.Errorf("t-c label count = %d, want 0", len(result["t-c"]))
	}
}

func TestDeleteLabel_CascadesJunction(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	lbl, err := repo.GetOrCreateLabel(ctx, "ws-4", "to-delete")
	if err != nil {
		t.Fatalf("GetOrCreateLabel: %v", err)
	}

	// Attach to two tasks.
	_ = repo.AddLabelToTask(ctx, "td-1", lbl.ID)
	_ = repo.AddLabelToTask(ctx, "td-2", lbl.ID)

	if err := repo.DeleteLabel(ctx, lbl.ID); err != nil {
		t.Fatalf("DeleteLabel: %v", err)
	}

	// Junction rows should be gone via ON DELETE CASCADE.
	lbls1, _ := repo.ListLabelsForTask(ctx, "td-1")
	if len(lbls1) != 0 {
		t.Errorf("td-1 label count after cascade delete = %d, want 0", len(lbls1))
	}
	lbls2, _ := repo.ListLabelsForTask(ctx, "td-2")
	if len(lbls2) != 0 {
		t.Errorf("td-2 label count after cascade delete = %d, want 0", len(lbls2))
	}
}
