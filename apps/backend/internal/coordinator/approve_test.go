package coordinator

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/authz"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	taskservice "github.com/kandev/kandev/internal/task/service"
	workflowmodels "github.com/kandev/kandev/internal/workflow/models"
)

// approveFixture builds a Service wired for ApproveProposal tests: a real
// Store, a coordinator in "ws-1", a fakeDecisionTaskService with "wf-1"
// (workspace ws-1), "repo-1" (workspace ws-1) and "task-0" (workspace
// ws-1), and a fakeStepReader with four steps: "step-1" (the start step,
// eligible, matching sampleSpec()'s StepID), "manual-step" (eligible,
// allows manual move), "auto-step" (ineligible, auto-starts an agent) and
// "feeder-step" (allows manual move but feeds auto-step, so ineligible via
// the feeder chain).
func approveFixture(t *testing.T) (*Store, *Coordinator, *fakeDecisionTaskService, *Service) {
	t.Helper()
	store := newTestStore(t)
	c := newTestCoordinator(t, store, "ws-1")
	tasks := &fakeDecisionTaskService{
		workflows: map[string]*taskmodels.Workflow{
			"wf-1": {ID: "wf-1", WorkspaceID: "ws-1"},
		},
		repos: map[string]*taskmodels.Repository{
			"repo-1": {ID: "repo-1", WorkspaceID: "ws-1"},
		},
		tasks: map[string]*taskmodels.Task{
			"task-0": {ID: "task-0", WorkspaceID: "ws-1"},
		},
	}
	steps := &fakeStepReader{steps: []*workflowmodels.WorkflowStep{
		{ID: "step-1", IsStartStep: true},
		{ID: "manual-step", AllowManualMove: true},
		{ID: "auto-step", Events: workflowmodels.StepEvents{
			OnEnter: []workflowmodels.OnEnterAction{{Type: workflowmodels.OnEnterAutoStartAgent}},
		}},
		{ID: "feeder-step", AllowManualMove: true, PullFromStepID: "auto-step"},
	}}
	svc := NewService(store, newValidatorForTest(nil, nil), &fakeWorkspaceAuthorizer{}, newTestLogger(t))
	svc.SetDecisionDeps(tasks, steps, nil)
	return store, c, tasks, svc
}

func insertProposal(t *testing.T, store *Store, c *Coordinator, spec ProposalSpec) *Proposal {
	t.Helper()
	p := &Proposal{CoordinatorID: c.ID, WorkspaceID: c.WorkspaceID, Spec: spec}
	if err := store.InsertProposal(context.Background(), p); err != nil {
		t.Fatalf("InsertProposal: %v", err)
	}
	return p
}

// claimDirectly claims p at claimedAt without going through the service, so
// tests can put a proposal into "approving" (optionally stale) before
// exercising ApproveProposal.
func claimDirectly(t *testing.T, store *Store, p *Proposal, token string, spec ProposalSpec, claimedAt time.Time) {
	t.Helper()
	matched, err := store.ClaimProposal(context.Background(), p.ID, token, spec, "", claimedAt)
	if err != nil {
		t.Fatalf("ClaimProposal: %v", err)
	}
	if !matched {
		t.Fatal("ClaimProposal: no row matched")
	}
}

func assertConflict(t *testing.T, err error, wantStatus ProposalStatus) *ProposalConflictError {
	t.Helper()
	var conflict *ProposalConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("err = %v, want *ProposalConflictError", err)
	}
	if conflict.Proposal.Status != wantStatus {
		t.Fatalf("conflict.Proposal.Status = %q, want %q", conflict.Proposal.Status, wantStatus)
	}
	return conflict
}

func createdResult(taskID string) taskservice.CreateTaskResult {
	return taskservice.CreateTaskResult{
		Task:    &taskmodels.Task{ID: taskID, WorkspaceID: "ws-1"},
		Outcome: taskservice.CreateTaskOutcomeCreated,
	}
}

