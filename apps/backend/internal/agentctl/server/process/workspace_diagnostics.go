package process

import (
	"context"

	"github.com/kandev/kandev/internal/common/fsdiagnostics"
)

type workspaceTriggerContextKey struct{}

const (
	workspaceManualRefreshTrigger = "manual_refresh"
	workspaceUserSelectTrigger    = "user_select"
)

// NormalizeWorkspaceTrigger keeps refresh diagnostics within the set of
// lifecycle and user-operation causes understood by the tracker.
func NormalizeWorkspaceTrigger(trigger string) string {
	switch trigger {
	case workspaceManualRefreshTrigger, workspaceUserSelectTrigger, "turn_complete", "startup_grace":
		return trigger
	default:
		return workspaceManualRefreshTrigger
	}
}

func withWorkspaceTrigger(ctx context.Context, trigger string) context.Context {
	return context.WithValue(ctx, workspaceTriggerContextKey{}, trigger)
}

func workspaceTrigger(ctx context.Context, fallback string) string {
	if trigger, ok := ctx.Value(workspaceTriggerContextKey{}).(string); ok && trigger != "" {
		return trigger
	}
	return fallback
}

// SetDiagnosticIdentity supplies the task and session that own this tracker.
// The workspace path remains the canonical operation target because agentctl
// instances do not receive a separate workspace UUID.
func (wt *WorkspaceTracker) SetDiagnosticIdentity(taskID, sessionID string) {
	wt.diagnosticMu.Lock()
	wt.diagnosticTaskID = taskID
	wt.diagnosticSessionID = sessionID
	wt.diagnosticMu.Unlock()
}

func (wt *WorkspaceTracker) diagnosticContext(operation, trigger string) fsdiagnostics.Context {
	wt.diagnosticMu.RLock()
	taskID := wt.diagnosticTaskID
	sessionID := wt.diagnosticSessionID
	wt.diagnosticMu.RUnlock()
	return fsdiagnostics.Context{
		Operation: operation,
		Target:    wt.workDir,
		Trigger:   trigger,
		Runtime:   wt.runtimeMode,
		TaskID:    taskID,
		SessionID: sessionID,
		PollMode:  string(wt.GetPollMode()),
	}
}

func (wt *WorkspaceTracker) recordFilesystemFailure(operation, trigger string, err error) {
	if wt.logger == nil || err == nil {
		return
	}
	operationContext := wt.diagnosticContext(operation, trigger)
	if fsdiagnostics.IsAccessDenied(err) {
		wt.filesystemWarnings.Warn(wt.logger.Zap(), "filesystem.access_denied", operationContext, err)
		wt.pauseAfterAccessDenied(operationContext, err)
		return
	}
	wt.logger.Warn("filesystem operation failed", operationContext.Fields(err)...)
}

func (wt *WorkspaceTracker) pauseAfterAccessDenied(operationContext fsdiagnostics.Context, err error) {
	if !wt.accessDenied.CompareAndSwap(false, true) {
		return
	}
	wt.pollModeMu.Lock()
	previous := wt.pollMode
	wt.pollMode = PollModePaused
	wt.pollModeMu.Unlock()
	if previous != PollModePaused {
		wt.wakePollLoops()
	}
	wt.logger.Warn("workspace.poll_paused_after_denial", operationContext.Fields(err)...)
}

func (wt *WorkspaceTracker) clearAccessDeniedForUserOperation(trigger string) {
	if trigger == workspaceManualRefreshTrigger || trigger == workspaceUserSelectTrigger {
		wt.accessDenied.Store(false)
	}
}
