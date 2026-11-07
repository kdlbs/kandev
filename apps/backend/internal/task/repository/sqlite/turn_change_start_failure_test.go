package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
	"github.com/stretchr/testify/require"
)

// @covers AC-TASKS-TURN-CHANGES-003.1, AC-TASKS-TURN-CHANGES-003.3
func TestTurnChangeUnavailableWithoutAcceptedStart(t *testing.T) {
	runTurnChangeUnavailableWithoutAcceptedStart(t, newRepoForSessionTests(t))
}

func TestPostgresTurnChangeUnavailableWithoutAcceptedStart(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	require.NoError(t, err)
	runTurnChangeUnavailableWithoutAcceptedStart(t, repo)
}

func runTurnChangeUnavailableWithoutAcceptedStart(t *testing.T, repo *Repository) {
	t.Helper()
	ctx := context.Background()
	require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: "start-failure-task", Title: "Start failure"}))
	require.NoError(t, repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{ID: "start-failure-env", TaskID: "start-failure-task"}))
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "start-failure-session", TaskID: "start-failure-task", TaskEnvironmentID: "start-failure-env",
	}))
	for _, test := range []struct {
		name          string
		availability  models.TurnChangeAvailability
		complete      bool
		summary       bool
		content       bool
		startAccepted bool
		stale         bool
		wantAccepted  bool
	}{
		{name: "unavailable", availability: models.TurnChangeAvailabilityUnavailable, wantAccepted: true},
		{name: "stale", availability: models.TurnChangeAvailabilityUnavailable, stale: true},
		{name: "ready", availability: models.TurnChangeAvailabilityReady},
		{name: "failed", availability: models.TurnChangeAvailabilityFailed},
		{name: "complete", availability: models.TurnChangeAvailabilityUnavailable, complete: true},
		{name: "summary", availability: models.TurnChangeAvailabilityUnavailable, summary: true},
		{name: "content", availability: models.TurnChangeAvailabilityUnavailable, content: true},
		{name: "accepted_start", availability: models.TurnChangeAvailabilityReady, startAccepted: true, wantAccepted: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			set := createUnacceptedStartChangeSet(t, repo, test.name)
			if test.startAccepted {
				accepted, err := repo.AcceptTurnChangeSetStart(ctx, set.ID, set.Revision, nil)
				require.NoError(t, err)
				require.True(t, accepted)
				set, err = repo.GetTurnChangeSet(ctx, set.TaskID, set.TaskSessionID, set.ID)
				require.NoError(t, err)
			}
			revision := set.Revision
			if test.stale {
				revision++
			}
			finalization := models.TurnChangeSetFinalization{
				Availability: test.availability, Reason: models.TurnChangeReasonCaptureFailed,
				TerminalAt: time.Date(2026, 10, 21, 17, 55, 0, 0, time.UTC), TerminalOutcome: "end_turn",
				Complete: test.complete, SummaryComplete: test.summary, ContentComplete: test.content,
			}
			accepted, err := repo.FinalizeTurnChangeSet(ctx, set.ID, revision, finalization)
			require.NoError(t, err)
			require.Equal(t, test.wantAccepted, accepted)
			loaded, err := repo.GetTurnChangeSet(ctx, set.TaskID, set.TaskSessionID, set.ID)
			require.NoError(t, err)
			require.True(t, loaded.CaptureEnabled)
			require.Equal(t, test.startAccepted, loaded.StartAccepted)
			if !test.wantAccepted {
				require.Nil(t, loaded.TerminalAt)
				require.Equal(t, set.Revision, loaded.Revision)
				return
			}
			require.Equal(t, finalization.TerminalAt, *loaded.TerminalAt)
			require.Equal(t, test.availability, loaded.Availability)
			require.False(t, loaded.Complete)
			accepted, err = repo.FinalizeTurnChangeSet(ctx, set.ID, loaded.Revision, finalization)
			require.NoError(t, err)
			require.False(t, accepted, "terminal rows are immutable")
		})
	}
}

func createUnacceptedStartChangeSet(t *testing.T, repo *Repository, suffix string) *models.TurnChangeSet {
	t.Helper()
	ctx := context.Background()
	require.NoError(t, repo.CreateTurn(ctx, &models.Turn{
		ID: "start-failure-turn-" + suffix, TaskID: "start-failure-task", TaskSessionID: "start-failure-session",
		StartedAt: time.Date(2026, 10, 21, 17, 39, 0, 0, time.UTC),
	}))
	set := &models.TurnChangeSet{
		ID: "start-failure-set-" + suffix, TaskID: "start-failure-task", TaskSessionID: "start-failure-session",
		TaskEnvironmentID: "start-failure-env", TurnID: "start-failure-turn-" + suffix,
		Revision: 1, CaptureEnabled: true, Availability: models.TurnChangeAvailabilityPending,
		SettingsUserID: "start-failure-user", ResolutionKind: "authenticated_user",
	}
	require.NoError(t, repo.CreateTurnChangeSet(ctx, set))
	return set
}
