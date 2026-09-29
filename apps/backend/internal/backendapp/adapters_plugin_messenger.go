package backendapp

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/orchestrator"
	orchexecutor "github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/plugins"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
	taskservice "github.com/kandev/kandev/internal/task/service"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/kandev/kandev/pkg/pluginsdk"
)

// messengerTaskSvc / messengerOrch are the narrow slices of the task service
// and orchestrator the messenger needs, so its delivery state machine can be
// unit-tested with fakes. *taskservice.Service and *orchestrator.Service
// satisfy them structurally.
type messengerTaskSvc interface {
	GetTask(ctx context.Context, taskID string) (*taskmodels.Task, error)
	GetTaskManagementClaim(ctx context.Context, taskID string) (*taskmodels.TaskManagementClaim, error)
	GetTaskSession(ctx context.Context, sessionID string) (*taskmodels.TaskSession, error)
	GetPrimarySession(ctx context.Context, taskID string) (*taskmodels.TaskSession, error)
	CreateMessage(ctx context.Context, req *taskservice.CreateMessageRequest) (*taskmodels.Message, error)
	CreateMessageIdempotent(ctx context.Context, id string, req *taskservice.CreateMessageRequest) (*taskmodels.Message, error)
	GetMessageWithPromptIndex(ctx context.Context, id string) (*taskmodels.Message, error)
	DeleteMessage(ctx context.Context, id string) error
	WaitForSessionReady(ctx context.Context, sessionID string) error
}

type messengerOrch interface {
	GetMessageQueue() *messagequeue.Service
	PublishQueueStatusEvent(ctx context.Context, sessionID string)
	StartCreatedSession(ctx context.Context, taskID, sessionID, agentProfileID, prompt string, skipMessageRecord, planMode, autoStart bool, attachments []v1.MessageAttachment, references []v1.EntityReference) (*orchexecutor.TaskExecution, error)
	PromptTask(ctx context.Context, taskID, sessionID, prompt, model string, planMode bool, attachments []v1.MessageAttachment, dispatchOnly bool) (*orchestrator.PromptResult, error)
	ResumeTaskSession(ctx context.Context, taskID, sessionID string) (*orchexecutor.TaskExecution, error)
}

type exactMessageAdmissionQueue interface {
	ResolveSessionIdentity(ctx context.Context, taskID, sessionID string) (messagequeue.QueueSessionIdentity, error)
	LifecycleGeneration(ctx context.Context, taskID string) (int64, error)
	TakeQueuedEntryForSession(ctx context.Context, identity messagequeue.QueueSessionIdentity, entryID string) (*messagequeue.QueuedMessage, bool, error)
	QueueMessageWithMetadataForSessionWithClientQueueIDAtWorkflowEntry(
		ctx context.Context,
		identity messagequeue.QueueSessionIdentity,
		entry messagequeue.WorkflowEntryIdentity,
		clientQueueID, content, model, userID string,
		planMode bool,
		attachments []messagequeue.MessageAttachment,
		metadata map[string]interface{},
	) (*messagequeue.QueuedMessage, bool, error)
}

type pluginsPendingTaskTransitionAdapter struct {
	queue *messagequeue.Service
}

func (a pluginsPendingTaskTransitionAdapter) ListPendingTaskTransitions(
	ctx context.Context,
) ([]plugins.PendingTaskTransitionRecord, error) {
	if a.queue == nil {
		return nil, fmt.Errorf("message queue is unavailable")
	}
	records, err := a.queue.ListPendingMoves(ctx)
	if err != nil {
		return nil, fmt.Errorf("list pending workflow moves: %w", err)
	}
	out := make([]plugins.PendingTaskTransitionRecord, 0, len(records))
	for _, record := range records {
		out = append(out, plugins.PendingTaskTransitionRecord{
			ID: record.Move.MoveID, TaskID: record.Move.TaskID, SessionID: record.SessionID,
			WorkflowID: record.Move.WorkflowID, WorkflowStepID: record.Move.WorkflowStepID,
			QueuedAt: record.Move.QueuedAt,
		})
	}
	return out, nil
}

