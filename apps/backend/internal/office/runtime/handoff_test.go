package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	settingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/office/agents"
	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	"github.com/kandev/kandev/internal/task/service"
	workflowctrl "github.com/kandev/kandev/internal/workflow/controller"
	workflowmodels "github.com/kandev/kandev/internal/workflow/models"
)

// --- fakes for HandoffDependencies's sub-interfaces ---

type fakeHandoffScoper struct {
	err   error
	calls []string
}

func (f *fakeHandoffScoper) Scope(ctx context.Context, taskID string) (context.Context, error) {
	f.calls = append(f.calls, taskID)
	if f.err != nil {
		return nil, f.err
	}
	return ctx, nil
}

type fakeHandoffTasks struct {
	workspace       *taskmodels.Workspace
	workspaceErr    error
	workflow        *taskmodels.Workflow
	workflowErr     error
	executorProfile *taskmodels.ExecutorProfile
	executorErr     error
	repository      *taskmodels.Repository
	repositoryErr   error

	createFunc  func(*service.CreateTaskRequest) (service.CreateTaskResult, error)
	createErr   error
	createCalls []*service.CreateTaskRequest

	settled   bool
	survivor  *taskmodels.Task
	settleErr error
}

func (f *fakeHandoffTasks) GetWorkspace(_ context.Context, _ string) (*taskmodels.Workspace, error) {
	return f.workspace, f.workspaceErr
}

func (f *fakeHandoffTasks) GetWorkflow(_ context.Context, _ string) (*taskmodels.Workflow, error) {
	return f.workflow, f.workflowErr
}

func (f *fakeHandoffTasks) GetExecutorProfile(_ context.Context, _ string) (*taskmodels.ExecutorProfile, error) {
	return f.executorProfile, f.executorErr
}

func (f *fakeHandoffTasks) GetRepository(_ context.Context, _ string) (*taskmodels.Repository, error) {
	return f.repository, f.repositoryErr
}

func (f *fakeHandoffTasks) CreateTask(_ context.Context, req *service.CreateTaskRequest) (service.CreateTaskResult, error) {
	f.createCalls = append(f.createCalls, req)
	if f.createErr != nil {
		return service.CreateTaskResult{}, f.createErr
	}
	if f.createFunc != nil {
		return f.createFunc(req)
	}
	return service.CreateTaskResult{
		Task: &taskmodels.Task{
			ID:             "delivery-task-1",
			WorkspaceID:    req.WorkspaceID,
			WorkflowID:     req.WorkflowID,
			WorkflowStepID: req.WorkflowStepID,
			Title:          req.Title,
			Description:    req.Description,
			Metadata:       req.Metadata,
			ExternalID:     req.ExternalID,
		},
		Outcome: service.CreateTaskOutcomeCreated,
	}, nil
}

func (f *fakeHandoffTasks) SettleExternalID(_ context.Context, _, _ string) (bool, *taskmodels.Task, error) {
	if f.settleErr != nil {
		return false, nil, f.settleErr
	}
	return f.settled, f.survivor, nil
}

type fakeHandoffWorkflowSteps struct {
	steps []*workflowmodels.WorkflowStep
	err   error
}

func (f *fakeHandoffWorkflowSteps) ListStepsByWorkflow(_ context.Context, _ workflowctrl.ListStepsRequest) (*workflowctrl.ListStepsResponse, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &workflowctrl.ListStepsResponse{Steps: f.steps}, nil
}

type fakeHandoffAgentProfiles struct {
	belongs bool
	err     error
}

func (f *fakeHandoffAgentProfiles) AgentProfileBelongsToWorkspace(_ context.Context, _, _ string) (bool, error) {
	return f.belongs, f.err
}

// fakeHandoffReverseLinks is an in-memory CAS store mirroring the
// task-metadata-backed reverse-link store's contract.
type fakeHandoffReverseLinks struct {
	mu            sync.Mutex
	raw           string
	readErr       error
	casErr        error
	conflictCount int
}

func (f *fakeHandoffReverseLinks) GetTaskHandoffsRaw(_ context.Context, _ string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.readErr != nil {
		return "", f.readErr
	}
	return f.raw, nil
}

