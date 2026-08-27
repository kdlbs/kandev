package lifecycle

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
	agentctltypes "github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/pkg/api/v1"
	"go.uber.org/zap"
)

const managedRuntimeStartupStderrTimeout = 2 * time.Second

const managedRuntimeStartupSettleDelay = 500 * time.Millisecond

// managedRuntimeNpmStartupFailure captures only the bounded stderr needed to
// classify an ACP initialization failure. The command is never built from
// this data.
func (m *Manager) managedRuntimeNpmStartupFailure(
	ctx context.Context,
	execution *AgentExecution,
	initErr error,
	packageSpec string,
) *routingerr.Error {
	if execution == nil || execution.agentctl == nil || initErr == nil {
		return nil
	}
	stderrCtx, cancel := context.WithTimeout(ctx, managedRuntimeStartupStderrTimeout)
	defer cancel()
	lines, err := execution.agentctl.GetAgentStderr(stderrCtx)
	if err != nil {
		return nil
	}
	evidence := strings.Join(lines, "\n")
	combined := strings.TrimSpace(evidence + "\n" + initErr.Error())
	if !routingerr.ManagedRuntimeNpmResolutionMatchesPackage(combined, packageSpec) {
		return nil
	}
	return routingerr.Classify(routingerr.Input{
		Phase:      routingerr.PhaseSessionInit,
		ProviderID: execution.AgentID,
		Stderr:     combined,
	})
}

func managedRuntimeRecoveryAborted(ctx context.Context, m *Manager) error {
	if err := context.Cause(ctx); err != nil {
		return err
	}
	if m.IsShuttingDown() {
		return errors.New("managed runtime recovery interrupted by shutdown")
	}
	return nil
}

func (m *Manager) updateExecutionFailure(executionID, message, code, details string) {
	if m.executionStore == nil {
		return
	}
	_ = m.executionStore.WithLock(executionID, func(execution *AgentExecution) {
		execution.ErrorMessage = message
		execution.FailureCode = code
		execution.FailureDetails = details
		execution.Status = v1.AgentStatusFailed
	})
}

func (m *Manager) publishManagedRuntimeStartupFailure(
	execution *AgentExecution,
	message string,
	code routingerr.Code,
	details string,
	cause error,
) error {
	m.updateExecutionFailure(execution.ID, message, string(code), details)
	return &routingerr.ManagedRuntimeStartupError{
		Code:    code,
		Details: details,
		Cause:   cause,
	}
}

type managedRuntimeStartupRetry struct {
	preferOnlineArgs []string
	packageSpec      string
	failureDetails   string
}

func managedRuntimeRetryFailureClassification(
	initialCode routingerr.Code,
	initialDetails string,
	retryErr error,
	initializationFailed bool,
	second *routingerr.Error,
) (routingerr.Code, string) {
	if retryErr == nil {
		return initialCode, initialDetails
	}
	if !initializationFailed || second == nil {
		return routingerr.CodeAgentRuntime, routingerr.Sanitize(retryErr.Error())
	}
	details := second.RawExcerpt
	if details == "" {
		details = routingerr.Sanitize(retryErr.Error())
	}
	return second.Code, details
}

func (m *Manager) prepareManagedRuntimeStartupRetry(
	ctx context.Context,
	execution *AgentExecution,
	initErr error,
	agentConfig agents.Agent,
) (*managedRuntimeStartupRetry, bool) {
	if execution == nil || !supportsManagedRuntimeCacheRepair(execution.RuntimeName) || execution.agentctl == nil {
		return nil, false
	}
	managed, ok := agentConfig.(agents.ManagedNPMRuntimeAgent)
	if !ok {
		return nil, false
	}
	preferOnlineArgs, packageSpec, ok := onlineManagedRuntimeArgs(execution.AgentArgs, managed.ManagedNPMRuntime())
	if !ok {
		return nil, false
	}
	classification := m.managedRuntimeNpmStartupFailure(ctx, execution, initErr, packageSpec)
	if classification == nil || classification.Code != routingerr.CodeManagedRuntimeNpmResolution {
		return nil, false
	}
	return &managedRuntimeStartupRetry{
		preferOnlineArgs: preferOnlineArgs,
		packageSpec:      packageSpec,
		failureDetails:   classification.RawExcerpt,
	}, true
}

func supportsManagedRuntimeCacheRepair(runtime agentruntime.Runtime) bool {
	switch runtime {
	case agentruntime.RuntimeStandalone, agentruntime.RuntimeDocker, agentruntime.RuntimeSSH:
		return true
	default:
		return false
	}
}

// stopAndRepairManagedRuntime stops the failed child and asks its colocated
// agentctl process to remove only the trusted npm execution tree.
// needsFailure distinguishes an ordinary repair error from cancellation or
// shutdown, which must win over recovery.
func (m *Manager) stopAndRepairManagedRuntime(
	ctx context.Context,
	execution *AgentExecution,
	retry *managedRuntimeStartupRetry,
) (needsFailure bool, err error) {
	if stopErr := execution.agentctl.Stop(ctx); stopErr != nil {
		if aborted := managedRuntimeRecoveryAborted(ctx, m); aborted != nil {
			return false, aborted
		}
		m.logger.Warn("managed runtime process stop failed before retry; continuing",
			zap.String("execution_id", execution.ID), zap.Error(stopErr))
	}
	execution.agentctl.CloseUpdatesStream()
	if aborted := managedRuntimeRecoveryAborted(ctx, m); aborted != nil {
		return false, aborted
	}

	if err := execution.agentctl.RepairManagedRuntimeCache(ctx, retry.packageSpec); err != nil {
		if aborted := managedRuntimeRecoveryAborted(ctx, m); aborted != nil {
			return false, aborted
		}
		return true, err
	}
	if aborted := managedRuntimeRecoveryAborted(ctx, m); aborted != nil {
		return false, aborted
	}
	return false, nil
}

