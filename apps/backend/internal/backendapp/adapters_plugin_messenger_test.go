package backendapp

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/orchestrator"
	orchexecutor "github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
	taskservice "github.com/kandev/kandev/internal/task/service"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

type fakeMessengerTaskSvc struct {
	primary    *taskmodels.TaskSession
	primaryErr error
	byID       map[string]*taskmodels.TaskSession
	created    *taskmodels.Message
	deleted    []string
	waitErr    error
}

func (f *fakeMessengerTaskSvc) GetTaskSession(_ context.Context, id string) (*taskmodels.TaskSession, error) {
	s, ok := f.byID[id]
	if !ok {
		return nil, errors.New("session not found")
	}
	return s, nil
}

func (f *fakeMessengerTaskSvc) GetPrimarySession(_ context.Context, _ string) (*taskmodels.TaskSession, error) {
	return f.primary, f.primaryErr
}

func (f *fakeMessengerTaskSvc) CreateMessage(_ context.Context, req *taskservice.CreateMessageRequest) (*taskmodels.Message, error) {
	f.created = &taskmodels.Message{ID: "msg-1", TaskSessionID: req.TaskSessionID, TaskID: req.TaskID, Content: req.Content}
	return f.created, nil
}

func (f *fakeMessengerTaskSvc) DeleteMessage(_ context.Context, id string) error {
	f.deleted = append(f.deleted, id)
	return nil
}

func (f *fakeMessengerTaskSvc) WaitForSessionReady(_ context.Context, _ string) error {
	return f.waitErr
}

type fakeMessengerOrch struct {
	queue               *messagequeue.Service
	startCalls          int
	promptCalls         int
	resumeCalls         int
	promptErr           error
	promptFailFirstOnly bool
}

func (f *fakeMessengerOrch) GetMessageQueue() *messagequeue.Service { return f.queue }

func (f *fakeMessengerOrch) StartCreatedSession(_ context.Context, _, _, _, _ string, _, _, _ bool, _ []v1.MessageAttachment, _ []v1.EntityReference) (*orchexecutor.TaskExecution, error) {
	f.startCalls++
	return &orchexecutor.TaskExecution{}, nil
}

func (f *fakeMessengerOrch) PromptTask(_ context.Context, _, _, _, _ string, _ bool, _ []v1.MessageAttachment, _ bool) (*orchestrator.PromptResult, error) {
	f.promptCalls++
	retriedAfterResume := f.promptFailFirstOnly && f.promptCalls > 1
	if f.promptErr != nil && !retriedAfterResume {
		return nil, f.promptErr
	}
	return &orchestrator.PromptResult{}, nil
}

func (f *fakeMessengerOrch) ResumeTaskSession(_ context.Context, _, _ string) (*orchexecutor.TaskExecution, error) {
	f.resumeCalls++
	return &orchexecutor.TaskExecution{}, nil
}

func newMessengerAdapter(t *testing.T, tasks *fakeMessengerTaskSvc, orch *fakeMessengerOrch) pluginsTaskMessengerAdapter {
	t.Helper()
	log, err := logger.NewFromZap(zap.NewNop())
	require.NoError(t, err)
	if orch.queue == nil {
		orch.queue = messagequeue.NewServiceMemory(log)
	}
	return pluginsTaskMessengerAdapter{tasks: tasks, orch: orch, log: log}
}

func TestPluginsMessenger_RunningSessionQueues(t *testing.T) {
	tasks := &fakeMessengerTaskSvc{primary: &taskmodels.TaskSession{ID: "s1", TaskID: "t1", State: taskmodels.TaskSessionStateRunning}}
	orch := &fakeMessengerOrch{}
	a := newMessengerAdapter(t, tasks, orch)

	res, err := a.SendMessage(context.Background(), "t1", "", "do the thing", "plugin:p")
	require.NoError(t, err)
	require.Equal(t, "s1", res.SessionID)
	require.Equal(t, "queued", res.Status)
	require.Equal(t, 1, orch.queue.GetStatus(context.Background(), "s1").Count, "message should be enqueued")
	require.Nil(t, tasks.created, "queued path records via the queue, not CreateMessage")
	require.Zero(t, orch.startCalls+orch.promptCalls)
}

func TestPluginsMessenger_CreatedSessionStarts(t *testing.T) {
	tasks := &fakeMessengerTaskSvc{primary: &taskmodels.TaskSession{ID: "s1", TaskID: "t1", State: taskmodels.TaskSessionStateCreated, AgentProfileID: "prof-1"}}
	orch := &fakeMessengerOrch{}
	a := newMessengerAdapter(t, tasks, orch)

	res, err := a.SendMessage(context.Background(), "t1", "", "kick off", "plugin:p")
	require.NoError(t, err)
	require.Equal(t, "started", res.Status)
	require.Equal(t, 1, orch.startCalls)
	require.NotNil(t, tasks.created, "the user message is recorded so it's tied to the launched turn")
	require.Empty(t, tasks.deleted)
}