func (f *fakeHandoffReverseLinks) SetTaskHandoffsIfUnchanged(_ context.Context, _, expected, newValue string) (bool, string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.casErr != nil {
		return false, f.raw, f.casErr
	}
	if f.conflictCount > 0 {
		f.conflictCount--
		return false, f.raw, nil
	}
	if f.raw != expected {
		return false, f.raw, nil
	}
	f.raw = newValue
	return true, f.raw, nil
}

type fakeHandoffLauncher struct {
	err   error
	calls []HandoffLaunchRequest
}

func (f *fakeHandoffLauncher) LaunchSession(_ context.Context, req HandoffLaunchRequest) error {
	f.calls = append(f.calls, req)
	return f.err
}

type fakeHandoffActivityCall struct {
	workspaceID, actorID, action, targetType, targetID, runID, sessionID string
}

type fakeHandoffActivity struct {
	calls []fakeHandoffActivityCall
}

func (f *fakeHandoffActivity) LogActivityWithRun(
	_ context.Context, workspaceID, _, actorID, action, targetType, targetID, _, runID, sessionID string,
) {
	f.calls = append(f.calls, fakeHandoffActivityCall{workspaceID, actorID, action, targetType, targetID, runID, sessionID})
}

// --- shared fixtures ---

func baseHandoffDeps() (HandoffDependencies, *fakeHandoffTasks, *fakeHandoffReverseLinks, *fakeHandoffLauncher, *fakeHandoffActivity) {
	tasks := &fakeHandoffTasks{
		workspace:       &taskmodels.Workspace{ID: "ws-target"},
		workflow:        &taskmodels.Workflow{ID: "wf-1", WorkspaceID: "ws-target"},
		executorProfile: &taskmodels.ExecutorProfile{ID: "exec-profile-1", ExecutorID: "executor-1"},
		settled:         true,
	}
	reverseLinks := &fakeHandoffReverseLinks{}
	launcher := &fakeHandoffLauncher{}
	activity := &fakeHandoffActivity{}
	deps := HandoffDependencies{
		Workspaces:    &fakeHandoffScoper{},
		Tasks:         tasks,
		Workflows:     &fakeHandoffWorkflowSteps{steps: []*workflowmodels.WorkflowStep{{ID: "step-1", Position: 0, IsStartStep: true}}},
		AgentProfiles: &fakeHandoffAgentProfiles{belongs: true},
		ReverseLinks:  reverseLinks,
		Launcher:      launcher,
		Activity:      activity,
	}
	return deps, tasks, reverseLinks, launcher, activity
}

func baseHandoffRequest() HandoffRequest {
	return HandoffRequest{
		TargetWorkspaceID: "ws-target",
		WorkflowID:        "wf-1",
		Title:             "Delivery task",
		Prompt:            "Do the delivery work",
		AgentProfileID:    "agent-profile-1",
		ExecutorProfileID: "exec-profile-1",
	}
}

func baseHandoffRunContext() RunContext {
	return RunContext{
		WorkspaceID:  "ws-source",
		AgentID:      "agent-1",
		TaskID:       "task-source",
		RunID:        "run-1",
		SessionID:    "sess-1",
		Capabilities: Capabilities{CanHandoffTasks: true},
	}
}

// --- Actions.Handoff coverage ---

func TestActionsHandoff_CreatesTaskRecordsReverseLinkAndLogsActivity(t *testing.T) {
	deps, tasks, reverseLinks, launcher, activity := baseHandoffDeps()
	actions := NewActions(ActionDependencies{Handoff: deps})

	result, err := actions.Handoff(context.Background(), baseHandoffRunContext(), baseHandoffRequest())
	if err != nil {
		t.Fatalf("Handoff() error = %v, want nil", err)
	}
	if result.Outcome != handoffOutcomeCreated {
		t.Errorf("Outcome = %q, want %q", result.Outcome, handoffOutcomeCreated)
	}
	if !result.ReverseLinkRecorded {
		t.Errorf("ReverseLinkRecorded = false, want true")
	}
	if result.Started {
		t.Errorf("Started = true, want false (start_agent not requested)")
	}
	if len(tasks.createCalls) != 1 {
		t.Fatalf("CreateTask calls = %d, want 1", len(tasks.createCalls))
	}
	if reverseLinks.raw == "" {
		t.Error("reverse-link store was not written")
	}
	if len(launcher.calls) != 0 {
		t.Errorf("launcher calls = %d, want 0", len(launcher.calls))
	}
	if len(activity.calls) != 2 {
		t.Fatalf("activity calls = %d, want 2 (source + target)", len(activity.calls))
	}
}