// --- Group A: status-based routing ---

// TestApproveProposal_ForbiddenRequiresManageScope proves the approve route
// authorizes with workspace.manage (not read) before touching the store, and
// that a denial propagates without a write
// (docs/plans/workspace-coordinator/task-07-proposals-backend.md's "a reader
// gets 403").
func TestApproveProposal_ForbiddenRequiresManageScope(t *testing.T) {
	store, c, _, svc := approveFixture(t)
	p := insertProposal(t, store, c, sampleSpec())
	svc.authz = &fakeWorkspaceAuthorizer{err: taskservice.ErrForbidden}

	_, err := svc.ApproveProposal(context.Background(), "ws-1", c.ID, p.ID, ApproveProposalRequest{})
	if !errors.Is(err, taskservice.ErrForbidden) {
		t.Fatalf("err = %v, want ErrForbidden", err)
	}
	assertLastScope(t, svc, authz.ScopeWorkspaceManage)

	reread, rerr := store.GetProposal(context.Background(), "ws-1", c.ID, p.ID)
	if rerr != nil {
		t.Fatalf("GetProposal: %v", rerr)
	}
	if reread.Status != ProposalStatusPending {
		t.Fatalf("Status = %q, want pending (no write should have happened)", reread.Status)
	}
}

func TestApproveProposal_PendingGoldenPath(t *testing.T) {
	store, c, tasks, svc := approveFixture(t)
	p := insertProposal(t, store, c, sampleSpec())
	tasks.createResult = createdResult("task-new")
	tasks.settled = true

	got, err := svc.ApproveProposal(context.Background(), "ws-1", c.ID, p.ID, ApproveProposalRequest{})
	if err != nil {
		t.Fatalf("ApproveProposal: %v", err)
	}
	if got.Status != ProposalStatusApproved {
		t.Fatalf("Status = %q, want %q", got.Status, ProposalStatusApproved)
	}
	if got.TaskID == nil || *got.TaskID != "task-new" {
		t.Fatalf("TaskID = %v, want task-new", got.TaskID)
	}

	if len(tasks.createCalls) != 1 {
		t.Fatalf("createCalls = %d, want 1", len(tasks.createCalls))
	}
	req := tasks.createCalls[0]
	if req.WorkspaceID != "ws-1" || req.WorkflowID != "wf-1" || req.WorkflowStepID != "step-1" {
		t.Fatalf("create request = %+v, want ws-1/wf-1/step-1", req)
	}
	if req.Title != "Do the thing" || req.Description != "A description" {
		t.Fatalf("create request title/description = %q/%q", req.Title, req.Description)
	}
	if req.ExternalID != proposalExternalID(p.ID) || !req.AllowReservedExternalID {
		t.Fatalf("create request external id = %q, allow = %v", req.ExternalID, req.AllowReservedExternalID)
	}
	if req.Origin != taskmodels.TaskOriginManual {
		t.Fatalf("create request origin = %q, want manual", req.Origin)
	}
	if len(req.Repositories) != 1 || req.Repositories[0].RepositoryID != "repo-1" {
		t.Fatalf("create request repositories = %+v, want [repo-1]", req.Repositories)
	}

	if len(tasks.settleCalls) != 1 || tasks.settleCalls[0].taskID != "task-new" || tasks.settleCalls[0].externalID != proposalExternalID(p.ID) {
		t.Fatalf("settleCalls = %+v, want one call for task-new/%s", tasks.settleCalls, proposalExternalID(p.ID))
	}
}

func TestApproveProposal_ApprovedIsConflict(t *testing.T) {
	store, c, tasks, svc := approveFixture(t)
	p := insertProposal(t, store, c, sampleSpec())
	tasks.createResult = createdResult("task-new")
	tasks.settled = true
	if _, err := svc.ApproveProposal(context.Background(), "ws-1", c.ID, p.ID, ApproveProposalRequest{}); err != nil {
		t.Fatalf("ApproveProposal (setup): %v", err)
	}

	_, err := svc.ApproveProposal(context.Background(), "ws-1", c.ID, p.ID, ApproveProposalRequest{})
	_ = assertConflict(t, err, ProposalStatusApproved)
}

