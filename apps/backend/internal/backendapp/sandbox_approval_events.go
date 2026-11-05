package backendapp

import (
	"context"
	"errors"
	"time"

	"github.com/kandev/kandev/internal/agent/sandbox"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	taskrepo "github.com/kandev/kandev/internal/task/repository/sqlite"
)

type sandboxApprovalAttemptReader interface {
	ReadAttempt(context.Context, string, string) (*sandbox.Attempt, error)
}

type sandboxApprovalNotifier struct {
	tasks    *taskrepo.Repository
	policies sandboxApprovalAttemptReader
	eventBus bus.EventBus
}

func configureSandboxApprovalEvents(policies *sandbox.Service, tasks *taskrepo.Repository, eventBus bus.EventBus, log *logger.Logger) {
	notifier := sandboxApprovalNotifier{tasks: tasks, policies: policies, eventBus: eventBus}
	policies.SetApprovalChangeNotifier(func(parent context.Context, orgID, attemptID string) {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(parent), 2*time.Second)
		defer cancel()
		if err := notifier.publish(ctx, orgID, attemptID); err != nil {
			log.Warn("Sandbox approval notification failed")
		}
	})
}

func (n sandboxApprovalNotifier) publish(ctx context.Context, orgID, attemptID string) error {
	attempt, err := n.policies.ReadAttempt(ctx, orgID, attemptID)
	if err != nil {
		return err
	}
	if attempt == nil || attempt.Snapshot.SessionID == "" {
		return nil
	}
	session, err := n.tasks.GetTaskSession(ctx, attempt.Snapshot.SessionID)
	if err != nil {
		return err
	}
	if session.TaskID != attempt.Snapshot.TaskID {
		return errors.New("sandbox approval session does not match its attempt")
	}
	task, err := n.tasks.GetTask(ctx, session.TaskID)
	if err != nil {
		return err
	}
	workspace, err := n.tasks.GetWorkspace(ctx, task.WorkspaceID)
	if err != nil {
		return err
	}
	if workspace.OrgID != orgID {
		return sandbox.ErrAuthority
	}
	payload := map[string]any{taskIDPayloadKey: task.ID, sessionIDPayloadKey: session.ID, workspaceIDPayloadKey: task.WorkspaceID, queueIncarnationPayloadKey: session.QueueIncarnationID}
	return n.eventBus.Publish(ctx, events.TaskSessionSandboxApprovalsChanged,
		bus.NewEvent(events.TaskSessionSandboxApprovalsChanged, "sandbox", payload))
}
