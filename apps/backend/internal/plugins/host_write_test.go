package plugins

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/plugins/manifest"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	"github.com/kandev/kandev/pkg/pluginsdk"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ── Write gating: denied without api_write:<resource> ───────────────────

func TestPluginHost_Tasks_CreateDeniedWithoutWriteCapability(t *testing.T) {
	// api_read:tasks is granted but api_write:tasks is NOT — proving the two
	// capabilities gate independently: a read-only plugin cannot write.
	d := newTestDataHost(manifest.Capabilities{APIRead: []string{"tasks"}})

	_, err := d.host.Tasks().Create(context.Background(), pluginsdk.CreateTaskInput{Title: "x", WorkspaceID: "ws-1", WorkflowID: "wf-1"})
	assertPermissionDenied(t, err, "api_write:tasks")

	_, err = d.host.Tasks().Update(context.Background(), pluginsdk.UpdateTaskInput{ID: "task-1"})
	assertPermissionDenied(t, err, "api_write:tasks")

	if d.taskWriter.createCalls != 0 || d.taskWriter.updateCalls != 0 {
		t.Fatalf("task writer called despite missing api_write:tasks (create=%d update=%d)", d.taskWriter.createCalls, d.taskWriter.updateCalls)
	}
}

func TestPluginHost_Messages_SendDeniedWithoutWriteCapability(t *testing.T) {
	// api_read:messages granted but api_write:messages is NOT — the read and
	// write capabilities on the messages resource gate independently.
	d := newTestDataHost(manifest.Capabilities{APIRead: []string{"messages"}})

	_, err := d.host.Messages().Send(context.Background(), "task-1", "", "hello")
	assertPermissionDenied(t, err, "api_write:messages")
	if d.messenger.calls != 0 {
		t.Fatalf("messenger called %d times despite missing api_write:messages", d.messenger.calls)
	}
}

// TestPluginHost_Tasks_WriteOnlyCannotRead is the mirror of the read-only case:
// a plugin with only api_write:tasks can Create but cannot List/Get — the two
// capabilities are enforced per-method on the same accessor.
func TestPluginHost_Tasks_WriteOnlyCannotRead(t *testing.T) {
	d := newTestDataHost(manifest.Capabilities{APIWrite: []string{"tasks"}})

	_, _, err := d.host.Tasks().List(context.Background(), pluginsdk.TaskFilter{}, pluginsdk.Page{})
	assertPermissionDenied(t, err, "api_read:tasks")

	_, err = d.host.Tasks().Create(context.Background(), pluginsdk.CreateTaskInput{Title: "x", WorkspaceID: "ws-1", WorkflowID: "wf-1"})
	if err != nil {
		t.Fatalf("Create() with api_write:tasks unexpected error: %v", err)
	}
}

// ── Task create ─────────────────────────────────────────────────────────

func TestPluginHost_Tasks_CreateSucceedsAndStampsSource(t *testing.T) {
	d := newTestDataHost(manifest.Capabilities{APIWrite: []string{"tasks"}})
	d.taskWriter.created = &taskmodels.Task{ID: "task-9", WorkspaceID: "ws-1", WorkflowID: "wf-1", Title: "Investigate"}

	task, err := d.host.Tasks().Create(context.Background(), pluginsdk.CreateTaskInput{
		WorkspaceID: "ws-1", WorkflowID: "wf-1", Title: "Investigate", Description: "details",
	})
	if err != nil {
		t.Fatalf("Create() unexpected error: %v", err)
	}
	if task == nil || task.ID != "task-9" {
		t.Fatalf("Create() = %+v, want task-9", task)
	}
	if d.taskWriter.createCalls != 1 {
		t.Fatalf("task writer create calls = %d, want 1", d.taskWriter.createCalls)
	}
	if d.taskWriter.lastCreate.Source != "plugin:p1" {
		t.Fatalf("create source = %q, want plugin:p1 (server-stamped provenance)", d.taskWriter.lastCreate.Source)
	}
	if d.starter.calls != 0 {
		t.Fatalf("start_agent not requested but starter called %d times", d.starter.calls)
	}
}

