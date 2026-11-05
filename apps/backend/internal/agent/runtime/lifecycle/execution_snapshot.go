package lifecycle

import (
	"context"

	"github.com/google/uuid"
	client "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/common/launchwrapper"
	contract "github.com/kandev/kandev/internal/common/sandbox"
	"github.com/kandev/kandev/internal/task/models"
)

type ExecutionSnapshotRecorder func(context.Context, *AgentExecution, models.ExecutionSnapshot, *contract.Launch) error

func (m *Manager) SetExecutionSnapshotRecorder(recorder ExecutionSnapshotRecorder) {
	m.executionSnapshotRecorder = recorder
}

func (m *Manager) captureExecutionSnapshot(ctx context.Context, execution *AgentExecution, transport string, argv []string, wrapper launchwrapper.Spec, attempt SandboxAttempt) error {
	if m.executionSnapshotRecorder == nil || execution.SessionID == "" {
		return nil
	}
	summary, err := wrapper.Summary()
	if err != nil {
		return err
	}
	var launch *contract.Launch
	attemptID := uuid.NewString()
	if attempt != nil {
		launch = attempt.Launch()
		attemptID = launch.AttemptID
	}
	snapshot := models.ExecutionSnapshot{
		SchemaVersion: 1, AttemptID: attemptID, SessionID: execution.SessionID, TaskID: execution.TaskID,
		ExecutionID: execution.ID, ExecutorType: execution.ExecutorType, AgentProfileID: execution.AgentProfileID,
		Transport: transport, Wrapper: summary, Agent: launchwrapper.SummarizeCommand(argv),
	}
	return m.executionSnapshotRecorder(ctx, execution, snapshot, launch)
}

func (m *Manager) startConfiguredProcessWithSnapshot(ctx context.Context, execution *AgentExecution, transport *client.Client) (string, error) {
	if err := m.captureExecutionSnapshot(ctx, execution, "chat", execution.AgentArgs, execution.LaunchWrapper, nil); err != nil {
		return "", err
	}
	return transport.Start(ctx)
}
