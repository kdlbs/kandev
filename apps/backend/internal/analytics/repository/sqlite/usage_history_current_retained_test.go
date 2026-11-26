package sqlite

import (
	"testing"
	"time"

	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestUsageHistoryCurrentTargetKeepsDeletedIncarnations(t *testing.T) {
	runCurrentTargetDeletedIncarnations(t, false)
}

func TestUsageHistoryPostgresCurrentTargetKeepsDeletedIncarnations(t *testing.T) {
	runCurrentTargetDeletedIncarnations(t, true)
}

// @covers AC-COSTS-USAGE-HISTORY-001.3 AC-COSTS-USAGE-HISTORY-004.5
func runCurrentTargetDeletedIncarnations(t *testing.T, postgres bool) {
	for _, prune := range []bool{false, true} {
		name := "task_delete"
		if prune {
			name = "session_prune"
		}
		t.Run(name, func(t *testing.T) {
			repo, tasks := currentTargetDatabaseFixture(t, postgres)
			old := currentTargetForTest(t, repo, "collector-a", "ws-1", currentUsageTargetSelector{TaskID: "task-1", SessionID: "session-1", ExternalSessionID: "transcript-1"})
			require.NotNil(t, old)
			if prune {
				session, err := tasks.GetTaskSession(t.Context(), "session-1")
				require.NoError(t, err)
				require.NoError(t, tasks.DeleteTaskSession(t.Context(), session))
			} else {
				require.NoError(t, tasks.DeleteTask(t.Context(), "task-1"))
			}
			finishCurrentFixtureDeliveryRetirement(t, tasks)
			require.Nil(t, currentTargetForTest(t, repo, "collector-a", "ws-1", currentUsageTargetSelector{SessionID: "session-1"}))
			require.Nil(t, currentTargetForTest(t, repo, "collector-a", "ws-1", currentUsageTargetSelector{ExternalSessionID: "transcript-1"}))
			query := currentUsageTargetSelector{SubjectBinding: old.BindingID, ExternalSessionID: "transcript-1"}
			detached := currentTargetForTest(t, repo, "collector-a", "ws-1", query)
			require.Equal(t, old.BindingID, detached.BindingID)
			require.Equal(t, old.SubjectID, detached.SubjectID)
			require.Equal(t, "eligible", detached.SourceScopeStatus)
			at := time.Date(2027, 2, 1, 0, 0, 0, 0, time.UTC)
			if !prune {
				require.NoError(t, tasks.CreateTask(t.Context(), &taskmodels.Task{ID: "task-1", WorkspaceID: "ws-1", Title: "Reused", CreatedAt: at, UpdatedAt: at}))
			}
			require.NoError(t, tasks.CreateTaskSession(t.Context(), &taskmodels.TaskSession{
				ID: "session-1", TaskID: "task-1", StartedAt: at,
				Metadata: map[string]any{"acp": map[string]any{"session_id": "fresh-transcript"}},
			}))
			fresh := currentTargetForTest(t, repo, "collector-a", "ws-1", currentUsageTargetSelector{TaskID: "task-1", SessionID: "session-1"})
			require.NotNil(t, fresh)
			require.NotEqual(t, old.BindingID, fresh.BindingID)
			require.NotEqual(t, old.SubjectID, fresh.SubjectID)
			require.NotEqual(t, old.SessionIncarnationID, fresh.SessionIncarnationID)
			require.Equal(t, "fresh-transcript", fresh.ExternalSessionID)
			detached = currentTargetForTest(t, repo, "collector-a", "ws-1", query)
			require.Equal(t, old.SubjectID, detached.SubjectID)
			require.Equal(t, old.SessionIncarnationID, detached.SessionIncarnationID)
			require.Equal(t, "transcript-1", detached.ExternalSessionID)
			require.Equal(t, old.SessionStartedAt, detached.SessionStartedAt)
			query.ExternalSessionID = "fresh-transcript"
			require.Nil(t, currentTargetForTest(t, repo, "collector-a", "ws-1", query), "an old binding cannot retarget a new transcript")
		})
	}
}
