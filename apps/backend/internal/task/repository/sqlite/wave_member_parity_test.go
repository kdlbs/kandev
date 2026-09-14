package sqlite_test

import (
	"context"
	"sort"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	officesqlite "github.com/kandev/kandev/internal/office/repository/sqlite"
	"github.com/kandev/kandev/internal/task/models"
	taskrepo "github.com/kandev/kandev/internal/task/repository/sqlite"
)

// TestListWaveMembersMatchesListChildCompletionRows is Task 07's
// cross-producer parity check for AC-OFFICE-WAKE-WAVE-IDENTITY-001.8: P1,
// P2, and P3 (via office/repository/sqlite.ListWaveMembers) and P4 (via
// task/repository/sqlite.ListChildCompletionRows) each apply what is meant
// to be the identical wave-member predicate — not archived, not ephemeral,
// not automation-origin. Both queries here run against literally the same
// tasks table for the same fixture, so a future drift between the two
// predicates (each maintained independently in its own package) fails this
// test instead of silently producing four different wave identities for
// the same parent and child set.
func TestListWaveMembersMatchesListChildCompletionRows(t *testing.T) {
	db, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	taskRepo, err := taskrepo.NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("new task repo: %v", err)
	}
	officeRepo, err := officesqlite.NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("new office repo: %v", err)
	}

	ctx := context.Background()
	const parentID = "parent-1"
	const wsID = "ws-1"

	if err := taskRepo.CreateTask(ctx, &models.Task{ID: parentID, WorkspaceID: wsID, Title: "Parent"}); err != nil {
		t.Fatalf("create parent: %v", err)
	}
	seedChild := func(id string, mutate func(*models.Task)) {
		task := &models.Task{ID: id, WorkspaceID: wsID, Title: id, ParentID: parentID}
		if mutate != nil {
			mutate(task)
		}
		if err := taskRepo.CreateTask(ctx, task); err != nil {
			t.Fatalf("create child %s: %v", id, err)
		}
	}
	seedChild("child-ordinary-b", nil)
	seedChild("child-ordinary-a", nil)
	seedChild("child-ephemeral", func(task *models.Task) { task.IsEphemeral = true })
	seedChild("child-automation", func(task *models.Task) { task.Origin = models.TaskOriginAutomationRun })
	seedChild("child-archived", nil)
	if err := taskRepo.ArchiveTask(ctx, "child-archived"); err != nil {
		t.Fatalf("archive child: %v", err)
	}

	waveMembers, err := officeRepo.ListWaveMembers(ctx, parentID)
	if err != nil {
		t.Fatalf("ListWaveMembers: %v", err)
	}
	completionRows, err := taskRepo.ListChildCompletionRows(ctx, parentID)
	if err != nil {
		t.Fatalf("ListChildCompletionRows: %v", err)
	}

	waveIDs := make([]string, len(waveMembers))
	for i, m := range waveMembers {
		waveIDs[i] = m.TaskID
	}
	completionIDs := make([]string, len(completionRows))
	for i, r := range completionRows {
		completionIDs[i] = r.ID
	}
	// ListChildCompletionRows orders by created_at first (see its own doc
	// comment) — every producer that reads it (P4) is responsible for its
	// own ascending-by-id re-sort before deriving wave identity (Task 05),
	// so this parity check compares the *set*, sorted the same way P4's
	// childCompletionPayload does, not ListChildCompletionRows' raw order.
	sort.Strings(completionIDs)

	want := []string{"child-ordinary-a", "child-ordinary-b"}
	assertParityIDsEqual(t, "ListWaveMembers", waveIDs, want)
	assertParityIDsEqual(t, "ListChildCompletionRows (id-sorted)", completionIDs, want)
}

func assertParityIDsEqual(t *testing.T, label string, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s = %v, want %v", label, got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("%s = %v, want %v", label, got, want)
		}
	}
}
