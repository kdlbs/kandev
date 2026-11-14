package orchestrator

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/orchestrator/executor"
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

const primaryRecoveryBlockedID = "session-recovery-recovery-blocked"

type primaryRecoveryReadError struct {
	*sqliterepo.Repository
}

func (r *primaryRecoveryReadError) GetOpenSessionRecoveryBlock(
	ctx context.Context, sessionID, incarnationID string, generation int64,
) (*models.SessionRecoveryBlock, error) {
	if sessionID == primaryRecoveryBlockedID {
		return nil, errors.New("injected recovery block read failed")
	}
	return r.Repository.GetOpenSessionRecoveryBlock(ctx, sessionID, incarnationID, generation)
}

// Hide optional recovery-store methods while retaining the existing repo seam.
type primaryRecoveryLegacyStore struct {
	sessionExecutorStore
}

func openPrimaryRecoveryBlock(t *testing.T, repo *sqliterepo.Repository, sessionID string) {
	t.Helper()
	ctx := context.Background()
	session, err := repo.GetTaskSession(ctx, sessionID)
	require.NoError(t, err)
	require.NoError(t, repo.UpsertSessionRecoveryBlock(ctx, &models.SessionRecoveryBlock{
		ID: "recovery-block-" + sessionID, SessionID: sessionID,
		IncarnationID: session.QueueIncarnationID,
		Reason:        "unknown_prompt_outcome", State: models.RecoveryBlockOpen,
	}))
}

func primaryRecoveryAttempt(t *testing.T, svc *Service) *resumeAttempt {
	t.Helper()
	attempt, started := svc.resumeAttemptStore().begin(context.Background(), primaryTaskID, primaryStoppedID)
	require.True(t, started)
	t.Cleanup(func() { attempt.finish(svc.resumeAttemptStore()) })
	return attempt
}

// @covers AC-TASKS-PRIMARY-SESSION-OWNERSHIP-001.2
func TestPrimaryRecoveryStopKeepsStoppedPrimary(t *testing.T) {
	for _, kind := range []string{"open-block", "action-required", "unreadable"} {
		t.Run(kind, func(t *testing.T) {
			blocked := &models.TaskSession{ID: primaryRecoveryBlockedID, State: models.TaskSessionStateRunning}
			if kind == "action-required" {
				blocked.RouteState = dynamicRouteStatusActionRequired
				blocked.RouteGeneration = 1
			}
			svc, repo := newPrimaryPromotionFixture(t, "", blocked)
			switch kind {
			case "open-block":
				openPrimaryRecoveryBlock(t, repo, primaryRecoveryBlockedID)
			case "unreadable":
				svc.repo = &primaryRecoveryReadError{Repository: repo}
			}
			stopPrimary(t, svc, repo)
			require.Equal(t, primaryStoppedID, primarySessionID(t, repo))
		})
	}
}

// @covers AC-TASKS-PRIMARY-SESSION-OWNERSHIP-001.2
func TestPrimaryRecoveryStopPrefersHealthyParked(t *testing.T) {
	svc, repo := newPrimaryPromotionFixture(t, "",
		&models.TaskSession{ID: primaryRecoveryBlockedID, State: models.TaskSessionStateRunning},
		&models.TaskSession{ID: "recovery-parked", State: models.TaskSessionStateStarting},
	)
	openPrimaryRecoveryBlock(t, repo, primaryRecoveryBlockedID)
	parkSession(t, repo, "recovery-parked")
	stopPrimary(t, svc, repo)
	require.Equal(t, "recovery-parked", primarySessionID(t, repo))
}

// @covers AC-TASKS-PRIMARY-SESSION-OWNERSHIP-001.2
func TestPrimaryRecoveryRemovalPrefersHealthy(t *testing.T) {
	for _, unreadable := range []bool{false, true} {
		t.Run(map[bool]string{false: "open-block", true: "unreadable"}[unreadable], func(t *testing.T) {
			svc, repo := newPrimaryPromotionFixture(t, "",
				&models.TaskSession{ID: primaryRecoveryBlockedID, State: models.TaskSessionStateRunning},
				&models.TaskSession{ID: "recovery-idle", State: models.TaskSessionStateIdle},
			)
			if unreadable {
				svc.repo = &primaryRecoveryReadError{Repository: repo}
			} else {
				openPrimaryRecoveryBlock(t, repo, primaryRecoveryBlockedID)
			}
			svc.promoteNextPrimaryAfterRemoval(context.Background(), primaryTaskID, primaryStoppedID)
			require.Equal(t, "recovery-idle", primarySessionID(t, repo))
		})
	}
}

// @covers AC-TASKS-PRIMARY-SESSION-OWNERSHIP-001.2
func TestPrimaryRecoveryRemovalKeepsBlockedFallback(t *testing.T) {
	svc, repo := newPrimaryPromotionFixture(t, "",
		&models.TaskSession{ID: primaryRecoveryBlockedID, State: models.TaskSessionStateIdle},
	)
	openPrimaryRecoveryBlock(t, repo, primaryRecoveryBlockedID)
	svc.promoteNextPrimaryAfterRemoval(context.Background(), primaryTaskID, primaryStoppedID)
	require.Equal(t, primaryRecoveryBlockedID, primarySessionID(t, repo))
}

