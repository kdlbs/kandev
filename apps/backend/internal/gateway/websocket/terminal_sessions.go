package websocket

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
)

const (
	passthroughReadyTimeout = 30 * time.Second
	remoteReadyProbeTimeout = 500 * time.Millisecond
	pollInterval            = 500 * time.Millisecond
)

func (h *TerminalHandler) shouldUseWorkspaceShell(ctx context.Context, sessionID string) bool {
	// Delegates to lifecycle manager which checks both in-memory execution
	// and database records for containerized/remote executors.
	return h.lifecycleMgr.ShouldUseContainerShell(ctx, sessionID)
}

func (h *TerminalHandler) waitForRemoteExecutionReadyWithTimeout(
	ctx context.Context,
	sessionID string,
	timeout time.Duration,
) (*lifecycle.AgentExecution, bool) {
	deadline := time.Now().Add(timeout)

	h.logger.Info("waiting for remote execution to become ready",
		zap.String("session_id", sessionID),
		zap.Duration("timeout", timeout))

	for {
		if execution, ok := h.remoteExecutionWithClientReady(ctx, sessionID, deadline); ok {
			return execution, true
		}

		remaining := time.Until(deadline)
		if remaining <= 0 {
			h.logger.Warn("timed out waiting for remote execution readiness",
				zap.String("session_id", sessionID),
				zap.Duration("timeout", timeout))
			return nil, false
		}

		wait := pollInterval
		if remaining < wait {
			wait = remaining
		}
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, false
		case <-timer.C:
		}
	}
}

func (h *TerminalHandler) remoteExecutionWithClientReady(
	ctx context.Context,
	sessionID string,
	deadline time.Time,
) (*lifecycle.AgentExecution, bool) {
	execution, exists := h.lifecycleMgr.GetExecutionBySessionID(sessionID)
	if !exists || execution.GetAgentCtlClient() == nil {
		return nil, false
	}
	remaining := time.Until(deadline)
	if remaining <= 0 {
		return nil, false
	}

	probeTimeout := remoteReadyProbeTimeout
	if remaining < probeTimeout {
		probeTimeout = remaining
	}
	probeCtx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	if err := execution.GetAgentCtlClient().Health(probeCtx); err != nil {
		h.logger.Debug("remote execution agentctl client not ready",
			zap.String("session_id", sessionID),
			zap.String("execution_id", execution.ID),
			zap.Error(err))
		return nil, false
	}
	h.logger.Info("remote execution became ready",
		zap.String("session_id", sessionID),
		zap.String("execution_id", execution.ID))
	return execution, true
}

func (h *TerminalHandler) ensurePassthroughExecutionReady(ctx context.Context, sessionID string) (*lifecycle.AgentExecution, error) {
	execution, err := h.lifecycleMgr.EnsurePassthroughExecution(ctx, sessionID)
	if err == nil {
		return execution, nil
	}
	if !errors.Is(err, lifecycle.ErrSessionWorkspaceNotReady) {
		return nil, err
	}

	h.logger.Info("waiting for passthrough workspace to become ready",
		zap.String("session_id", sessionID),
		zap.Duration("timeout", passthroughReadyTimeout))

	timeoutCtx, cancel := context.WithTimeout(ctx, passthroughReadyTimeout)
	defer cancel()

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-timeoutCtx.Done():
			return nil, fmt.Errorf("%w: timed out after %s", lifecycle.ErrSessionWorkspaceNotReady, passthroughReadyTimeout)
		case <-ticker.C:
			execution, err := h.lifecycleMgr.EnsurePassthroughExecution(timeoutCtx, sessionID)
			if err == nil {
				h.logger.Info("passthrough workspace became ready",
					zap.String("session_id", sessionID),
					zap.String("execution_id", execution.ID))
				return execution, nil
			}
			if !errors.Is(err, lifecycle.ErrSessionWorkspaceNotReady) {
				return nil, err
			}
		}
	}
}
