package lifecycle

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
	"unicode"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/mcpconfig"
	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	agentctltypes "github.com/kandev/kandev/internal/agentctl/types"
)

const cursorMCPRecoveryTimeout = 60 * time.Second
const cursorMCPRecoveryReasonStale = "stale"
const cursorMCPAuthenticationWindowsShell = "windows"

var ErrCursorMCPAuthenticationUnsupported = errors.New("cursor MCP authentication terminal is unsupported on this shell")

type cursorMCPRecoveryTarget struct {
	execution   *AgentExecution
	agentConfig agents.Agent
	profile     *AgentProfileInfo
	candidate   mcpconfig.DiscoveredServerCandidate
	serverID    string
	fingerprint string
	cursorHome  string
	sourceRepo  string
	acpServers  []agentctltypes.McpServer
}

// CursorMCPAuthenticationSpec resolves a selected, owned native server into
// the trusted command and task scope used to create its ordinary login shell.
func (m *Manager) CursorMCPAuthenticationSpec(ctx context.Context, sessionID, serverID string) (CursorMCPAuthenticationSpec, error) {
	target, err := m.resolveCursorMCPRecoveryTarget(ctx, sessionID, serverID)
	if err != nil {
		return CursorMCPAuthenticationSpec{}, err
	}
	if target.execution.TaskEnvironmentID == "" || target.execution.TaskID == "" {
		return CursorMCPAuthenticationSpec{}, ErrCursorMCPRecoveryUnavailable
	}
	workspaceKey := cursorProjectMCPWorkspaceKey(target.execution.WorkspacePath)
	release, err := acquireCursorMCPWorkspacePreparation(ctx, workspaceKey)
	if err != nil {
		return CursorMCPAuthenticationSpec{}, ErrCursorMCPRecoveryUnavailable
	}
	defer release()
	if err := m.validateCursorMCPRecoveryTarget(ctx, target, workspaceKey, 0); err != nil {
		return CursorMCPAuthenticationSpec{}, ErrCursorMCPRecoveryUnavailable
	}
	command, err := cursorMCPAuthenticationCommand(cursorNativeMCPExecutable(target.agentConfig), target.serverID, runtime.GOOS)
	if err != nil {
		return CursorMCPAuthenticationSpec{}, err
	}
	return CursorMCPAuthenticationSpec{
		TaskID:            target.execution.TaskID,
		TaskEnvironmentID: target.execution.TaskEnvironmentID,
		WorkspacePath:     target.execution.WorkspacePath,
		InitialCommand:    command,
		Label:             cursorMCPAuthenticationLabel(target.serverID),
		ServerID:          target.serverID,
	}, nil
}