// TestValidateHandoffShape is D3a step 2 (the R1 table): every required-field
// and shape check validateHandoffShape performs before any dependency is
// consulted, including AC-CROSS-WORKSPACE-TASK-HANDOFF-CLI-ROUTE-001.12's
// title-length boundary (60 runes shall pass, 61 shall be rejected naming
// both the limit and the actual rune length).
func TestValidateHandoffShape(t *testing.T) {
	blank := ""
	title60 := strings.Repeat("a", 60)
	title61 := strings.Repeat("a", 61)

	tests := []struct {
		name    string
		mutate  func(*HandoffRequest)
		wantErr string
	}{
		{
			name:    "missing target_workspace_id",
			mutate:  func(r *HandoffRequest) { r.TargetWorkspaceID = "  " },
			wantErr: "target_workspace_id is required",
		},
		{
			name:    "missing workflow_id",
			mutate:  func(r *HandoffRequest) { r.WorkflowID = "" },
			wantErr: "workflow_id is required",
		},
		{
			name:    "missing title",
			mutate:  func(r *HandoffRequest) { r.Title = "   " },
			wantErr: "title is required",
		},
		{
			name:    "title exactly 60 runes is accepted",
			mutate:  func(r *HandoffRequest) { r.Title = title60 },
			wantErr: "",
		},
		{
			name:    "title of 61 runes is rejected naming limit and actual length",
			mutate:  func(r *HandoffRequest) { r.Title = title61 },
			wantErr: "title must be 60 characters or fewer (got 61)",
		},
		{
			name:    "missing prompt",
			mutate:  func(r *HandoffRequest) { r.Prompt = "" },
			wantErr: "prompt is required",
		},
		{
			name:    "missing agent_profile_id",
			mutate:  func(r *HandoffRequest) { r.AgentProfileID = "" },
			wantErr: "agent_profile_id is required",
		},
		{
			name:    "missing executor_profile_id",
			mutate:  func(r *HandoffRequest) { r.ExecutorProfileID = "" },
			wantErr: "executor_profile_id is required",
		},
		{
			name:    "blank repository_id when supplied",
			mutate:  func(r *HandoffRequest) { r.RepositoryID = &blank },
			wantErr: "repository_id must not be blank when supplied",
		},
		{
			name:    "blank base_branch when supplied",
			mutate:  func(r *HandoffRequest) { r.BaseBranch = &blank },
			wantErr: "base_branch must not be blank when supplied",
		},
		{
			name:    "blank external_id when supplied",
			mutate:  func(r *HandoffRequest) { r.ExternalID = &blank },
			wantErr: "external_id must not be blank when supplied",
		},
		{
			name: "external_id with a raw control character is rejected, not silently trimmed",
			mutate: func(r *HandoffRequest) {
				v := "key\n"
				r.ExternalID = &v
			},
			wantErr: "external_id is invalid: invalid external_id: control characters are not allowed",
		},
		{
			name: "oversized external_id is rejected",
			mutate: func(r *HandoffRequest) {
				v := strings.Repeat("a", service.ExternalIDMaxBytes+1)
				r.ExternalID = &v
			},
			wantErr: fmt.Sprintf("external_id is invalid: invalid external_id: must be %d UTF-8 bytes or fewer", service.ExternalIDMaxBytes),
		},
		{
			name:    "fully valid request",
			mutate:  func(_ *HandoffRequest) {},
			wantErr: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := baseHandoffRequest()
			tt.mutate(&req)

			_, err := validateHandoffShape(req)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("validateHandoffShape() error = %v, want nil", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("validateHandoffShape() error = nil, want %q", tt.wantErr)
			}
			if err.Error() != tt.wantErr {
				t.Errorf("validateHandoffShape() error = %q, want %q", err.Error(), tt.wantErr)
			}
		})
	}
}

