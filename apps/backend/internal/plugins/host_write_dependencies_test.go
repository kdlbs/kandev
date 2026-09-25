package plugins

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/plugins/manifest"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	taskservice "github.com/kandev/kandev/internal/task/service"
	"github.com/kandev/kandev/pkg/pluginsdk"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestPluginHost_Tasks_CreateAttachesDependencyProjection(t *testing.T) {
	d := newTestDataHost(manifest.Capabilities{APIWrite: []string{"tasks"}, APIRead: []string{"tasks"}})
	d.taskWriter.created = &taskmodels.Task{ID: "task-9", WorkspaceID: "ws-1", WorkflowID: "wf-1", Title: "Investigate"}
	d.tasks.dependencyViews = map[string]taskservice.DependencyView{
		"task-9": {Blocked: true, BlockedReason: taskservice.BlockedReasonPending},
	}

	task, err := d.host.Tasks().Create(context.Background(), pluginsdk.CreateTaskInput{
		WorkspaceID: "ws-1", WorkflowID: "wf-1", Title: "Investigate",
	})
	require.NoError(t, err)
	require.True(t, task.Blocked)
	require.Equal(t, taskservice.BlockedReasonPending, task.BlockedReason)
	require.Equal(t, 1, d.tasks.dependencyViewsCalls, "Create derives via the unbounded variant")
	require.Equal(t, 0, d.tasks.dependencyViewsBoundedCalls)
}

func TestPluginHost_Tasks_UpdateAttachesDependencyProjection(t *testing.T) {
	d := newTestDataHost(manifest.Capabilities{APIWrite: []string{"tasks"}, APIRead: []string{"tasks"}})
	d.taskWriter.updated = &taskmodels.Task{ID: "task-1", Title: "updated"}
	d.tasks.dependencyViews = map[string]taskservice.DependencyView{
		"task-1": {Blocked: false, BlockedReason: ""},
	}

	got, err := d.host.Tasks().Update(context.Background(), pluginsdk.UpdateTaskInput{ID: "task-1"})
	require.NoError(t, err)
	require.False(t, got.Blocked)
	require.Equal(t, 1, d.tasks.dependencyViewsCalls)
}

func TestPluginHost_Tasks_MoveAttachesDependencyProjectionOnTheResultingTask(t *testing.T) {
	d := newTestDataHost(manifest.Capabilities{APIWrite: []string{"tasks"}, APIRead: []string{"tasks"}})
	moved := &taskmodels.Task{ID: "task-1", WorkflowStepID: "step-2"}
	d.taskWriter.moveResult = &TaskMoveResult{Task: moved, Transitioned: true}
	d.tasks.dependencyViews = map[string]taskservice.DependencyView{
		"task-1": {Blocked: true, BlockedReason: taskservice.BlockedReasonFailed},
	}

	outcome, err := d.host.Tasks().Move(context.Background(), pluginsdk.MoveTaskInput{TaskID: "task-1", WorkflowStepID: "step-2"})
	require.NoError(t, err)
	require.True(t, outcome.Task.Blocked)
	require.Equal(t, taskservice.BlockedReasonFailed, outcome.Task.BlockedReason)
	require.Equal(t, 1, d.tasks.dependencyViewsCalls, "Move derives via the unbounded variant, once, on the object that was serialized")
}

func TestPluginHost_PluginOwnedTaskTreePreviewAttachesDependencyProjectionBounded(t *testing.T) {
	d := newTestDataHost(manifest.Capabilities{APIWrite: []string{"tasks"}, APIRead: []string{"tasks"}})
	root := &taskmodels.Task{ID: "root", WorkspaceID: "ws-1", Metadata: map[string]any{taskSourceMetadataKey: "plugin:p1"}}
	d.tasks.tasksByID = map[string]*taskmodels.Task{"root": root}
	d.tasks.tasksByWorkspace = map[string][]*taskmodels.Task{"ws-1": {root}}
	d.tasks.dependencyViews = map[string]taskservice.DependencyView{
		"root": {Blocked: true, BlockedReason: taskservice.BlockedReasonPending},
	}

	preview, err := d.host.PluginOwnedTaskTrees().Preview(context.Background(), "root")
	require.NoError(t, err)
	require.Len(t, preview, 1)
	require.True(t, preview[0].Blocked)
	require.Equal(t, 1, d.tasks.dependencyViewsBoundedCalls, "Preview is one of the flows F46 binds to the fan-out cap")
	require.Equal(t, 0, d.tasks.dependencyViewsCalls)
}

