package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
)

// TestUnansweredClarificationsPostgres runs the unanswered read, the
// metadata-patch writes, and lazy retention against PostgreSQL syntax:
// jsonb_build_object patches, the BOOLEAN setting predicate, and the
// turn-authority subquery. Skips unless KANDEV_TEST_POSTGRES_DSN is set.
func TestUnansweredClarificationsPostgres(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	ctx := context.Background()
	seedUnansweredWorkspace(t, repo, "ws-pg", true, 7)
	seedSupersededQuestion(t, repo, "ws-pg", "task-pg-a", "pending-pg-a", unansweredBase)
	seedSupersededQuestion(t, repo, "ws-pg", "task-pg-b", "pending-pg-b", unansweredBase.AddDate(0, 0, 5))
	seedSupersededQuestion(t, repo, "ws-pg", "task-pg-old", "pending-pg-old", unansweredBase.AddDate(0, 0, -10))

	ids := listUnansweredAt(t, repo, unansweredBase.Add(time.Hour), "ws-pg")
	if len(ids) != 2 || ids[0] != "pending-pg-a" || ids[1] != "pending-pg-b" {
		t.Fatalf("unanswered = %v, want [pending-pg-a pending-pg-b]", ids)
	}
	if meta := bundleMetadata(t, repo, "pending-pg-old"); meta[models.ClarificationMetaClosedReason] != string(models.ClarificationClosedExpiredUnanswered) {
		t.Fatalf("retention metadata = %v", meta)
	}

	closed, err := repo.CloseUnansweredClarificationBundle(ctx, "pending-pg-a", models.CloseUnansweredClarification{
		Reason: models.ClarificationClosedAnsweredLate, At: unansweredBase.Add(time.Hour),
		MessageID: "late-pg", SessionID: "session-task-pg-a",
	})
	if err != nil || len(closed) != 1 {
		t.Fatalf("CloseUnansweredClarificationBundle = (%d rows, %v), want (1, nil)", len(closed), err)
	}
	if meta := bundleMetadata(t, repo, "pending-pg-a"); meta[models.ClarificationMetaLateAnswerMessageID] != "late-pg" || meta["status"] != "pending" {
		t.Fatalf("late-answer metadata = %v", meta)
	}

	merged, err := repo.MergeUnansweredClarificationDuplicates(ctx, "task-pg-b", "pending-pg-new",
		[]models.ClarificationQuestionIdentity{{Prompt: "WHICH database should we use?"}})
	if err != nil || len(merged) != 1 {
		t.Fatalf("MergeUnansweredClarificationDuplicates = (%v, %v), want one merge", merged, err)
	}
	if ids := listUnansweredAt(t, repo, unansweredBase.Add(time.Hour), "ws-pg"); len(ids) != 0 {
		t.Fatalf("unanswered after close and merge = %v, want none", ids)
	}
}