// TestActionsHandoff_SameWorkspaceRefused is AC-22: D3a step 3 rejects a
// same-workspace target before any dependency is consulted.
func TestActionsHandoff_SameWorkspaceRefused(t *testing.T) {
	deps, tasks, _, _, _ := baseHandoffDeps()
	actions := NewActions(ActionDependencies{Handoff: deps})
	runCtx := baseHandoffRunContext()
	req := baseHandoffRequest()
	req.TargetWorkspaceID = runCtx.WorkspaceID

	_, err := actions.Handoff(context.Background(), runCtx, req)
	var validation *HandoffValidationError
	if !errors.As(err, &validation) {
		t.Fatalf("error = %v, want *HandoffValidationError", err)
	}
	if len(tasks.createCalls) != 0 {
		t.Error("CreateTask was called despite same-workspace refusal")
	}
}

// TestActionsHandoff_CapabilityDenied is AC-9/AC-10: D3a step 4.
func TestActionsHandoff_CapabilityDenied(t *testing.T) {
	deps, tasks, _, _, _ := baseHandoffDeps()
	actions := NewActions(ActionDependencies{Handoff: deps})
	runCtx := baseHandoffRunContext()
	runCtx.Capabilities = Capabilities{CanHandoffTasks: false}

	_, err := actions.Handoff(context.Background(), runCtx, baseHandoffRequest())
	if !errors.Is(err, errHandoffPermissionDenied) {
		t.Fatalf("error = %v, want errHandoffPermissionDenied", err)
	}
	if len(tasks.createCalls) != 0 {
		t.Error("CreateTask was called despite capability denial")
	}
}

// TestActionsHandoff_TargetWorkspaceNotFound is AC-11.
func TestActionsHandoff_TargetWorkspaceNotFound(t *testing.T) {
	deps, _, _, _, _ := baseHandoffDeps()
	deps.Tasks.(*fakeHandoffTasks).workspace = nil
	deps.Tasks.(*fakeHandoffTasks).workspaceErr = repoerrors.ErrWorkspaceNotFound
	actions := NewActions(ActionDependencies{Handoff: deps})

	_, err := actions.Handoff(context.Background(), baseHandoffRunContext(), baseHandoffRequest())
	var validation *HandoffValidationError
	if !errors.As(err, &validation) {
		t.Fatalf("error = %v, want *HandoffValidationError", err)
	}
}

// TestActionsHandoff_WorkflowNotInTargetWorkspace is AC-12: a workflow that
// resolves but belongs to a different workspace is reported the same as
// not-found (AC-12a), never disclosing its real owner.
func TestActionsHandoff_WorkflowNotInTargetWorkspace(t *testing.T) {
	deps, tasks, _, _, _ := baseHandoffDeps()
	tasks.workflow = &taskmodels.Workflow{ID: "wf-1", WorkspaceID: "some-other-workspace"}
	actions := NewActions(ActionDependencies{Handoff: deps})

	_, err := actions.Handoff(context.Background(), baseHandoffRunContext(), baseHandoffRequest())
	var validation *HandoffValidationError
	if !errors.As(err, &validation) {
		t.Fatalf("error = %v, want *HandoffValidationError", err)
	}
	if !strings.Contains(validation.Error(), "not a workflow of the target workspace") {
		t.Errorf("error = %q, want it to name the workflow_id membership failure", validation.Error())
	}
}

// TestActionsHandoff_AgentProfileNotInTargetWorkspace is AC-14b.
func TestActionsHandoff_AgentProfileNotInTargetWorkspace(t *testing.T) {
	deps, _, _, _, _ := baseHandoffDeps()
	deps.AgentProfiles = &fakeHandoffAgentProfiles{belongs: false}
	actions := NewActions(ActionDependencies{Handoff: deps})

	_, err := actions.Handoff(context.Background(), baseHandoffRunContext(), baseHandoffRequest())
	var validation *HandoffValidationError
	if !errors.As(err, &validation) {
		t.Fatalf("error = %v, want *HandoffValidationError", err)
	}
}

// TestActionsHandoff_ExecutorProfileDoesNotResolve is AC-14b's
// executor_profile_id predicate.
func TestActionsHandoff_ExecutorProfileDoesNotResolve(t *testing.T) {
	deps, tasks, _, _, _ := baseHandoffDeps()
	tasks.executorProfile = nil
	tasks.executorErr = errors.New("executor profile not found")
	actions := NewActions(ActionDependencies{Handoff: deps})

	_, err := actions.Handoff(context.Background(), baseHandoffRunContext(), baseHandoffRequest())
	var validation *HandoffValidationError
	if !errors.As(err, &validation) {
		t.Fatalf("error = %v, want *HandoffValidationError", err)
	}
}

