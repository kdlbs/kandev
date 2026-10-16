package orchestrator

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/automation"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/stretchr/testify/require"
)

type transientRetryTaskRepository struct {
	*sqliterepo.Repository
	err error
}

func (r *transientRetryTaskRepository) GetTask(context.Context, string) (*models.Task, error) {
	return nil, r.err
}

func TestCreateAutomationTaskKeepsAmbiguousRecoveryPendingOnTaskLookupError(t *testing.T) {
	repo := setupTestRepo(t)
	agentMgr := &mockAgentManager{}
	base := &retryAutomationServiceStub{
		stubAutomationService: &stubAutomationService{automation: &automation.Automation{
			ID: "retry-automation", WorkspaceID: "retry-workspace", Name: "retry", Prompt: "retry", Enabled: true,
		}},
		run: &automation.AutomationRun{
			ID: "retry-run", AutomationID: "retry-automation", TriggerType: automation.TriggerTypeManual,
			RetryGroupID: "retry-group", RetryGroupGeneration: 1,
			RetryState: automation.RetryStateTriggered, Status: automation.RunStatusTriggered,
		},
		operation: &automation.RetryOperation{
			State: "ambiguous", GroupGeneration: 1,
			ExternalTaskID: "accepted-task", ExternalSessionID: "accepted-session", ExternalTurnID: "accepted-turn",
		},
	}
	base.run.RetryLaunchConfigSnapshot = retrySnapshotForTest(base.run, base.automation)
	base.run.RetryLaunchConfigVersion = automation.RetryLaunchConfigVersion

	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)
	svc.SetAutomationService(base)
	lookupErr := errors.New("temporary task repository failure")
	svc.repo = &transientRetryTaskRepository{Repository: repo, err: lookupErr}

	svc.createAutomationTaskLocked(context.Background(), &automation.AutomationTriggeredEvent{
		RunID: "retry-run", RetryGroupGeneration: 1,
		RetryExternalID: automation.RetryTaskExternalID("retry-run", 1),
		SnapshotVersion: 1, TriggerType: automation.TriggerTypeManual,
		RetryAmbiguousRecovery: true,
	})

	require.False(t, base.terminalized)
	require.False(t, base.acknowledged)
	require.Empty(t, agentMgr.capturedPrompts)
}
