package service

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	runtimeapi "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/task/models"
)

func TestDeleteQuickChatConfirmedDeadRuntimeRemovesHistory(t *testing.T) {
	svc, _, repo := createTestService(t)
	f := newAgentHistoryFixture(t, svc)
	token := "abababab-0000-4000-8000-000000000003"
	sessionID := seedAgentHistoryTask(t, repo, agentHistoryTaskSpec{
		taskID: "quick-dead-history", ephemeral: true, agentName: "claude-acp", resumeToken: token,
	})
	transcript := f.writeClaudeTranscript(t, filepath.Join(f.quickChatDir, sessionID), token)
	svc.executionStopper = &stubStopper{stopSessionErr: fmt.Errorf("absent: %w", runtimeapi.ErrNotFound)}
	svc.rowLivenessProber = stubRowLivenessProber{liveness: models.ProcessLivenessDead}
	if err := svc.DeleteTask(context.Background(), "quick-dead-history"); err != nil {
		t.Fatalf("DeleteTask: %v", err)
	}
	waitForCleanupDone(t, svc)
	assertPathExists(t, transcript, false)
}

func TestArchiveQuickChatKeepsAgentHistory(t *testing.T) {
	svc, _, repo := createTestService(t)
	f := newAgentHistoryFixture(t, svc)
	token := "abababab-0000-4000-8000-000000000004"
	sessionID := seedAgentHistoryTask(t, repo, agentHistoryTaskSpec{
		taskID: "quick-archive-history", ephemeral: true, agentName: "claude-acp", resumeToken: token,
	})
	transcript := f.writeClaudeTranscript(t, filepath.Join(f.quickChatDir, sessionID), token)
	if err := svc.ArchiveTask(context.Background(), "quick-archive-history"); err != nil {
		t.Fatalf("ArchiveTask: %v", err)
	}
	waitForCleanupDone(t, svc)
	assertPathExists(t, transcript, true)
}
