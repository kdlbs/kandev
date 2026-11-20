package orchestrator

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/system/sessioncapacity"
	"github.com/kandev/kandev/internal/task/models"
)

func TestAllPolicyMessageTurnsEnforceGradeLimits(t *testing.T) {
	for _, intent := range []turnCapacityIntent{turnCapacityBounded, turnCapacityOverride} {
		for _, limit := range []int{0, 1} {
			t.Run(fmt.Sprintf("%s/limit%d", intent.String(), limit), func(t *testing.T) {
				lister := &fakeGradedExecutionLister{}
				if limit == 1 {
					lister.rows = []models.AdmittedExecution{{ID: "other", Grade: "work", GradeKnown: true}}
				}
				controller := newSessionCeilingController(5, lister, zap.NewNop())
				controller.policy = sessioncapacity.PolicyAll
				controller.gradeLimits = map[string]int{"work": limit}
				controller.resolveGrade = func(context.Context, admissionRequest) (string, error) { return "work", nil }
				ticket, refusal := controller.admitTurn(context.Background(), turnAdmissionRequest{
					taskID: "task", sessionID: "waiting", intent: intent,
				})
				if ticket != nil {
					defer ticket.release()
				}
				require.Nil(t, ticket, "a paused or full grade cannot admit a new turn")
				require.NotNil(t, refusal)
				require.Equal(t, ceilingReasonGradeLimit, refusal.reasonCode)
			})
		}
	}
}

func TestAllPolicyConcurrentMessageTurnsShareGradeCapacity(t *testing.T) {
	controller := newSessionCeilingController(5, &fakeGradedExecutionLister{}, zap.NewNop())
	controller.policy = sessioncapacity.PolicyAll
	controller.gradeLimits = map[string]int{"work": 1}
	controller.resolveGrade = func(context.Context, admissionRequest) (string, error) { return "work", nil }
	start := make(chan struct{})
	results := make(chan *messageTurnReservation, 2)
	var joined sync.WaitGroup
	for _, id := range []string{"a", "b"} {
		joined.Add(1)
		go func() {
			defer joined.Done()
			<-start
			ticket, _ := controller.admitTurn(context.Background(), turnAdmissionRequest{taskID: id, sessionID: id})
			results <- ticket
		}()
	}
	close(start)
	joined.Wait()
	close(results)
	winners := 0
	for ticket := range results {
		if ticket != nil {
			winners++
			ticket.release()
		}
	}
	require.Equal(t, 1, winners)
}

func TestAllPolicyMessageTurnRetainsGradeThroughColdResume(t *testing.T) {
	ctx := context.Background()
	controller := newSessionCeilingController(5, &fakeGradedExecutionLister{}, zap.NewNop())
	controller.policy = sessioncapacity.PolicyAll
	controller.gradeLimits = map[string]int{"work": 1, "control": 0}
	grade := "work"
	controller.resolveGrade = func(context.Context, admissionRequest) (string, error) { return grade, nil }
	var persisted []string
	controller.persistGrade = func(_ context.Context, _ string, value string) error {
		persisted = append(persisted, value)
		return nil
	}
	ticket, refusal := controller.admitTurn(ctx, turnAdmissionRequest{taskID: "task", sessionID: "waiting"})
	require.Nil(t, refusal)
	require.NotNil(t, ticket)
	defer ticket.release()
	grade = "control"
	decision := controller.admit(ctx, admissionRequest{taskID: "task", sessionID: "waiting", origin: launchOriginManual})
	require.True(t, decision.admitted)
	require.Equal(t, []string{"work", "work"}, persisted, "nested resume must preserve the turn's admitted grade")

	controller.mu.Lock()
	population, err := controller.readGradedPopulationLocked(ctx, "work")
	controller.mu.Unlock()
	require.NoError(t, err)
	require.False(t, population.unknown)
	require.Equal(t, 1, population.total)
	require.Equal(t, 1, population.grade)
	grade = "work"
	blocked, refusal := controller.admitTurn(ctx, turnAdmissionRequest{taskID: "other", sessionID: "other"})
	if blocked != nil {
		defer blocked.release()
	}
	require.Nil(t, blocked)
	require.NotNil(t, refusal)
	require.Equal(t, ceilingReasonGradeLimit, refusal.reasonCode)
	ticket.release()
	next, refusal := controller.admitTurn(ctx, turnAdmissionRequest{taskID: "other", sessionID: "other"})
	require.Nil(t, refusal)
	require.NotNil(t, next)
	next.release()
}

func TestAllPolicyPromptColdResumeRefusesPausedGrade(t *testing.T) {
	ctx := context.Background()
	svc, manager, repo := capacityTestService(t, 5)
	seedTaskAndSession(t, repo, "target-task", "target-session", models.TaskSessionStateWaitingForInput)
	manager.isAgentRunning = false
	svc.sessionCeiling.lister = repo
	svc.sessionCeiling.policy = sessioncapacity.PolicyAll
	svc.sessionCeiling.gradeLimits = map[string]int{"work": 0}
	svc.sessionCeiling.resolveGrade = func(context.Context, admissionRequest) (string, error) { return "work", nil }
	svc.sessionCeiling.persistGrade = repo.SetAdmittedExecutionGrade
	_, err := svc.PromptTask(ctx, "target-task", "target-session", "paused follow-up", "", false, nil, false)
	require.True(t, IsMessageTurnCapacityRefusal(err), "cold resume must respect a paused grade: %v", err)
	require.Empty(t, manager.capturedPrompts)
	session, err := repo.GetTaskSession(ctx, "target-session")
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionStateWaitingForInput, session.State)
}