// pluginsTaskMessengerAdapter adapts the task service + orchestrator to the
// plugins package's taskMessenger interface (Host data API SendMessage RPC, ADR
// 0043 phase 2). It delivers a plugin's prompt to a task session through the
// orchestrator's real delivery path — the same behavior message_task uses,
// minus the peer-agent attribution and interrupt/parent semantics: resolve the
// target session (explicit id or the task's primary session), then dispatch by
// state — a running session queues; an idle/completed one is prompted (resuming
// if the agent process is gone); a never-started one is launched with the
// message as its first prompt. Lives here (the composition root) because it
// needs both the task service and the orchestrator, which no single service
// owns; internal/plugins can't reach either without an import cycle.
type pluginsTaskMessengerAdapter struct {
	tasks      messengerTaskSvc
	orch       messengerOrch
	exactQueue exactMessageAdmissionQueue
	log        *logger.Logger
}

func (a pluginsTaskMessengerAdapter) SendMessage(ctx context.Context, taskID, sessionID, text, source string) (plugins.PluginMessageResult, error) {
	session, err := a.resolveSession(ctx, taskID, sessionID)
	if err != nil {
		return plugins.PluginMessageResult{}, err
	}
	if session.State == taskmodels.TaskSessionStateFailed || session.State == taskmodels.TaskSessionStateCancelled {
		return plugins.PluginMessageResult{}, status.Errorf(codes.FailedPrecondition, "session is %s — cannot send message", session.State)
	}
	task, err := a.tasks.GetTask(ctx, taskID)
	if err != nil {
		return plugins.PluginMessageResult{}, err
	}
	if task == nil || task.ID != taskID || task.ArchivedAt != nil {
		return plugins.PluginMessageResult{}, status.Error(codes.FailedPrecondition, "task is no longer active")
	}
	claim, err := a.tasks.GetTaskManagementClaim(ctx, taskID)
	if err != nil {
		return plugins.PluginMessageResult{}, status.Error(codes.Unavailable, "task management claim state is unavailable")
	}
	if claim != nil && claim.OwnerKind != "" {
		return plugins.PluginMessageResult{}, status.Error(codes.Aborted, "task is managed by another owner")
	}
	claimFence := taskmodels.TaskManagementClaimFence{}
	if claim != nil {
		claimFence.Generation = claim.Generation
	}
	operationID, payloadDigest, err := pluginLegacyCommandIdentity("task-message", map[string]string{
		"task_id": taskID, "session_id": session.ID, "content": text, "source": source,
	})
	if err != nil {
		return plugins.PluginMessageResult{}, status.Error(codes.InvalidArgument, "invalid task message")
	}
	queueID, _, err := a.SendMessageExact(ctx, plugins.ExactTaskMessageInput{
		WorkspaceID: task.WorkspaceID, TaskID: taskID, SessionID: session.ID,
		ExpectedTaskResourceVersion:    task.UpdatedAt.UTC().Format(time.RFC3339Nano),
		ExpectedSessionResourceVersion: session.UpdatedAt.UTC().Format(time.RFC3339Nano),
		Content:                        text, Source: source, OperationID: operationID, PayloadDigest: payloadDigest,
		ClaimFence: claimFence,
	})
	if err != nil {
		return plugins.PluginMessageResult{}, err
	}
	if session.State == taskmodels.TaskSessionStateRunning || session.State == taskmodels.TaskSessionStateStarting {
		return plugins.PluginMessageResult{SessionID: session.ID, Status: "queued"}, nil
	}
	queue := a.exactQueue
	if queue == nil {
		queue = a.orch.GetMessageQueue()
	}
	identity, err := queue.ResolveSessionIdentity(ctx, taskID, session.ID)
	if err != nil {
		return plugins.PluginMessageResult{}, mapExactTaskMessageQueueError(err)
	}
	taken, found, err := queue.TakeQueuedEntryForSession(ctx, identity, queueID)
	if err != nil {
		return plugins.PluginMessageResult{}, mapExactTaskMessageQueueError(err)
	}
	if !found || taken == nil {
		return plugins.PluginMessageResult{}, status.Error(codes.Aborted, "task manager changed before message dispatch")
	}
	result, err := a.startOrPromptSession(ctx, taskID, session, text, map[string]interface{}{"source": source})
	if err != nil {
		return plugins.PluginMessageResult{}, err
	}
	a.orch.PublishQueueStatusEvent(context.WithoutCancel(ctx), session.ID)
	return result, nil
}

