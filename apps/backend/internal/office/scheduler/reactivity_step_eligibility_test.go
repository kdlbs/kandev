package scheduler

import (
	"context"
	"fmt"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

// insertTaskWithStep persists a minimal task row bound to workflow_step_id,
// since the eligibility gate reads the step from the DB
// (ss.repo.GetTaskWorkflowStepID), not from the TaskSnapshot passed to
// reactToAssigneeChange. Pass an empty stepID to bind no step at all.
func insertTaskWithStep(t *testing.T, ss *SchedulerService, taskID, stepID string) {
	t.Helper()
	if _, err := ss.repo.ExecRaw(context.Background(),
		`INSERT INTO tasks (id, workspace_id, workflow_step_id) VALUES (?, 'ws-1', ?)`,
		taskID, sqlNullableString(stepID),
	); err != nil {
		t.Fatalf("insert task %s: %v", taskID, err)
	}
}

// sqlNullableString maps an empty string to a genuine NULL so an unbound
// step reads back as "" via sql.NullString, matching a task that has never
// been placed on a workflow step.
func sqlNullableString(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

// autoStartStep and backlogStep are the two fixture steps every case below
// resolves against: one carries the auto_start_agent on_enter action
// (wfmodels.OnEnterAutoStartAgent) HasOnEnterAction checks for, the other
// has no on_enter actions at all (StepEvents.OnEnter is []OnEnterAction,
// distinct from the GenericAction shape used for OnChildrenCompleted).
func autoStartStep(id string) *wfmodels.WorkflowStep {
	return &wfmodels.WorkflowStep{
		ID: id,
		Events: wfmodels.StepEvents{
			OnEnter: []wfmodels.OnEnterAction{{Type: wfmodels.OnEnterAutoStartAgent}},
		},
	}
}

func backlogStep(id string) *wfmodels.WorkflowStep {
	return &wfmodels.WorkflowStep{ID: id, Events: wfmodels.StepEvents{}}
}

// TestBetaAssignmentEligibility is the ISSUE-5 regression: setting the
// assignee on a task sitting on a workflow step with no auto_start_agent
// on_enter action (e.g. Backlog) must not queue a task_assigned wake. The
// agent otherwise runs outside the workflow, calls step_complete_kandev
// (accepted:true, nothing advances), and then runs again when the card
// later moves to an eligible step.
//
// The gate must NOT suppress the interrupt of a previous assignee's
// session — only the new wake.
func TestBetaAssignmentEligibility(t *testing.T) {
	const stepAutoStart = "step-work"
	const stepBacklog = "step-backlog"

	cases := []struct {
		name             string
		stepID           string // "" = no step bound at all
		getter           WorkflowStepGetter
		priorAssignee    string
		newAssignee      string
		wantRuns         int
		wantInterruptSet bool
	}{
		{
			name:             "backlog step suppresses the wake but not the interrupt",
			stepID:           stepBacklog,
			getter:           &fakeWorkflowStepGetter{steps: map[string]*wfmodels.WorkflowStep{stepBacklog: backlogStep(stepBacklog)}},
			priorAssignee:    "agent-old",
			newAssignee:      "agent-new",
			wantRuns:         0,
			wantInterruptSet: true,
		},
		{
			name:             "auto-start step queues exactly one wake",
			stepID:           stepAutoStart,
			getter:           &fakeWorkflowStepGetter{steps: map[string]*wfmodels.WorkflowStep{stepAutoStart: autoStartStep(stepAutoStart)}},
			priorAssignee:    "agent-old",
			newAssignee:      "agent-new",
			wantRuns:         1,
			wantInterruptSet: true,
		},
		{
			name:             "same-agent reassignment on an auto-start step still wakes with no interrupt",
			stepID:           stepAutoStart,
			getter:           &fakeWorkflowStepGetter{steps: map[string]*wfmodels.WorkflowStep{stepAutoStart: autoStartStep(stepAutoStart)}},
			priorAssignee:    "agent-same",
			newAssignee:      "agent-same",
			wantRuns:         1,
			wantInterruptSet: false,
		},
		{
			name:             "same-agent reassignment on backlog wakes nothing",
			stepID:           stepBacklog,
			getter:           &fakeWorkflowStepGetter{steps: map[string]*wfmodels.WorkflowStep{stepBacklog: backlogStep(stepBacklog)}},
			priorAssignee:    "agent-same",
			newAssignee:      "agent-same",
			wantRuns:         0,
			wantInterruptSet: false,
		},
		{
			name:             "empty step fails open",
			stepID:           "",
			getter:           &fakeWorkflowStepGetter{},
			priorAssignee:    "",
			newAssignee:      "agent-new",
			wantRuns:         1,
			wantInterruptSet: false,
		},
		{
			name:             "step getter error fails open",
			stepID:           stepBacklog,
			getter:           &fakeWorkflowStepGetter{err: fmt.Errorf("boom")},
			priorAssignee:    "",
			newAssignee:      "agent-new",
			wantRuns:         1,
			wantInterruptSet: false,
		},
	}

	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := newReactivityTestRepo(t)
			ss := &SchedulerService{repo: repo, logger: logger.Default()}
			ss.SetWorkflowStepGetter(tc.getter)

			taskID := fmt.Sprintf("task-eligibility-%d", i)
			insertTaskWithStep(t, ss, taskID, tc.stepID)

			task := &TaskSnapshot{
				ID:                     taskID,
				WorkspaceID:            "ws-1",
				State:                  "TODO",
				AssigneeAgentProfileID: tc.priorAssignee,
			}
			change := TaskMutation{ActorID: "user-1", ActorType: "user"}
			res := &ApplyTaskMutationResult{}

			var calls []recordedQueueCall
			queue := func(agentID string, c RunContext) {
				calls = append(calls, recordedQueueCall{agentID: agentID, ctx: c})
			}

			ss.reactToAssigneeChange(context.Background(), task, tc.newAssignee, change, queue, res)

			if len(calls) != tc.wantRuns {
				t.Fatalf("queued %d runs, want %d: %+v", len(calls), tc.wantRuns, calls)
			}
			gotInterruptSet := res.InterruptSessionID != ""
			if gotInterruptSet != tc.wantInterruptSet {
				t.Fatalf("InterruptSessionID set = %v, want %v", gotInterruptSet, tc.wantInterruptSet)
			}
		})
	}
}
