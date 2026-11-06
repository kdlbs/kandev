package orchestrator

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/plancomments"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	"github.com/stretchr/testify/require"
)

const (
	queuedSwitchPrompt      = "follow-up after the previous turn"
	queuedSwitchPlanComment = "plan feedback after the previous turn"
	queuedSwitchDestExecID  = "execution-b"
	queuedSwitchSourceExeID = "execution-a"
)

type queuedTurnStartSwitchFixture struct {
	*profileSwitchFixture
	messages    *mockMessageCreator
	attachments *workflowAttachmentTransferStub
}

// newQueuedTurnStartSwitchFixture builds a waiting profile-a session whose
// step moves to a profile-b step on turn start. The profile-b replacement is
// created by the switch; its runtime record appears when it is attached, as
// the lifecycle manager does in production.
func newQueuedTurnStartSwitchFixture(t *testing.T, mode queuedTurnStartSwitchMode) *queuedTurnStartSwitchFixture {
	t.Helper()
	ctx := context.Background()
	fixture := newProfileSwitchFixture(
		t,
		models.WorkflowProfileSessionStartPolicyReuse,
		models.WorkflowProfileSessionEndPolicyPark,
	)
	source, err := fixture.repo.GetTaskSession(ctx, fixture.current.ID)
	require.NoError(t, err)
	source.State = models.TaskSessionStateWaitingForInput
	require.NoError(t, fixture.repo.UpdateTaskSession(ctx, source))

	fixture.stepGetter.steps["step-a"] = &wfmodels.WorkflowStep{
		ID: "step-a", WorkflowID: "wf1", Name: "Implement", Position: 0,
		AgentProfileID:            "profile-a",
		ProfileSessionStartPolicy: fixture.startPolicy,
		ProfileSessionEndPolicy:   fixture.endPolicy,
		Events: wfmodels.StepEvents{
			OnTurnStart: []wfmodels.OnTurnStartAction{{Type: wfmodels.OnTurnStartMoveToNext}},
		},
	}
	fixture.stepGetter.steps["step-b"] = &wfmodels.WorkflowStep{
		ID: "step-b", WorkflowID: "wf1", Name: "Review", Position: 1,
		AgentProfileID:            "profile-b",
		ProfileSessionStartPolicy: fixture.startPolicy,
		ProfileSessionEndPolicy:   fixture.endPolicy,
	}
	if mode.engine {
		fixture.svc.SetWorkflowStepGetter(fixture.stepGetter)
	} else {
		fixture.svc.workflowEngine = nil
	}
	if mode.sqliteQueue {
		// Production queue: ordinary dequeues keep a durable dispatch claim
		// and reserved deliveries block transfers until they are released.
		// It shares the task database, as in production.
		db := sqlx.NewDb(fixture.repo.DB(), "sqlite3")
		queueRepo, err := messagequeue.NewSQLiteRepository(db, db)
		require.NoError(t, err)
		fixture.svc.messageQueue = messagequeue.NewService(queueRepo, messagequeue.DefaultMaxPerSession, testLogger())
	}
	fixture.agentMgr.isAgentRunning = true
	fixture.agentMgr.launchAgentFunc = func(_ context.Context, req *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
		seedExecutorRunning(t, fixture.repo, req.SessionID, req.TaskID, queuedSwitchDestExecID)
		return &executor.LaunchAgentResponse{AgentExecutionID: queuedSwitchDestExecID}, nil
	}
	messages := &mockMessageCreator{}
	fixture.svc.messageCreator = messages
	attachments := &workflowAttachmentTransferStub{}
	fixture.svc.SetSessionAttachmentTransferer(attachments)
	return &queuedTurnStartSwitchFixture{
		profileSwitchFixture: fixture,
		messages:             messages,
		attachments:          attachments,
	}
}

type queuedTurnStartSwitchMode struct {
	engine      bool
	sqliteQueue bool
}

func forEachWorkflowMode(
	t *testing.T,
	queueRepositories []bool,
	run func(t *testing.T, mode queuedTurnStartSwitchMode),
) {
	t.Helper()
	for _, engine := range []bool{false, true} {
		for _, sqliteQueue := range queueRepositories {
			name := "legacy"
			if engine {
				name = "engine"
			}
			if sqliteQueue {
				name += "/sqlite-queue"
			} else {
				name += "/memory-queue"
			}
			mode := queuedTurnStartSwitchMode{engine: engine, sqliteQueue: sqliteQueue}
			t.Run(name, func(t *testing.T) { run(t, mode) })
		}
	}
}

