package runtime

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/kandev/kandev/internal/office/models"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/service"
)

// TestActionsHandoff_TasklessRunRefused proves a run with no owning task
// (runCtx.TaskID == "") is refused before any workspace scoping is
// attempted. An empty task ID has no owning identity to scope against;
// letting it reach Workspaces.Scope resolves to an unscoped caller, which
// the task service treats as an internal caller with owner rights on every
// workspace — bypassing the target-workspace ownership check entirely.
func TestActionsHandoff_TasklessRunRefused(t *testing.T) {
	deps, tasks, _, _, _ := baseHandoffDeps()
	scoper := deps.Workspaces.(*fakeHandoffScoper)
	actions := NewActions(ActionDependencies{Handoff: deps})
	runCtx := baseHandoffRunContext()
	runCtx.TaskID = ""

	_, err := actions.Handoff(context.Background(), runCtx, baseHandoffRequest())
	if !errors.Is(err, errHandoffTasklessRun) {
		t.Fatalf("error = %v, want errHandoffTasklessRun", err)
	}
	if len(scoper.calls) != 0 {
		t.Errorf("Workspaces.Scope was called (%v) despite a taskless run; must refuse before scoping", scoper.calls)
	}
	if len(tasks.createCalls) != 0 {
		t.Error("CreateTask was called despite a taskless run")
	}
}

// TestActionsHandoff_SessionlessRunRefused proves a run with no session id
// (runCtx.SessionID == "") is refused before any workspace scoping is
// attempted, mirroring TestActionsHandoff_TasklessRunRefused. AC-CROSS-
// WORKSPACE-TASK-HANDOFF-PROVENANCE-001.2 requires refusal on an empty
// source task id OR source session id: the forward provenance record and the
// activity log both name the source session, so a handoff recorded without
// one would leave provenance that cannot be attributed back to its run.
func TestActionsHandoff_SessionlessRunRefused(t *testing.T) {
	deps, tasks, _, _, _ := baseHandoffDeps()
	scoper := deps.Workspaces.(*fakeHandoffScoper)
	actions := NewActions(ActionDependencies{Handoff: deps})
	runCtx := baseHandoffRunContext()
	runCtx.SessionID = ""

	_, err := actions.Handoff(context.Background(), runCtx, baseHandoffRequest())
	if !errors.Is(err, errHandoffSessionlessRun) {
		t.Fatalf("error = %v, want errHandoffSessionlessRun", err)
	}
	if len(scoper.calls) != 0 {
		t.Errorf("Workspaces.Scope was called (%v) despite a sessionless run; must refuse before scoping", scoper.calls)
	}
	if len(tasks.createCalls) != 0 {
		t.Error("CreateTask was called despite a sessionless run")
	}
}

// TestActionsHandoff_LaunchIndependentOfReverseLinkFailure proves the launch
// dispatch (AC-32) and the reverse-link write (AC-17) are independent
// outcomes: a reverse-link write failure must not prevent a requested launch
// from happening, and vice versa the launch's own success is unaffected by
// ReverseLinkRecorded being false.
func TestActionsHandoff_LaunchIndependentOfReverseLinkFailure(t *testing.T) {
	deps, _, reverseLinks, launcher, _ := baseHandoffDeps()
	reverseLinks.casErr = errors.New("cas store unavailable")
	actions := NewActions(ActionDependencies{Handoff: deps})
	req := baseHandoffRequest()
	start := true
	req.StartAgent = &start

	result, err := actions.Handoff(context.Background(), baseHandoffRunContext(), req)
	if err != nil {
		t.Fatalf("Handoff() error = %v, want nil", err)
	}
	if result.ReverseLinkRecorded {
		t.Fatal("ReverseLinkRecorded = true, want false given a forced CAS error")
	}
	if result.ReverseLinkError == "" {
		t.Error("ReverseLinkError = \"\", want a non-empty message given a forced CAS error")
	}
	if !result.Started {
		t.Error("Started = false, want true: a reverse-link failure must not block the requested launch")
	}
	if len(launcher.calls) != 1 {
		t.Fatalf("launcher calls = %d, want 1", len(launcher.calls))
	}
}

// TestHandoffHandler_TargetWorkspaceForbiddenRespondsWith403 proves
// task/service.ErrForbidden — returned when the caller's target-workspace
// membership lacks task.write scope — classifies as a 403, not the generic
// 500 respondRuntimeError falls back to for an error it does not recognize.
// office/shared.ErrForbidden and task/service.ErrForbidden are distinct
// sentinels in different packages, so a denial from CreateTask must be
// checked against both, or it is neither reported as 403 nor logged as a
// denied run event.
func TestHandoffHandler_TargetWorkspaceForbiddenRespondsWith403(t *testing.T) {
	deps, tasks, _, _, _ := baseHandoffDeps()
	tasks.createErr = service.ErrForbidden
	agent := &models.AgentInstance{
		ID:          "agent-1",
		WorkspaceID: "ws-source",
		Name:        "CEO",
		Role:        models.AgentRoleCEO,
	}
	h := newHandoffHTTPHarness(t, Capabilities{CanHandoffTasks: true}, agent, deps)

	resp := h.request(t, baseHandoffRequest())

	if resp.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403 for task/service.ErrForbidden; body=%s", resp.Code, resp.Body.String())
	}
}

