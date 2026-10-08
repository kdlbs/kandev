package lifecycle

import (
	"context"
	"errors"
	"time"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"go.uber.org/zap"
)

const turnChangeTerminalCaptureTimeout = 20 * time.Second
const turnChangeCancelRequestTimeout = 2 * time.Second

func (m *Manager) admitTurnChangeCapture(ctx context.Context, execution *AgentExecution, generation uint64) error {
	if m == nil || m.turnChangeCaptureHandler == nil || !eligibleTurnChangeExecution(execution) || generation == 0 {
		return nil
	}
	// Launch preparation can finish before its environment checkout inventory
	// is persisted. Refresh at generation admission, after launch persistence,
	// so the immutable start is bound to the checkout the executor will use.
	m.populateLaunchTurnChangeCheckouts(ctx, execution)
	turnID := execution.promptTurnIDForGeneration(generation)
	if turnID == "" {
		return nil
	}
	client, release := execution.AcquireAgentCtlClient()
	defer release()
	return m.turnChangeCaptureHandler.AdmitTurnChanges(ctx, turnChangeAdmission(execution, generation, turnID), client)
}

func (m *Manager) populateLaunchTurnChangeCheckouts(ctx context.Context, execution *AgentExecution) {
	if m == nil || m.workspaceInfoProvider == nil || !eligibleTurnChangeExecution(execution) {
		return
	}
	info, err := m.workspaceInfoProvider.GetWorkspaceInfoForSession(ctx, execution.TaskID, execution.SessionID)
	if err != nil {
		m.logger.Warn("failed to load task checkout manifest for turn-change capture",
			zap.String("task_id", execution.TaskID), zap.String("session_id", execution.SessionID), zap.Error(err))
		return
	}
	if info == nil || info.TaskEnvironmentID != execution.TaskEnvironmentID {
		m.logger.Warn("task checkout manifest does not match execution environment for turn-change capture",
			zap.String("task_id", execution.TaskID), zap.String("session_id", execution.SessionID),
			zap.String("task_environment_id", execution.TaskEnvironmentID))
		return
	}
	execution.TurnChangeCheckouts = turnChangeCheckoutsFromWorkspaceRepositories(info.WorkspaceRepositories)
}

func (m *Manager) failTurnChangeDispatch(ctx context.Context, execution *AgentExecution, generation uint64, dispatchErr error) {
	if m == nil || m.turnChangeCaptureHandler == nil || !eligibleTurnChangeExecution(execution) || generation == 0 {
		return
	}
	turnID := execution.promptTurnIDForGeneration(generation)
	if turnID == "" {
		return
	}
	terminal := TurnChangeTerminal{
		TurnChangeAdmission: turnChangeAdmission(execution, generation, turnID),
		At:                  time.Now().UTC(),
		Outcome:             "dispatch_error",
	}
	if dispatchErr != nil {
		terminal.Outcome = "dispatch_error: " + dispatchErr.Error()
	}
	ctx, cancel := context.WithTimeout(ctx, turnChangeTerminalCaptureTimeout)
	defer cancel()
	client, release := execution.AcquireAgentCtlClient()
	defer release()
	if err := m.turnChangeCaptureHandler.FinishTurnChanges(ctx, terminal, client); err != nil {
		m.logger.Warn("failed to finalize turn-change capture after prompt dispatch error",
			zap.String("execution_id", execution.ID), zap.Uint64("prompt_generation", generation), zap.Error(err))
	}
}

func (m *Manager) finishTurnChangeCapture(execution *AgentExecution, event *agentctl.AgentEvent, isError bool, stopReason string) {
	if m == nil || m.turnChangeCaptureHandler == nil || event == nil || event.PromptGeneration == 0 ||
		!eligibleTurnChangeExecution(execution) || event.TurnID == "" {
		return
	}
	if execution.turnChangeCaptureGeneration != event.PromptGeneration {
		return
	}
	// The completion claim keeps the generation immutable. Release its mutex
	// while checkpoint and database I/O run; BeginPrompt observes the fence.
	execution.promptLifecycleMu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), turnChangeTerminalCaptureTimeout)
	client, release := execution.AcquireAgentCtlClient()
	terminal := TurnChangeTerminal{
		TurnChangeAdmission: turnChangeAdmission(execution, event.PromptGeneration, event.TurnID),
		At:                  time.Now().UTC(),
		Outcome:             stopReason,
	}
	if isError {
		terminal.Outcome = kubernetesLaunchOutcomeError
	}
	execution.messageMu.Lock()
	terminal.FinalAssistantMessageID = execution.lastAssistantMessageIDByGeneration[event.PromptGeneration]
	execution.messageMu.Unlock()
	finishErr := m.turnChangeCaptureHandler.FinishTurnChanges(ctx, terminal, client)
	if finishErr != nil {
		m.logger.Warn("failed to finalize turn-change capture at terminal boundary",
			zap.String("execution_id", execution.ID), zap.Uint64("prompt_generation", event.PromptGeneration), zap.Error(finishErr))
	}
	release()
	cancel()
	execution.promptLifecycleMu.Lock()
	if finishErr == nil {
		execution.messageMu.Lock()
		delete(execution.lastAssistantMessageIDByGeneration, event.PromptGeneration)
		if len(execution.lastAssistantMessageIDByGeneration) == 0 {
			execution.lastAssistantMessageIDByGeneration = nil
		}
		execution.messageMu.Unlock()
		finishTurnChangeCaptureLocked(execution, event.PromptGeneration)
	}
	// Keep the generation fence held if terminal persistence failed. A later
	// retry or process restart can settle the row without admitting successor
	// writes against an unrecorded boundary.
}

