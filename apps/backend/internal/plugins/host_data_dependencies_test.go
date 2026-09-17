package plugins

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/plugins/manifest"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	taskservice "github.com/kandev/kandev/internal/task/service"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/kandev/kandev/pkg/pluginsdk"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestAttachDependencies_CopiesViewOntoMatchingTask(t *testing.T) {
	d := newTestDataHost(manifest.Capabilities{APIRead: []string{"tasks"}})
	d.tasks.dependencyViews = map[string]taskservice.DependencyView{
		"task-1": {
			Blocked: true, BlockedReason: taskservice.BlockedReasonPending,
			DependsOn:          []taskservice.DependencyRef{{ID: "task-0", Title: "Predecessor", State: v1.TaskStateInProgress, Status: taskservice.DependencyPending}},
			Blocks:             []taskservice.DependencyRef{{ID: "task-9", Title: "Dependent", State: v1.TaskStateTODO, Status: taskservice.DependencyPending}},
			DependsOnTruncated: true,
		},
	}
	model := &taskmodels.Task{ID: "task-1"}
	tasks := []pluginsdk.Task{{ID: "task-1"}}

	err := d.host.attachDependencies(context.Background(), tasks, []*taskmodels.Task{model}, false)
	require.NoError(t, err)

	require.True(t, tasks[0].Blocked)
	require.Equal(t, taskservice.BlockedReasonPending, tasks[0].BlockedReason)
	require.Equal(t, []pluginsdk.TaskDependencyRef{{ID: "task-0", Title: "Predecessor", State: string(v1.TaskStateInProgress), Status: taskservice.DependencyPending}}, tasks[0].DependsOn)
	require.True(t, tasks[0].DependsOnTruncated)
	require.False(t, tasks[0].BlocksTruncated)
}

func TestAttachDependencies_BlocksEntriesNeverCarryStatus(t *testing.T) {
	d := newTestDataHost(manifest.Capabilities{APIRead: []string{"tasks"}})
	d.tasks.dependencyViews = map[string]taskservice.DependencyView{
		"task-1": {
			Blocks: []taskservice.DependencyRef{{ID: "task-9", Title: "Dependent", State: v1.TaskStateTODO, Status: taskservice.DependencyPending}},
		},
	}
	tasks := []pluginsdk.Task{{ID: "task-1"}}

	err := d.host.attachDependencies(context.Background(), tasks, []*taskmodels.Task{{ID: "task-1"}}, false)
	require.NoError(t, err)

	require.Len(t, tasks[0].Blocks, 1)
	require.Empty(t, tasks[0].Blocks[0].Status, "a blocks entry's status describes the dependent's own progress, not readiness, so the wire contract drops it")
}

func TestAttachDependencies_EmptyEdgeListsAreNeverNil(t *testing.T) {
	d := newTestDataHost(manifest.Capabilities{APIRead: []string{"tasks"}})
	tasks := []pluginsdk.Task{{ID: "task-1"}}

	err := d.host.attachDependencies(context.Background(), tasks, []*taskmodels.Task{{ID: "task-1"}}, false)
	require.NoError(t, err)

	require.NotNil(t, tasks[0].DependsOn)
	require.Empty(t, tasks[0].DependsOn)
	require.NotNil(t, tasks[0].Blocks)
	require.Empty(t, tasks[0].Blocks)
}

func TestAttachDependencies_StartWhenUnblockedReflectsStoredIntentWhenNotWithheld(t *testing.T) {
	d := newTestDataHost(manifest.Capabilities{APIRead: []string{"tasks"}})
	d.tasks.dependencyViews = map[string]taskservice.DependencyView{
		"task-1": {Blocked: true, BlockedReason: taskservice.BlockedReasonPending},
	}
	model := &taskmodels.Task{
		ID: "task-1",
		Metadata: map[string]interface{}{
			taskmodels.MetaKeyDeferredLaunch: map[string]interface{}{
				taskmodels.DeferredLaunchStartWhenUnblockedKey: true,
			},
		},
	}
	tasks := []pluginsdk.Task{{ID: "task-1"}}

	err := d.host.attachDependencies(context.Background(), tasks, []*taskmodels.Task{model}, false)
	require.NoError(t, err)
	require.True(t, tasks[0].StartWhenUnblocked)
}

