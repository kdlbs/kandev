package executor

import (
	"testing"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

// @covers AC-EXECUTORS-IDLE-PARKING-001.11
func TestIdleActivityMetadataUsesSameSessionResumeOnly(t *testing.T) {
	clock := map[string]interface{}{"execution_id": "execution-a", "activity_at": "2026-10-15T00:00:00Z"}
	running := &models.ExecutorRunning{SessionID: "session-a", TaskID: "task", AgentExecutionID: "execution-a",
		Metadata: map[string]interface{}{"idle_activity": clock, lifecycle.MetadataKeySSHHost: "example.com"}}
	sibling := &LaunchAgentRequest{TaskID: "task", SessionID: "session-b"}
	applyExecutorRunningMetadata(sibling, running)
	require.NotContains(t, sibling.Metadata, "idle_activity")
	require.Empty(t, sibling.PreviousExecutionID)
	require.Equal(t, "example.com", sibling.Metadata[lifecycle.MetadataKeySSHHost],
		"positive control: task-wide connection metadata still propagates")
	exec := newTestExecutor(t, &mockAgentManager{}, newMockRepository())
	resume := &LaunchAgentRequest{TaskID: "task", SessionID: "session-a"}
	exec.applyRunningRecordToResumeRequest(resume, &v1.Task{ID: "task"},
		&models.TaskSession{ID: "session-a", State: models.TaskSessionStateWaitingForInput}, false, running)
	require.Equal(t, "execution-a", resume.PreviousExecutionID)
	require.Equal(t, clock, resume.Metadata["idle_activity"], "same-session resume must preserve the owner clock")
}
