package sqlite

import (
	"context"
	"testing"
)

func TestSetTaskTitleIfPendingRequiresTrueMarker(t *testing.T) {
	repo := newRepoForHealTests(t)
	ctx := context.Background()
	insertTask(t, repo.db, "task-title-false")
	if _, err := repo.db.ExecContext(ctx, `
		UPDATE tasks SET title = ?, metadata = ? WHERE id = ?
	`, "Provisional title", `{"agent_title_pending":false,"keep":"value"}`, "task-title-false"); err != nil {
		t.Fatalf("seed false pending marker: %v", err)
	}

	accepted, err := repo.SetTaskTitleIfPending(ctx, "task-title-false", "Agent title")
	if err != nil {
		t.Fatalf("SetTaskTitleIfPending: %v", err)
	}
	if accepted {
		t.Fatal("accepted title update with a false pending marker")
	}
	task, err := repo.GetTask(ctx, "task-title-false")
	if err != nil {
		t.Fatalf("reload task: %v", err)
	}
	if task.Title != "Provisional title" {
		t.Fatalf("title = %q, want unchanged provisional title", task.Title)
	}
	if pending, ok := task.Metadata["agent_title_pending"].(bool); !ok || pending {
		t.Fatalf("pending marker = %#v, want explicit false", task.Metadata["agent_title_pending"])
	}
	if task.Metadata["keep"] != "value" {
		t.Fatalf("metadata = %#v, want unrelated key preserved", task.Metadata)
	}
}

func TestUpdateTaskPreservesWinningTitleAgainstStaleUpdate(t *testing.T) {
	repo := newRepoForHealTests(t)
	ctx := context.Background()
	insertTask(t, repo.db, "task-title-race")
	if _, err := repo.db.ExecContext(ctx, `
		UPDATE tasks SET title = ?, metadata = ? WHERE id = ?
	`, "Six word provisional title", `{"agent_title_pending":true}`, "task-title-race"); err != nil {
		t.Fatalf("seed pending title: %v", err)
	}

	stale, err := repo.GetTask(ctx, "task-title-race")
	if err != nil {
		t.Fatalf("load stale task: %v", err)
	}
	accepted, err := repo.SetTaskTitleIfPending(ctx, "task-title-race", "Agent chosen title")
	if err != nil {
		t.Fatalf("SetTaskTitleIfPending: %v", err)
	}
	if !accepted {
		t.Fatal("title CAS did not win")
	}

	// This models an ordinary task update that started before the CAS and
	// writes after it. Its stale title/marker must not overwrite the winner.
	stale.Description = "updated concurrently"
	stale.Metadata["stale_change"] = "retained"
	if err := repo.UpdateTask(ctx, stale); err != nil {
		t.Fatalf("stale UpdateTask: %v", err)
	}

	current, err := repo.GetTask(ctx, "task-title-race")
	if err != nil {
		t.Fatalf("reload task: %v", err)
	}
	if current.Title != "Agent chosen title" {
		t.Fatalf("title = %q, want CAS winner", current.Title)
	}
	if _, pending := current.Metadata["agent_title_pending"]; pending {
		t.Fatalf("pending marker restored by stale update: %#v", current.Metadata)
	}
	if current.Description != "updated concurrently" {
		t.Fatalf("description = %q, want stale update to retain its unrelated change", current.Description)
	}
	if current.Metadata["stale_change"] != "retained" {
		t.Fatalf("metadata = %#v, want unrelated stale metadata change retained", current.Metadata)
	}
}
