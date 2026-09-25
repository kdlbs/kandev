package lifecycle

import (
	"errors"
	"os"
	"path/filepath"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/mcpconfig"
	"github.com/kandev/kandev/internal/task/models"
)

func (m *Manager) prepareCursorMCPAuth(
	execution *AgentExecution,
	profileInfo *AgentProfileInfo,
	executorType string,
	strategy mcpconfig.PassthroughMCPStrategy,
) error {
	if execution == nil || execution.WorkspacePath == "" || !isCursorMCPAuthStrategy(strategy) {
		return nil
	}
	if !isCursorMCPAuthLocalExecutor(executorType) {
		return nil
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return nil
	}
	if runtimeHome, exists := execution.RuntimeEnvironment()["HOME"]; exists && !sameCursorMCPAuthHome(home, runtimeHome) {
		return nil
	}
	if profileInfo == nil {
		return errors.New("could not resolve Cursor MCP authentication preference")
	}

	if !profileInfo.CursorMCPAuthEnabled {
		if err := mcpconfig.PrepareCursorMCPAuth(execution.WorkspacePath, filepath.Join(home, ".cursor"), false); err != nil {
			return errors.New("failed to remove shared Cursor MCP credentials")
		}
		return nil
	}

	if err := mcpconfig.PrepareCursorMCPAuth(execution.WorkspacePath, filepath.Join(home, ".cursor"), true); err != nil && m.logger != nil {
		m.logger.Warn("could not prepare local Cursor MCP credentials",
			zap.String("reason", "prepare_failed"),
			zap.String("agent_id", execution.AgentID))
	}
	return nil
}

func isCursorMCPAuthStrategy(strategy mcpconfig.PassthroughMCPStrategy) bool {
	switch strategy.(type) {
	case mcpconfig.CursorStrategy, *mcpconfig.CursorStrategy:
		return true
	default:
		return false
	}
}

func isCursorMCPAuthLocalExecutor(executorType string) bool {
	switch models.ExecutorType(executorType) {
	case models.ExecutorTypeLocal, models.ExecutorTypeWorktree, legacyExecutorTypeLocalPC:
		return true
	default:
		return false
	}
}

func sameCursorMCPAuthHome(left, right string) bool {
	if left == "" || right == "" {
		return false
	}
	leftAbs, err := filepath.Abs(left)
	if err != nil {
		return false
	}
	rightAbs, err := filepath.Abs(right)
	if err != nil {
		return false
	}
	return filepath.Clean(leftAbs) == filepath.Clean(rightAbs)
}