func (f *queuedTurnStartSwitchFixture) promptsContaining(content string) []promptCall {
	f.agentMgr.mu.Lock()
	defer f.agentMgr.mu.Unlock()
	var calls []promptCall
	for _, call := range f.agentMgr.capturedPromptCalls {
		if strings.Contains(call.Prompt, content) {
			calls = append(calls, call)
		}
	}
	return calls
}

func (f *queuedTurnStartSwitchFixture) userMessagesContaining(content string) []mockUserMessage {
	f.messages.mu.Lock()
	defer f.messages.mu.Unlock()
	var recorded []mockUserMessage
	for _, message := range f.messages.userMessages {
		if strings.Contains(message.content, content) {
			recorded = append(recorded, message)
		}
	}
	return recorded
}

func (f *queuedTurnStartSwitchFixture) primarySession(t *testing.T) *models.TaskSession {
	t.Helper()
	sessions, err := f.repo.ListTaskSessions(context.Background(), "t1")
	require.NoError(t, err)
	var primary *models.TaskSession
	for _, session := range sessions {
		if session.IsPrimary {
			require.Nil(t, primary, "task has more than one primary session")
			primary = session
		}
	}
	require.NotNil(t, primary, "task has no primary session")
	return primary
}

func (f *queuedTurnStartSwitchFixture) queueCount(sessionID string) int {
	return f.svc.messageQueue.GetStatus(context.Background(), sessionID).Count
}

// waitForDelivery waits until the queued content reached an agent and no
// dispatch is still in flight for the task's sessions.
func (f *queuedTurnStartSwitchFixture) waitForDelivery(t *testing.T, content, sourceID string) {
	t.Helper()
	require.Eventually(t, func() bool {
		if len(f.promptsContaining(content)) == 0 || f.svc.isQueuedDispatchInFlight(sourceID) {
			return false
		}
		sessions, err := f.repo.ListTaskSessions(context.Background(), "t1")
		if err != nil {
			return false
		}
		for _, session := range sessions {
			if f.svc.isQueuedDispatchInFlight(session.ID) {
				return false
			}
		}
		return true
	}, 5*time.Second, 10*time.Millisecond,
		"queued message was never prompted after on_turn_start switched the task's profile session")
}

func (f *queuedTurnStartSwitchFixture) requireDeliveredOnceToDestination(t *testing.T, content, sourceID string) *models.TaskSession {
	t.Helper()
	ctx := context.Background()
	task, err := f.repo.GetTask(ctx, "t1")
	require.NoError(t, err)
	require.Equal(t, "step-b", task.WorkflowStepID)
	destination := f.primarySession(t)
	require.NotEqual(t, sourceID, destination.ID, "the profile-b session must be primary after the switch")
	require.Equal(t, "profile-b", destination.AgentProfileID)

	prompts := f.promptsContaining(content)
	require.Len(t, prompts, 1, "the queued message must be prompted exactly once")
	require.Equal(t, queuedSwitchDestExecID, prompts[0].ExecutionID,
		"the queued message must be prompted to the destination profile session")
	recorded := f.userMessagesContaining(content)
	require.Len(t, recorded, 1, "the queued message must be recorded exactly once")
	require.Equal(t, destination.ID, recorded[0].sessionID,
		"the queued message must be recorded on the destination profile session")
	require.Zero(t, f.queueCount(sourceID), "nothing may remain queued on the parked source")
	require.Zero(t, f.queueCount(destination.ID), "the delivered message must leave the destination queue")

	parked, err := f.repo.GetTaskSession(ctx, sourceID)
	require.NoError(t, err)
	require.False(t, parked.IsPrimary, "the source profile session must be parked behind the destination session")
	return destination
}