func TestApproveProposal_RejectedIsConflict(t *testing.T) {
	store, c, _, svc := approveFixture(t)
	p := insertProposal(t, store, c, sampleSpec())
	if _, err := store.RejectProposal(context.Background(), p.ID, "no thanks", "", time.Now()); err != nil {
		t.Fatalf("RejectProposal: %v", err)
	}

	_, err := svc.ApproveProposal(context.Background(), "ws-1", c.ID, p.ID, ApproveProposalRequest{})
	_ = assertConflict(t, err, ProposalStatusRejected)
}

func TestApproveProposal_ApprovingWithEditsIsConflict(t *testing.T) {
	store, c, _, svc := approveFixture(t)
	p := insertProposal(t, store, c, sampleSpec())
	claimDirectly(t, store, p, "tok", sampleSpec(), time.Now())

	edits := ApproveProposalRequest{ApproveFieldTitle: json.RawMessage(`"New Title"`)}
	_, err := svc.ApproveProposal(context.Background(), "ws-1", c.ID, p.ID, edits)
	_ = assertConflict(t, err, ProposalStatusApproving)
}

func TestApproveProposal_ApprovingNotStaleNoEditsIsConflict(t *testing.T) {
	store, c, _, svc := approveFixture(t)
	p := insertProposal(t, store, c, sampleSpec())
	claimDirectly(t, store, p, "tok", sampleSpec(), time.Now())

	_, err := svc.ApproveProposal(context.Background(), "ws-1", c.ID, p.ID, ApproveProposalRequest{})
	_ = assertConflict(t, err, ProposalStatusApproving)
}

func TestApproveProposal_ApprovingStaleNoFoundTaskReclaimsAndCreates(t *testing.T) {
	store, c, tasks, svc := approveFixture(t)
	p := insertProposal(t, store, c, sampleSpec())
	claimDirectly(t, store, p, "old-tok", sampleSpec(), time.Now().Add(-3*time.Minute))
	tasks.createResult = createdResult("task-new")
	tasks.settled = true

	got, err := svc.ApproveProposal(context.Background(), "ws-1", c.ID, p.ID, ApproveProposalRequest{})
	if err != nil {
		t.Fatalf("ApproveProposal: %v", err)
	}
	if got.Status != ProposalStatusApproved || got.TaskID == nil || *got.TaskID != "task-new" {
		t.Fatalf("got = %+v, want approved/task-new", got)
	}
	if len(tasks.createCalls) != 1 {
		t.Fatalf("createCalls = %d, want 1", len(tasks.createCalls))
	}
}

func TestApproveProposal_ApprovingStaleFoundTaskCompletesWithoutCreate(t *testing.T) {
	store, c, tasks, svc := approveFixture(t)
	p := insertProposal(t, store, c, sampleSpec())
	claimDirectly(t, store, p, "old-tok", sampleSpec(), time.Now().Add(-3*time.Minute))
	tasks.lookupByExternalID = map[string]*taskmodels.Task{
		proposalExternalID(p.ID): {ID: "task-existing", WorkspaceID: "ws-1"},
	}

	got, err := svc.ApproveProposal(context.Background(), "ws-1", c.ID, p.ID, ApproveProposalRequest{})
	if err != nil {
		t.Fatalf("ApproveProposal: %v", err)
	}
	if got.Status != ProposalStatusApproved || got.TaskID == nil || *got.TaskID != "task-existing" {
		t.Fatalf("got = %+v, want approved/task-existing", got)
	}
	if len(tasks.createCalls) != 0 {
		t.Fatalf("createCalls = %d, want 0", len(tasks.createCalls))
	}
}

