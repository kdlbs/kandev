package lifecycle

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/executor"
	"github.com/kandev/kandev/internal/agent/mcpconfig"
	agentctltypes "github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/task/models"
)

var cursorProjectMCPMutex sync.Mutex

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
	cursorProjectMCPMutex.Lock()
	defer cursorProjectMCPMutex.Unlock()

	if execution == nil || execution.WorkspacePath == "" || !isCursorMCPAuthStrategy(strategy) {
		return nil
	}
	if !isCursorMCPAuthLocalExecutor(executorType) {
		return nil
	}

	workspaceDir := execution.WorkspacePath
	projectMCPDir := filepath.Join(workspaceDir, ".cursor")
	projectMCPPath := filepath.Join(projectMCPDir, "mcp.json")
	ownershipPath := filepath.Join(projectMCPDir, ".kandev-mcp-imports.json")

	if escapes, err := workspacePathEscapes(workspaceDir, projectMCPPath); err != nil {
		return fmt.Errorf("validate Cursor MCP path: %w", err)
	} else if escapes {
		m.logger.Warn("Cursor MCP config path escapes workspace via symlink; skipping", zap.String("path", projectMCPPath))
		return nil
	}

	// 1. Resolve runtime policy & Profile MCP servers
	policy, allowedProfileServers, reservedProfileNames, err := m.resolveCursorProfileAndPolicy(ctx, execution, profileInfo, executorType)
	if err != nil {
		return err
	}

	// 2. Discover Plugin & Global MCP candidates (if enabled)
	allowedImportCandidates := m.discoverAllowedCursorImportCandidates(execution, profileInfo, policy, reservedProfileNames)

	// 3. Read previous ownership state & existing project MCP file
	topLevelObj, userOwnedProjectServers, fileExists, err := m.readClassifiedProjectMCPServers(projectMCPPath, ownershipPath)
	if err != nil {
		return err
	}
	if topLevelObj == nil && fileExists {
		// symlink or skipped
		return nil
	}

	// 4. Compose effective mcpServers and ownership mapping
	newMCPServers, newOwnership := composeEffectiveMCPServers(
		execution,
		allowedProfileServers,
		userOwnedProjectServers,
		allowedImportCandidates,
	)

	// 5. Write .cursor/mcp.json and .kandev-mcp-imports.json
	return writeCursorProjectMCPAndOwnership(
		projectMCPDir,
		projectMCPPath,
		ownershipPath,
		topLevelObj,
		newMCPServers,
		newOwnership,
		fileExists,
	)
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
	execution *AgentExecution,
	profileInfo *AgentProfileInfo,
	policy mcpconfig.Policy,
	reservedProfileNames map[string]struct{},
) map[string]agentctltypes.McpServer {
	allowedImportCandidates := make(map[string]agentctltypes.McpServer)
	home, ok := canImportCursorPlugins(execution, profileInfo)
	if !ok {
		return allowedImportCandidates
	}

	candidates, err := mcpconfig.DiscoverCursorPluginCandidates(filepath.Join(home, ".cursor"))
	if err != nil && m.logger != nil {
		m.logger.Warn("could not discover local Cursor plugin MCP servers", zap.Error(err))
	}
	for _, cand := range candidates {
		resolveAndAddImportCandidate(allowedImportCandidates, cand, policy, reservedProfileNames)
	}
	return allowedImportCandidates
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
	if _, isReserved := reservedProfileNames[lowerName]; isReserved {
		return
	}
	resolved, _, err := mcpconfig.ResolveSingleServer(cand.Name, cand.ToServerDef(), policy)
	if err != nil || resolved == nil {
		return
	}
	acpList := mcpconfig.ToACPServers([]mcpconfig.ResolvedServer{*resolved})
	if len(acpList) > 0 {
		if _, exists := dest[lowerName]; !exists {
			dest[lowerName] = acpList[0]
		}
	}
}

func (m *Manager) readClassifiedProjectMCPServers(
	projectMCPPath, ownershipPath string,
) (map[string]json.RawMessage, map[string]json.RawMessage, bool, error) {
	previousImported := readPreviousImportOwnership(ownershipPath)

	info, statErr := os.Lstat(projectMCPPath)
	if statErr != nil {
		return make(map[string]json.RawMessage), make(map[string]json.RawMessage), false, nil
	}
	if info.Mode()&os.ModeSymlink != 0 {
		m.logger.Warn("Cursor MCP config is a symlink; leaving it untouched", zap.String("path", projectMCPPath))
		return nil, nil, true, nil
	}

	data, err := os.ReadFile(projectMCPPath)
	if err != nil {
		return nil, nil, true, fmt.Errorf("read Cursor project MCP file: %w", err)
	}

	topLevelObj := make(map[string]json.RawMessage)
	if err := json.Unmarshal(data, &topLevelObj); err != nil {
		return nil, nil, true, fmt.Errorf("invalid Cursor project MCP json: %w", err)
	}

	existingMCPServers := make(map[string]json.RawMessage)
	if rawServers, exists := topLevelObj["mcpServers"]; exists {
		if err := json.Unmarshal(rawServers, &existingMCPServers); err != nil {
			return nil, nil, true, fmt.Errorf("invalid mcpServers in Cursor project MCP: %w", err)
		}
	}

	userOwnedProjectServers := classifyUserOwnedProjectServers(existingMCPServers, previousImported)
	return topLevelObj, userOwnedProjectServers, true, nil
}