func (m *Manager) startManagedRuntimeRetry(
	ctx context.Context,
	execution *AgentExecution,
	agentConfig agents.Agent,
	approvalPolicy string,
	taskDescription string,
	attachments []MessageAttachment,
	mcpServers []agentctltypes.McpServer,
) (initializationFailed bool, err error) {
	if _, err := m.configureAndStartAgent(ctx, execution, approvalPolicy); err != nil {
		return false, err
	}
	if err := managedRuntimeRecoveryAborted(ctx, m); err != nil {
		return false, err
	}
	settleTimer := time.NewTimer(managedRuntimeStartupSettleDelay)
	defer settleTimer.Stop()
	select {
	case <-settleTimer.C:
	case <-ctx.Done():
		if cause := context.Cause(ctx); cause != nil {
			return false, cause
		}
		return false, ctx.Err()
	}
	if err := managedRuntimeRecoveryAborted(ctx, m); err != nil {
		return false, err
	}
	if err := m.initializeACPSession(ctx, execution, agentConfig, taskDescription, attachments, mcpServers); err != nil {
		return true, err
	}
	return false, nil
}

func (m *Manager) resetManagedRuntimeExecutionForRetry(execution *AgentExecution, args []string) {
	_ = m.executionStore.WithLock(execution.ID, func(current *AgentExecution) {
		current.AgentArgs = append([]string(nil), args...)
		current.AgentCommand = strings.Join(args, " ")
		current.Status = v1.AgentStatusStarting
		current.ErrorMessage = ""
		current.FailureCode = ""
		current.FailureDetails = ""
		current.setSessionInitialized(false)
		m.resetStreamingStateWithHistory(current)
		select {
		case <-current.promptDoneCh:
		default:
		}
	})
}

// retryManagedRuntimeStartup performs the single online metadata retry for a
// host-local managed npm runtime. attempted is true only after the replacement
// process has been selected, so callers can preserve the original path when a
// failure is unrelated or recovery is unavailable.
func (m *Manager) retryManagedRuntimeStartup(
	ctx context.Context,
	execution *AgentExecution,
	initErr error,
	agentConfig agents.Agent,
	approvalPolicy string,
	taskDescription string,
	attachments []MessageAttachment,
	mcpServers []agentctltypes.McpServer,
) (attempted bool, err error) {
	retry, ok := m.prepareManagedRuntimeStartupRetry(ctx, execution, initErr, agentConfig)
	if !ok {
		return false, initErr
	}
	failureCode := routingerr.CodeManagedRuntimeNpmResolution
	failureDetails := retry.failureDetails
	var failureCause error
	if err := managedRuntimeRecoveryAborted(ctx, m); err != nil {
		return false, err
	}
	retryGeneration, ok := execution.beginStartupRecovery()
	if !ok {
		return false, initErr
	}
	defer execution.finishStartupRecovery()

	m.logger.Warn("retrying managed npm runtime startup with online metadata",
		zap.String("execution_id", execution.ID),
		zap.String("agent_id", execution.AgentID),
		zap.Uint64("startup_generation", retryGeneration))

	// Stop only the child process. The agentctl server remains alive so the
	// same execution can reconnect its streams and retain its identity.
	if needsFailure, err := m.stopAndRepairManagedRuntime(ctx, execution, retry); err != nil {
		if needsFailure {
			failureCode = routingerr.CodeAgentRuntime
			failureDetails = routingerr.Sanitize(err.Error())
			failureCause = err
			return false, m.publishManagedRuntimeStartupFailure(
				execution, "managed runtime cache repair failed", failureCode, failureDetails, failureCause,
			)
		}
		return false, err
	}
	if err := managedRuntimeRecoveryAborted(ctx, m); err != nil {
		return false, err
	}

	m.resetManagedRuntimeExecutionForRetry(execution, retry.preferOnlineArgs)

	initializationFailed, retryErr := m.startManagedRuntimeRetry(
		ctx,
		execution,
		agentConfig,
		approvalPolicy,
		taskDescription,
		attachments,
		mcpServers,
	)
	if retryErr != nil {
		if aborted := managedRuntimeRecoveryAborted(ctx, m); aborted != nil {
			return false, aborted
		}
		failureCause = retryErr
		var second *routingerr.Error
		if initializationFailed {
			second = m.managedRuntimeNpmStartupFailure(ctx, execution, retryErr, retry.packageSpec)
		}
		failureCode, failureDetails = managedRuntimeRetryFailureClassification(
			failureCode, failureDetails, retryErr, initializationFailed, second,
		)
		return true, m.publishManagedRuntimeStartupFailure(
			execution, "managed npm runtime failed to prepare", failureCode, failureDetails, failureCause,
		)
	}
	return true, nil
}