func TestApproveProposal_FailedFoundTaskWithEditsIsConflict(t *testing.T) {
	store, c, tasks, svc := approveFixture(t)
	p := insertProposal(t, store, c, sampleSpec())
	claimDirectly(t, store, p, "tok", sampleSpec(), time.Now())
	if _, err := store.FailProposal(context.Background(), p.ID, "tok", "boom", time.Now()); err != nil {
		t.Fatalf("FailProposal: %v", err)
	}
	tasks.lookupByExternalID = map[string]*taskmodels.Task{
		proposalExternalID(p.ID): {ID: "task-existing", WorkspaceID: "ws-1"},
	}

	edits := ApproveProposalRequest{ApproveFieldTitle: json.RawMessage(`"New Title"`)}
	_, err := svc.ApproveProposal(context.Background(), "ws-1", c.ID, p.ID, edits)
	_ = assertConflict(t, err, ProposalStatusFailed)
}

func TestApproveProposal_FailedFoundTaskNoEditsCompletesWithoutCreate(t *testing.T) {
	store, c, tasks, svc := approveFixture(t)
	p := insertProposal(t, store, c, sampleSpec())
	claimDirectly(t, store, p, "tok", sampleSpec(), time.Now())
	if _, err := store.FailProposal(context.Background(), p.ID, "tok", "boom", time.Now()); err != nil {
		t.Fatalf("FailProposal: %v", err)
	}
	tasks.lookupByExternalID = map[string]*taskmodels.Task{
		proposalExternalID(p.ID): {ID: "task-existing", WorkspaceID: "ws-1"},
	}

	got, err := svc.ApproveProposal(context.Background(), "ws-1", c.ID, p.ID, ApproveProposalRequest{})
	if err != nil {
		t.Fatalf("ApproveProposal: %v", err)
	}
	if got.Status != ProposalStatusApproved || got.TaskID == nil || *got.TaskID != "task-existing" {
		t.Fatalf("got = %+v, want approved/task-existing", got)
	}
	if len(tasks.createCalls) != 0 {
		t.Fatalf("createCalls = %d, want 0", len(tasks.createCalls))
	}
}

func TestApproveProposal_FailedNotFoundRevalidatesAndCreates(t *testing.T) {
	store, c, tasks, svc := approveFixture(t)
	p := insertProposal(t, store, c, sampleSpec())
	claimDirectly(t, store, p, "tok", sampleSpec(), time.Now())
	if _, err := store.FailProposal(context.Background(), p.ID, "tok", "boom", time.Now()); err != nil {
		t.Fatalf("FailProposal: %v", err)
	}
	tasks.createResult = createdResult("task-new")
	tasks.settled = true

	got, err := svc.ApproveProposal(context.Background(), "ws-1", c.ID, p.ID, ApproveProposalRequest{})
	if err != nil {
		t.Fatalf("ApproveProposal: %v", err)
	}
	if got.Status != ProposalStatusApproved || got.TaskID == nil || *got.TaskID != "task-new" {
		t.Fatalf("got = %+v, want approved/task-new", got)
	}
	if len(tasks.createCalls) != 1 {
		t.Fatalf("createCalls = %d, want 1", len(tasks.createCalls))
	}
}

// --- Group B: edits merge rules (proposals.md#edits) ---

func TestApproveProposal_EditNullFieldReturns400(t *testing.T) {
	store, c, _, svc := approveFixture(t)
	p := insertProposal(t, store, c, sampleSpec())

	edits := ApproveProposalRequest{ApproveFieldTitle: json.RawMessage(`null`)}
	_, err := svc.ApproveProposal(context.Background(), "ws-1", c.ID, p.ID, edits)
	assertFieldError(t, err, "title")

	reread, rerr := store.GetProposal(context.Background(), "ws-1", c.ID, p.ID)
	if rerr != nil {
		t.Fatalf("GetProposal: %v", rerr)
	}
	if reread.Status != ProposalStatusPending {
		t.Fatalf("Status = %q, want pending (no write should have happened)", reread.Status)
	}
}