// RetryCursorMCPConnection rechecks one imported server and reloads the same
// ACP session before returning ready. It never starts a new conversation or
// replays a prompt.
func (m *Manager) RetryCursorMCPConnection(ctx context.Context, sessionID, serverID string) (CursorMCPRetryResult, error) {
	ctx, cancel := context.WithTimeout(ctx, cursorMCPRecoveryTimeout)
	defer cancel()
	target, err := m.resolveCursorMCPRecoveryTarget(ctx, sessionID, serverID)
	if err != nil {
		return CursorMCPRetryResult{}, err
	}
	execution := target.execution
	if !execution.remoteInstanceLifecycleMu.TryLock() {
		return CursorMCPRetryResult{}, ErrCursorMCPRecoverySessionBusy
	}
	defer execution.remoteInstanceLifecycleMu.Unlock()
	if cursorMCPRecoverySessionBusy(execution) {
		return CursorMCPRetryResult{}, ErrCursorMCPRecoverySessionBusy
	}
	if execution.PassthroughProcessID != "" {
		return CursorMCPRetryResult{
			ProviderID: "cursor", ServerID: serverID,
			Status: string(mcpconfig.NativeMCPStatusUnavailable), ReasonCode: "session_reload_unsupported",
		}, nil
	}
	if execution.ACPSessionID == "" {
		return CursorMCPRetryResult{}, ErrCursorMCPRecoveryUnavailable
	}
	if err := m.ensureLaunchSessionStillActive(ctx, sessionID, executionAdmissionAgent); err != nil || !m.cursorMCPRecoveryExecutionIsCurrent(target) {
		return CursorMCPRetryResult{}, ErrCursorMCPRecoveryUnavailable
	}
	workspaceKey := cursorProjectMCPWorkspaceKey(execution.WorkspacePath)
	generation := beginCursorProjectMCPPreparation(workspaceKey)
	defer finishCursorProjectMCPPreparation(workspaceKey, generation)
	release, err := acquireCursorMCPWorkspacePreparation(ctx, workspaceKey)
	if err != nil {
		return CursorMCPRetryResult{}, ErrCursorMCPRecoveryUnavailable
	}
	defer release()
	previousSteps := cursorMCPRecoveryPreviousSteps(execution)
	recorder := m.newPreparationAttemptRecorder(execution.TaskID, execution.SessionID)
	seedCursorMCPRecoverySteps(execution, recorder, target.serverID)
	defer m.publishExecutionPrepareCompleted(execution, recorder, nil)
	result, retryErr := m.retryCursorMCPImport(ctx, target, workspaceKey, generation, recorder)
	if errors.Is(retryErr, ErrCursorMCPRecoverySessionBusy) &&
		cursorMCPRecoveryHasTargetSteps(previousSteps, target.serverID) {
		recorder.RestoreSteps(previousSteps)
	}
	return result, retryErr
}

func (m *Manager) resolveCursorMCPRecoveryTarget(ctx context.Context, sessionID, serverID string) (*cursorMCPRecoveryTarget, error) {
	if sessionID == "" || !mcpconfig.ValidNativeMCPServerID(serverID) {
		return nil, ErrCursorMCPRecoveryUnavailable
	}
	execution, agentConfig, profile, home, sourceRepo, err := m.resolveCursorMCPRecoveryContext(ctx, sessionID)
	if err != nil {
		return nil, ErrCursorMCPRecoveryUnavailable
	}
	candidates, err := m.discoverCursorMCPRecoveryCandidates(ctx, home)
	if err != nil {
		return nil, ErrCursorMCPRecoveryUnavailable
	}
	selected := selectCursorMCPRecoveryCandidate(candidates, profile, serverID)
	if selected == nil || cursorMCPDisabledReason(home, sourceRepo, execution.WorkspacePath, serverID) != "" {
		return nil, ErrCursorMCPRecoveryUnavailable
	}
	fingerprint, owned := cursorNativeOwnedDefinitionFingerprint(execution.WorkspacePath, serverID)
	if !owned {
		return nil, ErrCursorMCPRecoveryUnavailable
	}
	acpServers, err := m.resolveMcpServers(ctx, execution, agentConfig)
	if err != nil {
		return nil, ErrCursorMCPRecoveryUnavailable
	}
	return &cursorMCPRecoveryTarget{
		execution: execution, agentConfig: agentConfig, profile: profile,
		candidate: *selected, serverID: serverID, fingerprint: fingerprint,
		cursorHome: filepath.Join(home, ".cursor"), sourceRepo: sourceRepo,
		acpServers: acpServers,
	}, nil
}