func TestPluginHost_Tasks_CreateStartsAgentWhenRequested(t *testing.T) {
	d := newTestDataHost(manifest.Capabilities{APIWrite: []string{"tasks"}})
	d.taskWriter.created = &taskmodels.Task{ID: "task-9", WorkspaceID: "ws-1", WorkflowID: "wf-1"}

	_, err := d.host.Tasks().Create(context.Background(), pluginsdk.CreateTaskInput{
		WorkspaceID: "ws-1", WorkflowID: "wf-1", Title: "Investigate", StartAgent: true,
	})
	if err != nil {
		t.Fatalf("Create() unexpected error: %v", err)
	}
	if d.starter.calls != 1 || d.starter.lastID != "task-9" {
		t.Fatalf("starter calls=%d lastID=%q, want 1 call for task-9", d.starter.calls, d.starter.lastID)
	}
}

// TestPluginHost_Tasks_CreateStartAgentFailureIsNonFatal proves start_agent is
// best-effort: a launch error does not fail task creation (the task is already
// created and its event has fired).
func TestPluginHost_Tasks_CreateStartAgentFailureIsNonFatal(t *testing.T) {
	d := newTestDataHost(manifest.Capabilities{APIWrite: []string{"tasks"}})
	d.taskWriter.created = &taskmodels.Task{ID: "task-9"}
	d.starter.err = errors.New("no executor available")

	task, err := d.host.Tasks().Create(context.Background(), pluginsdk.CreateTaskInput{
		WorkspaceID: "ws-1", WorkflowID: "wf-1", Title: "Investigate", StartAgent: true,
	})
	if err != nil {
		t.Fatalf("Create() should not fail on a best-effort start error: %v", err)
	}
	if task == nil || task.ID != "task-9" {
		t.Fatalf("Create() = %+v, want the created task returned despite start failure", task)
	}
}

func TestPluginHost_Tasks_CreateRequiresTitle(t *testing.T) {
	d := newTestDataHost(manifest.Capabilities{APIWrite: []string{"tasks"}})
	_, err := d.host.Tasks().Create(context.Background(), pluginsdk.CreateTaskInput{WorkspaceID: "ws-1", WorkflowID: "wf-1"})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("Create() without title error = %v, want InvalidArgument", err)
	}
	if d.taskWriter.createCalls != 0 {
		t.Fatalf("task writer called despite empty title")
	}
}

// TestPluginHost_Tasks_CreateResolvesDefaults proves an omitted workspace_id /
// workflow_id resolves to the single workspace and that workspace's first
// workflow, mirroring REST/MCP default placement.
func TestPluginHost_Tasks_CreateResolvesDefaults(t *testing.T) {
	d := newTestDataHost(manifest.Capabilities{APIWrite: []string{"tasks"}})
	d.tasks.workspaces = []*taskmodels.Workspace{{ID: "ws-only"}}
	d.workflows.workflows = map[string][]*taskmodels.Workflow{
		"ws-only": {{ID: "wf-first", WorkspaceID: "ws-only"}, {ID: "wf-second", WorkspaceID: "ws-only"}},
	}

	_, err := d.host.Tasks().Create(context.Background(), pluginsdk.CreateTaskInput{Title: "Investigate"})
	if err != nil {
		t.Fatalf("Create() unexpected error: %v", err)
	}
	if d.taskWriter.lastCreate.WorkspaceID != "ws-only" || d.taskWriter.lastCreate.WorkflowID != "wf-first" {
		t.Fatalf("resolved placement = ws=%q wf=%q, want ws-only/wf-first", d.taskWriter.lastCreate.WorkspaceID, d.taskWriter.lastCreate.WorkflowID)
	}
}

func TestPluginHost_Tasks_CreateAmbiguousWorkspaceIsInvalidArgument(t *testing.T) {
	d := newTestDataHost(manifest.Capabilities{APIWrite: []string{"tasks"}})
	d.tasks.workspaces = []*taskmodels.Workspace{{ID: "ws-1"}, {ID: "ws-2"}}

	_, err := d.host.Tasks().Create(context.Background(), pluginsdk.CreateTaskInput{Title: "Investigate"})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("Create() with ambiguous workspace error = %v, want InvalidArgument", err)
	}
	if d.taskWriter.createCalls != 0 {
		t.Fatalf("task writer called despite unresolvable workspace default")
	}
}

// ── Task update ─────────────────────────────────────────────────────────

