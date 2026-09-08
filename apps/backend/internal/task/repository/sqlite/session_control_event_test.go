package sqlite

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestCreateSessionControlEventStoresOnlyNormalizedControlEvidence(t *testing.T) {
	ctx := context.Background()
	repo := newRepoForSessionTests(t)
	for _, taskID := range []string{"actor", "target"} {
		require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: taskID, Title: taskID}))
	}
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{ID: "target-session", TaskID: "target"}))
	event := &models.SessionControlEvent{
		ActorTaskID: "actor", ActorSessionID: "actor-session", TargetTaskID: "target", TargetSessionID: "target-session", TargetTurnID: "turn",
		AuthorityBasis: "direct_parent", EvidenceCode: "eligible_completion_intent", Result: "settled",
	}
	require.NoError(t, repo.CreateSessionControlEvent(ctx, event))
	var got struct{ Authority, Evidence, Result string }
	require.NoError(t, repo.db.QueryRowContext(ctx, `SELECT authority_basis, evidence_code, result FROM session_control_events WHERE id = ?`, event.ID).Scan(&got.Authority, &got.Evidence, &got.Result))
	require.Equal(t, "direct_parent", got.Authority)
	require.Equal(t, "eligible_completion_intent", got.Evidence)
	require.Equal(t, "settled", got.Result)
}
