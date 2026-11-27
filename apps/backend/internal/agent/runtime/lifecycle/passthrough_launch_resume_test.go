package lifecycle

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/agent/executor"
	"github.com/stretchr/testify/require"
)

func TestPassthroughLaunchResumesWithoutRuntimeOwner(t *testing.T) {
	manager := newTestManager(t)
	agent := &resumeTestAgent{}
	request := &LaunchRequest{TaskID: "task", SessionID: "session", IsPassthrough: true, ResumePassthrough: true}
	executorRequest := &ExecutorCreateRequest{InstanceID: "new-owner", TaskID: "task", SessionID: "session",
		AgentConfig: agent, Env: map[string]string{}, Metadata: map[string]interface{}{}}
	instance := &ExecutorInstance{InstanceID: "new-owner", SessionID: "session"}
	execution, err := manager.buildExecutionFromInstance(context.Background(), request, executorRequest,
		instance, &MockExecutor{name: executor.NameSSH}, nil, agent, nil)
	require.NoError(t, err)
	require.True(t, execution.isResumedSession)
	require.True(t, execution.IsPassthrough)
	require.Empty(t, request.PreviousExecutionID)
}

func TestPassthroughLaunchPromotesResumeWithoutRuntimeOwner(t *testing.T) {
	manager := newTestManager(t)
	manager.profileResolver = &countingProfileResolver{info: &AgentProfileInfo{
		ProfileID: "profile", AgentName: "auggie", CLIPassthrough: true,
	}}
	existing := &AgentExecution{ID: "execution", TaskID: "task", SessionID: "session", AgentProfileID: "profile"}
	require.NoError(t, manager.executionStore.Add(existing))
	got, err := manager.Launch(context.Background(), &LaunchRequest{TaskID: "task", SessionID: "session",
		AgentProfileID: "profile", IsPassthrough: true, ResumePassthrough: true})
	require.NoError(t, err)
	require.Same(t, existing, got)
	require.True(t, got.isResumedSession)
	require.True(t, got.IsPassthrough)
}
