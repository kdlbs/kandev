package lifecycle

import (
	"context"
	"errors"
	"math/rand"
	"strconv"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/agent/agents"
	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
	agentctltypes "github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/common/npmresolution"
	"github.com/kandev/kandev/pkg/api/v1"
	"go.uber.org/zap"
)

const managedRuntimeStartupStderrTimeout = 2 * time.Second

const managedRuntimeStartupSettleDelay = 500 * time.Millisecond

const (
	managedRuntimeStartupRecoveryBudget = 90 * time.Second
	managedRuntimeStartupBackoff        = 2 * time.Second
	managedRuntimeStartupJitter         = time.Second
)

func managedRuntimeStartupDelay(jitter time.Duration) time.Duration {
	if jitter < 0 {
		jitter = 0
	}
	if jitter >= managedRuntimeStartupJitter {
		jitter = managedRuntimeStartupJitter - time.Nanosecond
	}
	return managedRuntimeStartupBackoff + jitter
}

func managedRuntimeStartupRecoveryContext(parent context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(parent, managedRuntimeStartupRecoveryBudget)
}

func (m *Manager) waitManagedRuntimeStartupBackoff(ctx context.Context) error {
	jitter := time.Duration(rand.Int63n(int64(managedRuntimeStartupJitter)))
	delay := managedRuntimeStartupDelay(jitter)
	if m.startupRecoveryDelay != nil {
		delay = m.startupRecoveryDelay()
	}
	if delay <= 0 {
		return managedRuntimeRecoveryAborted(ctx, m)
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return managedRuntimeRecoveryAborted(ctx, m)
	case <-ctx.Done():
		if cause := context.Cause(ctx); cause != nil {
			return cause
		}
		return ctx.Err()
	}
}