func readPreviousImportOwnership(ownershipPath string) map[string]string {
	previousImported := make(map[string]string)
	ownBytes, err := os.ReadFile(ownershipPath)
	if err == nil {
		var ownRecord cursorImportOwnershipRecord
		if err := json.Unmarshal(ownBytes, &ownRecord); err == nil && ownRecord.Imported != nil {
			previousImported = ownRecord.Imported
		}
	}
	return previousImported
}

func classifyUserOwnedProjectServers(
	existingMCPServers map[string]json.RawMessage,
	previousImported map[string]string,
) map[string]json.RawMessage {
	userOwned := make(map[string]json.RawMessage)
	for sName, rawEntry := range existingMCPServers {
		lowerName := strings.ToLower(sName)
		if lowerName == kandevMCPServerName {
			continue
		}
		var parsedVal any
		if err := json.Unmarshal(rawEntry, &parsedVal); err != nil {
			userOwned[sName] = rawEntry
			continue
		}
		currFingerprint, _ := canonicalJSONFingerprint(parsedVal)
		if recordedFingerprint, wasImported := previousImported[lowerName]; wasImported {
			if currFingerprint == recordedFingerprint {
				// Unchanged previously imported server; do not mark as user-owned
				continue
			}
		}
		userOwned[sName] = rawEntry
	}
	return userOwned
}

func composeEffectiveMCPServers(
	execution *AgentExecution,
	allowedProfileServers []agentctltypes.McpServer,
	userOwnedProjectServers map[string]json.RawMessage,
	allowedImportCandidates map[string]agentctltypes.McpServer,
) (map[string]any, map[string]string) {
	newMCPServers := make(map[string]any)
	newOwnership := make(map[string]string)

	// A. Internal Kandev server
	port := passthroughMCPConfigPort(execution)
	if port > 0 {
		newMCPServers[kandevMCPServerName] = map[string]any{
			"url": fmt.Sprintf("http://localhost:%d/mcp", port),
		}
	}

	// B. Explicit Profile Servers
	for _, srv := range allowedProfileServers {
		sName := strings.TrimSpace(srv.Name)
		if sName == "" || strings.EqualFold(sName, kandevMCPServerName) {
			continue
		}
		newMCPServers[sName] = serverToCursorJSON(srv)
	}

	// C. User-Owned Project Servers
	for sName, raw := range userOwnedProjectServers {
		lowerName := strings.ToLower(sName)
		if _, isProfile := newMCPServers[sName]; isProfile || lowerName == kandevMCPServerName {
			continue
		}
		var parsed any
		if err := json.Unmarshal(raw, &parsed); err == nil {
			newMCPServers[sName] = parsed
		}
	}

	// D. Discovered Global & Plugin Servers
	for _, srv := range allowedImportCandidates {
		sName := strings.TrimSpace(srv.Name)
		lowerName := strings.ToLower(sName)
		if _, exists := newMCPServers[sName]; exists || lowerName == kandevMCPServerName {
			continue
		}
		entryVal := serverToCursorJSON(srv)
		newMCPServers[sName] = entryVal
		fp, _ := canonicalJSONFingerprint(entryVal)
		newOwnership[lowerName] = fp
	}

	return newMCPServers, newOwnership
}

func writeCursorProjectMCPAndOwnership(
	projectMCPDir, projectMCPPath, ownershipPath string,
	topLevelObj map[string]json.RawMessage,
	newMCPServers map[string]any,
	newOwnership map[string]string,
	fileExists bool,
) error {
	if !fileExists && len(newMCPServers) == 0 {
		return nil
	}

	if err := os.MkdirAll(projectMCPDir, 0o700); err != nil {
		return fmt.Errorf("create .cursor directory: %w", err)
	}

	if topLevelObj == nil {
		topLevelObj = make(map[string]json.RawMessage)
	}

	rawServersJSON, err := json.Marshal(newMCPServers)
	if err != nil {
		return fmt.Errorf("marshal mcpServers: %w", err)
	}
	topLevelObj["mcpServers"] = rawServersJSON

	finalFileJSON, err := json.MarshalIndent(topLevelObj, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal project MCP: %w", err)
	}

	if err := writeAtomicFile(projectMCPPath, finalFileJSON, 0o600); err != nil {
		return fmt.Errorf("write project MCP file: %w", err)
	}

	ownRecord := cursorImportOwnershipRecord{
		Version:  1,
		Imported: newOwnership,
	}
	if ownBytes, err := json.MarshalIndent(ownRecord, "", "  "); err == nil {
		_ = writeAtomicFile(ownershipPath, ownBytes, 0o600)
	}
	return nil
}

func serverToCursorJSON(srv agentctltypes.McpServer) map[string]any {
	if srv.Command != "" {
		res := map[string]any{
			"type":    "stdio",
			"command": srv.Command,
		}
		if len(srv.Args) > 0 {
			res["args"] = srv.Args
		}
		if len(srv.Env) > 0 {
			res["env"] = srv.Env
		}
		return res
	}

	res := map[string]any{
		"url": srv.URL,
	}
	if len(srv.Headers) > 0 {
		res["headers"] = srv.Headers
	}
	return res
}

func writeAtomicFile(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	cleanup := func() {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
	}
	if _, err := tmp.Write(data); err != nil {
		cleanup()
		return err
	}
	if err := tmp.Chmod(perm); err != nil {
		cleanup()
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	return os.Rename(tmpName, path)
}
