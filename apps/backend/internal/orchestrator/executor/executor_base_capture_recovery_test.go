package executor

import (
	"context"
	"testing"

	client "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/task/models"
)

// @covers AC-PLATFORM-GIT-CAPTURE-RECOVERY-001.5
func TestExecutorBaseCaptureRecoveryPersistsEnrichedBaseline(t *testing.T) {
	repo := newMockRepository()
	repo.sessions["session-123"] = &models.TaskSession{ID: "session-123"}
	manager := &recoveredGitStatusManager{
		mockAgentManager: &mockAgentManager{},
		status: &client.GitStatusResult{
			Success: true, StatusState: "ready", FilesComplete: true, DetailState: "ready",
			HeadCommit: "observed-head", BaseCommit: "recovered-merge-base",
		},
	}
	executor := newTestExecutor(t, manager, repo)

	executor.captureBaseCommit(context.Background(), "session-123")

	if manager.detailsCalls != 1 {
		t.Fatalf("enriched status calls = %d, want one", manager.detailsCalls)
	}
	if got := repo.sessions["session-123"].BaseCommitSHA; got != "recovered-merge-base" {
		t.Fatalf("saved base commit = %q, want recovered comparison baseline", got)
	}
}

// @covers AC-PLATFORM-GIT-CAPTURE-RECOVERY-001.5
func TestExecutorBaseCaptureRecoveryRejectsUnavailableDetails(t *testing.T) {
	repo := newMockRepository()
	repo.sessions["session-123"] = &models.TaskSession{ID: "session-123"}
	manager := &recoveredGitStatusManager{
		mockAgentManager: &mockAgentManager{},
		status: &client.GitStatusResult{
			Success: false, StatusState: "ready", FilesComplete: true, DetailState: "unavailable",
			HeadCommit: "unrelated-head", ErrorCode: "details_unavailable",
		},
	}
	executor := newTestExecutor(t, manager, repo)

	executor.captureBaseCommit(context.Background(), "session-123")

	if manager.detailsCalls != 1 {
		t.Fatalf("enriched status calls = %d, want one", manager.detailsCalls)
	}
	if got := repo.sessions["session-123"].BaseCommitSHA; got != "" {
		t.Fatalf("saved base commit = %q, want no baseline from unavailable enrichment", got)
	}
}

type recoveredGitStatusManager struct {
	*mockAgentManager
	status       *client.GitStatusResult
	detailsCalls int
}

func (m *recoveredGitStatusManager) GetGitStatusWithDetails(context.Context, string) (*client.GitStatusResult, error) {
	m.detailsCalls++
	return m.status, nil
}