// resolveSession returns the session a message targets: the explicit session
// (verified to belong to taskID, so a plugin can't reach another task's session
// through a mismatched pair) or the task's primary session.
func (a pluginsTaskMessengerAdapter) resolveSession(ctx context.Context, taskID, sessionID string) (*taskmodels.TaskSession, error) {
	if sessionID != "" {
		session, err := a.tasks.GetTaskSession(ctx, sessionID)
		if err != nil {
			return nil, status.Errorf(codes.NotFound, "session %q not found", sessionID)
		}
		if session.TaskID != taskID {
			return nil, status.Errorf(codes.NotFound, "session %q does not belong to task %q", sessionID, taskID)
		}
		return session, nil
	}
	session, err := a.tasks.GetPrimarySession(ctx, taskID)
	if err != nil {
		if errors.Is(err, tasksqlite.ErrNoPrimarySession) {
			return nil, status.Errorf(codes.NotFound, "task %q has no active session to message", taskID)
		}
		return nil, err
	}
	return session, nil
}

// queueMessage appends the prompt to a running/starting session's FIFO queue
func (a pluginsTaskMessengerAdapter) queueMessage(ctx context.Context, taskID string, session *taskmodels.TaskSession, text string, metadata map[string]interface{}) (plugins.PluginMessageResult, error) {
	queue := a.orch.GetMessageQueue()
	if queue == nil {
		return plugins.PluginMessageResult{}, errors.New("message queue not available")
	}
	if _, err := queue.QueueMessageWithMetadata(ctx, session.ID, taskID, text, "", messagequeue.QueuedByUser, false, nil, metadata); err != nil {
		return plugins.PluginMessageResult{}, fmt.Errorf("failed to queue message: %w", err)
	}
	a.orch.PublishQueueStatusEvent(context.WithoutCancel(ctx), session.ID)
	return plugins.PluginMessageResult{SessionID: session.ID, Status: "queued"}, nil
}

// startOrPromptSession records the user message (so it's tied to the turn the
// launch/prompt produces) then either starts a never-launched session or
// prompts an idle/completed one, deleting the recorded message if dispatch
// fails so a failed send leaves no orphan prompt behind.
func (a pluginsTaskMessengerAdapter) startOrPromptSession(ctx context.Context, taskID string, session *taskmodels.TaskSession, text string, metadata map[string]interface{}) (plugins.PluginMessageResult, error) {
	recorded := a.recordUserMessage(ctx, taskID, session.ID, text, metadata)
	if shouldStartMessagedSession(session) {
		if _, err := a.orch.StartCreatedSession(ctx, taskID, session.ID, session.AgentProfileID, text, true, false, true, nil, nil); err != nil {
			_ = a.deleteRecordedMessage(ctx, recorded)
			return plugins.PluginMessageResult{}, fmt.Errorf("failed to start session: %w", err)
		}
		return plugins.PluginMessageResult{SessionID: session.ID, Status: "started"}, nil
	}
	if err := a.promptWithResume(ctx, taskID, session.ID, text); err != nil {
		_ = a.deleteRecordedMessage(ctx, recorded)
		return plugins.PluginMessageResult{}, err
	}
	return plugins.PluginMessageResult{SessionID: session.ID, Status: "sent"}, nil
}

