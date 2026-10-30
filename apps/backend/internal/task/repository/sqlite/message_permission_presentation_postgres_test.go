package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
)

// TestPostgresPermissionPresentationMatchesSQLite pins the dialect-sensitive
// JSON paths used by presentation updates and the pending projections. It
// skips unless KANDEV_TEST_POSTGRES_DSN is configured.
func TestPostgresPermissionPresentationMatchesSQLite(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	ctx := context.Background()
	base := time.Date(2026, time.October, 29, 12, 0, 0, 0, time.UTC)

	seedPendingActionSession(t, repo, "task-presentation-pg", "session-presentation-pg")
	createPendingActionTurn(t, repo, "task-presentation-pg", "session-presentation-pg", "turn-presentation-pg", base, base)
	createPermissionPresentationMessage(t, repo, "presentation-pg", "task-presentation-pg", "session-presentation-pg",
		"turn-presentation-pg", "pending-presentation-pg", models.PermissionPresentationAutomatic,
		models.PermissionStatusPending, base)

	if pending := pendingPermissionSessionIDs(t, repo, []string{"session-presentation-pg"}); len(pending) != 0 {
		t.Fatalf("automatic permission reported as pending input on postgres: %#v", pending)
	}

	message, updated, err := repo.UpdatePermissionPresentation(ctx, models.PermissionPresentationUpdate{
		TaskID: "task-presentation-pg", SessionID: "session-presentation-pg",
		RequestID: "request-presentation-pg", PendingID: "pending-presentation-pg",
		Presentation:        models.PermissionPresentationInteractive,
		RequirePresentation: models.PermissionPresentationAutomatic,
		RequireUnclaimed:    true,
	})
	if err != nil || !updated || message == nil {
		t.Fatalf("promotion message=%+v updated=%v err=%v", message, updated, err)
	}
	if got := message.Metadata[models.PermissionPresentationMetadataKey]; got != string(models.PermissionPresentationInteractive) {
		t.Fatalf("permission_presentation = %#v, want interactive", got)
	}
}
