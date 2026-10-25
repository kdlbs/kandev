package backendapp

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/plugins"
	taskmodels "github.com/kandev/kandev/internal/task/models"
)

type exactMessengerTaskSvc struct {
	*fakeMessengerTaskSvc
	task *taskmodels.Task
}

func (f *exactMessengerTaskSvc) GetTask(context.Context, string) (*taskmodels.Task, error) {
	return f.task, nil
}

type exactMessageQueueFixture struct {
	identity   messagequeue.QueueSessionIdentity
	generation int64
	message    *messagequeue.QueuedMessage
	replayed   bool
	err        error
	entry      messagequeue.WorkflowEntryIdentity
	queueID    string
	calls      int
	takeCalls  int
}

func (f *exactMessageQueueFixture) ResolveSessionIdentity(context.Context, string, string) (messagequeue.QueueSessionIdentity, error) {
	return f.identity, nil
}

func (f *exactMessageQueueFixture) LifecycleGeneration(context.Context, string) (int64, error) {
	return f.generation, nil
}

func (f *exactMessageQueueFixture) TakeQueuedEntryForSession(context.Context, messagequeue.QueueSessionIdentity, string) (*messagequeue.QueuedMessage, bool, error) {
	f.takeCalls++
	return f.message, f.message != nil, nil
}

func (f *exactMessageQueueFixture) QueueMessageWithMetadataForSessionWithClientQueueIDAtWorkflowEntry(
	_ context.Context,
	identity messagequeue.QueueSessionIdentity,
	entry messagequeue.WorkflowEntryIdentity,
	clientQueueID, content, _, source string,
	_ bool,
	_ []messagequeue.MessageAttachment,
	metadata map[string]interface{},
) (*messagequeue.QueuedMessage, bool, error) {
	f.calls++
	f.entry = entry
	f.queueID = clientQueueID
	if f.err != nil {
		return nil, false, f.err
	}
	return &messagequeue.QueuedMessage{
		ID: clientQueueID, TaskID: identity.TaskID, SessionID: identity.SessionID,
		Content: content, Metadata: metadata,
	}, f.replayed, nil
}

func TestPluginsMessenger_ExactMessageUsesDurableVersionFencedQueueAdmission(t *testing.T) {
	now := time.Date(2026, 9, 26, 11, 0, 0, 0, time.UTC)
	version := now.Format(time.RFC3339Nano)
	tasks := &exactMessengerTaskSvc{
		fakeMessengerTaskSvc: &fakeMessengerTaskSvc{byID: map[string]*taskmodels.TaskSession{
			"session-exact": {ID: "session-exact", TaskID: "task-exact", QueueIncarnationID: "incarnation-exact", State: taskmodels.TaskSessionStateWaitingForInput, UpdatedAt: now},
		}},
		task: &taskmodels.Task{
			ID: "task-exact", WorkspaceID: "workspace-exact", WorkflowID: "workflow-exact",
			WorkflowStepID: "step-exact", UpdatedAt: now,
		},
	}
	queue := &exactMessageQueueFixture{
		identity:   messagequeue.QueueSessionIdentity{TaskID: "task-exact", SessionID: "session-exact", SessionIncarnationID: "incarnation-exact"},
		generation: 4, replayed: true,
	}
	orch := &fakeMessengerOrch{}
	a := newMessengerAdapter(t, tasks.fakeMessengerTaskSvc, orch)
	a.tasks = tasks
	a.exactQueue = queue
	tasks.byID["session-exact"].QueueIncarnationID = queue.identity.SessionIncarnationID

	queueID, alreadyApplied, err := a.SendMessageExact(context.Background(), plugins.ExactTaskMessageInput{
		WorkspaceID: "workspace-exact", TaskID: "task-exact", SessionID: "session-exact",
		ExpectedTaskResourceVersion: version, ExpectedSessionResourceVersion: version,
		Content: "Continue with the verified implementation.", Source: "plugin:coordinator",
		OperationID: "operation-exact-message", PayloadDigest: "sha256:payload",
	})
	require.NoError(t, err)
	require.True(t, alreadyApplied)
	require.Equal(t, "operation-exact-message", queueID)
	require.Equal(t, 1, queue.calls)
	require.Equal(t, queueID, queue.queueID)
	require.Equal(t, version, queue.entry.ExpectedTaskResourceVersion)
	require.Equal(t, version, queue.entry.ExpectedSessionResourceVersion)
	require.Equal(t, int64(4), queue.entry.LifecycleGeneration)
	require.True(t, queue.entry.RejectPendingMove)
	require.Equal(t, 1, orch.queueStatusCalls)
}

