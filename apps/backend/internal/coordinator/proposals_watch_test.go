package coordinator

import (
	"context"
	"errors"
	"testing"

	taskmodels "github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

func watchedProposalService(t *testing.T, f proposalTestFixture) *Service {
	t.Helper()
	svc := NewService(f.svc.store, newValidatorForTest(nil, nil), &fakeWorkspaceAuthorizer{}, newTestLogger(t), WithPhase2(true))
	svc.SetProposalDeps(f.svc.proposalWorkflows, f.svc.proposalRepositories, f.svc.proposalTasks, f.svc.proposalSteps)
	return svc
}

func wantProposalFieldError(t *testing.T, err error, field string) {
	t.Helper()
	var fe *FieldError
	if !errors.As(err, &fe) || fe.Field != field {
		t.Fatalf("err = %v, want FieldError on %q", err, field)
	}
}

// With phase 2 on, a proposal whose workflow, or whose source task's
// workflow, is outside the effective watch set is refused with a field error
// and leaves no proposal or activity row.
func TestProposeTask_OutsideWatchSetIsRefused(t *testing.T) {
	f := newProposalTestFixture(t)
	createWorkflowsTable(t, f.svc.store)
	addWorkflow(t, f.svc.store, f.workflowID, f.workspaceID)
	addWorkflow(t, f.svc.store, "wf-other", f.workspaceID)
	svc := watchedProposalService(t, f)
	mustSave(t, svc, f.workspaceID, f.coordinator.ID, `{"watches":{"scope":"selected","workflow_ids":["`+f.workflowID+`"]}}`)

	otherTask := &taskmodels.Task{ID: "task-other", WorkspaceID: f.workspaceID, WorkflowID: "wf-other"}
	watchedTask := &taskmodels.Task{ID: "task-watched", WorkspaceID: f.workspaceID, WorkflowID: f.workflowID}
	svc.SetProposalDeps(
		fakeWorkflowReader{workflows: map[string]*taskmodels.Workflow{
			f.workflowID: {ID: f.workflowID, WorkspaceID: f.workspaceID},
			"wf-other":   {ID: "wf-other", WorkspaceID: f.workspaceID},
		}},
		f.svc.proposalRepositories,
		fakeSourceTaskReader{tasks: map[string]*taskmodels.Task{otherTask.ID: otherTask, watchedTask.ID: watchedTask, "task-none": {ID: "task-none", WorkspaceID: f.workspaceID}}},
		fakeWorkflowStepReader{stepsByWorkflow: map[string][]*wfmodels.WorkflowStep{
			f.workflowID: {{ID: "start", IsStartStep: true}},
			"wf-other":   {{ID: "other-start", IsStartStep: true}},
		}},
	)
	ctx := context.Background()

	req := f.baseRequest()
	req.WorkflowID = "wf-other"
	_, _, err := svc.ProposeTask(ctx, f.coordinator.ID, req)
	wantProposalFieldError(t, err, "workflow_id")

	messages := map[string]bool{}
	for _, source := range []string{otherTask.ID, "task-none", "task-missing"} {
		req = f.baseRequest()
		req.SourceTaskID = source
		_, _, err = svc.ProposeTask(ctx, f.coordinator.ID, req)
		wantProposalFieldError(t, err, fieldSourceTaskID)
		messages[err.Error()] = true
	}
	if len(messages) != 1 {
		t.Fatalf("a missing and an out-of-scope source task must answer alike, got %v", messages)
	}

	if rows := listActivity(t, f.svc.store, f.coordinator.ID); len(rows) != 0 {
		t.Fatalf("refused proposals wrote %d activity rows, want 0", len(rows))
	}
	if n, err := svc.store.CountOpenProposals(ctx, f.coordinator.ID, true); err != nil || n != 0 {
		t.Fatalf("open proposals = %d, %v; want 0", n, err)
	}

	req = f.baseRequest()
	req.SourceTaskID = watchedTask.ID
	if _, _, err = svc.ProposeTask(ctx, f.coordinator.ID, req); err != nil {
		t.Fatalf("watched workflow and source task must be accepted: %v", err)
	}
}

// With phase 2 off the watch set is not consulted.
func TestProposeTask_WatchSetIgnoredWithPhase2Off(t *testing.T) {
	f := newProposalTestFixture(t)
	if _, _, err := f.svc.ProposeTask(context.Background(), f.coordinator.ID, f.baseRequest()); err != nil {
		t.Fatal(err)
	}
}
