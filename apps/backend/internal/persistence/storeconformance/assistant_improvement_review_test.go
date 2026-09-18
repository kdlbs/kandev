package storeconformance

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestration/models"
	orchstore "github.com/kandev/kandev/internal/orchestration/repository/sqlite"
	testconformance "github.com/kandev/kandev/internal/testutil/storeconformance"
	"github.com/stretchr/testify/require"
)

func TestAssistantImprovementReviewAndRecurrence(t *testing.T) {
	for _, engine := range []testconformance.EngineName{testconformance.EngineSQLite, testconformance.EnginePostgres} {
		t.Run(string(engine), func(t *testing.T) {
			repo, b := authorityStoreFixture(t, engine)
			ctx := context.Background()
			now := time.Now().UTC()
			row := models.Friction{TaskID: "one", SessionID: "session", ProfileID: "profile", AccountRevision: "account", Origin: "native", Operation: "question", Reason: "pending_approval", PolicyVersion: "unknown", Outcome: "blocked"}
			for _, id := range []string{"one", "two", "three"} {
				row.OccurrenceID, row.TaskID = id, id
				require.NoError(t, repo.RecordFriction(ctx, b, row, now))
			}
			rows, err := repo.ImprovementCandidates(ctx, b.ID, "", 10)
			require.NoError(t, err)
			require.Len(t, rows, 1)
			candidate := rows[0]
			require.Error(t, repo.ReviewImprovement(ctx, b, candidate.ID, candidate.Revision, "resolved", models.Evidence{}, now), "no repair or native success exists")
			require.ErrorIs(t, repo.ReviewImprovement(ctx, b, candidate.ID, 0, "rejected", models.Evidence{}, now), models.ErrConflict)
			require.NoError(t, repo.ReviewImprovement(ctx, b, candidate.ID, candidate.Revision, "rejected", models.Evidence{}, now))
			require.NoError(t, repo.Migrate())
			closed, err := repo.ImprovementReview(ctx, b.ID, candidate.ID)
			require.NoError(t, err)
			require.Equal(t, b.OwnerUserID, closed.OwnerUserID)
			require.Equal(t, "rejected", closed.State)
			_, err = repo.ImprovementReview(ctx, "foreign", candidate.ID)
			require.Error(t, err)
			for i, id := range []string{"four", "five", "six"} {
				row.OccurrenceID, row.TaskID, row.ObservedAt = id, id, now.Add(time.Minute)
				require.NoError(t, repo.RecordFriction(ctx, b, row, now.Add(time.Minute)))
				rows, err = repo.ImprovementCandidates(ctx, b.ID, "", 10)
				require.NoError(t, err)
				if i < 2 {
					require.Len(t, rows, 1, "old incidents cannot reopen a rejected proposal")
				} else {
					require.Len(t, rows, 2, "a later repeated problem has its own review history")
				}
			}
			assertImprovementCohorts(t, repo, b, rows)
			require.NoError(t, repo.PruneFriction(ctx, now.Add(32*24*time.Hour)))
			evidence, err := repo.ImprovementEvidence(ctx, b.ID, candidate.Fingerprint, "", 100)
			require.NoError(t, err)
			require.Empty(t, evidence)
			closed, err = repo.ImprovementReview(ctx, b.ID, candidate.ID)
			require.NoError(t, err, "retention preserves the explicit human review receipt")
			require.Equal(t, candidate.ID, closed.CandidateID)
		})
	}
}

func assertImprovementCohorts(t *testing.T, repo *orchstore.Repository, b *models.AssistantBinding, rows []models.ImprovementCandidate) {
	t.Helper()
	for _, candidate := range rows {
		evidence, err := repo.CandidateEvidence(context.Background(), &candidate, "", 100)
		require.NoError(t, err)
		require.Len(t, evidence, 3, "closed and recurring proposals keep distinct incident evidence")
		affected, err := repo.ImprovementAffectedTask(context.Background(), b.ID, candidate.ID, "one")
		require.NoError(t, err)
		require.Equal(t, candidate.State == "rejected", affected, "old affected tasks do not authorize resolution of a recurrence")
	}
}
