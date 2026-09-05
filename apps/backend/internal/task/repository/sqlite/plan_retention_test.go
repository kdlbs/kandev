package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

// TestListObsoletePlanRevisionCandidatesProtectsHeadAndAncestry is this
// wave's coverage for the plan-revision half of "non-destructive retention
// candidate selection": HEAD and any revision referenced by another
// revision's revert-of ancestry link must never be reported, no matter how
// old they are.
func TestListObsoletePlanRevisionCandidatesProtectsHeadAndAncestry(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedTaskForDocs(t, repo, "task-planrev-retention")

	base := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	revertOf1 := "planrev-ret-1"
	revisions := []*models.TaskPlanRevision{
		{ID: "planrev-ret-1", TaskID: "task-planrev-retention", RevisionNumber: 1, Title: "v1", Content: "one", CreatedAt: base},
		{ID: "planrev-ret-2", TaskID: "task-planrev-retention", RevisionNumber: 2, Title: "v2", Content: "two", CreatedAt: base.Add(time.Hour)},
		// Revision 3 reverts back to revision 1's content, so revision 1 must
		// stay protected by ancestry despite not being HEAD.
		{ID: "planrev-ret-3", TaskID: "task-planrev-retention", RevisionNumber: 3, Title: "v1-restored", Content: "one",
			RevertOfRevisionID: &revertOf1, CreatedAt: base.Add(2 * time.Hour)},
		{ID: "planrev-ret-4", TaskID: "task-planrev-retention", RevisionNumber: 4, Title: "v4 (HEAD)", Content: "four", CreatedAt: base.Add(3 * time.Hour)},
	}
	for _, rev := range revisions {
		if err := repo.InsertTaskPlanRevision(ctx, rev); err != nil {
			t.Fatalf("InsertTaskPlanRevision(%s): %v", rev.ID, err)
		}
	}

	candidates, err := repo.ListObsoletePlanRevisionCandidates(ctx, "task-planrev-retention", 0)
	if err != nil {
		t.Fatalf("ListObsoletePlanRevisionCandidates: %v", err)
	}
	// Revision 1 is protected because revision 3 reverts to it (ancestry);
	// revision 4 is protected because it's HEAD. Revision 3 itself is
	// neither HEAD nor referenced by any other revision, so it is reported
	// too - the ancestry protection is expected to preserve the *target* of
	// a revert, not the revert action itself, since the target already
	// carries the content anyone would want to restore.
	want := map[string]bool{"planrev-ret-2": true, "planrev-ret-3": true}
	if len(candidates) != len(want) {
		t.Fatalf("candidates = %+v, want exactly %v", candidates, want)
	}
	for _, c := range candidates {
		if !want[c.ID] {
			t.Fatalf("unexpected candidate %q; HEAD (planrev-ret-4) and ancestry target (planrev-ret-1) must be protected", c.ID)
		}
		if c.ID == "planrev-ret-2" && c.ContentBytes != int64(len("two")) {
			t.Fatalf("ContentBytes = %d, want %d", c.ContentBytes, len("two"))
		}
	}
}

// TestListObsoletePlanRevisionCandidatesRespectsRecencyWindow confirms
// keepLastN additionally protects the most recent non-HEAD revisions from
// being reported, on top of the HEAD/ancestry protections.
func TestListObsoletePlanRevisionCandidatesRespectsRecencyWindow(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedTaskForDocs(t, repo, "task-planrev-window")

	base := time.Date(2026, 2, 2, 0, 0, 0, 0, time.UTC)
	for i := 1; i <= 5; i++ {
		rev := &models.TaskPlanRevision{
			ID: "planrev-win-" + string(rune('0'+i)), TaskID: "task-planrev-window",
			RevisionNumber: i, Title: "v", Content: "c",
			CreatedAt: base.Add(time.Duration(i) * time.Hour),
		}
		if err := repo.InsertTaskPlanRevision(ctx, rev); err != nil {
			t.Fatalf("InsertTaskPlanRevision(%d): %v", i, err)
		}
	}

	// HEAD is revision 5. keepLastN=2 additionally protects revision 4,
	// leaving revisions 1-3 as candidates.
	candidates, err := repo.ListObsoletePlanRevisionCandidates(ctx, "task-planrev-window", 2)
	if err != nil {
		t.Fatalf("ListObsoletePlanRevisionCandidates: %v", err)
	}
	if len(candidates) != 3 {
		t.Fatalf("candidates = %d, want 3 (revisions 1-3)", len(candidates))
	}
	for _, c := range candidates {
		if c.RevisionNumber > 3 {
			t.Fatalf("candidate revision_number = %d, want <= 3 (recency window must protect revision 4)", c.RevisionNumber)
		}
	}
}

// TestListObsoletePlanRevisionCandidatesIsNonDestructive confirms the
// selection is read-only: calling it does not remove or alter any revision
// row, matching the plan's "non-destructive until maintenance executes"
// constraint.
func TestListObsoletePlanRevisionCandidatesIsNonDestructive(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedTaskForDocs(t, repo, "task-planrev-nondestructive")

	for i := 1; i <= 2; i++ {
		if err := repo.InsertTaskPlanRevision(ctx, &models.TaskPlanRevision{
			ID: "planrev-nd-" + string(rune('0'+i)), TaskID: "task-planrev-nondestructive",
			RevisionNumber: i, Title: "v", Content: "c",
		}); err != nil {
			t.Fatalf("InsertTaskPlanRevision(%d): %v", i, err)
		}
	}

	before := countRows(t, repo, `SELECT COUNT(*) FROM task_plan_revisions WHERE task_id = ?`, "task-planrev-nondestructive")
	if _, err := repo.ListObsoletePlanRevisionCandidates(ctx, "task-planrev-nondestructive", 0); err != nil {
		t.Fatalf("ListObsoletePlanRevisionCandidates: %v", err)
	}
	after := countRows(t, repo, `SELECT COUNT(*) FROM task_plan_revisions WHERE task_id = ?`, "task-planrev-nondestructive")
	if before != after {
		t.Fatalf("row count changed from %d to %d after a read-only candidate listing", before, after)
	}
}
