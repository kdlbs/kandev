package lifecycle

import (
	"context"
	"fmt"

	agentctlclient "github.com/kandev/kandev/internal/agent/runtime/agentctl"
)

// RenameBranchForSession renames the branch for an existing workspace
// execution. The repo argument is the workspace-relative repository path and
// is empty for the primary repository.
//
// The lifecycle manager owns the in-memory execution state and the
// executors_running row. The latter is updated through an optional narrow
// interface so existing lifecycle test doubles do not need to implement a
// branch-specific persistence method.
func (m *Manager) RenameBranchForSession(
	ctx context.Context,
	sessionID string,
	newName string,
	repo string,
) (*agentctlclient.GitOperationResult, error) {
	return m.renameBranchForSession(ctx, sessionID, newName, repo, repo == "")
}

// RenameBranchForSessionWithPrimary is the multi-repository variant of
// RenameBranchForSession. Agentctl still receives the repository subpath, but
// the caller explicitly identifies whether that repository owns the primary
// execution snapshot.
func (m *Manager) RenameBranchForSessionWithPrimary(
	ctx context.Context,
	sessionID string,
	newName string,
	repo string,
	primary bool,
) (*agentctlclient.GitOperationResult, error) {
	return m.renameBranchForSession(ctx, sessionID, newName, repo, primary)
}

func (m *Manager) renameBranchForSession(
	ctx context.Context,
	sessionID string,
	newName string,
	repo string,
	primary bool,
) (*agentctlclient.GitOperationResult, error) {
	if sessionID == "" {
		return nil, fmt.Errorf("session_id is required")
	}
	if newName == "" {
		return nil, fmt.Errorf("new branch name is required")
	}
	if m == nil || m.executionStore == nil {
		return nil, fmt.Errorf("lifecycle manager execution store is not configured")
	}

	execution, err := m.GetOrEnsureExecution(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if execution == nil {
		return nil, fmt.Errorf("execution for session %s is unavailable", sessionID)
	}
	client := execution.GetAgentCtlClient()
	if client == nil {
		return nil, fmt.Errorf("agentctl client for session %s is unavailable", sessionID)
	}

	result, err := client.GitRenameBranch(ctx, newName, repo)
	if err != nil {
		return result, err
	}
	if result == nil {
		return nil, fmt.Errorf("agentctl returned no branch rename result")
	}
	if !result.Success {
		return result, nil
	}

	// Only the repository identified as primary updates the single primary
	// execution metadata/running snapshot. Other repositories have their
	// durable branch snapshots updated by the orchestrator.
	if primary {
		if execution.Metadata == nil {
			execution.Metadata = make(map[string]interface{})
		}
		execution.Metadata[MetadataKeyWorktreeBranch] = newName
		if updater, ok := m.runningWriter.(interface {
			UpdateExecutorRunningWorktreeBranch(context.Context, string, string) error
		}); ok {
			// Git has already been renamed. Persistence failure must not turn a
			// successful workspace operation into a retryable Git operation.
			_ = updater.UpdateExecutorRunningWorktreeBranch(ctx, sessionID, newName)
		}
	}

	return result, nil
}