// TestActionsHandoff_SettlementFailureReturnsParseableSuffix is F53/AC-24d:
// the settlement error must carry the delivery task id via the stable
// suffix, and no reverse link, activity, or launch happens afterward.
func TestActionsHandoff_SettlementFailureReturnsParseableSuffix(t *testing.T) {
	deps, tasks, reverseLinks, launcher, activity := baseHandoffDeps()
	tasks.settleErr = errors.New("db unavailable")
	actions := NewActions(ActionDependencies{Handoff: deps})

	_, err := actions.Handoff(context.Background(), baseHandoffRunContext(), baseHandoffRequest())
	var settlement *HandoffSettlementError
	if !errors.As(err, &settlement) {
		t.Fatalf("error = %v, want *HandoffSettlementError", err)
	}
	if settlement.TaskID != "delivery-task-1" {
		t.Errorf("TaskID = %q, want %q", settlement.TaskID, "delivery-task-1")
	}
	wantSuffix := handoffSettlementTaskIDSuffix + "delivery-task-1"
	if !strings.HasSuffix(err.Error(), wantSuffix) {
		t.Errorf("error = %q, want it to end with %q", err.Error(), wantSuffix)
	}
	if strings.Contains(err.Error(), "db unavailable") {
		t.Errorf("error = %q, must not echo the raw settlement cause beyond the delivery task id (spec's settlement-500 carve-out)", err.Error())
	}
	if unwrapped := settlement.Unwrap(); unwrapped == nil || unwrapped.Error() != "db unavailable" {
		t.Errorf("Unwrap() = %v, want the raw settlement cause to remain reachable for logging", unwrapped)
	}
	if reverseLinks.raw != "" {
		t.Error("reverse link was written despite settlement failure")
	}
	if len(launcher.calls) != 0 {
		t.Error("launch was dispatched despite settlement failure")
	}
	if len(activity.calls) != 0 {
		t.Error("activity was logged despite settlement failure")
	}
}

// TestActionsHandoff_CreateTaskExternalIDInvalidSurfacesAs400 is defense in
// depth for validateHandoffShape's own external_id check: even if CreateTask
// itself rejects the (already-normalized) external_id via
// service.ErrExternalIDInvalid, the handoff action must translate that into
// a *HandoffValidationError (400), not fall through to the generic 500 path.
func TestActionsHandoff_CreateTaskExternalIDInvalidSurfacesAs400(t *testing.T) {
	deps, tasks, _, _, _ := baseHandoffDeps()
	tasks.createErr = fmt.Errorf("%w: must be %d UTF-8 bytes or fewer", service.ErrExternalIDInvalid, service.ExternalIDMaxBytes)
	req := baseHandoffRequest()
	externalID := "ext-1"
	req.ExternalID = &externalID
	actions := NewActions(ActionDependencies{Handoff: deps})

	_, err := actions.Handoff(context.Background(), baseHandoffRunContext(), req)
	var validation *HandoffValidationError
	if !errors.As(err, &validation) {
		t.Fatalf("error = %v, want *HandoffValidationError", err)
	}
	if !strings.Contains(validation.Error(), "external_id is invalid") {
		t.Errorf("error = %q, want it to name external_id as invalid", validation.Error())
	}
}

// TestActionsHandoff_CreatedIdentityLostUsesOriginalTaskID is AC-24d's other
// branch: SettleExternalID succeeds (no error) but reports the row was not
// settled. The response still names the newly-created task, not the
// survivor that kept the external_id.
func TestActionsHandoff_CreatedIdentityLostUsesOriginalTaskID(t *testing.T) {
	deps, tasks, _, _, _ := baseHandoffDeps()
	tasks.settled = false
	tasks.survivor = &taskmodels.Task{ID: "other-task", WorkflowID: "wf-1", WorkflowStepID: "step-2"}
	req := baseHandoffRequest()
	externalID := "ext-1"
	req.ExternalID = &externalID
	actions := NewActions(ActionDependencies{Handoff: deps})

	result, err := actions.Handoff(context.Background(), baseHandoffRunContext(), req)
	if err != nil {
		t.Fatalf("Handoff() error = %v, want nil", err)
	}
	if result.Outcome != handoffOutcomeCreatedIdentityLost {
		t.Errorf("Outcome = %q, want %q", result.Outcome, handoffOutcomeCreatedIdentityLost)
	}
	if result.TaskID != "delivery-task-1" {
		t.Errorf("TaskID = %q, want the created task's id, not the survivor's", result.TaskID)
	}
	if result.Message == "" {
		t.Error("Message is empty, want AC-24d's caller guidance")
	}
}

