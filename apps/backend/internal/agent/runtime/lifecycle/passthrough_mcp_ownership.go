package lifecycle

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"

	"go.uber.org/zap"
)

const metadataKeyPassthroughMCPClaims = "passthrough_mcp_claims"

// passthroughMCPFileClaim is deliberately limited to ownership facts. It does
// not contain MCP configuration, URLs, headers, or any credential-bearing
// value. The fingerprint is the generation guard used before teardown.
type passthroughMCPFileClaim struct {
	Path        string `json:"path"`
	Fingerprint string `json:"fingerprint"`
	Owned       bool   `json:"owned"`
}

func passthroughMCPFingerprint(content []byte) string {
	digest := sha256.Sum256(content)
	return hex.EncodeToString(digest[:])
}

func passthroughMCPClaimPath(path string) string {
	if identity, err := passthroughMCPFileLockIdentity(path); err == nil {
		return identity
	}
	return filepath.Clean(path)
}

func getPassthroughMCPClaims(execution *AgentExecution) []passthroughMCPFileClaim {
	if execution == nil {
		return nil
	}
	value, _ := execution.metadataValue(metadataKeyPassthroughMCPClaims)
	switch claims := value.(type) {
	case []passthroughMCPFileClaim:
		return append([]passthroughMCPFileClaim(nil), claims...)
	case []interface{}:
		out := make([]passthroughMCPFileClaim, 0, len(claims))
		for _, item := range claims {
			encoded, err := json.Marshal(item)
			if err != nil {
				continue
			}
			var claim passthroughMCPFileClaim
			if err := json.Unmarshal(encoded, &claim); err != nil || claim.Path == "" || claim.Fingerprint == "" {
				continue
			}
			claim.Path = passthroughMCPClaimPath(claim.Path)
			out = append(out, claim)
		}
		return out
	default:
		return nil
	}
}

func setPassthroughMCPClaims(execution *AgentExecution, claims []passthroughMCPFileClaim) {
	if execution == nil {
		return
	}
	if len(claims) == 0 {
		execution.deleteMetadataValues(metadataKeyPassthroughMCPClaims)
		return
	}
	execution.setMetadataValue(metadataKeyPassthroughMCPClaims, append([]passthroughMCPFileClaim(nil), claims...))
}

func passthroughMCPExecutionKey(execution *AgentExecution) string {
	if execution == nil {
		return ""
	}
	return execution.ID
}

func (m *Manager) recordPassthroughMCPClaimLocked(execution *AgentExecution, claim passthroughMCPFileClaim) {
	if execution == nil || claim.Path == "" || claim.Fingerprint == "" || !claim.Owned {
		return
	}
	claim.Path = passthroughMCPClaimPath(claim.Path)
	claims := getPassthroughMCPClaims(execution)
	found := false
	for index := range claims {
		if passthroughMCPClaimPath(claims[index].Path) != claim.Path {
			continue
		}
		claims[index] = claim
		found = true
		break
	}
	if !found {
		claims = append(claims, claim)
	}
	setPassthroughMCPClaims(execution, claims)

	if m.passthroughMCPActiveClaims == nil {
		m.passthroughMCPActiveClaims = make(map[string]map[string]passthroughMCPFileClaim)
	}
	if m.passthroughMCPActiveExecutions == nil {
		m.passthroughMCPActiveExecutions = make(map[string]map[string]*AgentExecution)
	}
	pathClaims := m.passthroughMCPActiveClaims[claim.Path]
	if pathClaims == nil {
		pathClaims = make(map[string]passthroughMCPFileClaim)
		m.passthroughMCPActiveClaims[claim.Path] = pathClaims
	}
	executionKey := passthroughMCPExecutionKey(execution)
	pathClaims[executionKey] = claim
	pathExecutions := m.passthroughMCPActiveExecutions[claim.Path]
	if pathExecutions == nil {
		pathExecutions = make(map[string]*AgentExecution)
		m.passthroughMCPActiveExecutions[claim.Path] = pathExecutions
	}
	pathExecutions[executionKey] = execution
}

func (m *Manager) forgetPassthroughMCPClaimLocked(execution *AgentExecution, path string) {
	if execution == nil {
		return
	}
	path = passthroughMCPClaimPath(path)
	claims := getPassthroughMCPClaims(execution)
	filtered := claims[:0]
	for _, claim := range claims {
		if passthroughMCPClaimPath(claim.Path) != path {
			filtered = append(filtered, claim)
		}
	}
	setPassthroughMCPClaims(execution, filtered)

	if pathClaims := m.passthroughMCPActiveClaims[path]; pathClaims != nil {
		executionKey := passthroughMCPExecutionKey(execution)
		delete(pathClaims, executionKey)
		if len(pathClaims) == 0 {
			delete(m.passthroughMCPActiveClaims, path)
		}
	}
	if pathExecutions := m.passthroughMCPActiveExecutions[path]; pathExecutions != nil {
		delete(pathExecutions, passthroughMCPExecutionKey(execution))
		if len(pathExecutions) == 0 {
			delete(m.passthroughMCPActiveExecutions, path)
		}
	}
}