func TestAttachDependencies_StartWhenUnblockedForcedFalseUnderWithheldVerdict(t *testing.T) {
	d := newTestDataHost(manifest.Capabilities{APIRead: []string{"tasks"}})
	d.tasks.dependencyViews = map[string]taskservice.DependencyView{
		"task-1": {Blocked: true, BlockedReason: taskservice.BlockedReasonUnknown},
	}
	model := &taskmodels.Task{
		ID: "task-1",
		Metadata: map[string]interface{}{
			taskmodels.MetaKeyDeferredLaunch: map[string]interface{}{
				taskmodels.DeferredLaunchStartWhenUnblockedKey: true,
			},
		},
	}
	tasks := []pluginsdk.Task{{ID: "task-1"}}

	err := d.host.attachDependencies(context.Background(), tasks, []*taskmodels.Task{model}, false)
	require.NoError(t, err)
	require.False(t, tasks[0].StartWhenUnblocked, "the withheld verdict forces start_when_unblocked false regardless of the stored intent")
}

func TestAttachDependencies_BoundedFanOutRefusalBecomesResourceExhausted(t *testing.T) {
	d := newTestDataHost(manifest.Capabilities{APIRead: []string{"tasks"}})
	d.tasks.dependencyViewsErr = taskservice.ErrDependencyFanOutExceeded
	tasks := []pluginsdk.Task{{ID: "task-1"}}

	err := d.host.attachDependencies(context.Background(), tasks, []*taskmodels.Task{{ID: "task-1"}}, true)
	require.Error(t, err)
	require.Equal(t, codes.ResourceExhausted, status.Code(err))
}

func TestAttachDependencies_UnboundedVariantNeverConsultsBoundedSource(t *testing.T) {
	d := newTestDataHost(manifest.Capabilities{APIRead: []string{"tasks"}})
	tasks := []pluginsdk.Task{{ID: "task-1"}}

	err := d.host.attachDependencies(context.Background(), tasks, []*taskmodels.Task{{ID: "task-1"}}, false)
	require.NoError(t, err)
	require.Equal(t, 1, d.tasks.dependencyViewsCalls)
	require.Equal(t, 0, d.tasks.dependencyViewsBoundedCalls)
}

func TestAttachDependencies_NoopOnEmptyTasks(t *testing.T) {
	d := newTestDataHost(manifest.Capabilities{APIRead: []string{"tasks"}})
	err := d.host.attachDependencies(context.Background(), nil, nil, true)
	require.NoError(t, err)
	require.Equal(t, 0, d.tasks.dependencyViewsBoundedCalls)
}

func TestPluginHost_Tasks_GetAttachesDependencyProjectionUnbounded(t *testing.T) {
	d := newTestDataHost(manifest.Capabilities{APIRead: []string{"tasks"}})
	d.tasks.tasksByID = map[string]*taskmodels.Task{"task-1": {ID: "task-1", Title: "Task 1"}}
	d.tasks.dependencyViews = map[string]taskservice.DependencyView{
		"task-1": {Blocked: true, BlockedReason: taskservice.BlockedReasonUnknown},
	}

	got, err := d.host.Tasks().Get(context.Background(), "task-1")
	require.NoError(t, err)
	require.True(t, got.Blocked)
	require.Equal(t, taskservice.BlockedReasonUnknown, got.BlockedReason)
	require.Equal(t, 1, d.tasks.dependencyViewsCalls)
	require.Equal(t, 0, d.tasks.dependencyViewsBoundedCalls, "Get is one of the single-task flows exempt from the fan-out cap")
}

func TestPluginHost_Tasks_ListAttachesDependencyProjectionBoundedOverThePageOnly(t *testing.T) {
	d := newTestDataHost(manifest.Capabilities{APIRead: []string{"tasks"}})
	d.tasks.workspaces = []*taskmodels.Workspace{{ID: "ws-1"}}
	d.tasks.tasksByWorkspace = map[string][]*taskmodels.Task{
		"ws-1": {
			{ID: "task-1", WorkspaceID: "ws-1", CreatedAt: time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC)},
			{ID: "task-2", WorkspaceID: "ws-1", CreatedAt: time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)},
			{ID: "task-3", WorkspaceID: "ws-1", CreatedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)},
		},
	}
	d.tasks.dependencyViews = map[string]taskservice.DependencyView{
		"task-1": {Blocked: true, BlockedReason: taskservice.BlockedReasonPending},
	}

	tasks, _, err := d.host.Tasks().List(context.Background(), pluginsdk.TaskFilter{}, pluginsdk.Page{Limit: 1})
	require.NoError(t, err)
	require.Len(t, tasks, 1)
	require.True(t, tasks[0].Blocked)
	require.Equal(t, []string{"task-1"}, d.tasks.dependencyViewsTasks, "derivation runs over the returned page only, not the whole workspace")
	require.Equal(t, 1, d.tasks.dependencyViewsBoundedCalls, "ListTasks is bound by the fan-out cap")
}