// @covers AC-TASKS-PRIMARY-SESSION-OWNERSHIP-001.2
// @covers AC-TASKS-PRIMARY-SESSION-OWNERSHIP-001.4
func TestPrimaryRecoveryRollbackPreservesRoleAndQueue(t *testing.T) {
	svc, repo := newPrimaryPromotionFixture(t, "",
		&models.TaskSession{ID: "recovery-healthy", State: models.TaskSessionStateStarting},
	)
	ctx := context.Background()
	queued, err := svc.messageQueue.QueueMessage(ctx, primaryStoppedID, primaryTaskID,
		"injected original queue", "", "user", false, nil)
	require.NoError(t, err)
	attempt := primaryRecoveryAttempt(t, svc)
	require.NoError(t, repo.UpdateTaskSessionState(ctx, primaryStoppedID, models.TaskSessionStateStarting, ""))
	require.NoError(t, repo.SetSessionMetadataKey(ctx, primaryStoppedID,
		models.SessionMetaKeyAgentStartAttemptID, attempt.identity()))
	changed, err := svc.rollbackResumeFailureIfCurrentAttempt(ctx, executor.ResumeFailureRollbackRequest{
		TaskID: primaryTaskID, SessionID: primaryStoppedID, AttemptID: attempt.identity(),
		ExpectedState: models.TaskSessionStateStarting, NextState: models.TaskSessionStateFailed,
		ErrorMessage: "native_state_missing",
	})
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, primaryStoppedID, primarySessionID(t, repo))
	entries, _, err := svc.messageQueue.SnapshotSession(ctx, primaryStoppedID)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	require.Equal(t, queued.ID, entries[0].ID)
}

// @covers AC-TASKS-PRIMARY-SESSION-OWNERSHIP-001.2
func TestPrimaryRecoveryCancelledAttemptDoesNotKeepRole(t *testing.T) {
	svc, repo := newPrimaryPromotionFixture(t, "",
		&models.TaskSession{ID: "recovery-healthy", State: models.TaskSessionStateStarting},
	)
	attempt := primaryRecoveryAttempt(t, svc)
	attempt.cancel()
	ctx := context.Background()
	require.NoError(t, repo.UpdateTaskSessionState(ctx, primaryStoppedID, models.TaskSessionStateFailed, ""))
	svc.maybePromotePrimary(ctx, primaryTaskID, primaryStoppedID, models.TaskSessionStateFailed)
	require.Equal(t, "recovery-healthy", primarySessionID(t, repo))
}

// @covers AC-TASKS-PRIMARY-SESSION-OWNERSHIP-001.2
// @covers AC-TASKS-PRIMARY-SESSION-OWNERSHIP-001.4
func TestPrimaryRecoveryExplicitChoicePreservesUnknownBlock(t *testing.T) {
	svc, repo := newPrimaryPromotionFixture(t, "",
		&models.TaskSession{ID: primaryRecoveryBlockedID, State: models.TaskSessionStateRunning},
	)
	openPrimaryRecoveryBlock(t, repo, primaryRecoveryBlockedID)
	ctx := context.Background()
	queued, err := svc.messageQueue.QueueMessage(ctx, primaryStoppedID, primaryTaskID,
		"explicit source queue", "", "user", false, nil)
	require.NoError(t, err)
	require.NoError(t, svc.SetPrimarySession(ctx, primaryRecoveryBlockedID))
	require.Equal(t, primaryRecoveryBlockedID, primarySessionID(t, repo))
	block, err := svc.GetOpenSessionRecoveryBlock(ctx, primaryRecoveryBlockedID)
	require.NoError(t, err)
	require.NotNil(t, block)
	require.Equal(t, "unknown_prompt_outcome", block.Reason)
	entries, _, err := svc.messageQueue.SnapshotSession(ctx, primaryStoppedID)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	require.Equal(t, queued.ID, entries[0].ID)
}

// @covers AC-TASKS-PRIMARY-SESSION-OWNERSHIP-001.2
func TestPrimaryRecoveryLegacyStoreKeepsOrdinaryPromotion(t *testing.T) {
	svc, repo := newPrimaryPromotionFixture(t, "",
		&models.TaskSession{ID: "recovery-legacy", State: models.TaskSessionStateStarting},
	)
	svc.repo = &primaryRecoveryLegacyStore{sessionExecutorStore: svc.repo}
	stopPrimary(t, svc, repo)
	require.Equal(t, "recovery-legacy", primarySessionID(t, repo))
}

// @covers AC-TASKS-PRIMARY-SESSION-OWNERSHIP-001.2
// @covers AC-TASKS-PRIMARY-SESSION-OWNERSHIP-001.4
func TestPrimaryRecoveryAcceptedAttemptAllowsPromotionBeforeFinish(t *testing.T) {
	svc, repo := newPrimaryPromotionFixture(t, "",
		&models.TaskSession{ID: "recovery-healthy", State: models.TaskSessionStateStarting},
	)
	ctx := context.Background()
	queued, err := svc.messageQueue.QueueMessage(ctx, primaryStoppedID, primaryTaskID,
		"accepted source queue", "", "user", false, nil)
	require.NoError(t, err)
	attempt := primaryRecoveryAttempt(t, svc)
	registry := svc.resumeAttemptStore()
	require.True(t, registry.accept(attempt, "recovery-accepted-execution"))
	current, active := registry.current(primaryStoppedID)
	require.True(t, active, "provider acceptance precedes deferred attempt finish")
	require.Same(t, attempt, current)
	require.NoError(t, attempt.ctx.Err())
	require.NoError(t, repo.UpdateTaskSessionState(ctx, primaryStoppedID, models.TaskSessionStateFailed, ""))

	svc.maybePromotePrimary(ctx, primaryTaskID, primaryStoppedID, models.TaskSessionStateFailed)

	require.Equal(t, "recovery-healthy", primarySessionID(t, repo))
	entries, _, err := svc.messageQueue.SnapshotSession(ctx, primaryStoppedID)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	require.Equal(t, queued.ID, entries[0].ID)
}
