package sqlite

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

func newTestClarificationResolution(pendingID, sessionID, taskID string) *models.ClarificationResolution {
	return &models.ClarificationResolution{
		PendingID:  pendingID,
		SessionID:  sessionID,
		TaskID:     taskID,
		Status:     models.ClarificationResolutionStatusAnswered,
		Response:   `{"pending_id":"` + pendingID + `","answers":[],"rejected":false,"reject_reason":""}`,
		Resume:     models.ClarificationResolutionResumePending,
		ResolvedBy: "",
		Source:     models.ClarificationResolutionSourceMCP,
		ResolvedAt: time.Now().UTC(),
	}
}

func TestInsertClarificationResolution_FirstCallerClaims(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedForMsgTest(t, repo, "task-C1", "sess-C1", "turn-C1")

	res := newTestClarificationResolution("pending-C1", "sess-C1", "task-C1")
	claimed, stored, err := repo.InsertClarificationResolution(ctx, res)
	if err != nil {
		t.Fatalf("InsertClarificationResolution: %v", err)
	}
	if !claimed {
		t.Fatalf("expected first caller to claim, got claimed=false")
	}
	if stored.PendingID != "pending-C1" {
		t.Fatalf("stored.PendingID = %q, want pending-C1", stored.PendingID)
	}

	got, err := repo.GetClarificationResolution(ctx, "pending-C1")
	if err != nil {
		t.Fatalf("GetClarificationResolution: %v", err)
	}
	if got.Status != models.ClarificationResolutionStatusAnswered {
		t.Errorf("got.Status = %q, want answered", got.Status)
	}
	if got.Resume != models.ClarificationResolutionResumePending {
		t.Errorf("got.Resume = %q, want pending", got.Resume)
	}
}

// TestInsertClarificationResolution_SecondCallerLoses proves the claim is a
// database constraint, not an application-level check: a second insert for
// the same pending_id with a DIFFERENT outcome does not overwrite the first
// winner's row, and the caller learns it lost by getting the winner's row
// back with claimed=false (spec R1-R3, M8).
func TestInsertClarificationResolution_SecondCallerLoses(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedForMsgTest(t, repo, "task-C2", "sess-C2", "turn-C2")

	winner := newTestClarificationResolution("pending-C2", "sess-C2", "task-C2")
	claimed, _, err := repo.InsertClarificationResolution(ctx, winner)
	if err != nil || !claimed {
		t.Fatalf("winner insert: claimed=%v err=%v", claimed, err)
	}

	loser := newTestClarificationResolution("pending-C2", "sess-C2", "task-C2")
	loser.Status = models.ClarificationResolutionStatusRejected
	loser.Response = `{"pending_id":"pending-C2","answers":[],"rejected":true,"reject_reason":"nope"}`
	claimed, stored, err := repo.InsertClarificationResolution(ctx, loser)
	if err != nil {
		t.Fatalf("loser insert: %v", err)
	}
	if claimed {
		t.Fatalf("expected second caller to lose the claim")
	}
	if stored.Status != models.ClarificationResolutionStatusAnswered {
		t.Fatalf("stored.Status = %q, want the winner's answered (row must not be overwritten)", stored.Status)
	}
}

// TestInsertClarificationResolution_SessionMissing proves M8a: a claim whose
// session_id foreign key cannot be satisfied fails distinctly from both the
// won and lost cases, and leaves no row behind.
func TestInsertClarificationResolution_SessionMissing(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()

	res := newTestClarificationResolution("pending-C3", "sess-does-not-exist", "task-C3")
	claimed, stored, err := repo.InsertClarificationResolution(ctx, res)
	if !errors.Is(err, ErrClarificationSessionMissing) {
		t.Fatalf("InsertClarificationResolution error = %v, want ErrClarificationSessionMissing", err)
	}
	if claimed || stored != nil {
		t.Fatalf("expected no claim and no stored row, got claimed=%v stored=%+v", claimed, stored)
	}

	if _, err := repo.GetClarificationResolution(ctx, "pending-C3"); !errors.Is(err, ErrClarificationResolutionNotFound) {
		t.Fatalf("GetClarificationResolution after failed claim = %v, want ErrClarificationResolutionNotFound", err)
	}
}

func TestGetClarificationResolution_NotFound(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()

	if _, err := repo.GetClarificationResolution(ctx, "pending-unknown"); !errors.Is(err, ErrClarificationResolutionNotFound) {
		t.Fatalf("GetClarificationResolution = %v, want ErrClarificationResolutionNotFound", err)
	}
}

// TestUpdateClarificationResolutionResume proves M7: the resume outcome can
// be updated in place after the claim without touching any other column.
func TestUpdateClarificationResolutionResume(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedForMsgTest(t, repo, "task-C4", "sess-C4", "turn-C4")

	res := newTestClarificationResolution("pending-C4", "sess-C4", "task-C4")
	if _, _, err := repo.InsertClarificationResolution(ctx, res); err != nil {
		t.Fatalf("insert: %v", err)
	}

	if err := repo.UpdateClarificationResolutionResume(ctx, "pending-C4", models.ClarificationResolutionResumePublished); err != nil {
		t.Fatalf("UpdateClarificationResolutionResume: %v", err)
	}

	got, err := repo.GetClarificationResolution(ctx, "pending-C4")
	if err != nil {
		t.Fatalf("get after update: %v", err)
	}
	if got.Resume != models.ClarificationResolutionResumePublished {
		t.Errorf("got.Resume = %q, want published", got.Resume)
	}
	if got.Status != models.ClarificationResolutionStatusAnswered {
		t.Errorf("resume update changed Status to %q, want unchanged answered", got.Status)
	}
}

func TestUpdateClarificationResolutionResume_NotFound(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()

	if err := repo.UpdateClarificationResolutionResume(ctx, "pending-missing", models.ClarificationResolutionResumeFailed); !errors.Is(err, ErrClarificationResolutionNotFound) {
		t.Fatalf("UpdateClarificationResolutionResume = %v, want ErrClarificationResolutionNotFound", err)
	}
}

// TestClarificationResolutionCascadesOnSessionDelete proves M2: deleting the
// bundle's session row removes its resolution row too.
func TestClarificationResolutionCascadesOnSessionDelete(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedForMsgTest(t, repo, "task-C5", "sess-C5", "turn-C5")

	res := newTestClarificationResolution("pending-C5", "sess-C5", "task-C5")
	if _, _, err := repo.InsertClarificationResolution(ctx, res); err != nil {
		t.Fatalf("insert: %v", err)
	}

	if _, err := repo.db.Exec(repo.db.Rebind(`DELETE FROM task_sessions WHERE id = ?`), "sess-C5"); err != nil {
		t.Fatalf("delete session: %v", err)
	}

	if _, err := repo.GetClarificationResolution(ctx, "pending-C5"); !errors.Is(err, ErrClarificationResolutionNotFound) {
		t.Fatalf("GetClarificationResolution after session delete = %v, want ErrClarificationResolutionNotFound (cascade)", err)
	}
}