// TestActionsHandoff_FoundSettledRepairsReverseLinkWithoutRecreating is
// AC-24a/AC-25: a replay that resolves to an existing settled task skips
// SettleExternalID and the launch, but still logs activity and repairs the
// reverse link.
func TestActionsHandoff_FoundSettledRepairsReverseLinkWithoutRecreating(t *testing.T) {
	deps, tasks, reverseLinks, launcher, activity := baseHandoffDeps()
	found := &taskmodels.Task{
		ID:             "delivery-task-1",
		WorkflowID:     "wf-1",
		WorkflowStepID: "step-1",
		Metadata: map[string]interface{}{
			taskmodels.MetaKeyHandoffSource: map[string]interface{}{
				"source_task_id": "task-source",
				"handed_off_at":  "2026-01-01T00:00:00.000Z",
			},
		},
	}
	tasks.createFunc = func(_ *service.CreateTaskRequest) (service.CreateTaskResult, error) {
		return service.CreateTaskResult{Task: found, Outcome: service.CreateTaskOutcomeFoundSettled}, nil
	}
	actions := NewActions(ActionDependencies{Handoff: deps})

	result, err := actions.Handoff(context.Background(), baseHandoffRunContext(), baseHandoffRequest())
	if err != nil {
		t.Fatalf("Handoff() error = %v, want nil", err)
	}
	if result.Outcome != handoffOutcomeFoundSettled {
		t.Errorf("Outcome = %q, want %q", result.Outcome, handoffOutcomeFoundSettled)
	}
	if !result.CreationComplete {
		t.Error("CreationComplete = false, want true for found_settled")
	}
	if !result.ReverseLinkRecorded {
		t.Error("ReverseLinkRecorded = false, want true")
	}
	if reverseLinks.raw == "" {
		t.Error("reverse-link store was not written on the found-settled repair path")
	}
	if len(launcher.calls) != 0 {
		t.Error("launch was dispatched on a found outcome")
	}
	if len(activity.calls) != 2 {
		t.Errorf("activity calls = %d, want 2 even on the found outcome (AC-19)", len(activity.calls))
	}
}

// TestActionsHandoff_FoundOutcomeRefusedWhenSourceMismatched is AC-25a: a
// found task whose handoff_source does not name this source task is a hard
// refusal, disclosing neither the task id nor a reverse link.
func TestActionsHandoff_FoundOutcomeRefusedWhenSourceMismatched(t *testing.T) {
	deps, _, reverseLinks, _, activity := baseHandoffDeps()
	found := &taskmodels.Task{
		ID: "someone-elses-task",
		Metadata: map[string]interface{}{
			taskmodels.MetaKeyHandoffSource: map[string]interface{}{
				"source_task_id": "a-different-source-task",
				"handed_off_at":  "2026-01-01T00:00:00.000Z",
			},
		},
	}
	deps.Tasks.(*fakeHandoffTasks).createFunc = func(_ *service.CreateTaskRequest) (service.CreateTaskResult, error) {
		return service.CreateTaskResult{Task: found, Outcome: service.CreateTaskOutcomeFoundSettled}, nil
	}
	actions := NewActions(ActionDependencies{Handoff: deps})

	_, err := actions.Handoff(context.Background(), baseHandoffRunContext(), baseHandoffRequest())
	var validation *HandoffValidationError
	if !errors.As(err, &validation) {
		t.Fatalf("error = %v, want *HandoffValidationError", err)
	}
	if reverseLinks.raw != "" {
		t.Error("reverse link was written despite the source mismatch refusal")
	}
	if len(activity.calls) != 0 {
		t.Error("activity was logged despite the source mismatch refusal")
	}
}

