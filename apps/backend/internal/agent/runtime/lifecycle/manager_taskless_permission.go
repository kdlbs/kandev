package lifecycle

import (
	"context"
	"time"

	"go.uber.org/zap"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
)

const tasklessPermissionTimeout = 30 * time.Second

// permissionLogger is the logging surface settleTasklessPermission needs.
type permissionLogger interface {
	Warn(msg string, fields ...zap.Field)
	Error(msg string, fields ...zap.Field)
}

// permissionResponder is the agentctl surface needed to settle a permission
// request without a task session.
type permissionResponder interface {
	ResolvePermission(ctx context.Context, requestID, pendingID, optionID string) (*agentctl.PermissionResolveResponse, error)
	CancelPermission(ctx context.Context, requestID, pendingID string) (*agentctl.PermissionCancelResponse, error)
}

// resolveTasklessPermission settles a permission request raised by a taskless
// run (e.g. an Office routine). Such a run has no task session, so the request
// can neither be published on the per-session permission subject nor shown to
// a user; leaving it to the orchestrator hangs the agent until the run is
// recovered as stale.
func (m *Manager) resolveTasklessPermission(execution *AgentExecution, event agentctl.AgentEvent) {
	client, release := execution.AcquireAgentCtlClient()
	defer release()
	if client == nil {
		m.logger.Warn("taskless permission request has no agentctl client",
			zap.String("execution_id", execution.ID), zap.String("pending_id", event.PendingID))
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), tasklessPermissionTimeout)
	defer cancel()
	settleTasklessPermission(ctx, m.logger, client, execution.ID, event)
}

// settleTasklessPermission confirms an automatic approval that agentctl is
// holding for the backend, and rejects every other request because nobody can
// answer it.
func settleTasklessPermission(
	ctx context.Context, log permissionLogger, responder permissionResponder, executionID string, event agentctl.AgentEvent,
) {
	switch {
	case event.AutoApprovedOptionID != "" && !event.AutoApprovalPending:
		// An older agentctl already delivered its decision; this is only a record.
		return
	case event.AutoApprovedOptionID != "":
		if _, err := responder.ResolvePermission(ctx, event.RequestID, event.PendingID, event.AutoApprovedOptionID); err != nil {
			log.Error("failed to confirm automatic permission for taskless run",
				zap.String("execution_id", executionID), zap.String("pending_id", event.PendingID), zap.Error(err))
		}
	default:
		if _, err := responder.CancelPermission(ctx, event.RequestID, event.PendingID); err != nil {
			log.Error("failed to reject unanswerable permission for taskless run",
				zap.String("execution_id", executionID), zap.String("pending_id", event.PendingID), zap.Error(err))
			return
		}
		log.Warn("rejected permission request for taskless run: no session to answer it",
			zap.String("execution_id", executionID), zap.String("pending_id", event.PendingID),
			zap.String("title", event.PermissionTitle))
	}
}
