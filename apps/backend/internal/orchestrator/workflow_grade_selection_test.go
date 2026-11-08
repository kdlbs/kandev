package orchestrator

import (
	"context"
	"reflect"
	"testing"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	settingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/kandev/kandev/internal/system/sessioncapacity"
	"github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

func TestWorkflowGradeSelectionUsesActualTargetBeforeAdmission(t *testing.T) {
	for _, tc := range []struct {
		name                           string
		active, explicit, wantDeferred bool
	}{
		{"initial_target_over_caller", false, false, false},
		{"active_target_uses_frozen_grade_and_existing_slot", true, false, false},
		{"explicit_caller_bypasses_target", false, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			svc, repo := newServiceWithCeiling(t, 1)
			svc.sessionCeiling.lister = repo
			profiles := gradeProfileStore(t)
			if tc.active {
				profile, err := profiles.GetAgentProfile(ctx, "logical-grade")
				if err != nil {
					t.Fatal(err)
				}
				profile.ExecutionGrade = "Changed-Profile"
				if err := profiles.UpdateAgentProfile(settingsstore.WithExecutionGradeUpdate(ctx, &profile.ExecutionGrade), profile); err != nil {
					t.Fatal(err)
				}
			}
			svc.SetProfileExecutionResolver(agentruntime.NewProfileExecutionResolver(profiles, nil, false))
			svc.SetExecutionCapacity(sessioncapacity.Effective{Enabled: true, MaxSessions: 1, Policy: sessioncapacity.PolicyAll, GradeLimits: map[string]int{"Profile-A": 1, "Logical-Dynamic": 0}})
			task := &models.Task{ID: "workflow-grade-task", Title: "Task", WorkflowID: "wf-grade", WorkflowStepID: "destination"}
			if err := repo.CreateTask(ctx, task); err != nil {
				t.Fatal(err)
			}
			initial := &models.TaskSession{ID: "workflow-grade-initial", TaskID: task.ID, AgentProfileID: "logical-grade", State: models.TaskSessionStateWaitingForInput}
			if tc.active {
				initial.State = models.TaskSessionStateRunning
				initial.AdmittedExecutionGrade = "Profile-A"
				initial.AdmittedExecutionGradeKnown = true
			}
			if err := repo.CreateTaskSession(ctx, initial); err != nil {
				t.Fatal(err)
			}
			steps := newMockStepGetter()
			steps.steps[task.WorkflowStepID] = &wfmodels.WorkflowStep{ID: task.WorkflowStepID, WorkflowID: task.WorkflowID, Position: 1, SessionTarget: &wfmodels.WorkflowSessionTarget{Kind: wfmodels.WorkflowSessionTargetInitial}, ProfileSessionStartPolicy: models.WorkflowProfileSessionStartPolicyReuse}
			svc.workflowStepGetter = steps
			before, err := repo.GetTask(ctx, task.ID)
			if err != nil {
				t.Fatal(err)
			}
			caller := "unconfigured-grade"
			if tc.explicit {
				caller = "dynamic-grade"
			}
			payload := map[string]any{metaKeyAgentProfileID: caller, metaKeyWorkflowStepID: task.WorkflowStepID, "profile_explicit": tc.explicit}
			reservation, deferred, err := svc.admitOrDeferSeam1(ctx, task.ID, launchOriginAutomatic, payload)
			if err != nil || deferred != tc.wantDeferred {
				t.Fatalf("actual target admission deferred=%v err=%v want=%v", deferred, err, tc.wantDeferred)
			}
			if deferred {
				return
			}
			if reservation == nil {
				t.Fatal("target did not own an admission reservation")
			}
			after, err := repo.GetTask(ctx, task.ID)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(before.Metadata, after.Metadata) {
				t.Fatalf("selection preview changed durable task metadata: before=%v after=%v", before.Metadata, after.Metadata)
			}
			if tc.active && reservation.key == initial.ID {
				t.Fatal("start borrowed the active destination's reservation identity")
			}
			reservation.releaseIfNotConsumed()
		})
	}
}
