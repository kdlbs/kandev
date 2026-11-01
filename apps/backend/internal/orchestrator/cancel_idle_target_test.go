package orchestrator

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/stretchr/testify/require"
)

// TestExplicitCancellationIdleTargetMatrix covers every term of the idle
// definition. Any other state keeps the existing cancellation behavior.
func TestExplicitCancellationIdleTargetMatrix(t *testing.T) {
	for _, tt := range []struct {
		name  string
		state models.TaskSessionState
		want  bool
	}{
		{name: "waiting for input", state: models.TaskSessionStateWaitingForInput, want: true},
		{name: "idle", state: models.TaskSessionStateIdle, want: true},
		{name: "starting", state: models.TaskSessionStateStarting},
		{name: "running", state: models.TaskSessionStateRunning},
		{name: "created", state: models.TaskSessionStateCreated},
		{name: "failed", state: models.TaskSessionStateFailed},
		{name: "completed", state: models.TaskSessionStateCompleted},
		{name: "cancelled", state: models.TaskSessionStateCancelled},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			repo := setupTestRepo(t)
			seedTaskAndSession(t, repo, "task-idle", "session-idle", tt.state)
			svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), &mockAgentManager{})
			svc.turnService = &inactiveTurnService{}

			require.Equal(t, tt.want, svc.explicitCancellationTargetIsIdle(ctx, "session-idle"))
		})
	}
}

// TestExplicitCancellationIdleTargetRefinements pins the conditions that turn a
// nominally idle session back into a live cancellation target.
func TestExplicitCancellationIdleTargetRefinements(t *testing.T) {
	for _, tt := range []struct {
		name  string
		setup func(*testing.T, *Service, *sqliterepo.Repository)
	}{
		{
			name: "active turn in the database",
			setup: func(_ *testing.T, svc *Service, _ *sqliterepo.Repository) {
				svc.turnService = &failingActiveTurnLookup{turn: &models.Turn{ID: "turn-1"}}
			},
		},
		{
			name: "reserved prompt turn",
			setup: func(_ *testing.T, svc *Service, _ *sqliterepo.Repository) {
				svc.activeTurns.Store("session-idle", "turn-reserved")
			},
		},
		{
			name: "pending queued dispatch",
			setup: func(_ *testing.T, svc *Service, _ *sqliterepo.Repository) {
				svc.markQueuedDispatchInFlight("session-idle", "entry-1")
			},
		},
		{
			name: "resume attempt",
			setup: func(t *testing.T, svc *Service, _ *sqliterepo.Repository) {
				_, owner := svc.resumeAttemptStore().begin(context.Background(), "task-idle", "session-idle")
				require.True(t, owner)
			},
		},
		{
			name: "in-flight prompt admission",
			setup: func(t *testing.T, svc *Service, _ *sqliterepo.Repository) {
				entered := make(chan struct{})
				release := make(chan struct{})
				done := make(chan struct{})
				go func() {
					defer close(done)
					_ = svc.messageQueue.WithPromptAdmission(
						context.Background(), "session-idle", func(context.Context) error {
							close(entered)
							<-release
							return nil
						},
					)
				}()
				<-entered
				t.Cleanup(func() {
					close(release)
					<-done
				})
			},
		},
		{
			name: "pending clarification",
			setup: func(t *testing.T, _ *Service, repo *sqliterepo.Repository) {
				seedPendingClarificationMessage(t, repo, "task-idle", "session-idle")
			},
		},
		{
			name: "open background work",
			setup: func(_ *testing.T, svc *Service, _ *sqliterepo.Repository) {
				svc.SetBackgroundWorkObserver(&settlingBackgroundWorkObserver{open: []string{"session-idle"}})
			},
		},
		{
			name: "in-flight steer outside prompt admission",
			setup: func(_ *testing.T, svc *Service, _ *sqliterepo.Repository) {
				svc.steerInFlight.Store("session-idle", struct{}{})
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			repo := setupTestRepo(t)
			seedTaskAndSession(t, repo, "task-idle", "session-idle", models.TaskSessionStateWaitingForInput)
			svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), &mockAgentManager{})
			svc.turnService = &inactiveTurnService{}
			tt.setup(t, svc, repo)

			require.False(t, svc.explicitCancellationTargetIsIdle(ctx, "session-idle"))
		})
	}
}

// TestExplicitCancellationOnIdleTargetIsNoop covers the user-visible contract:
// a click that lands after the turn ended succeeds without side effects.
func TestExplicitCancellationOnIdleTargetIsNoop(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task-idle-cancel", "session-idle-cancel", models.TaskSessionStateWaitingForInput)
	agentManager := &mockAgentManager{isAgentRunning: true, repoForExecutionLookup: repo}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentManager)
	// inactiveTurnService panics on StartTurn, so a created turn fails loudly.
	svc.turnService = &inactiveTurnService{}
	messageCreator := &mockMessageCreator{}
	svc.messageCreator = messageCreator

	svc.messageQueue.SetMergeEnabled(false)
	svc.messageQueue.SetAutoMergeEnabled(false)
	_, err := svc.messageQueue.QueueMessage(
		ctx, "session-idle-cancel", "task-idle-cancel", "still queued", "",
		messagequeue.QueuedByUser, false, nil,
	)
	require.NoError(t, err)
	require.NoError(t, svc.messageQueue.SetAutoRun(ctx, "session-idle-cancel", true))
	before, err := repo.GetTask(ctx, "task-idle-cancel")
	require.NoError(t, err)

	require.NoError(t, svc.CancelAgent(ctx, "session-idle-cancel"))

	after, err := repo.GetTask(ctx, "task-idle-cancel")
	require.NoError(t, err)
	require.Equal(t, before.State, after.State, "cancel must not move the task")
	require.Equal(t, before.WorkflowStepID, after.WorkflowStepID, "cancel must not evaluate workflow completion")
	require.Empty(t, messageCreator.sessionMessages, "cancel must not add a Turn cancelled message")
	require.Empty(t, agentManager.stopAgentArgs, "cancel must not touch an idle runtime")
	require.Empty(t, agentManager.capturedPrompts)

	if _, ok := svc.activeTurns.Load("session-idle-cancel"); ok {
		t.Fatal("idle cancel created a turn")
	}
	status := svc.messageQueue.GetStatus(ctx, "session-idle-cancel")
	require.True(t, status.AutoRun, "idle cancel must not pause Auto-run")
	require.Equal(t, 1, status.Count, "idle cancel must not deliver or drop queued entries")
	require.False(t, svc.CancellationPending("session-idle-cancel"),
		"accepted progress must clear after the no-op")
}