func updatePassthroughMCPClaimFingerprint(execution *AgentExecution, path, fingerprint string) bool {
	claims := getPassthroughMCPClaims(execution)
	updated := false
	for index := range claims {
		if claims[index].Owned && passthroughMCPClaimPath(claims[index].Path) == path {
			claims[index].Fingerprint = fingerprint
			updated = true
		}
	}
	if updated {
		setPassthroughMCPClaims(execution, claims)
	}
	return updated
}

// adoptPassthroughMCPGenerationLocked updates every active claim for a path
// when a known Kandev-owned project file is replaced by a successor. Keeping
// all related executions on the same current fingerprint means either stop
// order is safe: the file remains until the last related execution releases
// the current generation. Tracked executions are persisted so the same rule
// survives a backend restart.
func (m *Manager) adoptPassthroughMCPGenerationLocked(path, fingerprint string) {
	path = passthroughMCPClaimPath(path)
	updated := m.updateActivePassthroughMCPClaimsLocked(path, fingerprint)
	if m.executionStore == nil {
		return
	}
	for _, execution := range m.executionStore.List() {
		if execution == nil || !updatePassthroughMCPClaimFingerprint(execution, path, fingerprint) {
			continue
		}
		updated[execution.ID] = execution
		m.recordPassthroughMCPClaimLocked(execution, passthroughMCPFileClaim{
			Path: path, Fingerprint: fingerprint, Owned: true,
		})
	}
	for _, execution := range updated {
		if _, tracked := m.executionStore.Get(execution.ID); tracked {
			m.persistExecutorRunning(context.Background(), execution)
		}
	}
}

func (m *Manager) updateActivePassthroughMCPClaimsLocked(path, fingerprint string) map[string]*AgentExecution {
	updated := make(map[string]*AgentExecution)
	pathClaims := m.passthroughMCPActiveClaims[path]
	if pathClaims == nil {
		return updated
	}
	pathExecutions := m.passthroughMCPActiveExecutions[path]
	for executionID, claim := range pathClaims {
		if !claim.Owned {
			continue
		}
		claim.Fingerprint = fingerprint
		pathClaims[executionID] = claim
		if pathExecutions == nil {
			continue
		}
		execution := pathExecutions[executionID]
		if execution != nil && updatePassthroughMCPClaimFingerprint(execution, path, fingerprint) {
			updated[executionID] = execution
		}
	}
	return updated
}

func (m *Manager) knownPassthroughMCPGenerationLocked(path, fingerprint string) bool {
	path = passthroughMCPClaimPath(path)
	if pathClaims := m.passthroughMCPActiveClaims[path]; pathClaims != nil {
		for _, claim := range pathClaims {
			if claim.Owned && claim.Fingerprint == fingerprint {
				return true
			}
		}
	}
	if m.executionStore == nil {
		return false
	}
	for _, execution := range m.executionStore.List() {
		for _, claim := range getPassthroughMCPClaims(execution) {
			if claim.Owned && passthroughMCPClaimPath(claim.Path) == path && claim.Fingerprint == fingerprint {
				return true
			}
		}
	}
	return false
}

func (m *Manager) hasOtherPassthroughMCPClaimLocked(path, executionID, fingerprint string) bool {
	path = passthroughMCPClaimPath(path)
	if pathClaims := m.passthroughMCPActiveClaims[path]; pathClaims != nil {
		for id, claim := range pathClaims {
			if id != executionID && claim.Owned && claim.Fingerprint == fingerprint {
				return true
			}
		}
	}
	if m.executionStore == nil {
		return false
	}
	for _, execution := range m.executionStore.List() {
		if execution == nil || execution.ID == executionID {
			continue
		}
		for _, claim := range getPassthroughMCPClaims(execution) {
			if claim.Owned && passthroughMCPClaimPath(claim.Path) == path && claim.Fingerprint == fingerprint {
				return true
			}
		}
	}
	return false
}

func (m *Manager) cleanupPassthroughMCPClaimLocked(execution *AgentExecution, claim passthroughMCPFileClaim) {
	if execution == nil || !claim.Owned || claim.Path == "" || claim.Fingerprint == "" {
		return
	}
	path := passthroughMCPClaimPath(claim.Path)
	info, err := os.Lstat(path)
	if err != nil {
		if !os.IsNotExist(err) {
			m.logger.Warn("failed to inspect owned passthrough MCP config",
				zap.String("path", path), zap.Error(err))
		}
		return
	}
	if !info.Mode().IsRegular() {
		m.logger.Warn("refusing to remove non-regular passthrough MCP config",
			zap.String("path", path))
		return
	}
	content, err := os.ReadFile(path)
	if err != nil {
		m.logger.Warn("failed to read owned passthrough MCP config before cleanup",
			zap.String("path", path), zap.Error(err))
		return
	}
	fingerprint := passthroughMCPFingerprint(content)
	if fingerprint != claim.Fingerprint {
		// The file was replaced by a successor generation or edited by the user.
		// In both cases, deleting it would be unsafe.
		return
	}
	if m.hasOtherPassthroughMCPClaimLocked(path, execution.ID, fingerprint) {
		return
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		m.logger.Warn("failed to remove owned passthrough MCP config",
			zap.String("path", path), zap.Error(err))
	}
}