func (m *Manager) resolveCursorMCPRecoveryContext(
	ctx context.Context,
	sessionID string,
) (*AgentExecution, agents.Agent, *AgentProfileInfo, string, string, error) {
	if err := m.ensureLaunchSessionStillActive(ctx, sessionID, executionAdmissionAgent); err != nil {
		return nil, nil, nil, "", "", err
	}
	execution, exists := m.executionStore.GetBySessionID(sessionID)
	if !exists || execution.WorkspacePath == "" || !isCursorMCPAuthLocalExecutor(execution.ExecutorType) {
		return nil, nil, nil, "", "", ErrCursorMCPRecoveryUnavailable
	}
	agentConfig, profile, err := m.getAgentConfigAndProfileForExecution(ctx, execution)
	if err != nil || !cursorMCPAgentConfig(agentConfig) {
		return nil, nil, nil, "", "", ErrCursorMCPRecoveryUnavailable
	}
	home, eligible := canImportCursorPlugins(execution, profile)
	if !eligible {
		return nil, nil, nil, "", "", ErrCursorMCPRecoveryUnavailable
	}
	sourceRepo, sourceAvailable := cursorMCPSourceRepository(execution)
	if !sourceAvailable {
		return nil, nil, nil, "", "", ErrCursorMCPRecoveryUnavailable
	}
	return execution, agentConfig, profile, home, sourceRepo, nil
}

func (m *Manager) discoverCursorMCPRecoveryCandidates(ctx context.Context, home string) ([]mcpconfig.DiscoveredServerCandidate, error) {
	inventory, _ := m.loadCursorMCPRecoveryInventory(ctx)
	return mcpconfig.DiscoverCursorPluginCandidatesWithInventory(filepath.Join(home, ".cursor"), inventory)
}

func (m *Manager) loadCursorMCPRecoveryInventory(ctx context.Context) (mcpconfig.CursorNativeInventory, error) {
	if m.cursorInventoryLoader != nil {
		return m.cursorInventoryLoader(ctx)
	}
	return mcpconfig.LoadCursorNativeInventory(ctx)
}

func selectCursorMCPRecoveryCandidate(
	candidates []mcpconfig.DiscoveredServerCandidate,
	profile *AgentProfileInfo,
	serverID string,
) *mcpconfig.DiscoveredServerCandidate {
	for index := range candidates {
		candidate := candidates[index]
		if cursorNativeCandidateServerID(candidate) != serverID {
			continue
		}
		if !cursorMCPProfileSelectsCandidate(profile, candidate) {
			continue
		}
		return &candidate
	}
	return nil
}

func (m *Manager) validateCursorMCPRecoveryTarget(ctx context.Context, target *cursorMCPRecoveryTarget, workspaceKey string, generation uint64) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if generation != 0 && !cursorProjectMCPPreparationIsCurrent(workspaceKey, generation) {
		return ErrCursorMCPRecoveryUnavailable
	}
	if err := m.ensureLaunchSessionStillActive(ctx, target.execution.SessionID, executionAdmissionAgent); err != nil || !m.cursorMCPRecoveryExecutionIsCurrent(target) {
		return ErrCursorMCPRecoveryUnavailable
	}
	candidate, home, sourceRepo, policy, reserved, err := m.resolveCurrentCursorMCPRecoveryCandidate(ctx, target)
	if err != nil {
		return ErrCursorMCPRecoveryUnavailable
	}
	currentFingerprint, err := cursorMCPRecoveryCandidateFingerprint(candidate, policy, reserved, target.serverID)
	if err != nil || currentFingerprint != target.fingerprint {
		return ErrCursorMCPRecoveryUnavailable
	}
	if cursorMCPDisabledReason(home, sourceRepo, target.execution.WorkspacePath, target.serverID) != "" {
		return ErrCursorMCPRecoveryUnavailable
	}
	if !m.cursorNativeMCPDefinitionIsCurrent(ctx, target.execution.WorkspacePath, workspaceKey, generation,
		cursorNativeMCPApprovalTarget{serverID: target.serverID, fingerprint: target.fingerprint}) {
		return ErrCursorMCPRecoveryUnavailable
	}
	return nil
}