// TestActionsHandoff_LogsActivityWithExactContent proves the two
// LogActivityWithRun calls carry the correct action verb, workspace and
// counterpart task id per entry, not just a count of two. Swapping the
// source/target verbs or workspace ids would still satisfy a count-only
// assertion but would misattribute the activity in each workspace's feed.
func TestActionsHandoff_LogsActivityWithExactContent(t *testing.T) {
	deps, _, _, _, activity := baseHandoffDeps()
	actions := NewActions(ActionDependencies{Handoff: deps})
	runCtx := baseHandoffRunContext()

	_, err := actions.Handoff(context.Background(), runCtx, baseHandoffRequest())
	if err != nil {
		t.Fatalf("Handoff() error = %v, want nil", err)
	}
	if len(activity.calls) != 2 {
		t.Fatalf("activity calls = %d, want 2 (source + target)", len(activity.calls))
	}

	source := activity.calls[0]
	wantSource := fakeHandoffActivityCall{
		workspaceID: runCtx.WorkspaceID,
		actorID:     runCtx.AgentID,
		action:      "task.handed_off",
		targetType:  "task",
		targetID:    runCtx.TaskID,
		runID:       runCtx.RunID,
		sessionID:   runCtx.SessionID,
	}
	if source != wantSource {
		t.Errorf("source-side activity call = %+v, want %+v", source, wantSource)
	}

	target := activity.calls[1]
	wantTarget := fakeHandoffActivityCall{
		workspaceID: "ws-target",
		actorID:     runCtx.AgentID,
		action:      "task.handoff_received",
		targetType:  "task",
		targetID:    "delivery-task-1",
		runID:       runCtx.RunID,
		sessionID:   runCtx.SessionID,
	}
	if target != wantTarget {
		t.Errorf("target-side activity call = %+v, want %+v", target, wantTarget)
	}
}

// TestParseHandoffEntries is AC-27's exhaustive corruption-detection table:
// an absent handoffs key is empty-but-not-corrupt, while every other
// malformed shape is refused so a reverse-link append never silently drops
// or mangles sibling entries. A well-formed entry carrying unknown fields
// must survive byte-for-byte, since decode/re-encode through
// map[string]interface{} would silently corrupt additive numeric fields
// outside float64's exact integer range.
func TestParseHandoffEntries(t *testing.T) {
	tests := []struct {
		name           string
		raw            string
		deliveryTaskID string
		wantEntries    int
		wantPresent    bool
		wantCorrupt    bool
	}{
		{
			name:        "empty raw is absent, not corrupt",
			raw:         "",
			wantEntries: 0,
			wantCorrupt: false,
		},
		{
			name:        "whitespace-only raw is absent, not corrupt",
			raw:         "   ",
			wantEntries: 0,
			wantCorrupt: false,
		},
		{
			name:        "literal null is corrupt (present but not an array)",
			raw:         "null",
			wantCorrupt: true,
		},
		{
			name:        "non-array JSON is corrupt",
			raw:         `{"task_id":"t1","handed_off_at":"2026-01-01T00:00:00.000Z"}`,
			wantCorrupt: true,
		},
		{
			name:        "unparseable JSON is corrupt",
			raw:         `[{"task_id":`,
			wantCorrupt: true,
		},
		{
			name:        "array element that is not an object is corrupt",
			raw:         `[42]`,
			wantCorrupt: true,
		},
		{
			name:        "entry missing task_id is corrupt",
			raw:         `[{"task_id":"","handed_off_at":"2026-01-01T00:00:00.000Z"}]`,
			wantCorrupt: true,
		},
		{
			name:        "entry with unparseable handed_off_at is corrupt",
			raw:         `[{"task_id":"t1","handed_off_at":"not-a-timestamp"}]`,
			wantCorrupt: true,
		},
		{
			name:           "well-formed entries, delivery task not yet present",
			raw:            `[{"task_id":"t1","handed_off_at":"2026-01-01T00:00:00.000Z"},{"task_id":"t2","handed_off_at":"2026-01-02T00:00:00.000Z"}]`,
			deliveryTaskID: "t3",
			wantEntries:    2,
			wantPresent:    false,
		},
		{
			name:           "delivery task already present is reported, not duplicated",
			raw:            `[{"task_id":"t1","handed_off_at":"2026-01-01T00:00:00.000Z"}]`,
			deliveryTaskID: "t1",
			wantEntries:    1,
			wantPresent:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entries, present, corrupt := parseHandoffEntries(tt.raw, tt.deliveryTaskID)
			if (corrupt != "") != tt.wantCorrupt {
				t.Fatalf("corrupt = %q (non-empty=%v), want non-empty=%v", corrupt, corrupt != "", tt.wantCorrupt)
			}
			if tt.wantCorrupt {
				return
			}
			if len(entries) != tt.wantEntries {
				t.Errorf("len(entries) = %d, want %d", len(entries), tt.wantEntries)
			}
			if present != tt.wantPresent {
				t.Errorf("alreadyPresent = %v, want %v", present, tt.wantPresent)
			}
		})
	}
}

