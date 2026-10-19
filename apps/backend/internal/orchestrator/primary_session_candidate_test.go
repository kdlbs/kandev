package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

const (
	primaryTaskID      = "task-primary-promotion"
	primaryStoppedID   = "session-primary-stopped"
	primaryStepID      = "step-primary-review"
	primaryStepProfile = "profile-review"
)

// newPrimaryPromotionFixture seeds a task whose primary session has stopped,
// on stepID (empty for a task without a step), plus the given sessions.
func newPrimaryPromotionFixture(t *testing.T, stepID string, sessions ...*models.TaskSession) (*Service, *sqliterepo.Repository) {
	t.Helper()
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, primaryTaskID, primaryStoppedID, stepID)
	now := time.Now().UTC()
	for _, session := range sessions {
		session.TaskID = primaryTaskID
		session.StartedAt, session.UpdatedAt = now, now
		require.NoError(t, repo.CreateTaskSession(ctx, session))
	}
	require.NoError(t, repo.SetSessionPrimary(ctx, primaryStoppedID))

	steps := newMockStepGetter()
	steps.steps[primaryStepID] = &wfmodels.WorkflowStep{
		ID: primaryStepID, WorkflowID: "wf1", Name: "Review", AgentProfileID: primaryStepProfile,
	}
	return createTestServiceWithScheduler(repo, steps, newMockTaskRepo(), &mockAgentManager{}), repo
}

func parkSession(t *testing.T, repo *sqliterepo.Repository, sessionID string) {
	t.Helper()
	require.NoError(t, repo.SetSessionMetadataKey(context.Background(), sessionID, models.SessionMetaKeyWorkflowParking,
		map[string]interface{}{
			"stamp": "park-" + sessionID, "parked_at": time.Now().UTC().Format(time.RFC3339Nano), "source_session_id": sessionID,
		}))
}

func stopPrimary(t *testing.T, svc *Service, repo *sqliterepo.Repository) {
	t.Helper()
	ctx := context.Background()
	require.NoError(t, repo.UpdateTaskSessionState(ctx, primaryStoppedID, models.TaskSessionStateCompleted, ""))
	svc.maybePromotePrimary(ctx, primaryTaskID, primaryStoppedID, models.TaskSessionStateCompleted)
}

func primarySessionID(t *testing.T, repo *sqliterepo.Repository) string {
	t.Helper()
	primary, err := repo.GetPrimarySessionByTaskID(context.Background(), primaryTaskID)
	require.NoError(t, err)
	if primary == nil {
		return ""
	}
	return primary.ID
}

// AC-TASKS-PRIMARY-SESSION-OWNERSHIP-001.2: a session parked by a profile
// switch ranks last, even when it is running.
func TestPrimaryPromotionAfterStopSkipsParkedSession(t *testing.T) {
	svc, repo := newPrimaryPromotionFixture(t, "",
		&models.TaskSession{ID: "session-parked", State: models.TaskSessionStateRunning},
		&models.TaskSession{ID: "session-live", State: models.TaskSessionStateStarting},
	)
	parkSession(t, repo, "session-parked")

	stopPrimary(t, svc, repo)

	require.Equal(t, "session-live", primarySessionID(t, repo))
}

// AC-TASKS-PRIMARY-SESSION-OWNERSHIP-001.2: a session using the current step's
// profile wins over a running session of another profile.
func TestPrimaryPromotionAfterStopPrefersCurrentStepProfile(t *testing.T) {
	svc, repo := newPrimaryPromotionFixture(t, primaryStepID,
		&models.TaskSession{ID: "session-other-profile", State: models.TaskSessionStateRunning, AgentProfileID: "profile-other"},
		&models.TaskSession{ID: "session-step-profile", State: models.TaskSessionStateWaitingForInput, AgentProfileID: primaryStepProfile},
	)

	stopPrimary(t, svc, repo)

	require.Equal(t, "session-step-profile", primarySessionID(t, repo))
}

// AC-TASKS-PRIMARY-SESSION-OWNERSHIP-001.3: after a stop, only an active
// session can inherit the role.
func TestPrimaryPromotionAfterStopRequiresActiveSession(t *testing.T) {
	svc, repo := newPrimaryPromotionFixture(t, primaryStepID,
		&models.TaskSession{ID: "session-idle", State: models.TaskSessionStateIdle, AgentProfileID: primaryStepProfile},
	)

	stopPrimary(t, svc, repo)

	require.Equal(t, primaryStoppedID, primarySessionID(t, repo), "an idle session must not inherit the primary role after a stop")
}

// AC-TASKS-PRIMARY-SESSION-OWNERSHIP-001.3: after a deletion, a terminal
// session is never promoted.
func TestPrimaryPromotionAfterRemovalSkipsTerminalSession(t *testing.T) {
	svc, repo := newPrimaryPromotionFixture(t, "",
		&models.TaskSession{ID: "session-completed", State: models.TaskSessionStateCompleted},
	)

	svc.promoteNextPrimaryAfterRemoval(context.Background(), primaryTaskID, primaryStoppedID)

	completed, err := repo.GetTaskSession(context.Background(), "session-completed")
	require.NoError(t, err)
	require.False(t, completed.IsPrimary, "a completed session became primary after a deletion")
}

// AC-TASKS-PRIMARY-SESSION-OWNERSHIP-001.3: when only sessions of another
// profile remain, one of them still becomes primary after a deletion.
func TestPrimaryPromotionAfterRemovalKeepsUnmatchedSession(t *testing.T) {
	svc, repo := newPrimaryPromotionFixture(t, primaryStepID,
		&models.TaskSession{ID: "session-unmatched", State: models.TaskSessionStateIdle, AgentProfileID: "profile-other"},
	)

	svc.promoteNextPrimaryAfterRemoval(context.Background(), primaryTaskID, primaryStoppedID)

	require.Equal(t, "session-unmatched", primarySessionID(t, repo))
}
