package storeconformance

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestration/models"
	testconformance "github.com/kandev/kandev/internal/testutil/storeconformance"
	"github.com/stretchr/testify/require"
)

func TestAssistantFrictionThresholdAndRedaction(t *testing.T) {
	for _, engine := range []testconformance.EngineName{testconformance.EngineSQLite, testconformance.EnginePostgres} {
		t.Run(string(engine), func(t *testing.T) {
			repo, b := authorityStoreFixture(t, engine)
			ctx := context.Background()
			now := time.Date(2026, 9, 18, 0, 0, 0, 0, time.UTC)
			incident := models.Friction{TaskID: "worker-one", SessionID: "session", OccurrenceID: "first", ProfileID: "profile", AccountRevision: "account-v1", Origin: "provider", Operation: "permission", Reason: "pending_approval", PolicyVersion: "unknown", Outcome: "blocked"}
			require.NoError(t, repo.RecordFriction(ctx, b, incident, now))
			require.NoError(t, repo.RecordFriction(ctx, b, incident, now))
			incident.OccurrenceID = "second"
			require.NoError(t, repo.RecordFriction(ctx, b, incident, now))
			rows, err := repo.ImprovementCandidates(ctx, b.ID, "", 100)
			require.NoError(t, err)
			require.Empty(t, rows)
			incident.OccurrenceID, incident.TaskID = "third", "worker-two"
			require.NoError(t, repo.RecordFriction(ctx, b, incident, now))
			rows, err = repo.ImprovementCandidates(ctx, b.ID, "", 100)
			require.NoError(t, err)
			require.Len(t, rows, 1)
			require.Equal(t, "proposed", rows[0].State)
			require.Equal(t, 3, rows[0].IncidentCount)
			require.Equal(t, 2, rows[0].TaskCount)
			require.Empty(t, rows[0].RepairTaskID)
			require.NoError(t, repo.Migrate())
			rows, err = repo.ImprovementCandidates(ctx, b.ID, "", 100)
			require.NoError(t, err)
			require.Len(t, rows, 1)
			incident.Reason = "CANARY_PRIVATE_PROMPT sk-test-do-not-store"
			require.Error(t, repo.RecordFriction(ctx, b, incident, now))
			data, err := json.Marshal(rows)
			require.NoError(t, err)
			require.NotContains(t, string(data), "CANARY_PRIVATE_PROMPT")
			foreign, err := repo.ImprovementCandidates(ctx, "foreign", "", 100)
			require.NoError(t, err)
			require.Empty(t, foreign)
		})
	}
}

func TestAssistantFrictionWindowAndScope(t *testing.T) {
	for _, engine := range []testconformance.EngineName{testconformance.EngineSQLite, testconformance.EnginePostgres} {
		t.Run(string(engine), func(t *testing.T) {
			repo, b := authorityStoreFixture(t, engine)
			ctx := context.Background()
			now := time.Now().UTC()
			incident := models.Friction{TaskID: "task-one", SessionID: "session", OccurrenceID: "old", ProfileID: "profile", AccountRevision: "account", Origin: "native", Operation: "launch", Reason: "task_defect", PolicyVersion: "unknown", Outcome: "blocked"}
			require.NoError(t, repo.RecordFriction(ctx, b, incident, now.Add(-8*24*time.Hour)))
			for _, occurrence := range []string{"one", "two", "three"} {
				incident.OccurrenceID = occurrence
				require.NoError(t, repo.RecordFriction(ctx, b, incident, now))
			}
			rows, err := repo.ImprovementCandidates(ctx, b.ID, "", 100)
			require.NoError(t, err)
			require.Empty(t, rows, "three incidents on one task do not meet the threshold")
			incident.TaskID, incident.OccurrenceID, incident.AccountRevision = "task-two", "other-account", "different"
			require.NoError(t, repo.RecordFriction(ctx, b, incident, now))
			rows, err = repo.ImprovementCandidates(ctx, b.ID, "", 100)
			require.NoError(t, err)
			require.Empty(t, rows, "accounts must not be merged")
			incident.AccountRevision, incident.OccurrenceID = "account", "new-task"
			require.NoError(t, repo.RecordFriction(ctx, b, incident, now))
			rows, err = repo.ImprovementCandidates(ctx, b.ID, "", 100)
			require.NoError(t, err)
			require.Len(t, rows, 1)
			require.Equal(t, 4, rows[0].IncidentCount)
			b.Version++
			incident.OccurrenceID = "after-revoke"
			require.ErrorIs(t, repo.RecordFriction(ctx, b, incident, now), models.ErrConflict)
		})
	}
}

func TestAssistantFrictionOldSourceDoesNotBecomeNew(t *testing.T) {
	for _, engine := range []testconformance.EngineName{testconformance.EngineSQLite, testconformance.EnginePostgres} {
		t.Run(string(engine), func(t *testing.T) {
			repo, b := authorityStoreFixture(t, engine)
			ctx := context.Background()
			now := time.Now().UTC()
			for _, id := range []string{"one", "two", "three"} {
				row := models.Friction{TaskID: "task-" + id, SessionID: "session", OccurrenceID: id, ProfileID: "profile", AccountRevision: "account", Origin: "native", Operation: "question", Reason: "pending_approval", PolicyVersion: "unknown", Outcome: "blocked", ObservedAt: now.Add(-8 * 24 * time.Hour)}
				require.NoError(t, repo.RecordFriction(ctx, b, row, now))
			}
			rows, err := repo.ImprovementCandidates(ctx, b.ID, "", 10)
			require.NoError(t, err)
			require.Empty(t, rows, "reconciliation time must not move old native events into the seven-day window")
		})
	}
}
