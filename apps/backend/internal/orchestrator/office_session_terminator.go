package orchestrator

import (
	"context"
	"fmt"

	"github.com/kandev/kandev/internal/task/models"
	"go.uber.org/zap"
)

// officeSessionTerminator implements office/dashboard.SessionTerminator and
// office/agents.SessionTerminator on top of the orchestrator's session repo.
// Flipping a session row to COMPLETED is the persistent-row counterpart of
// the reactivity pipeline's hard-cancel: it removes the row from "live"
// indicators and forces the next EnsureSessionForAgent to create a fresh row
// rather than reusing this one.
type officeSessionTerminator struct {
	svc *Service
}

// newOfficeSessionTerminator wires a Service-bound terminator. The returned
// value satisfies the SessionTerminator interfaces defined in the dashboard
// and agents packages without those packages importing orchestrator.
func newOfficeSessionTerminator(svc *Service) *officeSessionTerminator {
	return &officeSessionTerminator{svc: svc}
}

// TerminateOfficeSession flips the (task, agent) office session row to
// COMPLETED. Idempotent: skips when the row is missing or already terminal,
// or when the inputs are empty (which never indicates a bug — the caller
// just wants the no-op convenience).
func (t *officeSessionTerminator) TerminateOfficeSession(
	ctx context.Context, taskID, agentInstanceID, reason string,
) error {
	if taskID == "" || agentInstanceID == "" {
		return nil
	}
	session, err := t.svc.repo.GetTaskSessionByTaskAndAgent(ctx, taskID, agentInstanceID)
	if err != nil {
		return fmt.Errorf("lookup (task,agent) session: %w", err)
	}
	if session == nil || isTerminalSessionState(session.State) {
		return nil
	}
	t.svc.logger.Info("terminating office session row",
		zap.String("task_id", taskID),
		zap.String("session_id", session.ID),
		zap.String("agent_profile_id", agentInstanceID),
		zap.String("reason", reason),
		zap.String("from_state", string(session.State)))
	t.svc.updateTaskSessionState(ctx, taskID, session.ID, models.TaskSessionStateCompleted, reason, false, session)
	return nil
}

// TerminateAllForAgent cascades termination across every live session
// belonging to agentInstanceID (every task it participates on). Idempotent.
// Used by the agent-instance deletion path.
func (t *officeSessionTerminator) TerminateAllForAgent(
	ctx context.Context, agentInstanceID, reason string,
) error {
	if agentInstanceID == "" {
		return nil
	}
	sessions, err := t.svc.repo.ListNonTerminalSessionsByAgentInstance(ctx, agentInstanceID)
	if err != nil {
		return fmt.Errorf("list live sessions for agent: %w", err)
	}
	for _, sess := range sessions {
		t.svc.logger.Info("terminating office session row (agent deletion cascade)",
			zap.String("session_id", sess.ID),
			zap.String("task_id", sess.TaskID),
			zap.String("agent_profile_id", agentInstanceID),
			zap.String("from_state", string(sess.State)))
		t.svc.updateTaskSessionState(ctx, sess.TaskID, sess.ID, models.TaskSessionStateCompleted, reason, false, sess)
	}
	return nil
}

// OfficeSessionTerminator returns a SessionTerminator backed by the Service's
// session repo. Exposed so cmd/kandev can wire it into both the dashboard
// service and the agents service without leaking orchestrator internals.
func (s *Service) OfficeSessionTerminator() *officeSessionTerminator {
	return newOfficeSessionTerminator(s)
}