func (m *Manager) resolveCurrentCursorMCPRecoveryCandidate(
	ctx context.Context,
	target *cursorMCPRecoveryTarget,
) (mcpconfig.DiscoveredServerCandidate, string, string, mcpconfig.Policy, map[string]struct{}, error) {
	agentConfig, profile, err := m.getAgentConfigAndProfileForExecution(ctx, target.execution)
	if err != nil || !cursorMCPAgentConfig(agentConfig) || profile == nil || !profile.CursorPluginsMCPEnabled {
		return mcpconfig.DiscoveredServerCandidate{}, "", "", mcpconfig.Policy{}, nil, ErrCursorMCPRecoveryUnavailable
	}
	home, eligible := canImportCursorPlugins(target.execution, profile)
	if !eligible || filepath.Clean(filepath.Join(home, ".cursor")) != filepath.Clean(target.cursorHome) {
		return mcpconfig.DiscoveredServerCandidate{}, "", "", mcpconfig.Policy{}, nil, ErrCursorMCPRecoveryUnavailable
	}
	sourceRepo, sourceAvailable := cursorMCPSourceRepository(target.execution)
	if !sourceAvailable || sourceRepo != target.sourceRepo {
		return mcpconfig.DiscoveredServerCandidate{}, "", "", mcpconfig.Policy{}, nil, ErrCursorMCPRecoveryUnavailable
	}
	policy, _, reserved, err := m.resolveCursorProfileAndPolicy(ctx, target.execution, profile, target.execution.ExecutorType)
	if err != nil {
		return mcpconfig.DiscoveredServerCandidate{}, "", "", mcpconfig.Policy{}, nil, ErrCursorMCPRecoveryUnavailable
	}
	if !cursorMCPProfileSelectsCandidate(profile, target.candidate) {
		return mcpconfig.DiscoveredServerCandidate{}, "", "", mcpconfig.Policy{}, nil, ErrCursorMCPRecoveryUnavailable
	}
	return target.candidate, home, sourceRepo, policy, reserved, nil
}

func cursorMCPRecoveryCandidateFingerprint(
	candidate mcpconfig.DiscoveredServerCandidate,
	policy mcpconfig.Policy,
	reserved map[string]struct{},
	serverID string,
) (string, error) {
	currentCandidates := make(map[string]agentctltypes.McpServer, 1)
	resolveAndAddImportCandidate(currentCandidates, candidate, policy, reserved)
	var currentServer *agentctltypes.McpServer
	for _, server := range currentCandidates {
		if server.Name == serverID {
			copy := server
			currentServer = &copy
			break
		}
	}
	if currentServer == nil {
		return "", ErrCursorMCPRecoveryUnavailable
	}
	return canonicalJSONFingerprint(serverToCursorJSON(*currentServer))
}

func (m *Manager) cursorMCPRecoveryExecutionIsCurrent(target *cursorMCPRecoveryTarget) bool {
	if target == nil || target.execution == nil {
		return false
	}
	current, exists := m.executionStore.GetBySessionID(target.execution.SessionID)
	return exists && current == target.execution && current.ID == target.execution.ID &&
		current.WorkspacePath == target.execution.WorkspacePath && isCursorMCPAuthLocalExecutor(current.ExecutorType)
}

