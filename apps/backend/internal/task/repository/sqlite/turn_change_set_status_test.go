package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
)

func TestTurnChangeSetTerminalStatesAndSuccessfulZeroRoundTrip(t *testing.T) {
	runTurnChangeSetTerminalStatesRoundTrip(t, newRepoForSessionTests(t))
}

func TestPostgresTurnChangeSetTerminalStatesAndSuccessfulZeroRoundTrip(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("initialize postgres schema: %v", err)
	}
	runTurnChangeSetTerminalStatesRoundTrip(t, repo)
}

func runTurnChangeSetTerminalStatesRoundTrip(t *testing.T, repo *Repository) {
	t.Helper()
	ctx := context.Background()
	const taskID, environmentID, sessionID = "turn-state-task", "turn-state-env", "turn-state-session"
	if err := repo.CreateTask(ctx, &models.Task{ID: taskID, Title: "Turn state coverage"}); err != nil {
		t.Fatalf("create task: %v", err)
	}
	if err := repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
		ID: environmentID, TaskID: taskID, Status: models.TaskEnvironmentStatusCreating,
	}); err != nil {
		t.Fatalf("create environment: %v", err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: sessionID, TaskID: taskID, TaskEnvironmentID: environmentID,
	}); err != nil {
		t.Fatalf("create session: %v", err)
	}
	assertTurnChangeSetTerminalStates(t, repo, ctx, taskID, environmentID, sessionID)
}

func assertTurnChangeSetTerminalStates(
	t *testing.T,
	repo *Repository,
	ctx context.Context,
	taskID, environmentID, sessionID string,
) {
	t.Helper()
	zero := int64(0)
	terminalAt := time.Now().UTC()
	cases := []struct {
		name            string
		availability    models.TurnChangeAvailability
		reason          models.TurnChangeReason
		complete        bool
		summaryComplete bool
		contentComplete bool
		retentionReason string
		wantPartial     bool
		wantKnownZero   bool
	}{
		{name: "zero", availability: models.TurnChangeAvailabilityReady, complete: true, summaryComplete: true, contentComplete: true, wantKnownZero: true},
		{name: "partial", availability: models.TurnChangeAvailabilityReady, reason: models.TurnChangeReasonComparisonFailed, summaryComplete: true, wantPartial: true},
		{name: "failed", availability: models.TurnChangeAvailabilityFailed, reason: models.TurnChangeReasonComparisonFailed, wantPartial: true},
		{name: "expired", availability: models.TurnChangeAvailabilityExpired, reason: models.TurnChangeReasonExpiredAge, retentionReason: "expired_age", wantPartial: true},
	}
	for index, test := range cases {
		turnID := "turn-state-" + test.name
		if err := repo.CreateTurn(ctx, &models.Turn{
			ID: turnID, TaskID: taskID, TaskSessionID: sessionID, StartedAt: terminalAt.Add(time.Duration(index) * time.Second),
		}); err != nil {
			t.Fatalf("create %s turn: %v", test.name, err)
		}
		changeSet := &models.TurnChangeSet{
			ID: "change-set-state-" + test.name, TaskID: taskID, TaskSessionID: sessionID,
			TurnID: turnID, TaskEnvironmentID: environmentID, CaptureEnabled: true,
			SettingsUserID: "turn-state-user", ResolutionKind: "authenticated_user",
			Availability: test.availability, Reason: test.reason, Complete: test.complete,
			SummaryComplete: test.summaryComplete, ContentComplete: test.contentComplete,
			TerminalAt: &terminalAt, FileCount: 0, AddedLines: &zero, DeletedLines: &zero,
			ExpiryReason:     test.retentionReason,
			OverlapIntervals: []models.TurnChangeOverlap{{ChangeSetID: "overlap-" + test.name, CheckoutID: "checkout-a", StartedAt: terminalAt}},
		}
		if err := repo.CreateTurnChangeSet(ctx, changeSet); err != nil {
			t.Fatalf("create %s change set: %v", test.name, err)
		}
		loaded, err := repo.GetTurnChangeSet(ctx, taskID, sessionID, changeSet.ID)
		if err != nil {
			t.Fatalf("read %s change set: %v", test.name, err)
		}
		assertTurnChangeSetStatus(t, test.name, loaded, test)
	}
}

func assertTurnChangeSetStatus(
	t *testing.T,
	name string,
	loaded *models.TurnChangeSet,
	test struct {
		name            string
		availability    models.TurnChangeAvailability
		reason          models.TurnChangeReason
		complete        bool
		summaryComplete bool
		contentComplete bool
		retentionReason string
		wantPartial     bool
		wantKnownZero   bool
	},
) {
	t.Helper()
	if loaded.Availability != test.availability || loaded.Reason != test.reason || loaded.Partial() != test.wantPartial {
		t.Fatalf("%s status = availability %q, reason %q, partial %t", name, loaded.Availability, loaded.Reason, loaded.Partial())
	}
	if loaded.Complete != test.complete || loaded.SummaryComplete != test.summaryComplete || loaded.ContentComplete != test.contentComplete {
		t.Fatalf("%s completeness = %t/%t/%t", name, loaded.Complete, loaded.SummaryComplete, loaded.ContentComplete)
	}
	if loaded.ExpiryReason != test.retentionReason || len(loaded.OverlapIntervals) != 1 {
		t.Fatalf("%s retention/overlap = %q/%d", name, loaded.ExpiryReason, len(loaded.OverlapIntervals))
	}
	if test.wantKnownZero && (loaded.FileCount != 0 || loaded.AddedLines == nil || *loaded.AddedLines != 0 || loaded.DeletedLines == nil || *loaded.DeletedLines != 0) {
		t.Fatalf("successful zero summary = count %d, lines %v/%v", loaded.FileCount, loaded.AddedLines, loaded.DeletedLines)
	}
}
