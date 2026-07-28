package backendapp

import (
	"context"
	"errors"
	"fmt"

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
)

// messengerTaskSvc / messengerOrch are the narrow slices of the task service
// and orchestrator the messenger needs, so its delivery state machine can be
// unit-tested with fakes. *taskservice.Service and *orchestrator.Service
// satisfy them structurally.
type messengerTaskSvc interface {
	GetTaskSession(ctx context.Context, sessionID string) (*taskmodels.TaskSession, error)
	GetPrimarySession(ctx context.Context, taskID string) (*taskmodels.TaskSession, error)
	CreateMessage(ctx context.Context, req *taskservice.CreateMessageRequest) (*taskmodels.Message, error)
	DeleteMessage(ctx context.Context, id string) error
	WaitForSessionReady(ctx context.Context, sessionID string) error
}

type messengerOrch interface {
	GetMessageQueue() *messagequeue.Service
	StartCreatedSession(ctx context.Context, taskID, sessionID, agentProfileID, prompt string, skipMessageRecord, planMode, autoStart bool, attachments []v1.MessageAttachment, references []v1.EntityReference) (*orchexecutor.TaskExecution, error)
	PromptTask(ctx context.Context, taskID, sessionID, prompt, model string, planMode bool, attachments []v1.MessageAttachment, dispatchOnly bool) (*orchestrator.PromptResult, error)
	ResumeTaskSession(ctx context.Context, taskID, sessionID string) (*orchexecutor.TaskExecution, error)
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
	tasks messengerTaskSvc
	orch  messengerOrch
	log   *logger.Logger
}

func (a pluginsTaskMessengerAdapter) SendMessage(ctx context.Context, taskID, sessionID, text, source string) (plugins.PluginMessageResult, error) {
	session, err := a.resolveSession(ctx, taskID, sessionID)
	if err != nil {
		return plugins.PluginMessageResult{}, err
	}
	metadata := map[string]interface{}{"source": source}
	switch session.State {
	case taskmodels.TaskSessionStateFailed, taskmodels.TaskSessionStateCancelled:
		return plugins.PluginMessageResult{}, status.Errorf(codes.FailedPrecondition, "session is %s — cannot send message", session.State)
	case taskmodels.TaskSessionStateRunning, taskmodels.TaskSessionStateStarting:
		return a.queueMessage(ctx, taskID, session, text, metadata)
	default:
		return a.startOrPromptSession(ctx, taskID, session, text, metadata)
	}
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
// for delivery at its next turn boundary.
func (a pluginsTaskMessengerAdapter) queueMessage(ctx context.Context, taskID string, session *taskmodels.TaskSession, text string, metadata map[string]interface{}) (plugins.PluginMessageResult, error) {
	queue := a.orch.GetMessageQueue()
	if queue == nil {
		return plugins.PluginMessageResult{}, errors.New("message queue not available")
	}
	if _, err := queue.QueueMessageWithMetadata(ctx, session.ID, taskID, text, "", messagequeue.QueuedByUser, false, nil, metadata); err != nil {
		return plugins.PluginMessageResult{}, fmt.Errorf("failed to queue message: %w", err)
	}
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
			a.deleteRecordedMessage(ctx, recorded)
			return plugins.PluginMessageResult{}, fmt.Errorf("failed to start session: %w", err)
		}
		return plugins.PluginMessageResult{SessionID: session.ID, Status: "started"}, nil
	}
	if err := a.promptWithResume(ctx, taskID, session.ID, text); err != nil {
		a.deleteRecordedMessage(ctx, recorded)
		return plugins.PluginMessageResult{}, err
	}
	return plugins.PluginMessageResult{SessionID: session.ID, Status: "sent"}, nil
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

func (a pluginsTaskMessengerAdapter) deleteRecordedMessage(ctx context.Context, message *taskmodels.Message) {
	if message == nil {
		return
	}
	if err := a.tasks.DeleteMessage(ctx, message.ID); err != nil {
		a.log.Warn("plugins: failed to delete recorded message after failed SendMessage")
	}
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