func TestPluginHost_Tasks_ListTranslatesFanOutRefusalToResourceExhausted(t *testing.T) {
	d := newTestDataHost(manifest.Capabilities{APIRead: []string{"tasks"}})
	d.tasks.workspaces = []*taskmodels.Workspace{{ID: "ws-1"}}
	d.tasks.tasksByWorkspace = map[string][]*taskmodels.Task{"ws-1": {{ID: "task-1", WorkspaceID: "ws-1"}}}
	d.tasks.dependencyViewsErr = taskservice.ErrDependencyFanOutExceeded

	_, _, err := d.host.Tasks().List(context.Background(), pluginsdk.TaskFilter{}, pluginsdk.Page{})
	require.Error(t, err)
	require.Equal(t, codes.ResourceExhausted, status.Code(err))
}

func TestAttachDependencies_NoopWhenTaskDataSourceNil(t *testing.T) {
	host := &pluginHost{capabilities: manifest.Capabilities{APIRead: []string{"tasks"}}}
	tasks := []pluginsdk.Task{{ID: "task-1"}}
	err := host.attachDependencies(context.Background(), tasks, []*taskmodels.Task{{ID: "task-1"}}, true)
	require.NoError(t, err)
	require.False(t, tasks[0].Blocked)
}

func TestAttachDependencies_WithholdsWhenCallerLacksReadCapability(t *testing.T) {
	d := newTestDataHost(manifest.Capabilities{})
	d.tasks.dependencyViews = map[string]taskservice.DependencyView{
		"task-1": {Blocked: false, BlockedReason: ""},
	}
	model := &taskmodels.Task{
		ID: "task-1",
		Metadata: map[string]interface{}{
			taskmodels.MetaKeyDeferredLaunch: map[string]interface{}{
				taskmodels.DeferredLaunchStartWhenUnblockedKey: true,
			},
		},
	}
	tasks := []pluginsdk.Task{{ID: "task-1"}}

	err := d.host.attachDependencies(context.Background(), tasks, []*taskmodels.Task{model}, false)
	require.NoError(t, err)

	require.True(t, tasks[0].Blocked, "a caller without api_read:tasks gets the fail-closed withheld verdict, not real data")
	require.Equal(t, taskservice.BlockedReasonUnknown, tasks[0].BlockedReason)
	require.Equal(t, []pluginsdk.TaskDependencyRef{}, tasks[0].DependsOn)
	require.Equal(t, []pluginsdk.TaskDependencyRef{}, tasks[0].Blocks)
	require.False(t, tasks[0].DependsOnTruncated)
	require.False(t, tasks[0].BlocksTruncated)
	require.False(t, tasks[0].StartWhenUnblocked)
	require.Equal(t, 0, d.tasks.dependencyViewsCalls, "withholding for lack of capability issues no derivation call")
	require.Equal(t, 0, d.tasks.dependencyViewsBoundedCalls)
}

func TestAttachDependencies_WriteOnlyCapabilityWithholdsEvenWithTaskDataSource(t *testing.T) {
	d := newTestDataHost(manifest.Capabilities{APIWrite: []string{"tasks"}})
	d.tasks.dependencyViews = map[string]taskservice.DependencyView{
		"task-1": {Blocked: true, BlockedReason: taskservice.BlockedReasonPending},
	}
	tasks := []pluginsdk.Task{{ID: "task-1"}}

	err := d.host.attachDependencies(context.Background(), tasks, []*taskmodels.Task{{ID: "task-1"}}, false)
	require.NoError(t, err)

	require.Equal(t, taskservice.BlockedReasonUnknown, tasks[0].BlockedReason, "api_write:tasks alone never unlocks real dependency data")
}
