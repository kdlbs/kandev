package executor

import (
	"errors"
	"testing"
)

func TestResumeSessionWithOptionsRequiresExecutorActorWithoutAdmission(t *testing.T) {
	repo := newMockRepository()
	setupLiveResumeTestFixture(repo)
	agentManager := &mockAgentManager{}
	exec := newTestExecutor(t, agentManager, repo)

	_, err := exec.ResumeSessionWithOptions(
		t.Context(), repo.sessions["sess-1"], true, ResumeOptions{RequireExecutorActor: true},
	)
	if !errors.Is(err, ErrExecutorActorUnavailable) {
		t.Fatalf("ResumeSessionWithOptions error = %v, want %v", err, ErrExecutorActorUnavailable)
	}
	if agentManager.launchAgentCallCount != 0 {
		t.Fatalf("LaunchAgent calls = %d, want 0", agentManager.launchAgentCallCount)
	}
}