// TestParseHandoffEntries_UnknownFieldsSurviveByteForByte is AC-27: an entry
// carrying fields this code does not know about is well-formed and must be
// preserved as its original raw bytes, not reconstructed from the decoded
// task_id/handed_off_at pair alone.
func TestParseHandoffEntries_UnknownFieldsSurviveByteForByte(t *testing.T) {
	const raw = `[{"task_id":"t1","handed_off_at":"2026-01-01T00:00:00.000Z","note":"kept as-is","big_number":9007199254740993}]`

	entries, present, corrupt := parseHandoffEntries(raw, "t1")
	if corrupt != "" {
		t.Fatalf("corrupt = %q, want empty", corrupt)
	}
	if !present {
		t.Fatal("alreadyPresent = false, want true")
	}
	if len(entries) != 1 {
		t.Fatalf("len(entries) = %d, want 1", len(entries))
	}
	if got := string(entries[0].raw); got != `{"task_id":"t1","handed_off_at":"2026-01-01T00:00:00.000Z","note":"kept as-is","big_number":9007199254740993}` {
		t.Errorf("raw bytes were not preserved unchanged: %s", got)
	}
}

// TestValidateHandoffWorkflow_RefusesTargetWorkspaceOfficeWorkflow is AC-
// CROSS-WORKSPACE-TASK-HANDOFF-AUTHORIZATION-001.9: workflow_id must be
// refused with a *HandoffValidationError when it names the target
// workspace's own office workflow, not a delivery workflow.
func TestValidateHandoffWorkflow_RefusesTargetWorkspaceOfficeWorkflow(t *testing.T) {
	actions := NewActions(ActionDependencies{Handoff: HandoffDependencies{
		Tasks: &fakeHandoffTasks{
			workflow: &taskmodels.Workflow{ID: "wf-office", WorkspaceID: "ws-target"},
		},
	}})
	workspace := &taskmodels.Workspace{ID: "ws-target", OfficeWorkflowID: "wf-office"}

	err := actions.validateHandoffWorkflow(context.Background(), "wf-office", workspace)

	var validation *HandoffValidationError
	if !errors.As(err, &validation) {
		t.Fatalf("error = %v (%T), want *HandoffValidationError", err, err)
	}
}

// TestValidateHandoffWorkflow_UnconfiguredOfficeWorkflowDoesNotRefuse proves
// the AC's carve-out: a target workspace with no configured office workflow
// (OfficeWorkflowID == "") has nothing to compare against, so an otherwise
// valid delivery workflow must not be refused on this ground.
func TestValidateHandoffWorkflow_UnconfiguredOfficeWorkflowDoesNotRefuse(t *testing.T) {
	actions := NewActions(ActionDependencies{Handoff: HandoffDependencies{
		Tasks: &fakeHandoffTasks{
			workflow: &taskmodels.Workflow{ID: "wf-delivery", WorkspaceID: "ws-target"},
		},
	}})
	workspace := &taskmodels.Workspace{ID: "ws-target", OfficeWorkflowID: ""}

	if err := actions.validateHandoffWorkflow(context.Background(), "wf-delivery", workspace); err != nil {
		t.Fatalf("validateHandoffWorkflow() error = %v, want nil", err)
	}
}

// TestValidateHandoffWorkflow_DeliveryWorkflowDistinctFromOfficeWorkflowIsAllowed
// proves a delivery workflow that is merely a *different* workflow from the
// configured office workflow passes, so the refusal is scoped to an exact
// match rather than misfiring whenever an office workflow is configured at
// all.
func TestValidateHandoffWorkflow_DeliveryWorkflowDistinctFromOfficeWorkflowIsAllowed(t *testing.T) {
	actions := NewActions(ActionDependencies{Handoff: HandoffDependencies{
		Tasks: &fakeHandoffTasks{
			workflow: &taskmodels.Workflow{ID: "wf-delivery", WorkspaceID: "ws-target"},
		},
	}})
	workspace := &taskmodels.Workspace{ID: "ws-target", OfficeWorkflowID: "wf-office"}

	if err := actions.validateHandoffWorkflow(context.Background(), "wf-delivery", workspace); err != nil {
		t.Fatalf("validateHandoffWorkflow() error = %v, want nil", err)
	}
}
