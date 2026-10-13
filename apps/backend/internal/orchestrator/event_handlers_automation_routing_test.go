package orchestrator

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/automation"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/stretchr/testify/require"
)

type retryDestinationService struct {
	*retryAutomationServiceStub
	runErr     error
	dispatched chan string
}

func (s *retryDestinationService) GetRun(ctx context.Context, id string) (*automation.AutomationRun, error) {
	if s.runErr != nil {
		return nil, s.runErr
	}
	return s.retryAutomationServiceStub.GetRun(ctx, id)
}

func (s *retryDestinationService) GetAutomationForDispatch(_ context.Context, id string) (*automation.Automation, error) {
	if id != s.automation.ID {
		return nil, nil
	}
	return s.automation, nil
}

func (s *retryDestinationService) DispatchManagedAutomationRun(_ context.Context, id string) error {
	s.dispatched <- id
	return nil
}

func TestAutomationEventRoutesRunOnlyRetryUsingImmutableDestination(t *testing.T) {
	repo := setupTestRepo(t)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	a := &automation.Automation{ID: "routing-automation", WorkspaceID: "routing-workspace",
		Name: "routing", TaskMode: automation.TaskModeManagedConversation}
	run := &automation.AutomationRun{ID: "routing-run", AutomationID: a.ID,
		RetryGroupID: "routing-group", RetryGroupGeneration: 1, TriggerType: automation.TriggerTypeManual,
		RetryLaunchConfigVersion: automation.RetryLaunchConfigVersion}
	run.RetryLaunchConfigSnapshot = retrySnapshotForTest(run, a)
	// The saved destination changed after admission. The admitted snapshot still owns this firing.
	a.TaskMode = automation.TaskModeNormalTask
	destination := &retryDestinationService{
		retryAutomationServiceStub: &retryAutomationServiceStub{
			stubAutomationService: &stubAutomationService{automation: a}, run: run,
		},
		dispatched: make(chan string, 1),
	}
	svc.SetAutomationService(destination)
	evt := &automation.AutomationTriggeredEvent{RunID: run.ID, AutomationID: "untrusted-owner", SnapshotVersion: 1}
	require.NoError(t, svc.handleAutomationTriggered(context.Background(), &bus.Event{Data: evt}))
	select {
	case id := <-destination.dispatched:
		require.Equal(t, run.ID, id)
	case <-time.After(time.Second):
		t.Fatal("run-only retry did not reach its snapshot-owned destination")
	}
	require.Equal(t, "untrusted-owner", evt.AutomationID)
}

func TestAutomationEventRoutingFailsClosedOnRunReadAndSnapshotErrors(t *testing.T) {
	for _, failure := range []string{"read", "snapshot", "owner"} {
		t.Run(failure, func(t *testing.T) {
			repo := setupTestRepo(t)
			svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
			a := &automation.Automation{ID: "routing-automation", WorkspaceID: "routing-workspace", Name: "routing", TaskMode: automation.TaskModeManagedConversation}
			run := &automation.AutomationRun{ID: "routing-run", AutomationID: a.ID, RetryGroupID: "routing-group", TriggerType: automation.TriggerTypeManual, RetryLaunchConfigVersion: 1}
			run.RetryLaunchConfigSnapshot = retrySnapshotForTest(run, a)
			destination := &retryDestinationService{retryAutomationServiceStub: &retryAutomationServiceStub{stubAutomationService: &stubAutomationService{automation: a}, run: run}, dispatched: make(chan string, 1)}
			switch failure {
			case "read":
				destination.runErr = errors.New("run unavailable")
			case "snapshot":
				run.RetryLaunchConfigSnapshot = "invalid"
			case "owner":
				run.AutomationID = "different-owner"
			}
			svc.SetAutomationService(destination)
			require.Error(t, svc.handleAutomationTriggered(context.Background(), &bus.Event{Data: &automation.AutomationTriggeredEvent{RunID: run.ID, AutomationID: a.ID}}))
			select {
			case <-destination.dispatched:
				t.Fatal("invalid ownership dispatched")
			default:
			}
		})
	}
}