func TestPluginsMessenger_WaitingSessionPrompts(t *testing.T) {
	tasks := &fakeMessengerTaskSvc{primary: &taskmodels.TaskSession{ID: "s1", TaskID: "t1", State: taskmodels.TaskSessionStateWaitingForInput, AgentExecutionID: "exec-1"}}
	orch := &fakeMessengerOrch{}
	a := newMessengerAdapter(t, tasks, orch)

	res, err := a.SendMessage(context.Background(), "t1", "", "follow up", "plugin:p")
	require.NoError(t, err)
	require.Equal(t, "sent", res.Status)
	require.Equal(t, 1, orch.promptCalls)
	require.Zero(t, orch.startCalls)
	require.Empty(t, tasks.deleted)
}

func TestPluginsMessenger_PromptResumesWhenExecutionGone(t *testing.T) {
	tasks := &fakeMessengerTaskSvc{primary: &taskmodels.TaskSession{ID: "s1", TaskID: "t1", State: taskmodels.TaskSessionStateWaitingForInput, AgentExecutionID: "exec-1"}}
	orch := &fakeMessengerOrch{promptErr: orchexecutor.ErrExecutionNotFound, promptFailFirstOnly: true}
	a := newMessengerAdapter(t, tasks, orch)

	res, err := a.SendMessage(context.Background(), "t1", "", "wake up", "plugin:p")
	require.NoError(t, err)
	require.Equal(t, "sent", res.Status)
	require.Equal(t, 1, orch.resumeCalls, "a missing execution triggers a resume")
	require.Equal(t, 2, orch.promptCalls, "prompt is retried after resume")
	require.Empty(t, tasks.deleted)
}

// TestPluginsMessenger_ResumeWaitTimeoutDeletesMessage pins the explicit
// choice: when the agent execution is gone, resume succeeds, but the session
// isn't ready in time (WaitForSessionReady errors), the send fails and the
// recorded user message is deleted — so a plugin retry doesn't stack a
// duplicate prompt (queueing is not idempotent). The retry prompt is never
// reached.
func TestPluginsMessenger_ResumeWaitTimeoutDeletesMessage(t *testing.T) {
	tasks := &fakeMessengerTaskSvc{
		primary: &taskmodels.TaskSession{ID: "s1", TaskID: "t1", State: taskmodels.TaskSessionStateWaitingForInput, AgentExecutionID: "exec-1"},
		waitErr: errors.New("session not ready in time"),
	}
	orch := &fakeMessengerOrch{promptErr: orchexecutor.ErrExecutionNotFound}
	a := newMessengerAdapter(t, tasks, orch)

	_, err := a.SendMessage(context.Background(), "t1", "", "wake up", "plugin:p")
	require.Error(t, err)
	require.Equal(t, 1, orch.resumeCalls, "resume is attempted")
	require.Equal(t, 1, orch.promptCalls, "the retry prompt is not reached after a wait timeout")
	require.Equal(t, []string{"msg-1"}, tasks.deleted, "the recorded message is removed so a retry can't duplicate it")
}

func TestPluginsMessenger_PromptFailureDeletesRecordedMessage(t *testing.T) {
	tasks := &fakeMessengerTaskSvc{primary: &taskmodels.TaskSession{ID: "s1", TaskID: "t1", State: taskmodels.TaskSessionStateWaitingForInput, AgentExecutionID: "exec-1"}}
	orch := &fakeMessengerOrch{promptErr: errors.New("dispatch boom")}
	a := newMessengerAdapter(t, tasks, orch)

	_, err := a.SendMessage(context.Background(), "t1", "", "nope", "plugin:p")
	require.Error(t, err)
	require.Equal(t, []string{"msg-1"}, tasks.deleted, "a failed dispatch must not leave an orphan message")
}

func TestPluginsMessenger_ExplicitSessionMustBelongToTask(t *testing.T) {
	tasks := &fakeMessengerTaskSvc{byID: map[string]*taskmodels.TaskSession{
		"s1": {ID: "s1", TaskID: "other-task", State: taskmodels.TaskSessionStateRunning},
	}}
	orch := &fakeMessengerOrch{}
	a := newMessengerAdapter(t, tasks, orch)

	_, err := a.SendMessage(context.Background(), "t1", "s1", "sneaky", "plugin:p")
	require.Equal(t, codes.NotFound, status.Code(err), "a session from another task must not be reachable via a mismatched pair")
}

func TestPluginsMessenger_NoPrimarySessionIsNotFound(t *testing.T) {
	tasks := &fakeMessengerTaskSvc{primaryErr: tasksqlite.ErrNoPrimarySession}
	orch := &fakeMessengerOrch{}
	a := newMessengerAdapter(t, tasks, orch)

	_, err := a.SendMessage(context.Background(), "t1", "", "hello", "plugin:p")
	require.Equal(t, codes.NotFound, status.Code(err))
}

func TestPluginsMessenger_TerminalSessionRejected(t *testing.T) {
	tasks := &fakeMessengerTaskSvc{primary: &taskmodels.TaskSession{ID: "s1", TaskID: "t1", State: taskmodels.TaskSessionStateFailed}}
	orch := &fakeMessengerOrch{}
	a := newMessengerAdapter(t, tasks, orch)

	_, err := a.SendMessage(context.Background(), "t1", "", "hello", "plugin:p")
	require.Equal(t, codes.FailedPrecondition, status.Code(err))
}