func TestApproveProposal_EditEmptyTitleReturns400(t *testing.T) {
	store, c, _, svc := approveFixture(t)
	p := insertProposal(t, store, c, sampleSpec())

	edits := ApproveProposalRequest{ApproveFieldTitle: json.RawMessage(`"   "`)}
	_, err := svc.ApproveProposal(context.Background(), "ws-1", c.ID, p.ID, edits)
	assertFieldError(t, err, "title")
}

func TestApproveProposal_EditEmptyDescriptionClears(t *testing.T) {
	store, c, tasks, svc := approveFixture(t)
	p := insertProposal(t, store, c, sampleSpec())
	tasks.createResult = createdResult("task-new")
	tasks.settled = true

	edits := ApproveProposalRequest{ApproveFieldDescription: json.RawMessage(`""`)}
	got, err := svc.ApproveProposal(context.Background(), "ws-1", c.ID, p.ID, edits)
	if err != nil {
		t.Fatalf("ApproveProposal: %v", err)
	}
	if got.Status != ProposalStatusApproved {
		t.Fatalf("Status = %q, want approved", got.Status)
	}
	if tasks.createCalls[0].Description != "" {
		t.Fatalf("Description = %q, want empty", tasks.createCalls[0].Description)
	}
}

func TestApproveProposal_EditEmptyWorkflowIDReturns400(t *testing.T) {
	store, c, _, svc := approveFixture(t)
	p := insertProposal(t, store, c, sampleSpec())

	edits := ApproveProposalRequest{ApproveFieldWorkflowID: json.RawMessage(`""`)}
	_, err := svc.ApproveProposal(context.Background(), "ws-1", c.ID, p.ID, edits)
	assertFieldError(t, err, "workflow_id")
}

func TestApproveProposal_EditEmptyStepIDResolvesToStart(t *testing.T) {
	store, c, tasks, svc := approveFixture(t)
	spec := sampleSpec()
	spec.StepID = "manual-step"
	p := insertProposal(t, store, c, spec)
	tasks.createResult = createdResult("task-new")
	tasks.settled = true

	edits := ApproveProposalRequest{ApproveFieldStepID: json.RawMessage(`""`)}
	got, err := svc.ApproveProposal(context.Background(), "ws-1", c.ID, p.ID, edits)
	if err != nil {
		t.Fatalf("ApproveProposal: %v", err)
	}
	if got.Status != ProposalStatusApproved {
		t.Fatalf("Status = %q, want approved", got.Status)
	}
	if tasks.createCalls[0].WorkflowStepID != "step-1" {
		t.Fatalf("WorkflowStepID = %q, want step-1 (the start step)", tasks.createCalls[0].WorkflowStepID)
	}
}

func TestApproveProposal_EditEmptyRepositoryIDClears(t *testing.T) {
	store, c, tasks, svc := approveFixture(t)
	p := insertProposal(t, store, c, sampleSpec())
	tasks.createResult = createdResult("task-new")
	tasks.settled = true

	edits := ApproveProposalRequest{ApproveFieldRepositoryID: json.RawMessage(`""`)}
	got, err := svc.ApproveProposal(context.Background(), "ws-1", c.ID, p.ID, edits)
	if err != nil {
		t.Fatalf("ApproveProposal: %v", err)
	}
	if got.Status != ProposalStatusApproved {
		t.Fatalf("Status = %q, want approved", got.Status)
	}
	if len(tasks.createCalls[0].Repositories) != 0 {
		t.Fatalf("Repositories = %+v, want none", tasks.createCalls[0].Repositories)
	}
}

