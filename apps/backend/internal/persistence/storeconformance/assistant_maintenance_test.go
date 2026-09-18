package storeconformance

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestration/models"
	testconformance "github.com/kandev/kandev/internal/testutil/storeconformance"
	"github.com/stretchr/testify/require"
)

func TestAssistantMaintenanceGrantPersistence(t *testing.T) {
	for _, engine := range []testconformance.EngineName{testconformance.EngineSQLite, testconformance.EnginePostgres} {
		t.Run(string(engine), func(t *testing.T) {
			repo, b := authorityStoreFixture(t, engine)
			ctx := context.Background()
			now := time.Now().UTC()
			for _, id := range []string{"one", "two", "three"} {
				require.NoError(t, repo.RecordFriction(ctx, b, models.Friction{TaskID: "task-" + id, SessionID: "session", OccurrenceID: id, ProfileID: "profile", AccountRevision: "account", Origin: "native", Operation: "question", Reason: "pending_approval", PolicyVersion: "unknown", Outcome: "blocked"}, now))
			}
			candidates, err := repo.ImprovementCandidates(ctx, b.ID, "", 10)
			require.NoError(t, err)
			require.Len(t, candidates, 1)
			candidate := candidates[0]
			grant := &models.MaintenanceGrant{CandidateID: candidate.ID, BindingVersion: b.Version, AuthorityRevision: "authority", ProfileRevision: "profile-revision", BaseOID: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", ExpiresAt: now.Add(time.Hour),
				Scope: models.MaintenanceScope{RepositoryID: "repo", WorkflowID: "workflow", WorkflowStepID: "review", ProfileID: "profile", Files: []string{"scripts/sample.js"}, Actions: []string{"read", "patch", "test", "commit"}, Image: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Positive: []string{"node", "tests/positive.js"}, Negative: []string{"node", "tests/negative.js"}}}
			require.ErrorIs(t, repo.SaveMaintenanceGrant(ctx, b, grant, 7, candidate.Revision), models.ErrConflict)
			require.NoError(t, repo.SaveMaintenanceGrant(ctx, b, grant, 0, candidate.Revision))
			require.EqualValues(t, 1, grant.Revision)
			require.Equal(t, b.OwnerUserID, grant.OwnerUserID)
			current, err := repo.ImprovementCandidate(ctx, b.ID, candidate.ID)
			require.NoError(t, err)
			require.NotEmpty(t, current.ObjectiveID, "grant confirmation creates an auditable objective, not a repair task")
			require.Empty(t, current.RepairTaskID)
			objective, err := repo.Objective(ctx, b.ID, current.ObjectiveID)
			require.NoError(t, err)
			source, err := repo.GetCommentByID(ctx, b.ConversationID, objective.SourceCommentID)
			require.NoError(t, err)
			require.Equal(t, b.OwnerUserID, source.AuthorID)
			require.Equal(t, "maintenance_grant", source.Source)
			require.ErrorIs(t, repo.SaveMaintenanceGrant(ctx, b, grant, 0, candidate.Revision), models.ErrConflict)
			saved, err := repo.MaintenanceGrant(ctx, b.ID, candidate.ID)
			require.NoError(t, err)
			require.Equal(t, grant.Scope, saved.Scope)
			require.NoError(t, repo.Migrate())
			_, err = repo.MaintenanceGrant(ctx, "foreign", candidate.ID)
			require.Error(t, err)
			require.ErrorIs(t, repo.RevokeMaintenanceGrant(ctx, b, candidate.ID, 0), models.ErrConflict)
			require.NoError(t, repo.RevokeMaintenanceGrant(ctx, b, candidate.ID, 1))
			saved, err = repo.MaintenanceGrant(ctx, b.ID, candidate.ID)
			require.NoError(t, err)
			require.NotNil(t, saved.RevokedAt)
			require.EqualValues(t, 2, saved.Revision)
			b.Version++
			require.ErrorIs(t, repo.SaveMaintenanceGrant(ctx, b, grant, 2, candidate.Revision), models.ErrConflict)
		})
	}
}