// TestActionsHandoff_ReverseLinkCASRetrySucceeds is AC-27: a stale
// compare-and-set is retried from a fresh read rather than failing outright.
func TestActionsHandoff_ReverseLinkCASRetrySucceeds(t *testing.T) {
	deps, _, reverseLinks, _, _ := baseHandoffDeps()
	reverseLinks.conflictCount = 2
	actions := NewActions(ActionDependencies{Handoff: deps})

	result, err := actions.Handoff(context.Background(), baseHandoffRunContext(), baseHandoffRequest())
	if err != nil {
		t.Fatalf("Handoff() error = %v, want nil", err)
	}
	if !result.ReverseLinkRecorded {
		t.Errorf("ReverseLinkRecorded = false, want true after retrying past 2 stale writes")
	}
}

// TestActionsHandoff_LaunchDispatchedWhenStartAgentTrue is AC-32.
func TestActionsHandoff_LaunchDispatchedWhenStartAgentTrue(t *testing.T) {
	deps, _, _, launcher, _ := baseHandoffDeps()
	actions := NewActions(ActionDependencies{Handoff: deps})
	req := baseHandoffRequest()
	start := true
	req.StartAgent = &start

	result, err := actions.Handoff(context.Background(), baseHandoffRunContext(), req)
	if err != nil {
		t.Fatalf("Handoff() error = %v, want nil", err)
	}
	if !result.Started {
		t.Error("Started = false, want true")
	}
	if len(launcher.calls) != 1 {
		t.Fatalf("launcher calls = %d, want 1", len(launcher.calls))
	}
	if launcher.calls[0].TaskID != "delivery-task-1" {
		t.Errorf("launch TaskID = %q, want %q", launcher.calls[0].TaskID, "delivery-task-1")
	}
}

// TestActionsHandoff_LaunchErrorSurfacedAsStartErrorNotFailure is AC-32: a
// launch failure does not fail the handoff call itself.
func TestActionsHandoff_LaunchErrorSurfacedAsStartErrorNotFailure(t *testing.T) {
	deps, _, _, launcher, _ := baseHandoffDeps()
	launcher.err = errors.New("no capacity")
	actions := NewActions(ActionDependencies{Handoff: deps})
	req := baseHandoffRequest()
	start := true
	req.StartAgent = &start

	result, err := actions.Handoff(context.Background(), baseHandoffRunContext(), req)
	if err != nil {
		t.Fatalf("Handoff() error = %v, want nil (launch failure is non-fatal)", err)
	}
	if result.Started {
		t.Error("Started = true, want false")
	}
	if result.StartError == "" {
		t.Error("StartError is empty, want the launch error surfaced")
	}
}

// TestActionsHandoff_EmptyRunIDStillLogsActivity is AC-19a/F54(b): identity
// fields are read directly off RunContext with no run lookup, so an empty
// run id is written as empty rather than skipped.
func TestActionsHandoff_EmptyRunIDStillLogsActivity(t *testing.T) {
	deps, _, _, _, activity := baseHandoffDeps()
	actions := NewActions(ActionDependencies{Handoff: deps})
	runCtx := baseHandoffRunContext()
	runCtx.RunID = ""

	_, err := actions.Handoff(context.Background(), runCtx, baseHandoffRequest())
	if err != nil {
		t.Fatalf("Handoff() error = %v, want nil", err)
	}
	if len(activity.calls) != 2 {
		t.Fatalf("activity calls = %d, want 2 (source + target)", len(activity.calls))
	}
	for _, call := range activity.calls {
		if call.runID != "" {
			t.Errorf("logged runID = %q, want empty string to be written rather than skipped", call.runID)
		}
	}
}

// --- HTTP-level coverage: Handler.contextFromRequest's live-permission
// precedence (AC-9), reachable only through the route, and F53's response
// envelope. ---

type handoffHTTPHarness struct {
	router *gin.Engine
	token  string
}