// byWorkflowStepReader is a WorkflowStepReader test double keyed by
// workflow id, used only by the workflow-changed edit tests below: the
// shared fakeStepReader ignores its workflowID argument, which cannot tell
// apart "wf-1"'s and "wf-2"'s distinct start steps.
type byWorkflowStepReader struct {
	steps map[string][]*workflowmodels.WorkflowStep
}

func (r *byWorkflowStepReader) ListStepsByWorkflow(_ context.Context, workflowID string) ([]*workflowmodels.WorkflowStep, error) {
	return r.steps[workflowID], nil
}

func TestApproveProposal_EditWorkflowChangedStepAbsentResetsToNewStart(t *testing.T) {
	store, c, tasks, svc := approveFixture(t)
	p := insertProposal(t, store, c, sampleSpec())
	tasks.workflows["wf-2"] = &taskmodels.Workflow{ID: "wf-2", WorkspaceID: "ws-1"}
	svc.SetDecisionDeps(tasks, &byWorkflowStepReader{steps: map[string][]*workflowmodels.WorkflowStep{
		"wf-1": {{ID: "step-1", IsStartStep: true}},
		"wf-2": {{ID: "wf2-start", IsStartStep: true}},
	}}, nil)
	tasks.createResult = createdResult("task-new")
	tasks.settled = true

	edits := ApproveProposalRequest{ApproveFieldWorkflowID: json.RawMessage(`"wf-2"`)}
	got, err := svc.ApproveProposal(context.Background(), "ws-1", c.ID, p.ID, edits)
	if err != nil {
		t.Fatalf("ApproveProposal: %v", err)
	}
	if got.Status != ProposalStatusApproved {
		t.Fatalf("Status = %q, want approved", got.Status)
	}
	req := tasks.createCalls[0]
	if req.WorkflowID != "wf-2" || req.WorkflowStepID != "wf2-start" {
		t.Fatalf("workflow/step = %q/%q, want wf-2/wf2-start", req.WorkflowID, req.WorkflowStepID)
	}
}

func TestApproveProposal_EditWorkflowChangedExplicitStepIDWins(t *testing.T) {
	store, c, tasks, svc := approveFixture(t)
	p := insertProposal(t, store, c, sampleSpec())
	tasks.workflows["wf-2"] = &taskmodels.Workflow{ID: "wf-2", WorkspaceID: "ws-1"}
	svc.SetDecisionDeps(tasks, &byWorkflowStepReader{steps: map[string][]*workflowmodels.WorkflowStep{
		"wf-1": {{ID: "step-1", IsStartStep: true}},
		"wf-2": {
			{ID: "wf2-start", IsStartStep: true},
			{ID: "wf2-other", AllowManualMove: true},
		},
	}}, nil)
	tasks.createResult = createdResult("task-new")
	tasks.settled = true

	edits := ApproveProposalRequest{
		ApproveFieldWorkflowID: json.RawMessage(`"wf-2"`),
		ApproveFieldStepID:     json.RawMessage(`"wf2-other"`),
	}
	got, err := svc.ApproveProposal(context.Background(), "ws-1", c.ID, p.ID, edits)
	if err != nil {
		t.Fatalf("ApproveProposal: %v", err)
	}
	if got.Status != ProposalStatusApproved {
		t.Fatalf("Status = %q, want approved", got.Status)
	}
	req := tasks.createCalls[0]
	if req.WorkflowID != "wf-2" || req.WorkflowStepID != "wf2-other" {
		t.Fatalf("workflow/step = %q/%q, want wf-2/wf2-other", req.WorkflowID, req.WorkflowStepID)
	}
}

// --- Group C: create-outcome branches ---

