package lifecycle

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"go.uber.org/zap"

	agentctltypes "github.com/kandev/kandev/internal/agentctl/types"
)

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

	port := passthroughMCPConfigPort(execution)
	if port > 0 {
		newMCPServers[kandevMCPServerName] = map[string]any{
			"url": fmt.Sprintf("http://localhost:%d/mcp", port),
		}
	}

	for _, srv := range allowedProfileServers {
		sName := strings.TrimSpace(srv.Name)
		if sName == "" || strings.EqualFold(sName, kandevMCPServerName) {
			continue
		}
		newMCPServers[sName] = serverToCursorJSON(srv)
	}

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

	for logicalName, srv := range allowedImportCandidates {
		sName := strings.TrimSpace(srv.Name)
		lowerName := strings.ToLower(sName)
		if hasCursorMCPName(newMCPServers, sName) || hasCursorMCPName(newMCPServers, logicalName) || lowerName == kandevMCPServerName {
			continue
		}
		entryVal := serverToCursorJSON(srv)
		newMCPServers[sName] = entryVal
		fp, _ := canonicalJSONFingerprint(entryVal)
		newOwnership[lowerName] = fp
	}

	return newMCPServers, newOwnership
}

func hasCursorMCPName(servers map[string]any, name string) bool {
	for existing := range servers {
		if strings.EqualFold(existing, name) {
			return true
		}
	}
	return false
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
	ownBytes, err := json.MarshalIndent(ownRecord, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal Cursor MCP import ownership: %w", err)
	}
	if err := writeAtomicFile(ownershipPath, ownBytes, 0o600); err != nil {
		return fmt.Errorf("write Cursor MCP import ownership: %w", err)
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
