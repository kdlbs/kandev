package orchestrator

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/automation"
	"github.com/stretchr/testify/require"
)

func TestCreateAutomationTaskRecoversAmbiguousContinuationWithoutPrompt(t *testing.T) {
	repo := setupTestRepo(t)
	ctx := context.Background()
	task := retryOwnedTask("accepted-task", "retry-workspace", "retry-automation", "retry-run", 1)
	require.NoError(t, repo.CreateTask(ctx, task))

	agentMgr := &mockAgentManager{}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)
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
	svc.SetAutomationService(base)

	svc.createAutomationTaskLocked(ctx, &automation.AutomationTriggeredEvent{
		RunID: "retry-run", RetryGroupGeneration: 1,
		RetryExternalID: automation.RetryTaskExternalID("retry-run", 1),
		SnapshotVersion: 1, TriggerType: automation.TriggerTypeManual,
		RetryAmbiguousRecovery: true,
	})

	require.True(t, base.acknowledged)
	require.Equal(t, "accepted-task", base.boundTaskID)
	require.Equal(t, "accepted-session", base.boundSessionID)
	require.Equal(t, "accepted-turn", base.boundTurnID)
	require.Empty(t, agentMgr.capturedPrompts)
}

type capacityRetryStub struct {
	*retryAutomationServiceStub
	deferred bool
	released bool
}

func (s *capacityRetryStub) RetryClaimCapacityAvailable(context.Context, string) (bool, error) {
	return false, nil
}

func (s *capacityRetryStub) ReleaseRetryClaim(context.Context, string, string, int64) error {
	s.released = true
	return nil
}
func (s *capacityRetryStub) DeferRetryClaimForCapacity(context.Context, string, string, int64) error {
	s.deferred = true
	return nil
}

func (s *capacityRetryStub) PromoteClaimedRetry(context.Context, string, string, int64) error {
	return nil
}

func TestRetryCapacityDeferralDoesNotImmediatelyReleaseClaim(t *testing.T) {
	base := &retryAutomationServiceStub{
		stubAutomationService: &stubAutomationService{automation: &automation.Automation{
			ID: "retry-automation", WorkspaceID: "retry-workspace", Name: "retry", Enabled: true,
		}},
		run: &automation.AutomationRun{
			ID: "retry-run", AutomationID: "retry-automation", RetryGroupID: "retry-group",
			RetryGroupGeneration: 1, RetryState: automation.RetryStateClaimed,
			Status: automation.RunStatusTriggered,
		},
		operation: &automation.RetryOperation{State: "requested", GroupGeneration: 1},
	}
	base.run.RetryLaunchConfigSnapshot = retrySnapshotForTest(base.run, base.automation)
	base.run.RetryLaunchConfigVersion = automation.RetryLaunchConfigVersion
	capacity := &capacityRetryStub{retryAutomationServiceStub: base}
	svc := createTestService(setupTestRepo(t), newMockStepGetter(), newMockTaskRepo())
	svc.SetAutomationService(capacity)

	svc.createAutomationTaskLocked(context.Background(), &automation.AutomationTriggeredEvent{
		RunID: "retry-run", RetryClaimToken: "claim-token", RetryGroupGeneration: 1,
	})

	require.True(t, capacity.deferred)
	require.False(t, capacity.released)
}