func beginTurnChangeCaptureLocked(execution *AgentExecution, generation uint64) {
	if execution.turnChangeCaptureGeneration == generation {
		return
	}
	execution.turnChangeCaptureGeneration = generation
	execution.turnChangeCaptureDone = make(chan struct{})
}

func finishTurnChangeCaptureLocked(execution *AgentExecution, generation uint64) {
	if execution.turnChangeCaptureGeneration != generation {
		return
	}
	execution.turnChangeCaptureGeneration = 0
	if execution.turnChangeCaptureDone != nil {
		close(execution.turnChangeCaptureDone)
		execution.turnChangeCaptureDone = nil
	}
}

// preserveTurnChangesBeforeStop asks an active turn to stop, then records its
// terminal interval before the runtime tears down the checkout. A completion
// callback that already owns the boundary is joined instead of duplicated.
func (m *Manager) preserveTurnChangesBeforeStop(ctx context.Context, execution *AgentExecution) {
	if m == nil || m.turnChangeCaptureHandler == nil || !eligibleTurnChangeExecution(execution) {
		return
	}
	m.cancelTurnChangePromptBeforeStop(ctx, execution)

	execution.promptLifecycleMu.Lock()
	generation := execution.promptGeneration
	turnID := execution.promptTurnIDs[generation]
	if generation == 0 || turnID == "" {
		execution.promptLifecycleMu.Unlock()
		return
	}
	if execution.turnChangeCaptureGeneration == generation {
		done := execution.turnChangeCaptureDone
		execution.promptLifecycleMu.Unlock()
		m.waitForTurnChangeCaptureBeforeStop(ctx, execution, generation, done)
		return
	}
	if execution.promptCompletionGeneration == generation {
		execution.promptLifecycleMu.Unlock()
		return
	}
	beginTurnChangeCaptureLocked(execution, generation)
	event := &agentctl.AgentEvent{PromptGeneration: generation, TurnID: turnID}
	m.finishTurnChangeCapture(execution, event, false, "stopped")
	execution.promptLifecycleMu.Unlock()
}

func (m *Manager) cancelTurnChangePromptBeforeStop(ctx context.Context, execution *AgentExecution) {
	if execution.dispatchedPromptPending.Load() {
		client, release := execution.AcquireAgentCtlClient()
		defer release()
		if client != nil {
			cancelCtx, cancel := context.WithTimeout(ctx, turnChangeCancelRequestTimeout)
			err := client.Cancel(cancelCtx)
			cancel()
			if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
				m.logger.Debug("agent turn cancellation before checkpoint returned an error",
					zap.String("execution_id", execution.ID), zap.Error(err))
			}
		}
	}
}

func (m *Manager) waitForTurnChangeCaptureBeforeStop(ctx context.Context, execution *AgentExecution, generation uint64, done <-chan struct{}) {
	if done == nil {
		return
	}
	waitCtx, cancel := context.WithTimeout(ctx, turnChangeTerminalCaptureTimeout)
	defer cancel()
	select {
	case <-done:
	case <-waitCtx.Done():
		m.logger.Warn("timed out waiting for in-flight turn-change capture before executor stop",
			zap.String("execution_id", execution.ID), zap.Uint64("prompt_generation", generation), zap.Error(waitCtx.Err()))
	}
}

func eligibleTurnChangeExecution(execution *AgentExecution) bool {
	return execution != nil && execution.TaskID != "" && execution.SessionID != "" &&
		execution.TaskEnvironmentID != "" && execution.Owner.Kind != ExecutionOwnerRun
}

func turnChangeAdmission(execution *AgentExecution, generation uint64, turnID string) TurnChangeAdmission {
	checkouts := append([]TurnChangeCheckout(nil), execution.TurnChangeCheckouts...)
	return TurnChangeAdmission{
		TaskID: execution.TaskID, SessionID: execution.SessionID, TaskEnvironmentID: execution.TaskEnvironmentID,
		TurnID: turnID, ExecutionID: execution.ID, StartupAttemptID: execution.currentStartupAttemptID(),
		PromptGeneration: generation, ExecutionProfileID: execution.AgentProfileID, Checkouts: checkouts,
	}
}