func TestApproveProposal_CreateFoundSettledCompletesWithoutSettle(t *testing.T) {
	store, c, tasks, svc := approveFixture(t)
	p := insertProposal(t, store, c, sampleSpec())
	tasks.createResult = taskservice.CreateTaskResult{
		Task:    &taskmodels.Task{ID: "task-existing", WorkspaceID: "ws-1"},
		Outcome: taskservice.CreateTaskOutcomeFoundSettled,
	}

	got, err := svc.ApproveProposal(context.Background(), "ws-1", c.ID, p.ID, ApproveProposalRequest{})
	if err != nil {
		t.Fatalf("ApproveProposal: %v", err)
	}
	if got.Status != ProposalStatusApproved || got.TaskID == nil || *got.TaskID != "task-existing" {
		t.Fatalf("got = %+v, want approved/task-existing", got)
	}
	if len(tasks.settleCalls) != 0 {
		t.Fatalf("settleCalls = %+v, want none", tasks.settleCalls)
	}
}

func TestApproveProposal_CreateFoundUnsettledCompletesWithoutSettle(t *testing.T) {
	store, c, tasks, svc := approveFixture(t)
	p := insertProposal(t, store, c, sampleSpec())
	tasks.createResult = taskservice.CreateTaskResult{
		Task:    &taskmodels.Task{ID: "task-existing", WorkspaceID: "ws-1"},
		Outcome: taskservice.CreateTaskOutcomeFoundUnsettled,
	}

	got, err := svc.ApproveProposal(context.Background(), "ws-1", c.ID, p.ID, ApproveProposalRequest{})
	if err != nil {
		t.Fatalf("ApproveProposal: %v", err)
	}
	if got.Status != ProposalStatusApproved || got.TaskID == nil || *got.TaskID != "task-existing" {
		t.Fatalf("got = %+v, want approved/task-existing", got)
	}
	if len(tasks.settleCalls) != 0 {
		t.Fatalf("settleCalls = %+v, want none", tasks.settleCalls)
	}
}

func TestApproveProposal_CreateSettleNotFoundFailsProposal(t *testing.T) {
	store, c, tasks, svc := approveFixture(t)
	p := insertProposal(t, store, c, sampleSpec())
	tasks.createResult = createdResult("task-new")
	tasks.settleErr = repoerrors.ErrTaskNotFound

	got, err := svc.ApproveProposal(context.Background(), "ws-1", c.ID, p.ID, ApproveProposalRequest{})
	if err != nil {
		t.Fatalf("ApproveProposal: %v", err)
	}
	if got.Status != ProposalStatusFailed {
		t.Fatalf("Status = %q, want failed", got.Status)
	}
	if got.TaskID != nil {
		t.Fatalf("TaskID = %v, want nil", got.TaskID)
	}
	if got.Error == nil || *got.Error == "" {
		t.Fatal("Error = nil, want an explanation")
	}
}

func TestApproveProposal_CreateSettleOtherErrorPropagatesAndLeavesApproving(t *testing.T) {
	store, c, tasks, svc := approveFixture(t)
	p := insertProposal(t, store, c, sampleSpec())
	tasks.createResult = createdResult("task-new")
	wantErr := errors.New("settle boom")
	tasks.settleErr = wantErr

	_, err := svc.ApproveProposal(context.Background(), "ws-1", c.ID, p.ID, ApproveProposalRequest{})
	if !errors.Is(err, wantErr) {
		t.Fatalf("err = %v, want %v", err, wantErr)
	}
	var conflict *ProposalConflictError
	if errors.As(err, &conflict) {
		t.Fatalf("err = %v, want a plain error, not a conflict", err)
	}

	reread, rerr := store.GetProposal(context.Background(), "ws-1", c.ID, p.ID)
	if rerr != nil {
		t.Fatalf("GetProposal: %v", rerr)
	}
	if reread.Status != ProposalStatusApproving {
		t.Fatalf("Status = %q, want approving (the row must stay untouched)", reread.Status)
	}
}

