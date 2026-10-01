package lifecycle

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/executor"
	"github.com/kandev/kandev/internal/agent/mcpconfig"
	agentctltypes "github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/task/models"
)

var cursorProjectMCPMutex sync.Mutex

var cursorProjectMCPGenerations = struct {
	next   uint64
	active map[string]uint64
}{active: make(map[string]uint64)}

var cursorMCPWorkspacePreparationLocks = struct {
	mu    sync.Mutex
	locks map[string]*cursorMCPWorkspacePreparationLock
}{locks: make(map[string]*cursorMCPWorkspacePreparationLock)}

type cursorMCPWorkspacePreparationLock struct {
	semaphore chan struct{}
	users     int
}

const cursorInventoryTimeoutReason = "timeout"
const cursorMCPFailureCodeCanceled = "canceled"

type cursorImportOwnershipRecord struct {
	Version  int               `json:"version"`
	Imported map[string]string `json:"imported"` // map of serverName (lowercase) -> sha256 hex fingerprint
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func canonicalJSONFingerprint(val any) (string, error) {
	bytes, err := json.Marshal(val)
	if err != nil {
		return "", err
	}
	return sha256Hex(bytes), nil
}

func (m *Manager) reconcileAndMaterializeCursorProjectMCP(
	ctx context.Context,
	execution *AgentExecution,
	agentConfig agents.Agent,
	profileInfo *AgentProfileInfo,
	executorType string,
	strategy mcpconfig.PassthroughMCPStrategy,
) error {
	return m.reconcileAndMaterializeCursorProjectMCPWithPreparation(ctx, execution, agentConfig, profileInfo, executorType, strategy, nil)
}

const cursorMCPPreparationTimeout = 60 * time.Second

type cursorNativeMCPApprovalTarget struct {
	serverID    string
	fingerprint string
}

func (m *Manager) reconcileAndMaterializeCursorProjectMCPWithPreparation(
	ctx context.Context,
	execution *AgentExecution,
	agentConfig agents.Agent,
	profileInfo *AgentProfileInfo,
	executorType string,
	strategy mcpconfig.PassthroughMCPStrategy,
	progress *prepareProgressRecorder,
) error {
	if execution == nil || execution.WorkspacePath == "" || !isCursorMCPAuthStrategy(strategy) {
		return nil
	}
	if !isCursorMCPAuthLocalExecutor(executorType) {
		return nil
	}
	prepCtx, cancel := context.WithTimeout(ctx, cursorMCPPreparationTimeout)
	defer cancel()

	workspaceDir := execution.WorkspacePath
	workspaceKey := cursorProjectMCPWorkspaceKey(workspaceDir)
	generation := beginCursorProjectMCPPreparation(workspaceKey)
	defer finishCursorProjectMCPPreparation(workspaceKey, generation)

	// 1. Resolve runtime policy & Profile MCP servers
	policy, allowedProfileServers, reservedProfileNames, err := m.resolveCursorProfileAndPolicy(prepCtx, execution, profileInfo, executorType)
	if err != nil {
		return err
	}

	// 2. Discover Plugin & Global MCP candidates (if enabled)
	allowedImportCandidates := m.discoverAllowedCursorImportCandidates(prepCtx, execution, profileInfo, policy, reservedProfileNames, progress)

	// Inventory, credential and preference I/O stay outside the file mutation lock.
	if err := prepCtx.Err(); err != nil {
		return err
	}
	release, err := acquireCursorMCPWorkspacePreparation(prepCtx, workspaceKey)
	if err != nil {
		return err
	}
	defer release()
	targets, err := m.reconcileCursorProjectMCPFiles(prepCtx, workspaceDir, workspaceKey, generation, execution,
		allowedProfileServers, allowedImportCandidates)
	if err != nil {
		return err
	}
	m.approveCursorProjectMCPImports(prepCtx, execution, agentConfig, profileInfo, workspaceKey, generation, targets, progress)
	return nil
}

func (m *Manager) reconcileCursorProjectMCPFiles(
	ctx context.Context,
	workspaceDir, workspaceKey string,
	generation uint64,
	execution *AgentExecution,
	allowedProfileServers []agentctltypes.McpServer,
	allowedImportCandidates map[string]agentctltypes.McpServer,
) ([]cursorNativeMCPApprovalTarget, error) {
	projectMCPDir := filepath.Join(workspaceDir, ".cursor")
	projectMCPPath := filepath.Join(projectMCPDir, "mcp.json")
	ownershipPath := filepath.Join(projectMCPDir, ".kandev-mcp-imports.json")
	cursorProjectMCPMutex.Lock()
	defer cursorProjectMCPMutex.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !isCurrentCursorProjectMCPPreparation(workspaceKey, generation) {
		return nil, nil
	}
	if escapes, err := workspacePathEscapes(workspaceDir, projectMCPPath); err != nil {
		return nil, fmt.Errorf("validate Cursor MCP path: %w", err)
	} else if escapes {
		return nil, fmt.Errorf("unsafe Cursor MCP config path")
	}
	// 3. Read previous ownership state & existing project MCP file
	topLevelObj, userOwnedProjectServers, fileExists, err := m.readClassifiedProjectMCPServers(projectMCPPath, ownershipPath)
	if err != nil {
		return nil, err
	}
	if topLevelObj == nil && fileExists {
		return nil, fmt.Errorf("unsafe Cursor MCP config path")
	}

	// 4. Compose effective mcpServers and ownership mapping
	newMCPServers, newOwnership := composeEffectiveMCPServers(
		execution,
		allowedProfileServers,
		userOwnedProjectServers,
		allowedImportCandidates,
	)
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// 5. Write .cursor/mcp.json and .kandev-mcp-imports.json
	if err := writeCursorProjectMCPAndOwnership(
		projectMCPDir,
		projectMCPPath,
		ownershipPath,
		topLevelObj,
		newMCPServers,
		newOwnership,
		fileExists,
	); err != nil {
		return nil, err
	}
	return cursorNativeMCPApprovalTargets(newMCPServers, newOwnership), nil
}

func cursorNativeMCPApprovalTargets(servers map[string]any, ownership map[string]string) []cursorNativeMCPApprovalTarget {
	targets := make([]cursorNativeMCPApprovalTarget, 0, len(ownership))
	for exactID := range servers {
		fingerprint, owned := ownership[strings.ToLower(exactID)]
		if !owned || fingerprint == "" {
			continue
		}
		targets = append(targets, cursorNativeMCPApprovalTarget{serverID: exactID, fingerprint: fingerprint})
	}
	sort.Slice(targets, func(i, j int) bool { return targets[i].serverID < targets[j].serverID })
	return targets
}

func (m *Manager) approveCursorProjectMCPImports(
	ctx context.Context,
	execution *AgentExecution,
	agentConfig agents.Agent,
	profileInfo *AgentProfileInfo,
	workspaceKey string,
	generation uint64,
	targets []cursorNativeMCPApprovalTarget,
	progress *prepareProgressRecorder,
) {
	if len(targets) == 0 {
		return
	}
	home, eligible := canImportCursorPlugins(execution, profileInfo)
	sourceRepository, sourceAvailable := cursorMCPSourceRepository(execution)
	adapter := mcpconfig.CursorNativeMCPAdapter{Executable: cursorNativeMCPExecutable(agentConfig), Runner: m.cursorNativeMCPRunner, Timeout: 20 * time.Second}
	for _, target := range targets {
		m.approveCursorProjectMCPImport(ctx, execution, workspaceKey, generation, target, progress, adapter, home, eligible, sourceRepository, sourceAvailable)
	}
}

func (m *Manager) approveCursorProjectMCPImport(
	ctx context.Context,
	execution *AgentExecution,
	workspaceKey string,
	generation uint64,
	target cursorNativeMCPApprovalTarget,
	progress *prepareProgressRecorder,
	adapter mcpconfig.CursorNativeMCPAdapter,
	home string,
	eligible bool,
	sourceRepository string,
	sourceAvailable bool,
) {
	started := time.Now().UTC()
	index := appendCursorMCPProgress(progress, target.serverID, PrepareStepKindAgentMCPApproval, PrepareStepRunning, "", &started, nil)
	if failure := m.cursorMCPApprovalPreflight(ctx, execution, workspaceKey, generation, target, home, eligible, sourceRepository, sourceAvailable); failure != "" {
		finishCursorMCPProgress(progress, index, target.serverID, PrepareStepKindAgentMCPApproval, PrepareStepSkipped, failure, started)
		return
	}
	approval := adapter.Enable(ctx, execution.WorkspacePath, execution.RuntimeEnvironment(), target.serverID)
	ended := time.Now().UTC()
	if !approval.ApprovalSucceeded {
		failure := nativeMCPReasonCode(approval)
		updateCursorMCPProgress(progress, index, target.serverID, PrepareStepKindAgentMCPApproval, PrepareStepFailed, failure, started, ended)
		appendCursorMCPProgress(progress, target.serverID, PrepareStepKindAgentMCPVerification, PrepareStepSkipped, failure, &ended, &ended)
		return
	}
	updateCursorMCPProgress(progress, index, target.serverID, PrepareStepKindAgentMCPApproval, PrepareStepCompleted, "", started, ended)
	m.verifyCursorProjectMCPImport(ctx, execution, workspaceKey, generation, target, progress, adapter, home, sourceRepository, sourceAvailable, ended)
}

func (m *Manager) cursorMCPApprovalPreflight(
	ctx context.Context,
	execution *AgentExecution,
	workspaceKey string,
	generation uint64,
	target cursorNativeMCPApprovalTarget,
	home string,
	eligible bool,
	sourceRepository string,
	sourceAvailable bool,
) string {
	if !eligible || !sourceAvailable {
		return pluginExecutorStateUnavailable
	}
	if err := ctx.Err(); err != nil {
		if errors.Is(err, context.Canceled) {
			return cursorMCPFailureCodeCanceled
		}
		return pluginExecutorStateUnavailable
	}
	if failure := cursorMCPDisabledReason(home, sourceRepository, execution.WorkspacePath, target.serverID); failure != "" {
		return failure
	}
	if !m.cursorNativeMCPDefinitionIsCurrent(ctx, execution.WorkspacePath, workspaceKey, generation, target) {
		return cursorMCPRecoveryReasonStale
	}
	return ""
}

func (m *Manager) verifyCursorProjectMCPImport(
	ctx context.Context,
	execution *AgentExecution,
	workspaceKey string,
	generation uint64,
	target cursorNativeMCPApprovalTarget,
	progress *prepareProgressRecorder,
	adapter mcpconfig.CursorNativeMCPAdapter,
	home, sourceRepository string,
	sourceAvailable bool,
	approvalEnded time.Time,
) {
	if !m.cursorNativeMCPDefinitionIsCurrent(ctx, execution.WorkspacePath, workspaceKey, generation, target) {
		appendCursorMCPProgress(progress, target.serverID, PrepareStepKindAgentMCPVerification, PrepareStepSkipped, cursorMCPRecoveryReasonStale, &approvalEnded, &approvalEnded)
		return
	}
	started := time.Now().UTC()
	index := appendCursorMCPProgress(progress, target.serverID, PrepareStepKindAgentMCPVerification, PrepareStepRunning, "", &started, nil)
	readiness := adapter.Verify(ctx, execution.WorkspacePath, execution.RuntimeEnvironment(), target.serverID)
	ended := time.Now().UTC()
	failure := cursorMCPVerificationFence(ctx, m, execution, workspaceKey, generation, target, home, sourceRepository, sourceAvailable)
	if failure != "" {
		updateCursorMCPProgress(progress, index, target.serverID, PrepareStepKindAgentMCPVerification, PrepareStepFailed, failure, started, ended)
		return
	}
	status := PrepareStepFailed
	if readiness.Status == mcpconfig.NativeMCPStatusReady {
		status = PrepareStepCompleted
	}
	updateCursorMCPProgress(progress, index, target.serverID, PrepareStepKindAgentMCPVerification, status, nativeMCPReasonCode(readiness), started, ended)
}

func cursorMCPVerificationFence(
	ctx context.Context,
	manager *Manager,
	execution *AgentExecution,
	workspaceKey string,
	generation uint64,
	target cursorNativeMCPApprovalTarget,
	home, sourceRepository string,
	sourceAvailable bool,
) string {
	if err := ctx.Err(); err != nil {
		if errors.Is(err, context.Canceled) {
			return cursorMCPFailureCodeCanceled
		}
		return pluginExecutorStateUnavailable
	}
	if !manager.cursorNativeMCPDefinitionIsCurrent(ctx, execution.WorkspacePath, workspaceKey, generation, target) {
		return cursorMCPRecoveryReasonStale
	}
	if !sourceAvailable {
		return pluginExecutorStateUnavailable
	}
	return cursorMCPDisabledReason(home, sourceRepository, execution.WorkspacePath, target.serverID)
}

func cursorMCPDisabledReason(home, sourceRepository, workspace, serverID string) string {
	disabled, err := mcpconfig.ReadCursorMCPDisabledServers(filepath.Join(home, ".cursor"), sourceRepository, workspace)
	if err != nil {
		return pluginExecutorStateUnavailable
	}
	if _, vetoed := disabled[serverID]; vetoed {
		return string(SSHReclaimSkipDisabled)
	}
	return ""
}

func cursorNativeMCPExecutable(agentConfig agents.Agent) string {
	if agentConfig != nil {
		if runtimeConfig := agentConfig.Runtime(); runtimeConfig != nil {
			if args := runtimeConfig.Cmd.Args(); len(args) > 0 && strings.TrimSpace(args[0]) != "" {
				return args[0]
			}
		}
	}
	return "cursor-agent"
}

func (m *Manager) cursorNativeMCPDefinitionIsCurrent(
	ctx context.Context,
	workspaceDir, workspaceKey string,
	generation uint64,
	target cursorNativeMCPApprovalTarget,
) bool {
	cursorProjectMCPMutex.Lock()
	defer cursorProjectMCPMutex.Unlock()
	if ctx.Err() != nil || (generation != 0 && !isCurrentCursorProjectMCPPreparation(workspaceKey, generation)) {
		return false
	}
	projectMCPPath := filepath.Join(workspaceDir, ".cursor", "mcp.json")
	ownershipPath := filepath.Join(workspaceDir, ".cursor", ".kandev-mcp-imports.json")
	if escapes, err := workspacePathEscapes(workspaceDir, projectMCPPath); err != nil || escapes {
		return false
	}
	info, err := os.Lstat(projectMCPPath)
	if err != nil || !info.Mode().IsRegular() {
		return false
	}
	data, err := os.ReadFile(projectMCPPath)
	if err != nil {
		return false
	}
	var topLevel map[string]json.RawMessage
	if json.Unmarshal(data, &topLevel) != nil {
		return false
	}
	var servers map[string]json.RawMessage
	if json.Unmarshal(topLevel["mcpServers"], &servers) != nil {
		return false
	}
	rawEntry, exists := servers[target.serverID]
	if !exists {
		return false
	}
	var entry any
	if json.Unmarshal(rawEntry, &entry) != nil {
		return false
	}
	fingerprint, err := canonicalJSONFingerprint(entry)
	if err != nil || fingerprint != target.fingerprint {
		return false
	}
	return readPreviousImportOwnership(ownershipPath)[strings.ToLower(target.serverID)] == target.fingerprint
}

func appendCursorMCPProgress(
	progress *prepareProgressRecorder,
	serverID, kind string,
	status PrepareStepStatus,
	failureCode string,
	startedAt, endedAt *time.Time,
) int {
	if progress == nil {
		return -1
	}
	return progress.AppendStep(PrepareStep{
		Name:        "Cursor MCP " + strings.TrimPrefix(kind, "agent_mcp_"),
		Kind:        kind,
		MCPProvider: "cursor",
		MCPServerID: serverID,
		Status:      status,
		FailureCode: failureCode,
		StartedAt:   startedAt,
		EndedAt:     endedAt,
	})
}

func finishCursorMCPProgress(progress *prepareProgressRecorder, index int, serverID, kind string, status PrepareStepStatus, failureCode string, startedAt time.Time) {
	updateCursorMCPProgress(progress, index, serverID, kind, status, failureCode, startedAt, time.Now().UTC())
}

func updateCursorMCPProgress(progress *prepareProgressRecorder, index int, serverID, kind string, status PrepareStepStatus, failureCode string, startedAt, endedAt time.Time) {
	if progress == nil || index < 0 {
		return
	}
	progress.UpdateStep(index, PrepareStep{
		Name:        "Cursor MCP " + strings.TrimPrefix(kind, "agent_mcp_"),
		Kind:        kind,
		MCPProvider: "cursor",
		MCPServerID: serverID,
		Status:      status,
		FailureCode: failureCode,
		StartedAt:   &startedAt,
		EndedAt:     &endedAt,
	})
}

func nativeMCPReasonCode(readiness mcpconfig.NativeMCPReadiness) string {
	if readiness.ReasonCode != "" {
		return readiness.ReasonCode
	}
	if readiness.Status == mcpconfig.NativeMCPStatusReady {
		return ""
	}
	return "connection_failed"
}

func (m *Manager) resolveCursorProfileAndPolicy(
	ctx context.Context,
	execution *AgentExecution,
	profileInfo *AgentProfileInfo,
	executorType string,
) (mcpconfig.Policy, []agentctltypes.McpServer, map[string]struct{}, error) {
	backendName := executor.ExecutorTypeToBackend(models.ExecutorType(executorType))
	policy := mcpconfig.DefaultPolicyForRuntime(backendName)
	if profileInfo != nil {
		var err error
		policy, _, err = m.applyExecutorMcpPolicy(profileInfo.ProfileID, "", execution.MetadataSnapshot(), policy)
		if err != nil {
			return policy, nil, nil, err
		}
	}

	reservedProfileNames := make(map[string]struct{})
	var allowedProfileServers []agentctltypes.McpServer
	if profileInfo != nil && m.mcpProvider != nil {
		rawConfig, err := m.mcpProvider.GetConfigByProfileID(ctx, profileInfo.ProfileID)
		if err == nil && rawConfig != nil {
			for name := range rawConfig.Servers {
				reservedProfileNames[strings.ToLower(strings.TrimSpace(name))] = struct{}{}
			}
			resolved, warnings, err := mcpconfig.Resolve(rawConfig, policy)
			if err == nil {
				for _, w := range warnings {
					m.logger.Warn("mcp config warning", zap.String("warning", w))
				}
				allowedProfileServers = mcpconfig.ToACPServers(resolved)
			}
		}
	}
	return policy, allowedProfileServers, reservedProfileNames, nil
}

func (m *Manager) discoverAllowedCursorImportCandidates(
	ctx context.Context,
	execution *AgentExecution,
	profileInfo *AgentProfileInfo,
	policy mcpconfig.Policy,
	reservedProfileNames map[string]struct{},
	progress *prepareProgressRecorder,
) map[string]agentctltypes.McpServer {
	allowedImportCandidates := make(map[string]agentctltypes.McpServer)
	home, ok := canImportCursorPlugins(execution, profileInfo)
	if !ok {
		return allowedImportCandidates
	}
	sourceRepository, sourceAvailable := cursorMCPSourceRepository(execution)
	if !sourceAvailable {
		m.logger.Warn("Cursor MCP source repository context unavailable; skipping automatic imports")
		return allowedImportCandidates
	}
	disabledServers, err := mcpconfig.ReadCursorMCPDisabledServers(filepath.Join(home, ".cursor"), sourceRepository, execution.WorkspacePath)
	if err != nil {
		m.logger.Warn("Cursor MCP disabled preferences unavailable; skipping automatic imports")
		appendCursorMCPProgress(progress, "", PrepareStepKindAgentMCPDiscovery, PrepareStepFailed, pluginExecutorStateUnavailable, nil, nil)
		return allowedImportCandidates
	}
	var inventory mcpconfig.CursorNativeInventory
	if m.cursorInventoryLoader != nil {
		inventory, err = m.cursorInventoryLoader(ctx)
	} else {
		inventory, err = mcpconfig.LoadCursorNativeInventory(ctx)
	}
	if err != nil {
		// Keep independently configured local and global definitions available.
		inventory = mcpconfig.CursorNativeInventory{}
		m.logger.Warn("Cursor native plugin inventory unavailable; skipping marketplace imports",
			zap.String("reason", cursorInventoryFailureReason(err)))
		appendCursorMCPProgress(progress, "", PrepareStepKindAgentMCPDiscovery, PrepareStepFailed, cursorInventoryFailureReason(err), nil, nil)
	}
	inventoryAvailable := err == nil

	candidates, discoveryErr := mcpconfig.DiscoverCursorPluginCandidatesWithInventory(filepath.Join(home, ".cursor"), inventory)
	if discoveryErr != nil && m.logger != nil {
		m.logger.Warn("could not discover local Cursor plugin MCP servers", zap.Error(discoveryErr))
		appendCursorMCPProgress(progress, "", PrepareStepKindAgentMCPDiscovery, PrepareStepFailed, pluginExecutorStateUnavailable, nil, nil)
	}
	if len(candidates) == 0 && inventoryAvailable && discoveryErr == nil {
		appendCursorMCPProgress(progress, "", PrepareStepKindAgentMCPDiscovery, PrepareStepCompleted, "", nil, nil)
	}
	for _, cand := range candidates {
		serverID := cursorNativeCandidateServerID(cand)
		appendCursorMCPProgress(progress, serverID, PrepareStepKindAgentMCPDiscovery, PrepareStepCompleted, "", nil, nil)
		selectionStarted := time.Now().UTC()
		selectionIndex := appendCursorMCPProgress(progress, serverID, PrepareStepKindAgentMCPSelection, PrepareStepRunning, "", &selectionStarted, nil)
		if !cursorMCPProfileSelectsCandidate(profileInfo, cand) {
			finishCursorMCPProgress(progress, selectionIndex, serverID, PrepareStepKindAgentMCPSelection, PrepareStepSkipped, "not_selected", selectionStarted)
			continue
		}
		if cursorImportCandidateDisabled(cand, disabledServers) {
			finishCursorMCPProgress(progress, selectionIndex, serverID, PrepareStepKindAgentMCPSelection, PrepareStepSkipped, string(SSHReclaimSkipDisabled), selectionStarted)
			continue
		}
		before := len(allowedImportCandidates)
		resolveAndAddImportCandidate(allowedImportCandidates, cand, policy, reservedProfileNames)
		if len(allowedImportCandidates) > before {
			finishCursorMCPProgress(progress, selectionIndex, serverID, PrepareStepKindAgentMCPSelection, PrepareStepCompleted, "", selectionStarted)
		} else {
			finishCursorMCPProgress(progress, selectionIndex, serverID, PrepareStepKindAgentMCPSelection, PrepareStepSkipped, pluginExecutorStateUnavailable, selectionStarted)
		}
	}
	return allowedImportCandidates
}

func cursorMCPProfileSelectsCandidate(profileInfo *AgentProfileInfo, candidate mcpconfig.DiscoveredServerCandidate) bool {
	if profileInfo == nil {
		return true
	}
	mode := strings.TrimSpace(profileInfo.MCPSelectionMode)
	if mode == "" || mode == "inherit" {
		return true
	}
	if mode != "selected" {
		return false
	}
	want := cursorNativeCandidateServerID(candidate)
	for _, selected := range profileInfo.MCPSelectedServers {
		if selected == want {
			return true
		}
	}
	return false
}

func cursorNativeCandidateServerID(candidate mcpconfig.DiscoveredServerCandidate) string {
	if candidate.SourceKind == mcpconfig.SourceKindPlugin && candidate.PluginName != "" {
		return cursorNativePluginServerName(candidate.PluginName, candidate.Name)
	}
	return candidate.Name
}

func cursorImportCandidateDisabled(candidate mcpconfig.DiscoveredServerCandidate, disabled map[string]struct{}) bool {
	name := strings.TrimSpace(candidate.Name)
	if candidate.SourceKind == mcpconfig.SourceKindPlugin && candidate.PluginName != "" {
		name = cursorNativePluginServerName(candidate.PluginName, name)
	}
	_, isDisabled := disabled[name]
	return isDisabled
}

func cursorMCPSourceRepository(execution *AgentExecution) (string, bool) {
	metadata := execution.MetadataSnapshot()
	if rawConfigured, exists := metadata[MetadataKeyRepositoryConfigured]; exists {
		configured, valid := rawConfigured.(bool)
		if !valid {
			return "", false
		}
		if !configured {
			return "", true
		}
		path := getMetadataString(metadata, MetadataKeyRepositoryPath)
		return path, filepath.IsAbs(path)
	}
	if path := getMetadataString(metadata, MetadataKeyRepositoryPath); path != "" {
		return path, filepath.IsAbs(path)
	}
	for _, key := range []string{MetadataKeyMainRepoGitDir, MetadataKeyWorktreeID, MetadataKeyWorktreeBranch} {
		if getMetadataString(metadata, key) != "" {
			return "", false
		}
	}
	return "", true
}

func cursorProjectMCPWorkspaceKey(workspace string) string {
	absolute, err := filepath.Abs(workspace)
	if err != nil {
		return filepath.Clean(workspace)
	}
	if canonical, err := filepath.EvalSymlinks(absolute); err == nil {
		absolute = canonical
	}
	return filepath.Clean(absolute)
}

func cursorInventoryFailureReason(err error) string {
	var inventoryErr *mcpconfig.CursorInventoryError
	if !errors.As(err, &inventoryErr) || inventoryErr == nil {
		return pluginExecutorStateUnavailable
	}
	switch inventoryErr.Reason {
	case "credential_reader_unavailable", "credentials_unavailable", cursorInventoryTimeoutReason, cursorMCPFailureCodeCanceled,
		"request_invalid", "request_failed", "service_status", "response_unreadable",
		"response_too_large", "response_invalid":
		return inventoryErr.Reason
	default:
		return pluginExecutorStateUnavailable
	}
}

func beginCursorProjectMCPPreparation(workspace string) uint64 {
	cursorProjectMCPMutex.Lock()
	defer cursorProjectMCPMutex.Unlock()
	cursorProjectMCPGenerations.next++
	generation := cursorProjectMCPGenerations.next
	cursorProjectMCPGenerations.active[workspace] = generation
	return generation
}

func isCurrentCursorProjectMCPPreparation(workspace string, generation uint64) bool {
	return cursorProjectMCPGenerations.active[workspace] == generation
}

func cursorProjectMCPPreparationIsCurrent(workspace string, generation uint64) bool {
	cursorProjectMCPMutex.Lock()
	defer cursorProjectMCPMutex.Unlock()
	return isCurrentCursorProjectMCPPreparation(workspace, generation)
}

func finishCursorProjectMCPPreparation(workspace string, generation uint64) {
	cursorProjectMCPMutex.Lock()
	defer cursorProjectMCPMutex.Unlock()
	if isCurrentCursorProjectMCPPreparation(workspace, generation) {
		delete(cursorProjectMCPGenerations.active, workspace)
	}
}

func acquireCursorMCPWorkspacePreparation(ctx context.Context, workspace string) (func(), error) {
	cursorMCPWorkspacePreparationLocks.mu.Lock()
	entry := cursorMCPWorkspacePreparationLocks.locks[workspace]
	if entry == nil {
		entry = &cursorMCPWorkspacePreparationLock{semaphore: make(chan struct{}, 1)}
		entry.semaphore <- struct{}{}
		cursorMCPWorkspacePreparationLocks.locks[workspace] = entry
	}
	entry.users++
	cursorMCPWorkspacePreparationLocks.mu.Unlock()

	select {
	case <-ctx.Done():
		releaseCursorMCPWorkspacePreparationReference(workspace, entry)
		return nil, ctx.Err()
	case <-entry.semaphore:
	}
	if err := ctx.Err(); err != nil {
		entry.semaphore <- struct{}{}
		releaseCursorMCPWorkspacePreparationReference(workspace, entry)
		return nil, err
	}
	return func() {
		entry.semaphore <- struct{}{}
		releaseCursorMCPWorkspacePreparationReference(workspace, entry)
	}, nil
}

func releaseCursorMCPWorkspacePreparationReference(workspace string, entry *cursorMCPWorkspacePreparationLock) {
	cursorMCPWorkspacePreparationLocks.mu.Lock()
	defer cursorMCPWorkspacePreparationLocks.mu.Unlock()
	entry.users--
	if entry.users == 0 && cursorMCPWorkspacePreparationLocks.locks[workspace] == entry {
		delete(cursorMCPWorkspacePreparationLocks.locks, workspace)
	}
}

func canImportCursorPlugins(execution *AgentExecution, profileInfo *AgentProfileInfo) (string, bool) {
	home, homeErr := os.UserHomeDir()
	if homeErr != nil || home == "" || profileInfo == nil || !profileInfo.CursorPluginsMCPEnabled {
		return "", false
	}
	if runtimeHome, exists := execution.RuntimeEnvironment()["HOME"]; exists && !sameCursorMCPAuthHome(home, runtimeHome) {
		return "", false
	}
	return home, true
}

func resolveAndAddImportCandidate(
	dest map[string]agentctltypes.McpServer,
	cand mcpconfig.DiscoveredServerCandidate,
	policy mcpconfig.Policy,
	reservedProfileNames map[string]struct{},
) {
	sName := strings.TrimSpace(cand.Name)
	if sName == "" || strings.EqualFold(sName, kandevMCPServerName) {
		return
	}
	lowerName := strings.ToLower(sName)
	nativeName := sName
	if cand.SourceKind == mcpconfig.SourceKindPlugin && cand.PluginName != "" {
		// Cursor indexes plugin OAuth credentials by this native identifier.
		nativeName = cursorNativePluginServerName(cand.PluginName, sName)
	}
	if _, isReserved := reservedProfileNames[strings.ToLower(nativeName)]; isReserved {
		return
	}
	if _, isReserved := reservedProfileNames[lowerName]; isReserved {
		return
	}
	resolved, _, err := mcpconfig.ResolveSingleServer(cand.Name, cand.ToServerDef(), policy)
	if err != nil || resolved == nil {
		return
	}
	for _, existing := range dest {
		if strings.EqualFold(existing.Name, nativeName) {
			return
		}
	}
	acpList := mcpconfig.ToACPServers([]mcpconfig.ResolvedServer{*resolved})
	if len(acpList) > 0 {
		acpList[0].Name = nativeName
		if _, exists := dest[lowerName]; !exists {
			dest[lowerName] = acpList[0]
		}
	}
}

func cursorNativePluginServerName(pluginName, serverName string) string {
	return "plugin-" + pluginName + "-" + serverName
}