func TestPluginsMessenger_ExactMessageRejectsStaleResourceVersion(t *testing.T) {
	now := time.Date(2026, 9, 26, 11, 0, 0, 0, time.UTC)
	tasks := &exactMessengerTaskSvc{
		fakeMessengerTaskSvc: &fakeMessengerTaskSvc{byID: map[string]*taskmodels.TaskSession{
			"session-exact": {ID: "session-exact", TaskID: "task-exact", QueueIncarnationID: "incarnation-exact", State: taskmodels.TaskSessionStateWaitingForInput, UpdatedAt: now},
		}},
		task: &taskmodels.Task{ID: "task-exact", WorkspaceID: "workspace-exact", UpdatedAt: now},
	}
	queue := &exactMessageQueueFixture{identity: messagequeue.QueueSessionIdentity{
		TaskID: "task-exact", SessionID: "session-exact", SessionIncarnationID: "incarnation-exact",
	}}
	a := newMessengerAdapter(t, tasks.fakeMessengerTaskSvc, &fakeMessengerOrch{})
	a.tasks = tasks
	a.exactQueue = queue
	tasks.byID["session-exact"].QueueIncarnationID = queue.identity.SessionIncarnationID

	_, _, err := a.SendMessageExact(context.Background(), plugins.ExactTaskMessageInput{
		WorkspaceID: "workspace-exact", TaskID: "task-exact", SessionID: "session-exact",
		ExpectedTaskResourceVersion: "2026-09-26T10:59:00Z", ExpectedSessionResourceVersion: now.Format(time.RFC3339Nano),
		Content: "stale", Source: "plugin:coordinator", OperationID: "operation-stale",
	})
	require.Equal(t, codes.Aborted, status.Code(err))
	require.Zero(t, queue.calls, "stale commands must not enter queue admission")
}

func TestPluginsMessenger_ExactMessageMapsQueueConflicts(t *testing.T) {
	now := time.Date(2026, 9, 26, 11, 0, 0, 0, time.UTC)
	tasks := &exactMessengerTaskSvc{
		fakeMessengerTaskSvc: &fakeMessengerTaskSvc{byID: map[string]*taskmodels.TaskSession{
			"session-exact": {ID: "session-exact", TaskID: "task-exact", QueueIncarnationID: "incarnation-exact", State: taskmodels.TaskSessionStateWaitingForInput, UpdatedAt: now},
		}},
		task: &taskmodels.Task{ID: "task-exact", WorkspaceID: "workspace-exact", UpdatedAt: now},
	}
	queue := &exactMessageQueueFixture{
		identity: messagequeue.QueueSessionIdentity{TaskID: "task-exact", SessionID: "session-exact", SessionIncarnationID: "incarnation-exact"},
		err:      messagequeue.ErrWorkflowEntryMismatch,
	}
	a := newMessengerAdapter(t, tasks.fakeMessengerTaskSvc, &fakeMessengerOrch{})
	a.tasks = tasks
	a.exactQueue = queue
	tasks.byID["session-exact"].QueueIncarnationID = queue.identity.SessionIncarnationID

	_, _, err := a.SendMessageExact(context.Background(), plugins.ExactTaskMessageInput{
		WorkspaceID: "workspace-exact", TaskID: "task-exact", SessionID: "session-exact",
		ExpectedTaskResourceVersion: now.Format(time.RFC3339Nano), ExpectedSessionResourceVersion: now.Format(time.RFC3339Nano),
		Content: "pending transition", Source: "plugin:coordinator", OperationID: "operation-pending",
	})
	require.Equal(t, codes.Aborted, status.Code(err))
}