func newHandoffHTTPHarness(t *testing.T, tokenCaps Capabilities, agent *models.AgentInstance, handoffDeps HandoffDependencies) *handoffHTTPHarness {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, _, err := settingsstore.Provide(db, db, nil); err != nil {
		t.Fatalf("settings store init: %v", err)
	}
	repo, err := sqlite.NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("new repo: %v", err)
	}
	log := logger.Default()
	agentSvc := agents.NewAgentService(repo, log, nil)
	agentSvc.SetAuth(agents.NewAgentAuth("handoff-handler-test-key"))
	if err := repo.CreateAgentInstance(context.Background(), agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	capabilityJSON, err := MarshalCapabilities(tokenCaps)
	if err != nil {
		t.Fatalf("marshal capabilities: %v", err)
	}
	token, err := agentSvc.MintRuntimeJWT(agent.ID, "task-source", agent.WorkspaceID, "run-1", "sess-1", capabilityJSON)
	if err != nil {
		t.Fatalf("mint runtime token: %v", err)
	}
	router := gin.New()
	RegisterRoutes(router.Group(""), NewHandler(
		agentSvc,
		NewActions(ActionDependencies{Handoff: handoffDeps}),
		nil,
		&recordingRunEvents{},
		&recordingDecisionRecorder{},
		log,
	))
	return &handoffHTTPHarness{router: router, token: token}
}

func (h *handoffHTTPHarness) request(t *testing.T, payload interface{}) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/runtime/handoffs", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+h.token)
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	h.router.ServeHTTP(resp, req)
	return resp
}

// TestHandoffHandler_LivePermissionGrantedAfterTokenMintHonoured is F54(a),
// AC-9's inverse: a run token signed before the permission was granted must
// not shadow a live grant. The token's capability snapshot says
// can_handoff_tasks=false, but the underlying agent is a CEO, which
// FromAgent grants handoff_task to by default — contextFromRequest must
// re-derive the live value rather than trusting the signed snapshot.
func TestHandoffHandler_LivePermissionGrantedAfterTokenMintHonoured(t *testing.T) {
	deps, _, _, _, _ := baseHandoffDeps()
	agent := &models.AgentInstance{
		ID:          "agent-1",
		WorkspaceID: "ws-source",
		Name:        "CEO",
		Role:        models.AgentRoleCEO,
	}
	h := newHandoffHTTPHarness(t, Capabilities{CanHandoffTasks: false}, agent, deps)

	resp := h.request(t, baseHandoffRequest())

	if resp.Code == http.StatusForbidden {
		t.Fatalf("handoff denied for missing capability despite a live grant; body=%s", resp.Body.String())
	}
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", resp.Code, resp.Body.String())
	}
}

// TestHandoffHandler_LivePermissionRevokedOverridesStaleGrant is the mirror
// direction: a stale token claiming can_handoff_tasks=true must not survive
// a live role that no longer grants it.
func TestHandoffHandler_LivePermissionRevokedOverridesStaleGrant(t *testing.T) {
	agent := &models.AgentInstance{
		ID:          "agent-1",
		WorkspaceID: "ws-source",
		Name:        "Worker",
		Role:        models.AgentRoleWorker,
	}
	h := newHandoffHTTPHarness(t, Capabilities{CanHandoffTasks: true}, agent, HandoffDependencies{})

	resp := h.request(t, baseHandoffRequest())

	if resp.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403 (live permission revoked must override a stale grant); body=%s", resp.Code, resp.Body.String())
	}
}

// TestHandoffHandler_SettlementFailureRespondsWithParseableTaskIDSuffix is
// F53 at the HTTP layer: the 500 body must carry the delivery task id via
// the stable suffix, not prose the caller would have to parse.
func TestHandoffHandler_SettlementFailureRespondsWithParseableTaskIDSuffix(t *testing.T) {
	deps, tasks, _, _, _ := baseHandoffDeps()
	tasks.settleErr = errors.New("db unavailable")
	agent := &models.AgentInstance{
		ID:          "agent-1",
		WorkspaceID: "ws-source",
		Name:        "CEO",
		Role:        models.AgentRoleCEO,
	}
	h := newHandoffHTTPHarness(t, Capabilities{CanHandoffTasks: true}, agent, deps)

	resp := h.request(t, baseHandoffRequest())

	if resp.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500; body=%s", resp.Code, resp.Body.String())
	}
	var body struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	wantSuffix := handoffSettlementTaskIDSuffix + "delivery-task-1"
	if !strings.HasSuffix(body.Error, wantSuffix) {
		t.Errorf("error = %q, want it to end with the parseable suffix %q", body.Error, wantSuffix)
	}
	if strings.Contains(body.Error, "db unavailable") {
		t.Errorf("error = %q, must not leak the raw settlement cause to the HTTP caller", body.Error)
	}
}