func TestPluginHost_PluginOwnedTaskTreePreviewTranslatesFanOutRefusal(t *testing.T) {
	d := newTestDataHost(manifest.Capabilities{APIWrite: []string{"tasks"}, APIRead: []string{"tasks"}})
	root := &taskmodels.Task{ID: "root", WorkspaceID: "ws-1", Metadata: map[string]any{taskSourceMetadataKey: "plugin:p1"}}
	d.tasks.tasksByID = map[string]*taskmodels.Task{"root": root}
	d.tasks.tasksByWorkspace = map[string][]*taskmodels.Task{"ws-1": {root}}
	d.tasks.dependencyViewsErr = taskservice.ErrDependencyFanOutExceeded

	_, err := d.host.PluginOwnedTaskTrees().Preview(context.Background(), "root")
	require.Error(t, err)
	require.Equal(t, codes.ResourceExhausted, status.Code(err))
}

// ── write-only capability (no api_read:tasks) never sees real dependency
// data through a write RPC, per AC-PLUGINS-TASK-DEPS-001.11 ─────────────

func TestPluginHost_Tasks_CreateWithholdsDependencyProjectionWithoutReadCapability(t *testing.T) {
	d := newTestDataHost(manifest.Capabilities{APIWrite: []string{"tasks"}})
	d.taskWriter.created = &taskmodels.Task{ID: "task-9", WorkspaceID: "ws-1", WorkflowID: "wf-1", Title: "Investigate"}
	d.tasks.dependencyViews = map[string]taskservice.DependencyView{
		"task-9": {Blocked: true, BlockedReason: taskservice.BlockedReasonPending},
	}

	task, err := d.host.Tasks().Create(context.Background(), pluginsdk.CreateTaskInput{
		WorkspaceID: "ws-1", WorkflowID: "wf-1", Title: "Investigate",
	})
	require.NoError(t, err)
	require.Equal(t, taskservice.BlockedReasonUnknown, task.BlockedReason, "a write-only caller must not read real dependency data through Create")
	require.Equal(t, 0, d.tasks.dependencyViewsCalls)
}

func TestPluginHost_Tasks_UpdateWithholdsDependencyProjectionWithoutReadCapability(t *testing.T) {
	d := newTestDataHost(manifest.Capabilities{APIWrite: []string{"tasks"}})
	d.taskWriter.updated = &taskmodels.Task{ID: "task-1", Title: "updated"}
	d.tasks.dependencyViews = map[string]taskservice.DependencyView{
		"task-1": {Blocked: true, BlockedReason: taskservice.BlockedReasonPending},
	}

	got, err := d.host.Tasks().Update(context.Background(), pluginsdk.UpdateTaskInput{ID: "task-1"})
	require.NoError(t, err)
	require.Equal(t, taskservice.BlockedReasonUnknown, got.BlockedReason, "a write-only caller must not read real dependency data through Update")
}

func TestPluginHost_Tasks_MoveWithholdsDependencyProjectionWithoutReadCapability(t *testing.T) {
	d := newTestDataHost(manifest.Capabilities{APIWrite: []string{"tasks"}})
	moved := &taskmodels.Task{ID: "task-1", WorkflowStepID: "step-2"}
	d.taskWriter.moveResult = &TaskMoveResult{Task: moved, Transitioned: true}
	d.tasks.dependencyViews = map[string]taskservice.DependencyView{
		"task-1": {Blocked: true, BlockedReason: taskservice.BlockedReasonFailed},
	}

	outcome, err := d.host.Tasks().Move(context.Background(), pluginsdk.MoveTaskInput{TaskID: "task-1", WorkflowStepID: "step-2"})
	require.NoError(t, err)
	require.Equal(t, taskservice.BlockedReasonUnknown, outcome.Task.BlockedReason, "a write-only caller must not read real dependency data through Move")
}

func TestPluginHost_PluginOwnedTaskTreePreviewWithholdsDependencyProjectionWithoutReadCapability(t *testing.T) {
	d := newTestDataHost(manifest.Capabilities{APIWrite: []string{"tasks"}})
	root := &taskmodels.Task{ID: "root", WorkspaceID: "ws-1", Metadata: map[string]any{taskSourceMetadataKey: "plugin:p1"}}
	d.tasks.tasksByID = map[string]*taskmodels.Task{"root": root}
	d.tasks.tasksByWorkspace = map[string][]*taskmodels.Task{"ws-1": {root}}
	d.tasks.dependencyViews = map[string]taskservice.DependencyView{
		"root": {Blocked: true, BlockedReason: taskservice.BlockedReasonPending},
	}

	preview, err := d.host.PluginOwnedTaskTrees().Preview(context.Background(), "root")
	require.NoError(t, err)
	require.Len(t, preview, 1)
	require.Equal(t, taskservice.BlockedReasonUnknown, preview[0].BlockedReason, "a write-only caller must not read real dependency data through Preview")
	require.Equal(t, 0, d.tasks.dependencyViewsBoundedCalls)
}