func TestPluginHost_Tasks_UpdateSucceedsWithMask(t *testing.T) {
	d := newTestDataHost(manifest.Capabilities{APIWrite: []string{"tasks"}})
	d.taskWriter.updated = &taskmodels.Task{ID: "task-1", Title: "Renamed"}

	title := "Renamed"
	state := "IN_PROGRESS"
	_, err := d.host.Tasks().Update(context.Background(), pluginsdk.UpdateTaskInput{ID: "task-1", Title: &title, State: &state})
	if err != nil {
		t.Fatalf("Update() unexpected error: %v", err)
	}
	if d.taskWriter.lastUpdate.ID != "task-1" {
		t.Fatalf("update id = %q, want task-1", d.taskWriter.lastUpdate.ID)
	}
	if d.taskWriter.lastUpdate.Title == nil || *d.taskWriter.lastUpdate.Title != "Renamed" {
		t.Fatalf("update title = %v, want Renamed", d.taskWriter.lastUpdate.Title)
	}
	if d.taskWriter.lastUpdate.Description != nil {
		t.Fatalf("unset description must stay nil, got %v", d.taskWriter.lastUpdate.Description)
	}
}

func TestPluginHost_Tasks_UpdateRequiresID(t *testing.T) {
	d := newTestDataHost(manifest.Capabilities{APIWrite: []string{"tasks"}})
	_, err := d.host.Tasks().Update(context.Background(), pluginsdk.UpdateTaskInput{})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("Update() without id error = %v, want InvalidArgument", err)
	}
}

func TestPluginHost_Tasks_UpdateNotFoundMapsToGRPCNotFound(t *testing.T) {
	d := newTestDataHost(manifest.Capabilities{APIWrite: []string{"tasks"}})
	d.taskWriter.updateErr = repoerrors.ErrTaskNotFound

	_, err := d.host.Tasks().Update(context.Background(), pluginsdk.UpdateTaskInput{ID: "no-such-task"})
	if status.Code(err) != codes.NotFound {
		t.Fatalf("Update() of missing task error = %v, want NotFound", err)
	}
}

// ── Message send ────────────────────────────────────────────────────────

func TestPluginHost_Messages_SendSucceedsAndStampsSource(t *testing.T) {
	d := newTestDataHost(manifest.Capabilities{APIWrite: []string{"messages"}})
	d.messenger.result = PluginMessageResult{SessionID: "session-9", Status: "queued"}

	dispatch, err := d.host.Messages().Send(context.Background(), "task-1", "session-9", "please rerun the tests")
	if err != nil {
		t.Fatalf("Send() unexpected error: %v", err)
	}
	if dispatch == nil || dispatch.SessionID != "session-9" || dispatch.Status != "queued" {
		t.Fatalf("Send() = %+v, want session-9/queued", dispatch)
	}
	if d.messenger.calls != 1 {
		t.Fatalf("messenger calls = %d, want 1 (routes through the orchestrator delivery path)", d.messenger.calls)
	}
	if d.messenger.lastTaskID != "task-1" || d.messenger.lastSession != "session-9" || d.messenger.lastText != "please rerun the tests" {
		t.Fatalf("messenger recorded taskID=%q session=%q text=%q", d.messenger.lastTaskID, d.messenger.lastSession, d.messenger.lastText)
	}
	if d.messenger.lastSource != "plugin:p1" {
		t.Fatalf("message source = %q, want plugin:p1 (server-stamped provenance)", d.messenger.lastSource)
	}
}

// TestPluginHost_Messages_ReadAndWriteGateIndependently proves List
// (api_read:messages) and Send (api_write:messages) are enforced separately on
// the shared Messages() accessor: a write-only manifest can Send but not List.
func TestPluginHost_Messages_ReadAndWriteGateIndependently(t *testing.T) {
	d := newTestDataHost(manifest.Capabilities{APIWrite: []string{"messages"}})

	_, _, err := d.host.Messages().List(context.Background(), pluginsdk.MessageFilter{}, pluginsdk.Page{})
	assertPermissionDenied(t, err, "api_read:messages")

	if _, err := d.host.Messages().Send(context.Background(), "task-1", "", "hi"); err != nil {
		t.Fatalf("Send() with api_write:messages unexpected error: %v", err)
	}
}

func TestPluginHost_Messages_SendRequiresTaskAndText(t *testing.T) {
	d := newTestDataHost(manifest.Capabilities{APIWrite: []string{"messages"}})

	if _, err := d.host.Messages().Send(context.Background(), "", "", "body"); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("Send() without task_id error = %v, want InvalidArgument", err)
	}
	if _, err := d.host.Messages().Send(context.Background(), "task-1", "", ""); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("Send() without text error = %v, want InvalidArgument", err)
	}
	if d.messenger.calls != 0 {
		t.Fatalf("messenger called %d times despite invalid input", d.messenger.calls)
	}
}