// TestQueuedTurnStartProfileSwitchDeliversOnceToDestination drains a queued
// prompt whose on_turn_start moves the task to a step owned by another agent
// profile. The prompt claim holds the source session's dispatch guard while
// on_turn_start runs, so profile retirement reuses that ownership. The
// message, with its attachment, must then reach the destination profile
// session exactly once instead of being dropped between the two queues.
func TestQueuedTurnStartProfileSwitchDeliversOnceToDestination(t *testing.T) {
	forEachWorkflowMode(t, []bool{false, true}, func(t *testing.T, mode queuedTurnStartSwitchMode) {
		ctx := context.Background()
		fixture := newQueuedTurnStartSwitchFixture(t, mode)
		sourceID := fixture.current.ID
		_, err := fixture.svc.messageQueue.QueueMessageWithMetadata(
			ctx, sourceID, "t1", queuedSwitchPrompt, "",
			messagequeue.QueuedByUser, false,
			[]messagequeue.MessageAttachment{{AttachmentID: "attachment-1", Name: "notes.txt", MimeType: "text/plain"}},
			nil,
		)
		require.NoError(t, err)

		require.True(t, fixture.svc.drainQueuedMessageForPromptableSession(ctx, sourceID),
			"the waiting source session must accept the queued prompt")
		fixture.waitForDelivery(t, queuedSwitchPrompt, sourceID)
		destination := fixture.requireDeliveredOnceToDestination(t, queuedSwitchPrompt, sourceID)

		var transferred bool
		for _, call := range fixture.attachments.calls {
			if call.oldSessionID == sourceID && call.newSessionID == destination.ID {
				transferred = true
			}
		}
		require.True(t, transferred, "the queued attachment must move with the message to the destination session")
	})
}

// TestQueuedTurnStartProfileSwitchFailureKeepsMessageQueuedOnSource makes the
// destination promotion fail. The switch rolls back, so the claimed message
// must stay queued on its original session instead of being prompted to the
// old profile or dropped.
func TestQueuedTurnStartProfileSwitchFailureKeepsMessageQueuedOnSource(t *testing.T) {
	forEachWorkflowMode(t, []bool{false, true}, func(t *testing.T, mode queuedTurnStartSwitchMode) {
		ctx := context.Background()
		fixture := newQueuedTurnStartSwitchFixture(t, mode)
		fixture.svc.repo = failProfileSwitchPromotionRepo{
			repoStore: fixture.repo,
			err:       errors.New("promotion unavailable"),
		}
		sourceID := fixture.current.ID
		queued, err := fixture.svc.messageQueue.QueueMessageWithMetadata(
			ctx, sourceID, "t1", queuedSwitchPrompt, "",
			messagequeue.QueuedByUser, false, nil, nil,
		)
		require.NoError(t, err)

		require.True(t, fixture.svc.drainQueuedMessageForPromptableSession(ctx, sourceID))
		require.Eventually(t, func() bool {
			return !fixture.svc.isQueuedDispatchInFlight(sourceID)
		}, 5*time.Second, 10*time.Millisecond)

		require.Empty(t, fixture.promptsContaining(queuedSwitchPrompt),
			"a failed profile switch must not prompt the queued message")
		require.Empty(t, fixture.userMessagesContaining(queuedSwitchPrompt),
			"a failed profile switch must not record the queued message")
		status := fixture.svc.messageQueue.GetStatus(ctx, sourceID)
		require.Equal(t, 1, status.Count, "the message must stay queued on its original session")
		require.Equal(t, queued.ID, status.Entries[0].ID)
		require.Equal(t, queuedSwitchPrompt, status.Entries[0].Content)
		require.Equal(t, sourceID, fixture.primarySession(t).ID, "the source session must stay primary")
	})
}

// TestQueuedPlanCommentTurnStartProfileSwitchDeliversToDestination covers the
// durable plan-comment delivery. Its transcript is written only after
// on_turn_start settles the owning session, so the comment is recorded on and
// prompted to the destination profile session once.
func TestQueuedPlanCommentTurnStartProfileSwitchDeliversToDestination(t *testing.T) {
	// Memory queue only: a reserved plan comment drained through the SQLite
	// FIFO path is not prompted even without a profile switch, which is a
	// separate defect outside this change.
	forEachWorkflowMode(t, []bool{false}, func(t *testing.T, mode queuedTurnStartSwitchMode) {
		ctx := context.Background()
		fixture := newQueuedTurnStartSwitchFixture(t, mode)
		sourceID := fixture.current.ID
		_, err := fixture.svc.messageQueue.QueueMessageWithMetadata(
			ctx, sourceID, "t1", queuedSwitchPlanComment, "",
			messagequeue.QueuedByUser, false, nil,
			map[string]interface{}{
				plancomments.MetadataClientQueueID:      "client-queue-switch",
				plancomments.MetadataRequestFingerprint: "fingerprint-switch",
			},
		)
		require.NoError(t, err)

		require.True(t, fixture.svc.drainQueuedMessageForPromptableSession(ctx, sourceID))
		fixture.waitForDelivery(t, queuedSwitchPlanComment, sourceID)
		fixture.requireDeliveredOnceToDestination(t, queuedSwitchPlanComment, sourceID)
	})
}
