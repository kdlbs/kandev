package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/managedruntime"
	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
	agentctltypes "github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/pkg/api/v1"
	"go.uber.org/zap"
)

const managedRuntimeStartupStderrTimeout = 2 * time.Second

// managedRuntimeNpmStartupFailure captures only the bounded stderr needed to
// classify an ACP initialization failure. The command is never built from
// this data.
func (m *Manager) managedRuntimeNpmStartupFailure(
	ctx context.Context,
	execution *AgentExecution,
	initErr error,
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
	return routingerr.Classify(routingerr.Input{
		Phase:      routingerr.PhaseSessionInit,
		ProviderID: execution.AgentID,
		Stderr:     evidence,
		Stdout:     initErr.Error(),
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

func invalidateManagedRuntimeExecutionTree(
	ctx context.Context,
	invalidator ManagedRuntimeCacheInvalidator,
	spec agents.ManagedNPMRuntimeSpec,
	packageSpec string,
) error {
	if invalidator == nil {
		return errors.New("managed runtime cache invalidator is unavailable")
	}
	if packageSpec == spec.Package {
		return invalidator.InvalidateExecutionCache(ctx, spec.Package)
	}
	version := strings.TrimPrefix(packageSpec, spec.Package+"@")
	if version == "" || version == packageSpec {
		return errors.New("managed runtime package spec is not versioned")
	}
	if _, err := managedruntime.ParseStableVersion(version); err != nil {
		return err
	}
	return invalidator.InvalidateExecutionCacheVersion(ctx, spec.Package, version)
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

type managedRuntimeStartupRetry struct {
	managed          agents.ManagedNPMRuntimeAgent
	preferOnlineArgs []string
	packageSpec      string
	failureDetails   string
}

func (m *Manager) prepareManagedRuntimeStartupRetry(
	ctx context.Context,
	execution *AgentExecution,
	initErr error,
	agentConfig agents.Agent,
) (*managedRuntimeStartupRetry, bool) {
	if execution == nil || execution.RuntimeName != agentruntime.RuntimeStandalone || execution.agentctl == nil {
		return nil, false
	}
	managed, ok := agentConfig.(agents.ManagedNPMRuntimeAgent)
	if !ok || m.managedRuntimeCacheInvalidator == nil {
		return nil, false
	}
	classification := m.managedRuntimeNpmStartupFailure(ctx, execution, initErr)
	if classification == nil || classification.Code != routingerr.CodeManagedRuntimeNpmResolution {
		return nil, false
	}
	preferOnlineArgs, packageSpec, ok := onlineManagedRuntimeArgs(execution.AgentArgs, managed.ManagedNPMRuntime())
	if !ok {
		return nil, false
	}
	return &managedRuntimeStartupRetry{
		managed:          managed,
		preferOnlineArgs: preferOnlineArgs,
		packageSpec:      packageSpec,
		failureDetails:   classification.RawExcerpt,
	}, true
}

// stopAndInvalidateManagedRuntime stops the failed child and removes only its
// trusted npm execution tree. needsFailure distinguishes an ordinary repair
// error from cancellation or shutdown, which must win over recovery.
func (m *Manager) stopAndInvalidateManagedRuntime(
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

	spec := retry.managed.ManagedNPMRuntime()
	if err := invalidateManagedRuntimeExecutionTree(ctx, m.managedRuntimeCacheInvalidator, spec, retry.packageSpec); err != nil {
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
		current.sessionInitialized = false
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
	failureDetails := retry.failureDetails
	failAfterRetry := func() error {
		m.updateExecutionFailure(
			execution.ID,
			"managed npm runtime failed to prepare",
			string(routingerr.CodeManagedRuntimeNpmResolution),
			failureDetails,
		)
		return fmt.Errorf("%s: %s", routingerr.CodeManagedRuntimeNpmResolution, failureDetails)
	}
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
	if needsFailure, err := m.stopAndInvalidateManagedRuntime(ctx, execution, retry); err != nil {
		if needsFailure {
			return true, failAfterRetry()
		}
		return true, err
	}
	if err := managedRuntimeRecoveryAborted(ctx, m); err != nil {
		return true, err
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
			return true, aborted
		}
		if initializationFailed {
			second := m.managedRuntimeNpmStartupFailure(ctx, execution, retryErr)
			if second != nil && second.RawExcerpt != "" {
				failureDetails = second.RawExcerpt
			}
		}
		return true, failAfterRetry()
	}
	return true, nil
}