// StartOrPromptIdempotent is the agent-conversation dispatcher's delivery
// primitive (see internal/task/service's agentConversationDispatcher
// interface): it starts a never-launched session or prompts/resumes an idle
// one, exactly like startOrPromptSession, but records the user message with
// a caller-supplied idempotencyID via CreateMessageIdempotent instead of
// always minting a new one. A durable occurrence claim means a successful
// dispatch never reaches this method twice. An existing message row therefore
// identifies a failed marker compensation; it is removed before retrying the
// runtime delivery. The caller (AgentConversationService) has already
// confirmed the session is not RUNNING/STARTING before calling this — session
// is passed in rather than re-resolved.
func (a pluginsTaskMessengerAdapter) StartOrPromptIdempotent(ctx context.Context, taskID string, session *taskmodels.TaskSession, text, source, idempotencyID string) (string, error) {
	// A committed row for this id means another caller already recorded this
	// occurrence. Normally the durable occurrence claim prevents this path
	// after a successful dispatch. If a prior dispatch failed while compensating
	// its marker, remove that stale marker before retrying the delivery.
	existing, err := a.tasks.GetMessageWithPromptIndex(ctx, idempotencyID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("check existing dispatch message: %w", err)
	}
	if err == nil && existing != nil {
		if err := a.deleteRecordedMessage(ctx, existing); err != nil {
			return "", fmt.Errorf("reconcile failed dispatch message: %w", err)
		}
	}

	metadata := map[string]interface{}{"source": source}
	recorded, err := a.tasks.CreateMessageIdempotent(ctx, idempotencyID, &taskservice.CreateMessageRequest{
		TaskSessionID: session.ID,
		TaskID:        taskID,
		Content:       text,
		AuthorType:    "user",
		Metadata:      metadata,
	})
	if err != nil {
		return "", fmt.Errorf("failed to record idempotent message: %w", err)
	}
	if shouldStartMessagedSession(session) {
		if _, err := a.orch.StartCreatedSession(ctx, taskID, session.ID, session.AgentProfileID, text, true, false, true, nil, nil); err != nil {
			if cleanupErr := a.deleteRecordedMessage(ctx, recorded); cleanupErr != nil {
				return "", fmt.Errorf("failed to start session: %w; failed to remove dispatch marker: %v", err, cleanupErr)
			}
			return "", fmt.Errorf("failed to start session: %w", err)
		}
		return "started", nil
	}
	if err := a.promptWithResume(ctx, taskID, session.ID, text); err != nil {
		var accepted interface{ DetachedResumeAccepted() bool }
		if errors.As(err, &accepted) && accepted.DetachedResumeAccepted() {
			return "sent", nil
		}
		if cleanupErr := a.deleteRecordedMessage(ctx, recorded); cleanupErr != nil {
			return "", fmt.Errorf("%w; failed to remove dispatch marker: %v", err, cleanupErr)
		}
		return "", err
	}
	return "sent", nil
}

// DispatchImmediate rechecks the current session and queue at the effect
// boundary. It never falls back to queue admission: a competing turn returns
// busy, and other dispatch failures remain visible to the caller.
//
//nolint:cyclop // Dispatch validates the exact conversation and task scope before any queue effect.
func (a pluginsTaskMessengerAdapter) DispatchImmediate(
	ctx context.Context,
	taskID string,
	session *taskmodels.TaskSession,
	text, source, idempotencyID string,
) (pluginsdk.ManagedAgentDispatchStatus, error) {
	if session == nil || session.ID == "" || taskID == "" || idempotencyID == "" {
		return "", status.Error(codes.InvalidArgument, "immediate dispatch identity is incomplete")
	}
	current, err := a.tasks.GetTaskSession(ctx, session.ID)
	if err != nil {
		return "", fmt.Errorf("reload session before immediate dispatch: %w", err)
	}
	if current == nil || current.ID != session.ID || current.TaskID != taskID {
		return "", status.Error(codes.Aborted, "immediate dispatch session identity changed")
	}
	if dispatchStatus, stateErr := immediateDispatchSessionState(current); stateErr != nil || dispatchStatus != "" {
		return dispatchStatus, stateErr
	}
	queue := a.orch.GetMessageQueue()
	if queue == nil {
		return "", status.Error(codes.Unavailable, "message queue is unavailable")
	}
	busy, err := immediateDispatchQueueBusy(ctx, queue, taskID, current)
	if err != nil {
		return "", err
	}
	if busy {
		return pluginsdk.ManagedAgentDispatchBusy, nil
	}
	result, err := a.StartOrPromptIdempotent(ctx, taskID, current, text, source, idempotencyID)
	if errors.Is(err, orchestrator.ErrAgentPromptInProgress) || errors.Is(err, orchestrator.ErrSessionNotPromptable) {
		return pluginsdk.ManagedAgentDispatchBusy, nil
	}
	if err != nil {
		return "", err
	}
	switch result {
	case "started":
		return pluginsdk.ManagedAgentDispatchStarted, nil
	case "sent":
		return pluginsdk.ManagedAgentDispatchSent, nil
	default:
		return "", fmt.Errorf("unexpected immediate dispatch result %q", result)
	}
}