func (m *Manager) retryCursorMCPImport(
	ctx context.Context,
	target *cursorMCPRecoveryTarget,
	workspaceKey string,
	generation uint64,
	recorder *prepareProgressRecorder,
) (CursorMCPRetryResult, error) {
	adapter := mcpconfig.CursorNativeMCPAdapter{
		Executable: cursorNativeMCPExecutable(target.agentConfig),
		Runner:     m.cursorNativeMCPRunner,
		Timeout:    20 * time.Second,
	}
	if err := m.validateCursorMCPRecoveryTarget(ctx, target, workspaceKey, generation); err != nil {
		failure := cursorMCPRecoveryFailureCode(target, err)
		appendCursorMCPProgress(recorder, target.serverID, PrepareStepKindAgentMCPApproval, PrepareStepSkipped, failure, nil, nil)
		appendCursorMCPProgress(recorder, target.serverID, PrepareStepKindAgentMCPVerification, PrepareStepSkipped, failure, nil, nil)
		return cursorMCPRetryReadiness(target.serverID, cursorMCPUnavailableReadiness()), nil
	}
	approvalStarted := time.Now().UTC()
	approvalIndex := appendCursorMCPProgress(recorder, target.serverID, PrepareStepKindAgentMCPApproval, PrepareStepRunning, "", &approvalStarted, nil)
	approval := adapter.Enable(ctx, target.execution.WorkspacePath, target.execution.RuntimeEnvironment(), target.serverID)
	approvalEnded := time.Now().UTC()
	if failure := cursorMCPPreparationFence(
		ctx, m, target.execution, workspaceKey, generation,
		cursorNativeMCPApprovalTarget{serverID: target.serverID, fingerprint: target.fingerprint},
		filepath.Dir(target.cursorHome), target.sourceRepo, true,
	); failure != "" {
		diagnostic := cursorMCPFenceDiagnostic(ctx, approval.Diagnostic)
		m.logCursorMCPDiagnostic(target.execution, recorder, target.serverID, diagnostic)
		updateCursorMCPProgress(recorder, approvalIndex, target.serverID, PrepareStepKindAgentMCPApproval, PrepareStepFailed, failure, approvalStarted, approvalEnded, diagnostic)
		appendCursorMCPProgress(recorder, target.serverID, PrepareStepKindAgentMCPVerification, PrepareStepSkipped, failure, &approvalEnded, &approvalEnded)
		return cursorMCPRetryReadiness(target.serverID, cursorMCPFenceReadiness(ctx, approval.Diagnostic)), nil
	}
	if !approval.ApprovalSucceeded {
		failure := nativeMCPReasonCode(approval)
		m.logCursorMCPDiagnostic(target.execution, recorder, target.serverID, approval.Diagnostic)
		updateCursorMCPProgress(recorder, approvalIndex, target.serverID, PrepareStepKindAgentMCPApproval, PrepareStepFailed, failure, approvalStarted, approvalEnded, approval.Diagnostic)
		appendCursorMCPProgress(recorder, target.serverID, PrepareStepKindAgentMCPVerification, PrepareStepSkipped, failure, &approvalEnded, &approvalEnded)
		return cursorMCPRetryReadiness(target.serverID, approval), nil
	}
	updateCursorMCPProgress(recorder, approvalIndex, target.serverID, PrepareStepKindAgentMCPApproval, PrepareStepCompleted, "", approvalStarted, approvalEnded)
	if err := m.validateCursorMCPRecoveryTarget(ctx, target, workspaceKey, generation); err != nil {
		appendCursorMCPProgress(recorder, target.serverID, PrepareStepKindAgentMCPVerification, PrepareStepSkipped, cursorMCPRecoveryFailureCode(target, err), &approvalEnded, &approvalEnded)
		return cursorMCPRetryReadiness(target.serverID, cursorMCPUnavailableReadiness()), nil
	}
	verificationStarted := time.Now().UTC()
	verificationIndex := appendCursorMCPProgress(recorder, target.serverID, PrepareStepKindAgentMCPVerification, PrepareStepRunning, "", &verificationStarted, nil)
	readiness := adapter.Verify(ctx, target.execution.WorkspacePath, target.execution.RuntimeEnvironment(), target.serverID)
	if failure := cursorMCPPreparationFence(
		ctx, m, target.execution, workspaceKey, generation,
		cursorNativeMCPApprovalTarget{serverID: target.serverID, fingerprint: target.fingerprint},
		filepath.Dir(target.cursorHome), target.sourceRepo, true,
	); failure != "" {
		diagnostic := cursorMCPFenceDiagnostic(ctx, readiness.Diagnostic)
		m.logCursorMCPDiagnostic(target.execution, recorder, target.serverID, diagnostic)
		updateCursorMCPProgress(recorder, verificationIndex, target.serverID, PrepareStepKindAgentMCPVerification, PrepareStepFailed, failure, verificationStarted, time.Now().UTC(), diagnostic)
		return cursorMCPRetryReadiness(target.serverID, cursorMCPFenceReadiness(ctx, readiness.Diagnostic)), nil
	}
	if readiness.Status != mcpconfig.NativeMCPStatusReady {
		m.logCursorMCPDiagnostic(target.execution, recorder, target.serverID, readiness.Diagnostic)
		updateCursorMCPProgress(recorder, verificationIndex, target.serverID, PrepareStepKindAgentMCPVerification, PrepareStepFailed, nativeMCPReasonCode(readiness), verificationStarted, time.Now().UTC(), readiness.Diagnostic)
		return cursorMCPRetryReadiness(target.serverID, readiness), nil
	}
	if err := m.validateCursorMCPRecoveryTarget(ctx, target, workspaceKey, generation); err != nil {
		updateCursorMCPProgress(recorder, verificationIndex, target.serverID, PrepareStepKindAgentMCPVerification, PrepareStepFailed, cursorMCPRecoveryFailureCode(target, err), verificationStarted, time.Now().UTC())
		return cursorMCPRetryReadiness(target.serverID, cursorMCPUnavailableReadiness()), nil
	}
	if err := m.reloadCursorMCPACPSession(ctx, target, workspaceKey, generation); err != nil {
		failure := pluginExecutorStateUnavailable
		if errors.Is(err, ErrCursorMCPRecoverySessionBusy) {
			failure = "session_busy"
		}
		updateCursorMCPProgress(recorder, verificationIndex, target.serverID, PrepareStepKindAgentMCPVerification, PrepareStepFailed, failure, verificationStarted, time.Now().UTC())
		if errors.Is(err, ErrCursorMCPRecoverySessionBusy) {
			return CursorMCPRetryResult{}, err
		}
		return cursorMCPRetryReadiness(target.serverID, cursorMCPUnavailableReadiness()), nil
	}
	updateCursorMCPProgress(recorder, verificationIndex, target.serverID, PrepareStepKindAgentMCPVerification, PrepareStepCompleted, "", verificationStarted, time.Now().UTC())
	return cursorMCPRetryReadiness(target.serverID, readiness), nil
}