func TestApproveProposal_CreateSettledFalseCompletesWithSurvivor(t *testing.T) {
	store, c, tasks, svc := approveFixture(t)
	p := insertProposal(t, store, c, sampleSpec())
	tasks.createResult = createdResult("task-new")
	tasks.settled = false
	tasks.survivor = &taskmodels.Task{ID: "survivor-id", WorkspaceID: "ws-1"}

	got, err := svc.ApproveProposal(context.Background(), "ws-1", c.ID, p.ID, ApproveProposalRequest{})
	if err != nil {
		t.Fatalf("ApproveProposal: %v", err)
	}
	if got.Status != ProposalStatusApproved || got.TaskID == nil || *got.TaskID != "survivor-id" {
		t.Fatalf("got = %+v, want approved/survivor-id", got)
	}
}

func TestApproveProposal_CreateErrorFailsProposal(t *testing.T) {
	store, c, tasks, svc := approveFixture(t)
	p := insertProposal(t, store, c, sampleSpec())
	tasks.createErr = errors.New("create boom")

	got, err := svc.ApproveProposal(context.Background(), "ws-1", c.ID, p.ID, ApproveProposalRequest{})
	if err != nil {
		t.Fatalf("ApproveProposal: %v", err)
	}
	if got.Status != ProposalStatusFailed {
		t.Fatalf("Status = %q, want failed", got.Status)
	}
	if got.Error == nil || *got.Error != "create boom" {
		t.Fatalf("Error = %v, want create boom", got.Error)
	}
}

// --- Group D: step eligibility changing between checkpoints ---

func TestApproveProposal_StepIneligibleAtCreateTimeFailsWithoutCreating(t *testing.T) {
	store, c, tasks, svc := approveFixture(t)
	p := insertProposal(t, store, c, sampleSpec())
	svc.SetDecisionDeps(tasks, &fakeStepReader{stepsSeq: [][]*workflowmodels.WorkflowStep{
		{{ID: "step-1", IsStartStep: true}},
		{{ID: "step-1", IsStartStep: true, Events: workflowmodels.StepEvents{
			OnEnter: []workflowmodels.OnEnterAction{{Type: workflowmodels.OnEnterAutoStartAgent}},
		}}},
	}}, nil)

	got, err := svc.ApproveProposal(context.Background(), "ws-1", c.ID, p.ID, ApproveProposalRequest{})
	if err != nil {
		t.Fatalf("ApproveProposal: %v", err)
	}
	if got.Status != ProposalStatusFailed {
		t.Fatalf("Status = %q, want failed", got.Status)
	}
	if got.Error == nil || *got.Error != "the target step is no longer eligible" {
		t.Fatalf("Error = %v, want \"the target step is no longer eligible\"", got.Error)
	}
	if len(tasks.createCalls) != 0 {
		t.Fatalf("createCalls = %d, want 0", len(tasks.createCalls))
	}
}

func TestApproveProposal_StaleReclaimStepIneligibleFailsWithoutCreating(t *testing.T) {
	store, c, tasks, svc := approveFixture(t)
	p := insertProposal(t, store, c, sampleSpec())
	claimDirectly(t, store, p, "old-tok", sampleSpec(), time.Now().Add(-3*time.Minute))
	svc.SetDecisionDeps(tasks, &fakeStepReader{steps: []*workflowmodels.WorkflowStep{
		{ID: "step-1", IsStartStep: true, Events: workflowmodels.StepEvents{
			OnEnter: []workflowmodels.OnEnterAction{{Type: workflowmodels.OnEnterAutoStartAgent}},
		}},
	}}, nil)

	got, err := svc.ApproveProposal(context.Background(), "ws-1", c.ID, p.ID, ApproveProposalRequest{})
	if err != nil {
		t.Fatalf("ApproveProposal: %v", err)
	}
	if got.Status != ProposalStatusFailed {
		t.Fatalf("Status = %q, want failed", got.Status)
	}
	if len(tasks.createCalls) != 0 {
		t.Fatalf("createCalls = %d, want 0", len(tasks.createCalls))
	}
}
