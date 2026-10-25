package lifecycle

import (
	"errors"
	"os"
	"path/filepath"
	"time"

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
	_, _, err := m.prepareCursorMCPAuthWithStatus(execution, profileInfo, executorType, strategy)
	return err
}

func (m *Manager) prepareCursorMCPAuthWithStatus(
	execution *AgentExecution,
	profileInfo *AgentProfileInfo,
	executorType string,
	strategy mcpconfig.PassthroughMCPStrategy,
) (PrepareStepStatus, string, error) {
	if execution == nil || execution.WorkspacePath == "" || !isCursorMCPAuthStrategy(strategy) {
		return PrepareStepSkipped, "ineligible", nil
	}
	if !isCursorMCPAuthLocalExecutor(executorType) {
		return PrepareStepSkipped, "ineligible", nil
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return PrepareStepSkipped, "home_unavailable", nil
	}
	if runtimeHome, exists := execution.RuntimeEnvironment()["HOME"]; exists && !sameCursorMCPAuthHome(home, runtimeHome) {
		return PrepareStepSkipped, "home_mismatch", nil
	}
	if profileInfo == nil {
		return PrepareStepFailed, pluginExecutorStateUnavailable, errors.New("could not resolve Cursor MCP authentication preference")
	}

	if !profileInfo.CursorMCPAuthEnabled {
		if err := mcpconfig.PrepareCursorMCPAuth(execution.WorkspacePath, filepath.Join(home, ".cursor"), false); err != nil {
			return PrepareStepFailed, pluginExecutorStateUnavailable, errors.New("failed to remove shared Cursor MCP credentials")
		}
		return PrepareStepSkipped, string(SSHReclaimSkipDisabled), nil
	}

	excludedRoots, err := m.cursorMCPAuthTaskRoots()
	if err != nil {
		m.logCursorMCPAuthPreparationFailure(execution, "task_root_unavailable")
		return PrepareStepFailed, pluginExecutorStateUnavailable, nil
	}
	if err := mcpconfig.PrepareCursorMCPAuth(execution.WorkspacePath, filepath.Join(home, ".cursor"), true, excludedRoots...); err != nil {
		m.logCursorMCPAuthPreparationFailure(execution, "prepare_failed")
		return PrepareStepFailed, pluginExecutorStateUnavailable, nil
	}
	return PrepareStepCompleted, "", nil
}

func (m *Manager) prepareCursorMCPAuthWithProgress(
	execution *AgentExecution,
	profileInfo *AgentProfileInfo,
	executorType string,
	strategy mcpconfig.PassthroughMCPStrategy,
	progress *prepareProgressRecorder,
) error {
	started := time.Now().UTC()
	index := appendCursorMCPProgress(progress, "", PrepareStepKindAgentMCPCredentials, PrepareStepRunning, "", &started, nil)
	status, failureCode, err := m.prepareCursorMCPAuthWithStatus(execution, profileInfo, executorType, strategy)
	if err != nil {
		status = PrepareStepFailed
		if failureCode == "" {
			failureCode = pluginExecutorStateUnavailable
		}
	}
	updateCursorMCPProgress(progress, index, "", PrepareStepKindAgentMCPCredentials, status, failureCode, started, time.Now().UTC())
	return err
}

func (m *Manager) cursorMCPAuthTaskRoots() ([]string, error) {
	roots := make([]string, 0, 2)
	if m.dataDir != "" {
		roots = append(roots, filepath.Join(m.dataDir, "tasks"))
	}
	if m.worktreeMgr == nil {
		return roots, nil
	}
	taskRoot, err := m.worktreeMgr.TasksBasePath()
	if err != nil {
		return nil, err
	}
	if taskRoot != "" {
		roots = append(roots, taskRoot)
	}
	return roots, nil
}

func (m *Manager) logCursorMCPAuthPreparationFailure(execution *AgentExecution, reason string) {
	if m.logger == nil {
		return
	}
	m.logger.Warn("could not prepare local Cursor MCP credentials",
		zap.String("reason", reason),
		zap.String("agent_id", execution.AgentID))
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
	if filepath.Clean(leftAbs) == filepath.Clean(rightAbs) {
		return true
	}
	leftInfo, err := os.Stat(leftAbs)
	if err != nil {
		return false
	}
	rightInfo, err := os.Stat(rightAbs)
	if err != nil {
		return false
	}
	return os.SameFile(leftInfo, rightInfo)
}