func (m *Manager) reloadCursorMCPACPSession(ctx context.Context, target *cursorMCPRecoveryTarget, workspaceKey string, generation uint64) error {
	if err := m.validateCursorMCPRecoveryTarget(ctx, target, workspaceKey, generation); err != nil {
		return ErrCursorMCPRecoveryUnavailable
	}
	client, release := target.execution.AcquireAgentCtlClient()
	if client == nil {
		release()
		return ErrCursorMCPRecoveryUnavailable
	}
	defer release()
	target.execution.promptLifecycleMu.Lock()
	defer target.execution.promptLifecycleMu.Unlock()
	if cursorMCPRecoverySessionBusyLocked(target.execution) {
		return ErrCursorMCPRecoverySessionBusy
	}
	if err := m.validateCursorMCPRecoveryTarget(ctx, target, workspaceKey, generation); err != nil {
		return ErrCursorMCPRecoveryUnavailable
	}
	runtimeConfig := m.captureSessionRuntimeConfigForReset(ctx, target.execution)
	if err := client.Stop(ctx); err != nil {
		return ErrCursorMCPRecoveryUnavailable
	}
	if _, err := m.configureAndStartAgent(ctx, target.execution); err != nil {
		return ErrCursorMCPRecoveryUnavailable
	}
	if err := client.WaitForReady(ctx, 30*time.Second); err != nil {
		return ErrCursorMCPRecoveryUnavailable
	}
	if err := m.ensureCursorMCPRecoveryAgentStream(ctx, target.execution, client); err != nil {
		return ErrCursorMCPRecoveryUnavailable
	}
	if err := m.validateCursorMCPRecoveryTarget(ctx, target, workspaceKey, generation); err != nil {
		return ErrCursorMCPRecoveryUnavailable
	}
	if _, err := client.Initialize(ctx, "kandev", "1.0.0"); err != nil {
		return ErrCursorMCPRecoveryUnavailable
	}
	if err := m.validateCursorMCPRecoveryTarget(ctx, target, workspaceKey, generation); err != nil {
		return ErrCursorMCPRecoveryUnavailable
	}
	if err := client.LoadSession(ctx, target.execution.ACPSessionID, target.acpServers); err != nil {
		return ErrCursorMCPRecoveryUnavailable
	}
	if err := m.validateCursorMCPRecoveryTarget(ctx, target, workspaceKey, generation); err != nil {
		return ErrCursorMCPRecoveryUnavailable
	}
	if err := m.restoreSessionRuntimeConfig(ctx, target.execution, target.execution.ACPSessionID, runtimeConfig); err != nil {
		return ErrCursorMCPRecoveryUnavailable
	}
	return m.validateCursorMCPRecoveryTarget(ctx, target, workspaceKey, generation)
}

