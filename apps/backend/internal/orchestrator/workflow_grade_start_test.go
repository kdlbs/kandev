package orchestrator

import (
	"context"
	"errors"
	"sync"
	"testing"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/system/sessioncapacity"
	"github.com/kandev/kandev/internal/task/models"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

type terminalGradePromotionRepo struct {
	*tasksqlite.Repository
	once   sync.Once
	before func(context.Context, string) error
}

func (r *terminalGradePromotionRepo) SetSessionPrimaryWithWorkflowSessionRouteIfNonterminal(ctx context.Context, id string, route models.WorkflowSessionRoute) (bool, error) {
	var mutationErr error
	r.once.Do(func() { mutationErr = r.before(ctx, id) })
	if mutationErr != nil {
		return false, mutationErr
	}
	return r.Repository.SetSessionPrimaryWithWorkflowSessionRouteIfNonterminal(ctx, id, route)
}

func TestWorkflowGradeSelectionStartTaskReadmitsAfterLostPromotion(t *testing.T) {
	ctx := context.Background()
	fixture := newProfileSwitchFixture(t, models.WorkflowProfileSessionStartPolicyReuse, models.WorkflowProfileSessionEndPolicyPark)
	profiles := gradeProfileStore(t)
	for _, p := range []*settingsmodels.AgentProfile{
		{ID: "profile-a", AgentID: "grade-agent", Name: "Initial A", Enabled: true, ExecutionGrade: "Profile-A"},
		{ID: "profile-b", AgentID: "grade-agent", Name: "Caller B", Enabled: true, ExecutionGrade: "Blocked"},
	} {
		if err := profiles.CreateAgentProfile(ctx, p); err != nil {
			t.Fatal(err)
		}
	}
	if err := fixture.repo.UpdateTaskSessionState(ctx, fixture.current.ID, models.TaskSessionStateWaitingForInput, ""); err != nil {
		t.Fatal(err)
	}
	if err := fixture.repo.SetAdmittedExecutionGrade(ctx, fixture.current.ID, "Profile-A"); err != nil {
		t.Fatal(err)
	}
	if err := fixture.repo.UpdateTaskSessionState(ctx, fixture.current.ID, models.TaskSessionStateRunning, ""); err != nil {
		t.Fatal(err)
	}
	fixture.stepGetter.steps["step-a"] = &wfmodels.WorkflowStep{ID: "step-a", WorkflowID: "wf1", Position: 1, SessionTarget: &wfmodels.WorkflowSessionTarget{Kind: wfmodels.WorkflowSessionTargetInitial}, ProfileSessionStartPolicy: models.WorkflowProfileSessionStartPolicyReuse}
	lostCAS := false
	repo := &terminalGradePromotionRepo{Repository: fixture.repo, before: func(ctx context.Context, id string) error {
		lostCAS = true
		if err := fixture.repo.UpdateTaskSessionState(ctx, id, models.TaskSessionStateCancelled, ""); err != nil {
			return err
		}
		blocked := "Blocked"
		_, err := fixture.repo.UpdateTaskFieldsWithParentAdmission(ctx, "t1", models.TaskFieldUpdate{ExecutionGrade: &blocked}, nil)
		return err
	}}
	fixture.svc.repo = repo
	fixture.svc.sessionCeiling = newSessionCeilingController(1, repo, nil)
	fixture.svc.SetProfileExecutionResolver(agentruntime.NewProfileExecutionResolver(profiles, nil, false))
	fixture.svc.SetExecutionCapacity(sessioncapacity.Effective{Enabled: true, MaxSessions: 1, Policy: sessioncapacity.PolicyAll, GradeLimits: map[string]int{"Profile-A": 1, "Blocked": 0}})
	launches := 0
	fixture.agentMgr.launchAgentFunc = func(context.Context, *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
		launches++
		return nil, errors.New("unexpected provider launch")
	}
	execution, err := fixture.svc.StartTask(ctx, "t1", "profile-b", "", "", "", "preserve original prompt", "", false, true, nil)
	if !errors.Is(err, ErrCeilingLaunchDeferred) || execution != nil || !lostCAS || launches != 0 {
		t.Fatalf("lost CAS was not readmitted: execution=%+v err=%v CAS=%v launches=%d", execution, err, lostCAS, launches)
	}
	record := deferredLaunchOf(t, fixture.svc, "t1")
	if record == nil || record[models.CeilingReasonCodeKey] != ceilingReasonGradeLimit {
		t.Fatalf("re-entry record=%v", record)
	}
	payload, _ := record[models.CeilingLaunchPayloadKey].(map[string]any)
	if payload["prompt"] != "preserve original prompt" {
		t.Fatalf("re-entry lost original intent=%v", payload)
	}
	sessions, err := fixture.repo.ListTaskSessions(ctx, "t1")
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 || sessions[0].State != models.TaskSessionStateCancelled || sessions[0].AdmittedExecutionGrade != "Profile-A" {
		t.Fatalf("lost destination or frozen history changed=%+v", sessions)
	}
	if len(fixture.svc.sessionCeiling.reservations) != 0 {
		t.Fatal("lost promotion retained a launch reservation")
	}
}
