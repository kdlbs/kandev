package engine_adapters

import (
	"context"
	"errors"
	"strings"
	"testing"

	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/workflow/engine"
)

// fakeParentRepo returns a static parent task or error.
type fakeParentRepo struct {
	task *taskmodels.Task
	err  error
}

func (f *fakeParentRepo) GetTask(_ context.Context, _ string) (*taskmodels.Task, error) {
	return f.task, f.err
}

// fakeChildCreator records calls.
type fakeChildCreator struct {
	calls []struct {
		Parent *taskmodels.Task
		Spec   ChildTaskCreateSpec
	}
	id  string
	err error
}

func (f *fakeChildCreator) CreateChildTask(
	_ context.Context, parent *taskmodels.Task, spec ChildTaskCreateSpec,
) (string, error) {
	f.calls = append(f.calls, struct {
		Parent *taskmodels.Task
		Spec   ChildTaskCreateSpec
	}{Parent: parent, Spec: spec})
	if f.err != nil {
		return "", f.err
	}
	if f.id == "" {
		return "child-id", nil
	}
	return f.id, nil
}

func TestTaskCreatorAdapter_HappyPath(t *testing.T) {
	parent := &taskmodels.Task{
		ID:                     "parent-1",
		WorkspaceID:            "ws-1",
		WorkflowID:             "wf-default",
		AssigneeAgentProfileID: "agent-default",
	}
	creator := &fakeChildCreator{id: "child-42"}
	a := NewTaskCreatorAdapter(&fakeParentRepo{task: parent}, creator)
	id, err := a.CreateChildTask(context.Background(), "parent-1", engine.ChildTaskSpec{
		Title:          "Implement /healthz",
		Description:    "Add a /healthz endpoint",
		WorkflowID:     "wf-kanban",
		StepID:         "step-1",
		AgentProfileID: "profile-claude",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != "child-42" {
		t.Errorf("id = %q, want child-42", id)
	}
	if len(creator.calls) != 1 {
		t.Fatalf("expected 1 CreateChildTask call, got %d", len(creator.calls))
	}
	got := creator.calls[0]
	if got.Parent != parent {
		t.Errorf("parent passed by reference mismatched: got %+v", got.Parent)
	}
	if got.Spec.Title != "Implement /healthz" {
		t.Errorf("title = %q", got.Spec.Title)
	}
	if got.Spec.WorkflowID != "wf-kanban" {
		t.Errorf("workflow_id = %q (should pass through caller's override)", got.Spec.WorkflowID)
	}
}

func TestTaskCreatorAdapter_RequiresParentID(t *testing.T) {
	a := NewTaskCreatorAdapter(&fakeParentRepo{}, &fakeChildCreator{})
	_, err := a.CreateChildTask(context.Background(), "", engine.ChildTaskSpec{Title: "X"})
	if err == nil || !strings.Contains(err.Error(), "parent_task_id") {
		t.Fatalf("expected parent_task_id error, got: %v", err)
	}
}

func TestTaskCreatorAdapter_RequiresTaskService(t *testing.T) {
	a := NewTaskCreatorAdapter(&fakeParentRepo{}, nil)
	_, err := a.CreateChildTask(context.Background(), "parent-1", engine.ChildTaskSpec{Title: "X"})
	if err == nil || !strings.Contains(err.Error(), "task service") {
		t.Fatalf("expected task service error, got: %v", err)
	}
}

func TestTaskCreatorAdapter_BubblesParentLookupError(t *testing.T) {
	repoErr := errors.New("boom")
	a := NewTaskCreatorAdapter(&fakeParentRepo{err: repoErr}, &fakeChildCreator{})
	_, err := a.CreateChildTask(context.Background(), "parent-1", engine.ChildTaskSpec{Title: "X"})
	if err == nil || !errors.Is(err, repoErr) {
		t.Fatalf("expected repo error to bubble, got: %v", err)
	}
}

func TestTaskCreatorAdapter_ReturnsErrorWhenParentNotFound(t *testing.T) {
	a := NewTaskCreatorAdapter(&fakeParentRepo{task: nil}, &fakeChildCreator{})
	_, err := a.CreateChildTask(context.Background(), "missing", engine.ChildTaskSpec{Title: "X"})
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected not-found error, got: %v", err)
	}
}

// fakeCarrierResolver returns a fixed carrier map for every task id, and
// records the id and causing agent it was asked to resolve.
type fakeCarrierResolver struct {
	metadata    map[string]interface{}
	lastID      string
	lastAgentID string
}

func (f *fakeCarrierResolver) TaskBoundaryCarrierMetadata(
	_ context.Context, taskID, causingAgentProfileID string,
) map[string]interface{} {
	f.lastID = taskID
	f.lastAgentID = causingAgentProfileID
	return f.metadata
}

// TestTaskCreatorAdapter_ThreadsCarrierFromParentTask is the Review
// round 1 finding 3 regression test: create_child_task previously never
// resolved a carrier at all, so a chain could silently reset its own
// depth ceiling by routing through this action instead of the MCP
// create-subtask runtime action (AC-OFFICE-RUN-CAUSATION-001.24). With a
// CarrierResolver wired, the parent task's own carrier must be resolved
// and forwarded onto the child's spec.
func TestTaskCreatorAdapter_ThreadsCarrierFromParentTask(t *testing.T) {
	parent := &taskmodels.Task{ID: "parent-1", WorkspaceID: "ws-1"}
	creator := &fakeChildCreator{}
	carrier := &fakeCarrierResolver{metadata: map[string]interface{}{
		"office_carrier_causation_id": "root-run-1",
	}}
	a := NewTaskCreatorAdapter(&fakeParentRepo{task: parent}, creator)
	a.SetCarrierResolver(carrier)

	if _, err := a.CreateChildTask(context.Background(), "parent-1", engine.ChildTaskSpec{Title: "X"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if carrier.lastID != "parent-1" {
		t.Errorf("carrier resolved for task %q, want %q", carrier.lastID, "parent-1")
	}
	if len(creator.calls) != 1 {
		t.Fatalf("expected 1 CreateChildTask call, got %d", len(creator.calls))
	}
	got := creator.calls[0].Spec.OfficeCarrierMetadata
	if got["office_carrier_causation_id"] != "root-run-1" {
		t.Errorf("OfficeCarrierMetadata = %+v, want the resolver's carrier forwarded", got)
	}
}

// TestTaskCreatorAdapter_ThreadsCausingAgentToCarrierResolver proves the
// adapter forwards the spec's CausingAgentProfileID to the carrier
// resolver, so the resolver can scope its claimed-run lookup to the agent
// actually executing this turn instead of an unscoped, task-only lookup
// that is ambiguous when more than one agent holds a claimed run on the
// same task.
func TestTaskCreatorAdapter_ThreadsCausingAgentToCarrierResolver(t *testing.T) {
	parent := &taskmodels.Task{ID: "parent-1", WorkspaceID: "ws-1"}
	creator := &fakeChildCreator{}
	carrier := &fakeCarrierResolver{metadata: map[string]interface{}{}}
	a := NewTaskCreatorAdapter(&fakeParentRepo{task: parent}, creator)
	a.SetCarrierResolver(carrier)

	if _, err := a.CreateChildTask(context.Background(), "parent-1", engine.ChildTaskSpec{
		Title:                 "X",
		CausingAgentProfileID: "turn-agent-1",
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if carrier.lastAgentID != "turn-agent-1" {
		t.Errorf("carrier resolved with causing agent %q, want %q", carrier.lastAgentID, "turn-agent-1")
	}
}

// TestTaskCreatorAdapter_NoCarrierResolverOmitsCarrier pins the
// pre-existing (safe) behaviour when no resolver is wired: no carrier is
// forwarded, rather than the adapter panicking on a nil Carrier.
func TestTaskCreatorAdapter_NoCarrierResolverOmitsCarrier(t *testing.T) {
	parent := &taskmodels.Task{ID: "parent-1"}
	creator := &fakeChildCreator{}
	a := NewTaskCreatorAdapter(&fakeParentRepo{task: parent}, creator)

	if _, err := a.CreateChildTask(context.Background(), "parent-1", engine.ChildTaskSpec{Title: "X"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := creator.calls[0].Spec.OfficeCarrierMetadata; got != nil {
		t.Errorf("OfficeCarrierMetadata = %+v, want nil (no resolver wired)", got)
	}
}