func immediateDispatchSessionState(session *taskmodels.TaskSession) (pluginsdk.ManagedAgentDispatchStatus, error) {
	if session.State == taskmodels.TaskSessionStateRunning || session.State == taskmodels.TaskSessionStateStarting {
		return pluginsdk.ManagedAgentDispatchBusy, nil
	}
	if session.State == taskmodels.TaskSessionStateFailed || session.State == taskmodels.TaskSessionStateCancelled {
		return "", status.Errorf(codes.FailedPrecondition, "session is %s", session.State)
	}
	return "", nil
}

func immediateDispatchQueueBusy(ctx context.Context, queue *messagequeue.Service, taskID string, session *taskmodels.TaskSession) (bool, error) {
	identity, err := queue.ResolveSessionIdentity(ctx, taskID, session.ID)
	if err != nil {
		return false, fmt.Errorf("resolve immediate dispatch queue identity: %w", err)
	}
	if identity.TaskID != taskID || identity.SessionID != session.ID ||
		identity.SessionIncarnationID == "" || identity.SessionIncarnationID != session.QueueIncarnationID {
		return false, status.Error(codes.Aborted, "immediate dispatch session identity changed")
	}
	queueStatus, err := queue.Snapshot(ctx, identity)
	if err != nil {
		return false, fmt.Errorf("read immediate dispatch queue: %w", err)
	}
	return queueStatus.Count > 0, nil
}

// promptWithResume dispatches the prompt, resuming the agent process first when
// its execution has gone (e.g. after a backend restart), mirroring the MCP
// message_task path's auto-resume.
//
// Once ResumeTaskSession succeeds the resume is a committed side effect (the
// agent is starting), so the subsequent wait+retry run on a detached context —
// the plugin's incoming deadline must not abort a session start already in
// flight, which would leave the agent awake with no prompt to act on.
func (a pluginsTaskMessengerAdapter) promptWithResume(ctx context.Context, taskID, sessionID, text string) error {
	_, err := a.orch.PromptTask(ctx, taskID, sessionID, text, "", false, nil, true)
	if err == nil {
		return nil
	}
	if !errors.Is(err, orchexecutor.ErrExecutionNotFound) {
		return fmt.Errorf("failed to send prompt: %w", err)
	}
	if _, resumeErr := a.orch.ResumeTaskSession(ctx, taskID, sessionID); resumeErr != nil {
		return fmt.Errorf("failed to resume session: %w", resumeErr)
	}
	resumeCtx := context.WithoutCancel(ctx)
	if waitErr := a.tasks.WaitForSessionReady(resumeCtx, sessionID); waitErr != nil {
		return fmt.Errorf("session not ready after resume: %w", waitErr)
	}
	if _, retryErr := a.orch.PromptTask(resumeCtx, taskID, sessionID, text, "", false, nil, true); retryErr != nil {
		return fmt.Errorf("failed to send prompt after resume: %w", retryErr)
	}
	return nil
}

func (a pluginsTaskMessengerAdapter) recordUserMessage(ctx context.Context, taskID, sessionID, text string, metadata map[string]interface{}) *taskmodels.Message {
	message, err := a.tasks.CreateMessage(ctx, &taskservice.CreateMessageRequest{
		TaskSessionID: sessionID,
		TaskID:        taskID,
		Content:       text,
		AuthorType:    "user",
		Metadata:      metadata,
	})
	if err != nil {
		a.log.Warn("plugins: failed to record user message for SendMessage")
		return nil
	}
	return message
}

func (a pluginsTaskMessengerAdapter) deleteRecordedMessage(ctx context.Context, message *taskmodels.Message) error {
	if message == nil {
		return nil
	}
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	if err := a.tasks.DeleteMessage(cleanupCtx, message.ID); err != nil {
		a.log.Warn("plugins: failed to delete recorded message after failed SendMessage")
		return err
	}
	return nil
}

// shouldStartMessagedSession reports whether a message targets a session that
// has never launched an agent (CREATED, or a WAITING_FOR_INPUT session that
// never bound an execution) and therefore needs the launch path rather than a
// prompt/resume.
func shouldStartMessagedSession(session *taskmodels.TaskSession) bool {
	if session.State == taskmodels.TaskSessionStateCreated {
		return true
	}
	return session.State == taskmodels.TaskSessionStateWaitingForInput && session.AgentExecutionID == ""
}