// managedRuntimeNpmStartupFailure captures only the bounded stderr needed to
// classify an ACP initialization failure. The command is never built from
// this data.
func (m *Manager) managedRuntimeNpmStartupFailure(
	ctx context.Context,
	client *agentctl.Client,
	execution *AgentExecution,
	initErr error,
	packageSpec string,
) *routingerr.Error {
	if execution == nil || initErr == nil || client == nil {
		return nil
	}
	stderrCtx, cancel := context.WithTimeout(ctx, managedRuntimeStartupStderrTimeout)
	defer cancel()
	lines, err := client.GetAgentStderr(stderrCtx)
	if err != nil {
		return nil
	}
	evidence := strings.Join(lines, "\n")
	combined := strings.TrimSpace(evidence + "\n" + initErr.Error())
	if !routingerr.ManagedRuntimeNpmResolutionMatchesPackage(combined, packageSpec) &&
		!routingerr.ManagedRuntimeNpmReleaseAgePolicyMatchesPackage(combined, packageSpec) {
		return nil
	}
	return routingerr.Classify(routingerr.Input{
		Phase:                     routingerr.PhaseSessionInit,
		ProviderID:                execution.AgentID,
		Stderr:                    combined,
		ManagedRuntimePackageSpec: packageSpec,
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

type managedRuntimeStartupFailureMetadata struct {
	reason   string
	attempts int
	npmCode  string
}

func (e *AgentExecution) setStartupFailureMetadata(metadata managedRuntimeStartupFailureMetadata) {
	e.startupFailureMu.Lock()
	defer e.startupFailureMu.Unlock()
	e.StartupFailureReason = metadata.reason
	e.StartupFailureAttempts = metadata.attempts
	e.StartupFailureNPMCode = metadata.npmCode
}

func (e *AgentExecution) startupFailureMetadataSnapshot() managedRuntimeStartupFailureMetadata {
	e.startupFailureMu.RLock()
	defer e.startupFailureMu.RUnlock()
	return managedRuntimeStartupFailureMetadata{
		reason:   e.StartupFailureReason,
		attempts: e.StartupFailureAttempts,
		npmCode:  e.StartupFailureNPMCode,
	}
}

type managedRuntimeRetryFailurePhase string

const (
	managedRuntimeRetryFailureNone          managedRuntimeRetryFailurePhase = ""
	managedRuntimeRetryFailureProcessSetup  managedRuntimeRetryFailurePhase = "process_setup"
	managedRuntimeRetryFailureACPInitialize managedRuntimeRetryFailurePhase = "acp_initialize"
	managedRuntimeRetryFailureSessionSetup  managedRuntimeRetryFailurePhase = "session_setup"
)

func (m *Manager) updateExecutionFailure(
	executionID, message, code, details string,
	startup managedRuntimeStartupFailureMetadata,
) {
	if m.executionStore == nil {
		return
	}
	_ = m.executionStore.WithLock(executionID, func(execution *AgentExecution) {
		execution.ErrorMessage = message
		execution.FailureCode = code
		execution.FailureDetails = details
		execution.setStartupFailureMetadata(startup)
		execution.Status = v1.AgentStatusFailed
	})
}

func (m *Manager) publishManagedRuntimeStartupFailure(
	execution *AgentExecution,
	message string,
	code routingerr.Code,
	details string,
	startup managedRuntimeStartupFailureMetadata,
	cause error,
) error {
	m.updateExecutionFailure(execution.ID, message, string(code), details, startup)
	return &routingerr.ManagedRuntimeStartupError{
		Code:     code,
		Details:  details,
		Reason:   startup.reason,
		Attempts: startup.attempts,
		NPMCode:  startup.npmCode,
		Cause:    cause,
	}
}

type managedRuntimeStartupRetry struct {
	retryArgs       []string
	failureCode     routingerr.Code
	failureReason   string
	failureNPMCode  string
	failureEvidence string
	terminal        bool
}

func managedRuntimeStartupFailureDetails(reason string, attempts int, evidence string) string {
	details := "reason=" + reason + " attempts=" + strconv.Itoa(attempts)
	if evidence != "" {
		details += "\n" + evidence
	}
	return details
}

func managedRuntimeStartupRetryForLegacyClassification(
	classification *routingerr.Error,
	preferOnlineArgs []string,
) (*managedRuntimeStartupRetry, bool) {
	if classification == nil {
		return nil, false
	}
	switch classification.Code {
	case routingerr.CodeManagedRuntimeNpmPolicy:
		return &managedRuntimeStartupRetry{
			failureCode:     classification.Code,
			failureReason:   "npm_release_policy",
			failureEvidence: classification.RawExcerpt,
			terminal:        true,
		}, true
	case routingerr.CodeManagedRuntimeNpmResolution:
		return &managedRuntimeStartupRetry{
			retryArgs:       preferOnlineArgs,
			failureCode:     classification.Code,
			failureReason:   "npm_resolution",
			failureNPMCode:  "ETARGET",
			failureEvidence: classification.RawExcerpt,
		}, true
	default:
		return nil, false
	}
}

func managedRuntimeStartupRetryForEvidence(
	evidence *agentctltypes.ManagedStartupEvidence,
	classification *routingerr.Error,
	preferOnlineArgs, originalArgs []string,
) *managedRuntimeStartupRetry {
	if !managedStartupEvidenceAllowsClassification(evidence) {
		return nil
	}
	if evidence.NPMCode == "ETARGET" || (evidence.NPMCode == "" && classification != nil && classification.Code == routingerr.CodeManagedRuntimeNpmResolution) {
		if classification == nil || classification.Code != routingerr.CodeManagedRuntimeNpmResolution {
			return nil
		}
		return &managedRuntimeStartupRetry{
			retryArgs:       preferOnlineArgs,
			failureCode:     classification.Code,
			failureReason:   "npm_resolution",
			failureNPMCode:  "ETARGET",
			failureEvidence: classification.RawExcerpt,
		}
	}
	if npmresolution.IsTransientManagedStartupCode(evidence.NPMCode) {
		return &managedRuntimeStartupRetry{
			retryArgs:       preferOnlineArgs,
			failureCode:     routingerr.CodeManagedRuntimeStartup,
			failureReason:   "npm_transient",
			failureNPMCode:  strings.ToUpper(evidence.NPMCode),
			failureEvidence: "npm_code=" + strings.ToUpper(evidence.NPMCode),
		}
	}
	if evidence.NPMCode != "" {
		return nil
	}
	if evidence.NPMDiagnosticPresent {
		return nil
	}
	return &managedRuntimeStartupRetry{
		retryArgs:     append([]string(nil), originalArgs...),
		failureCode:   routingerr.CodeManagedRuntimeStartup,
		failureReason: "early_exit",
	}
}

func managedStartupEvidenceAllowsClassification(evidence *agentctltypes.ManagedStartupEvidence) bool {
	if evidence == nil || !evidence.CollectionComplete || !evidence.NPMDiagnosticComplete || evidence.UnclassifiedNPMCode {
		return false
	}
	if evidence.NPMCode != "" && !evidence.NPMDiagnosticPresent {
		return false
	}
	return true
}

func (m *Manager) prepareManagedRuntimeStartupRetry(
	ctx context.Context,
	execution *AgentExecution,
	initErr error,
	agentConfig agents.Agent,
) (*managedRuntimeStartupRetry, bool) {
	if execution == nil || ctx.Err() != nil || !isManagedACPInitializeFailure(initErr) {
		return nil, false
	}
	if routingerr.IsAuthenticationFailureDiagnostic(initErr.Error()) {
		return nil, false
	}
	spec, ok := managedRuntimeSpecForArgs(agentConfig, execution.AgentArgs)
	if !ok {
		return nil, false
	}
	preferOnlineArgs, packageSpec, ok := onlineManagedRuntimeArgs(execution.AgentArgs, spec)
	if !ok {
		return nil, false
	}
	client, release := execution.AcquireAgentCtlClient()
	if client == nil {
		release()
		return nil, false
	}
	defer release()
	var initializeErr *agentctl.InitializeError
	if errors.As(initErr, &initializeErr) && initializeErr.StartupEvidence != nil {
		evidence := initializeErr.StartupEvidence
		if evidence.ProcessGeneration == 0 || evidence.ProcessGeneration != client.ProcessGeneration() ||
			!managedStartupEvidenceAllowsClassification(evidence) || evidence.ExitDisposition != agentctltypes.ManagedStartupExitOrdinary {
			return nil, false
		}
		if npmresolution.IsPermanentManagedStartupCode(evidence.NPMCode) {
			return &managedRuntimeStartupRetry{
				failureCode:     routingerr.CodeManagedRuntimeStartup,
				failureReason:   "permanent_npm_error",
				failureNPMCode:  strings.ToUpper(evidence.NPMCode),
				failureEvidence: "npm_code=" + strings.ToUpper(evidence.NPMCode),
				terminal:        true,
			}, true
		}
	}
	classification := m.managedRuntimeNpmStartupFailure(ctx, client, execution, initErr, packageSpec)
	if classification != nil && classification.Code == routingerr.CodeManagedRuntimeNpmPolicy {
		return &managedRuntimeStartupRetry{
			failureCode:     classification.Code,
			failureReason:   "npm_release_policy",
			failureEvidence: classification.RawExcerpt,
			terminal:        true,
		}, true
	}
	if !supportsManagedRuntimeStartupRecovery(execution.RuntimeName) {
		return nil, false
	}
	if initializeErr == nil || initializeErr.StartupEvidence == nil {
		return managedRuntimeStartupRetryForLegacyClassification(classification, preferOnlineArgs)
	}
	retry := managedRuntimeStartupRetryForEvidence(
		initializeErr.StartupEvidence, classification, preferOnlineArgs, execution.AgentArgs,
	)
	return retry, retry != nil
}

func isManagedACPInitializeFailure(err error) bool {
	var phaseErr *SessionInitializationPhaseError
	return errors.As(err, &phaseErr) && phaseErr.Phase == SessionInitializationPhaseACPInitialize
}

func supportsManagedRuntimeStartupRecovery(runtime agentruntime.Runtime) bool {
	switch runtime {
	case agentruntime.RuntimeStandalone, agentruntime.RuntimeDocker, agentruntime.RuntimeSSH:
		return true
	default:
		return false
	}
}

// stopManagedRuntimeBeforeRetry stops the failed child before a replacement.
// needsFailure distinguishes an ordinary repair error from cancellation or
// shutdown, which must win over recovery.
func (m *Manager) stopManagedRuntimeBeforeRetry(
	ctx context.Context,
	execution *AgentExecution,
) (needsFailure bool, err error) {
	client, release := execution.AcquireAgentCtlClient()
	defer release()
	if client == nil {
		return false, errors.New("managed runtime agentctl client is unavailable")
	}
	if stopErr := client.Stop(ctx); stopErr != nil {
		if aborted := managedRuntimeRecoveryAborted(ctx, m); aborted != nil {
			return false, aborted
		}
		return true, stopErr
	}
	client.CloseUpdatesStream()
	if aborted := managedRuntimeRecoveryAborted(ctx, m); aborted != nil {
		return false, aborted
	}

	return false, nil
}

func (m *Manager) startManagedRuntimeRetry(
	ctx context.Context,
	execution *AgentExecution,
	agentConfig agents.Agent,
	taskDescription string,
	attachments []MessageAttachment,
	mcpServers []agentctltypes.McpServer,
) (managedRuntimeRetryFailurePhase, error) {
	if _, err := m.configureAndStartAgent(ctx, execution); err != nil {
		return managedRuntimeRetryFailureProcessSetup, err
	}
	if err := managedRuntimeRecoveryAborted(ctx, m); err != nil {
		return managedRuntimeRetryFailureNone, err
	}
	settleTimer := time.NewTimer(managedRuntimeStartupSettleDelay)
	defer settleTimer.Stop()
	select {
	case <-settleTimer.C:
	case <-ctx.Done():
		if cause := context.Cause(ctx); cause != nil {
			return managedRuntimeRetryFailureNone, cause
		}
		return managedRuntimeRetryFailureNone, ctx.Err()
	}
	if err := managedRuntimeRecoveryAborted(ctx, m); err != nil {
		return managedRuntimeRetryFailureNone, err
	}
	if err := m.initializeACPSession(ctx, execution, agentConfig, taskDescription, attachments, mcpServers); err != nil {
		if isManagedACPInitializeFailure(err) {
			return managedRuntimeRetryFailureACPInitialize, err
		}
		return managedRuntimeRetryFailureSessionSetup, err
	}
	return managedRuntimeRetryFailureNone, nil
}

func managedRuntimeStartupFailureEvidenceWithFinalDiagnostic(evidence string, cause error) string {
	if cause == nil {
		return evidence
	}
	diagnostic := routingerr.Sanitize(cause.Error())
	if diagnostic == "" {
		return evidence
	}
	combined := "final_diagnostic=" + diagnostic
	if evidence != "" {
		combined += "\n" + evidence
	}
	return routingerr.Sanitize(combined)
}

func managedRuntimeRetryFailureReason(phase managedRuntimeRetryFailurePhase) string {
	switch phase {
	case managedRuntimeRetryFailureACPInitialize:
		return "retry_initialize_failed"
	case managedRuntimeRetryFailureSessionSetup:
		return "retry_session_setup_failed"
	default:
		return "retry_process_setup_failed"
	}
}

func (m *Manager) resetManagedRuntimeExecutionForRetry(execution *AgentExecution, args []string) {
	_ = m.executionStore.WithLock(execution.ID, func(current *AgentExecution) {
		current.AgentArgs = append([]string(nil), args...)
		current.AgentCommand = strings.Join(args, " ")
		current.Status = v1.AgentStatusStarting
		current.ErrorMessage = ""
		current.FailureCode = ""
		current.FailureDetails = ""
		current.setStartupFailureMetadata(managedRuntimeStartupFailureMetadata{})
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
	taskDescription string,
	attachments []MessageAttachment,
	mcpServers []agentctltypes.McpServer,
) (attempted bool, err error) {
	return m.retryManagedRuntimeStartupWithProgress(
		ctx, execution, initErr, agentConfig, taskDescription, attachments, mcpServers, nil,
	)
}

func (m *Manager) retryManagedRuntimeStartupWithProgress(
	ctx context.Context,
	execution *AgentExecution,
	initErr error,
	agentConfig agents.Agent,
	taskDescription string,
	attachments []MessageAttachment,
	mcpServers []agentctltypes.McpServer,
	onRetry func(),
) (attempted bool, err error) {
	if aborted := managedRuntimeRecoveryAborted(ctx, m); aborted != nil {
		return false, aborted
	}
	retry, ok := m.prepareManagedRuntimeStartupRetry(ctx, execution, initErr, agentConfig)
	if !ok {
		return false, initErr
	}
	if err := managedRuntimeRecoveryAborted(ctx, m); err != nil {
		return false, err
	}
	if retry.terminal {
		return true, m.publishManagedRuntimeStartupFailure(
			execution,
			"managed runtime startup was blocked",
			retry.failureCode,
			managedRuntimeStartupFailureDetails(retry.failureReason, 1, retry.failureEvidence),
			managedRuntimeStartupFailureMetadata{reason: retry.failureReason, attempts: 1, npmCode: retry.failureNPMCode},
			initErr,
		)
	}
	recoveryCtx, cancelRecovery := managedRuntimeStartupRecoveryContext(ctx)
	defer cancelRecovery()
	retryGeneration, ok := execution.beginStartupRecovery()
	if !ok {
		return false, initErr
	}
	defer execution.finishStartupRecovery()
	if onRetry != nil {
		onRetry()
	}

	m.logger.Warn("retrying managed runtime startup",
		zap.String("execution_id", execution.ID),
		zap.String("agent_id", execution.AgentID),
		zap.Uint64("startup_generation", retryGeneration))

	// Stop only the child process. The agentctl server remains alive so the
	// same execution can reconnect its streams and retain its identity.
	if needsFailure, err := m.stopManagedRuntimeBeforeRetry(recoveryCtx, execution); err != nil {
		if needsFailure {
			failureDetails := managedRuntimeStartupFailureDetails("cleanup_failed", 1, "")
			return false, m.publishManagedRuntimeStartupFailure(
				execution, "managed runtime process could not be stopped", routingerr.CodeManagedRuntimeStartup, failureDetails,
				managedRuntimeStartupFailureMetadata{reason: "cleanup_failed", attempts: 1}, err,
			)
		}
		return false, err
	}
	if err := managedRuntimeRecoveryAborted(recoveryCtx, m); err != nil {
		return false, err
	}
	if err := m.waitManagedRuntimeStartupBackoff(recoveryCtx); err != nil {
		return false, err
	}

	m.resetManagedRuntimeExecutionForRetry(execution, retry.retryArgs)

	failurePhase, retryErr := m.startManagedRuntimeRetry(
		recoveryCtx,
		execution,
		agentConfig,
		taskDescription,
		attachments,
		mcpServers,
	)
	if retryErr != nil {
		return m.publishManagedRuntimeRetryFailure(recoveryCtx, execution, agentConfig, failurePhase, retryErr)
	}
	return true, nil
}

func (m *Manager) publishManagedRuntimeRetryFailure(
	ctx context.Context,
	execution *AgentExecution,
	agentConfig agents.Agent,
	phase managedRuntimeRetryFailurePhase,
	cause error,
) (bool, error) {
	if aborted := managedRuntimeRecoveryAborted(ctx, m); aborted != nil {
		return false, aborted
	}
	if routingerr.IsAuthenticationFailureDiagnostic(routingerr.Sanitize(cause.Error())) {
		reason := managedRuntimeRetryFailureReason(phase)
		failureDetails := managedRuntimeStartupFailureDetails(
			reason, 2, managedRuntimeStartupFailureEvidenceWithFinalDiagnostic("", cause),
		)
		return true, m.publishManagedRuntimeStartupFailure(
			execution, "managed runtime startup failed", routingerr.CodeAgentRuntime, failureDetails,
			managedRuntimeStartupFailureMetadata{reason: reason, attempts: 2}, cause,
		)
	}
	if phase == managedRuntimeRetryFailureACPInitialize {
		second, recognized := m.prepareManagedRuntimeStartupRetry(ctx, execution, cause, agentConfig)
		if recognized {
			failureDetails := managedRuntimeStartupFailureDetails(
				second.failureReason, 2,
				managedRuntimeStartupFailureEvidenceWithFinalDiagnostic(second.failureEvidence, cause),
			)
			return true, m.publishManagedRuntimeStartupFailure(
				execution, "managed runtime startup failed", second.failureCode, failureDetails,
				managedRuntimeStartupFailureMetadata{reason: second.failureReason, attempts: 2, npmCode: second.failureNPMCode}, cause,
			)
		}
		reason := "retry_initialize_failed"
		failureDetails := managedRuntimeStartupFailureDetails(
			reason, 2, managedRuntimeStartupFailureEvidenceWithFinalDiagnostic("", cause),
		)
		return true, m.publishManagedRuntimeStartupFailure(
			execution, "managed runtime startup failed", routingerr.CodeManagedRuntimeStartup, failureDetails,
			managedRuntimeStartupFailureMetadata{reason: reason, attempts: 2}, cause,
		)
	}
	reason := managedRuntimeRetryFailureReason(phase)
	failureDetails := managedRuntimeStartupFailureDetails(
		reason, 2, managedRuntimeStartupFailureEvidenceWithFinalDiagnostic("", cause),
	)
	return true, m.publishManagedRuntimeStartupFailure(
		execution, "managed runtime startup failed", routingerr.CodeAgentRuntime, failureDetails,
		managedRuntimeStartupFailureMetadata{reason: reason, attempts: 2}, cause,
	)
}