func (m *Manager) ensureCursorMCPRecoveryAgentStream(ctx context.Context, execution *AgentExecution, client *agentctl.Client) error {
	if client.HasAgentStream() {
		return nil
	}
	if m.streamManager == nil {
		return ErrCursorMCPRecoveryUnavailable
	}
	ready := make(chan struct{})
	m.streamManager.ConnectAll(execution, ready)
	select {
	case <-ready:
		if !client.HasAgentStream() {
			return ErrCursorMCPRecoveryUnavailable
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(10 * time.Second):
		return ErrCursorMCPRecoveryUnavailable
	}
}

func seedCursorMCPRecoverySteps(execution *AgentExecution, recorder *prepareProgressRecorder, targetServerID string) {
	if execution == nil || recorder == nil {
		return
	}
	previous := cursorMCPRecoveryPreviousSteps(execution)
	environmentSteps := make([]PrepareStep, 0, len(previous))
	for _, step := range previous {
		if !strings.HasPrefix(step.Kind, "agent_mcp_") || step.MCPServerID != targetServerID ||
			(step.Kind != PrepareStepKindAgentMCPApproval && step.Kind != PrepareStepKindAgentMCPVerification) {
			environmentSteps = append(environmentSteps, step)
		}
	}
	recorder.SeedSteps(environmentSteps)
	if recorder.callback != nil {
		total := len(environmentSteps) + 2
		for index, step := range environmentSteps {
			recorder.callback(step, index, total)
		}
	}
}

func cursorMCPRecoveryPreviousSteps(execution *AgentExecution) []PrepareStep {
	if execution == nil {
		return nil
	}
	var previous []PrepareStep
	if execution.PrepareResult != nil {
		previous = append([]PrepareStep(nil), execution.PrepareResult.Steps...)
	} else {
		previous = persistedPrepareSteps(execution.MetadataSnapshot())
	}
	for index := range previous {
		if strings.HasPrefix(previous[index].Kind, "agent_mcp_") && previous[index].Diagnostic != nil {
			previous[index].Diagnostic = normalizeCursorMCPDiagnostic(previous[index].Diagnostic)
		}
	}
	return previous
}

func cursorMCPRecoveryHasTargetSteps(steps []PrepareStep, serverID string) bool {
	for _, step := range steps {
		if step.MCPServerID == serverID &&
			(step.Kind == PrepareStepKindAgentMCPApproval || step.Kind == PrepareStepKindAgentMCPVerification) {
			return true
		}
	}
	return false
}

func cursorMCPRecoveryFailureCode(target *cursorMCPRecoveryTarget, _ error) string {
	if target != nil && target.execution != nil {
		if cursorMCPDisabledReason(filepath.Dir(target.cursorHome), target.sourceRepo, target.execution.WorkspacePath, target.serverID) == string(SSHReclaimSkipDisabled) {
			return string(SSHReclaimSkipDisabled)
		}
	}
	return cursorMCPRecoveryReasonStale
}

func cursorMCPAgentConfig(agentConfig agents.Agent) bool {
	if agentConfig == nil {
		return false
	}
	if runtimeConfig := agentConfig.Runtime(); runtimeConfig != nil && isCursorMCPAuthStrategy(runtimeConfig.ProjectMCPStrategy) {
		return true
	}
	passthrough, ok := agentConfig.(agents.PassthroughAgent)
	return ok && isCursorMCPAuthStrategy(passthrough.PassthroughConfig().MCPStrategy)
}

func cursorMCPAuthenticationCommand(executable, serverID, goos string) (string, error) {
	if goos == cursorMCPAuthenticationWindowsShell {
		return "", ErrCursorMCPAuthenticationUnsupported
	}
	return "exec " + shellQuote(executable) + " mcp login " + shellQuote(serverID), nil
}

func cursorNativeOwnedDefinitionFingerprint(workspace, serverID string) (string, bool) {
	cursorProjectMCPMutex.Lock()
	defer cursorProjectMCPMutex.Unlock()
	projectDir := filepath.Join(workspace, ".cursor")
	configPath := filepath.Join(projectDir, "mcp.json")
	if escapes, err := workspacePathEscapes(workspace, configPath); err != nil || escapes {
		return "", false
	}
	info, err := os.Lstat(configPath)
	if err != nil || !info.Mode().IsRegular() {
		return "", false
	}
	data, err := os.ReadFile(configPath)
	if err != nil {
		return "", false
	}
	var config struct {
		Servers map[string]json.RawMessage `json:"mcpServers"`
	}
	if json.Unmarshal(data, &config) != nil {
		return "", false
	}
	raw, exists := config.Servers[serverID]
	if !exists {
		return "", false
	}
	var definition any
	if json.Unmarshal(raw, &definition) != nil {
		return "", false
	}
	fingerprint, err := canonicalJSONFingerprint(definition)
	if err != nil || readPreviousImportOwnership(filepath.Join(projectDir, ".kandev-mcp-imports.json"))[strings.ToLower(serverID)] != fingerprint {
		return "", false
	}
	return fingerprint, true
}

func cursorMCPRecoverySessionBusy(execution *AgentExecution) bool {
	execution.promptLifecycleMu.Lock()
	defer execution.promptLifecycleMu.Unlock()
	return cursorMCPRecoverySessionBusyLocked(execution)
}

func cursorMCPRecoverySessionBusyLocked(execution *AgentExecution) bool {
	return execution.promptGeneration != 0 && execution.promptCompletionGeneration != execution.promptGeneration
}

func cursorMCPRetryReadiness(serverID string, readiness mcpconfig.NativeMCPReadiness) CursorMCPRetryResult {
	result := CursorMCPRetryResult{
		ProviderID: "cursor", ServerID: serverID,
		Status: string(readiness.Status), ReasonCode: readiness.ReasonCode, Diagnostic: normalizeCursorMCPDiagnostic(readiness.Diagnostic),
	}
	if readiness.ToolCount != nil {
		result.ToolCount = *readiness.ToolCount
	}
	return result
}

func cursorMCPUnavailableReadiness() mcpconfig.NativeMCPReadiness {
	return mcpconfig.NativeMCPReadiness{Status: mcpconfig.NativeMCPStatusUnavailable, ReasonCode: pluginExecutorStateUnavailable}
}

func cursorMCPAuthenticationLabel(serverID string) string {
	var label strings.Builder
	label.WriteString("Authenticate ")
	for _, char := range serverID {
		if unicode.IsControl(char) {
			label.WriteByte('_')
			continue
		}
		label.WriteRune(char)
		if label.Len() >= 96 {
			break
		}
	}
	return strings.TrimSpace(label.String())
}
