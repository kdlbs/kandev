package backendapp

import (
	"context"
	"errors"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/plugins"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type exactMessengerTaskReader interface {
	GetTask(ctx context.Context, taskID string) (*taskmodels.Task, error)
}

//nolint:cyclop,funlen,gocognit // Message admission binds task, claim, approval, and durable queue identity.
func (a pluginsTaskMessengerAdapter) SendMessageExact(
	ctx context.Context,
	in plugins.ExactTaskMessageInput,
) (string, bool, error) {
	if in.WorkspaceID == "" || in.TaskID == "" || in.SessionID == "" || in.OperationID == "" || in.Content == "" {
		return "", false, status.Error(codes.InvalidArgument, "exact task message identity is incomplete")
	}
	reader, ok := a.tasks.(exactMessengerTaskReader)
	if !ok {
		return "", false, status.Error(codes.Unavailable, "exact task message reader is unavailable")
	}
	task, err := reader.GetTask(ctx, in.TaskID)
	if err != nil {
		if errors.Is(err, repository.ErrTaskNotFound) || errors.Is(err, repoerrors.ErrTaskNotFound) {
			return "", false, status.Error(codes.NotFound, "task was not found")
		}
		return "", false, status.Error(codes.Unavailable, "task state is unavailable")
	}
	if task == nil || task.ID != in.TaskID || task.WorkspaceID != in.WorkspaceID {
		return "", false, status.Error(codes.NotFound, "task was not found")
	}
	if task.ArchivedAt != nil {
		return "", false, status.Error(codes.FailedPrecondition, "archived tasks cannot receive messages")
	}
	if !matchesExactResourceVersion(task.UpdatedAt, in.ExpectedTaskResourceVersion) {
		return "", false, status.Error(codes.Aborted, "task resource version changed")
	}

	session, err := a.tasks.GetTaskSession(ctx, in.SessionID)
	if err != nil {
		if errors.Is(err, repository.ErrTaskNotFound) || errors.Is(err, repoerrors.ErrTaskNotFound) {
			return "", false, status.Error(codes.NotFound, "task session was not found")
		}
		return "", false, status.Error(codes.Unavailable, "task session state is unavailable")
	}
	if session == nil || session.ID != in.SessionID || session.TaskID != in.TaskID {
		return "", false, status.Error(codes.NotFound, "task session was not found")
	}
	if session.State == taskmodels.TaskSessionStateFailed || session.State == taskmodels.TaskSessionStateCancelled {
		return "", false, status.Errorf(codes.FailedPrecondition, "session is %s", session.State)
	}
	if !matchesExactResourceVersion(session.UpdatedAt, in.ExpectedSessionResourceVersion) {
		return "", false, status.Error(codes.Aborted, "session resource version changed")
	}
	queue := a.exactQueue
	if queue == nil && a.orch != nil {
		queue = a.orch.GetMessageQueue()
	}
	if queue == nil {
		return "", false, status.Error(codes.Unavailable, "message queue is unavailable")
	}
	identity, err := queue.ResolveSessionIdentity(ctx, in.TaskID, in.SessionID)
	if err != nil {
		return "", false, mapExactTaskMessageQueueError(err)
	}
	if identity.TaskID != in.TaskID || identity.SessionID != in.SessionID || identity.SessionIncarnationID == "" ||
		identity.SessionIncarnationID != session.QueueIncarnationID {
		return "", false, status.Error(codes.Aborted, "task session identity changed")
	}
	generation, err := queue.LifecycleGeneration(ctx, in.TaskID)
	if err != nil {
		return "", false, status.Error(codes.Unavailable, "task lifecycle state is unavailable")
	}
	entry := messagequeue.WorkflowEntryIdentity{
		WorkflowID: task.WorkflowID, WorkflowStepID: task.WorkflowStepID,
		LifecycleGeneration:            generation,
		ExpectedTaskResourceVersion:    in.ExpectedTaskResourceVersion,
		ExpectedSessionResourceVersion: in.ExpectedSessionResourceVersion,
		RejectPendingMove:              true,
		EnforceTaskManagementClaim:     true,
		ManagementInstallationID:       in.ClaimFence.InstallationID,
		ManagementInstanceKey:          in.ClaimFence.InstanceKey,
		ExpectedClaimGeneration:        in.ClaimFence.Generation,
	}
	queued, alreadyApplied, err := queue.QueueMessageWithMetadataForSessionWithClientQueueIDAtWorkflowEntry(
		ctx, identity, entry, in.OperationID, in.Content, "", messagequeue.QueuedByUser, false, nil,
		map[string]interface{}{"source": in.Source},
	)
	if err != nil {
		return "", false, mapExactTaskMessageQueueError(err)
	}
	if queued == nil || queued.ID == "" {
		return "", false, status.Error(codes.Unavailable, "queue did not return an accepted message receipt")
	}
	if a.orch != nil {
		a.orch.PublishQueueStatusEvent(context.WithoutCancel(ctx), session.ID)
	}
	return queued.ID, alreadyApplied, nil
}

func matchesExactResourceVersion(observed time.Time, submitted string) bool {
	if observed.IsZero() || submitted == "" {
		return false
	}
	expected, err := time.Parse(time.RFC3339Nano, submitted)
	return err == nil && observed.Equal(expected)
}

func mapExactTaskMessageQueueError(err error) error {
	switch {
	case errors.Is(err, messagequeue.ErrWorkflowEntryMismatch), errors.Is(err, messagequeue.ErrLifecycleCancelled), errors.Is(err, messagequeue.ErrTaskManagementClaimChanged):
		return status.Error(codes.Aborted, "task changed before the message was accepted")
	case errors.Is(err, messagequeue.ErrQueueIDConflict):
		return status.Error(codes.AlreadyExists, "message idempotency key was used with different content")
	case errors.Is(err, messagequeue.ErrQueueFull):
		return status.Error(codes.ResourceExhausted, "task session message queue is full")
	case errors.Is(err, messagequeue.ErrTaskInactive):
		return status.Error(codes.FailedPrecondition, "task is no longer active")
	case errors.Is(err, messagequeue.ErrSessionIdentityMismatch):
		return status.Error(codes.NotFound, "task session was not found")
	case errors.Is(err, messagequeue.ErrQueueAdmissionUnavailable):
		return status.Error(codes.Unavailable, "durable message admission is unavailable")
	case err != nil:
		return status.Errorf(codes.Unavailable, "message queue admission failed: %v", err)
	default:
		return status.Error(codes.Unavailable, "message queue admission failed")
	}
}
