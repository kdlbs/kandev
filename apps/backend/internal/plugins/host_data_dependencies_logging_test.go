package plugins

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/plugins/manifest"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	taskservice "github.com/kandev/kandev/internal/task/service"
	"github.com/kandev/kandev/pkg/pluginsdk"
)

// newObservedLogger wires a *logger.Logger backed by a zaptest/observer
// core, so a test can assert on the fields of a specific log record.
func newObservedLogger(t *testing.T) (*logger.Logger, *observer.ObservedLogs) {
	t.Helper()
	core, logs := observer.New(zapcore.DebugLevel)
	log, err := logger.NewFromZap(zap.New(core))
	require.NoError(t, err)
	return log, logs
}

// TestAttachDependencies_LogsWhenTaskDataSourceIsNil proves the
// AC-PLUGINS-TASK-DEPS-002.4 diagnostic fires for the "derivation source
// absent" failure class, carrying plugin, instance, resource type,
// operation, and result code — no task title, id, or edge content.
func TestAttachDependencies_LogsWhenTaskDataSourceIsNil(t *testing.T) {
	log, logs := newObservedLogger(t)
	host := &pluginHost{
		pluginID:     "plugin-1",
		capabilities: manifest.Capabilities{APIRead: []string{"tasks"}},
		log:          log,
	}
	tasks := []pluginsdk.Task{{ID: "task-1"}}

	err := host.attachDependencies(context.Background(), tasks, []*taskmodels.Task{{ID: "task-1"}}, false, "GetTask")
	require.NoError(t, err)

	entries := logs.All()
	require.Len(t, entries, 1, "exactly one record for this derivation attempt")
	fields := entries[0].ContextMap()
	require.Equal(t, "plugin-1", fields["plugin_id"])
	require.Equal(t, "plugin-1", fields["instance_id"], "a gRPC host has no separate instance id, so it falls back to plugin id")
	require.Equal(t, "task", fields["resource_type"])
	require.Equal(t, "GetTask", fields["operation"])
	require.Equal(t, "source_unavailable", fields["result_code"])
}

// TestAttachDependencies_UsesCanvasInstanceIDWhenSet proves a canvas-scoped
// host logs its real instance id rather than falling back to plugin id.
func TestAttachDependencies_UsesCanvasInstanceIDWhenSet(t *testing.T) {
	log, logs := newObservedLogger(t)
	host := &pluginHost{
		pluginID:     "plugin-1",
		instanceID:   "instance-9",
		capabilities: manifest.Capabilities{APIRead: []string{"tasks"}},
		log:          log,
	}
	tasks := []pluginsdk.Task{{ID: "task-1"}}

	err := host.attachDependencies(context.Background(), tasks, []*taskmodels.Task{{ID: "task-1"}}, false, "GetWebAppTask")
	require.NoError(t, err)

	require.Len(t, logs.All(), 1)
	require.Equal(t, "instance-9", logs.All()[0].ContextMap()["instance_id"])
}

// TestAttachDependencies_LogsOnceWhenDerivationWithheldAcrossManyTasks
// proves the log unit is the derivation attempt, not the withheld task: a
// batch-wide failure (every task's BlockedReason coming back "unknown")
// still produces exactly one record.
func TestAttachDependencies_LogsOnceWhenDerivationWithheldAcrossManyTasks(t *testing.T) {
	log, logs := newObservedLogger(t)
	d := newTestDataHost(manifest.Capabilities{APIRead: []string{"tasks"}})
	d.host.log = log
	d.tasks.dependencyViews = map[string]taskservice.DependencyView{
		"task-1": {Blocked: true, BlockedReason: taskservice.BlockedReasonUnknown},
		"task-2": {Blocked: true, BlockedReason: taskservice.BlockedReasonUnknown},
		"task-3": {Blocked: true, BlockedReason: taskservice.BlockedReasonUnknown},
	}
	tasks := []pluginsdk.Task{{ID: "task-1"}, {ID: "task-2"}, {ID: "task-3"}}
	models := []*taskmodels.Task{{ID: "task-1"}, {ID: "task-2"}, {ID: "task-3"}}

	err := d.host.attachDependencies(context.Background(), tasks, models, true, "ListTasks")
	require.NoError(t, err)

	entries := logs.All()
	require.Len(t, entries, 1, "one record per derivation attempt, not one per withheld task")
	require.Equal(t, "derivation_withheld", entries[0].ContextMap()["result_code"])
}

// TestAttachDependencies_NoLogOnSuccessfulDerivation proves an ordinary
// non-withheld read logs nothing.
func TestAttachDependencies_NoLogOnSuccessfulDerivation(t *testing.T) {
	log, logs := newObservedLogger(t)
	d := newTestDataHost(manifest.Capabilities{APIRead: []string{"tasks"}})
	d.host.log = log
	d.tasks.dependencyViews = map[string]taskservice.DependencyView{
		"task-1": {Blocked: true, BlockedReason: taskservice.BlockedReasonPending},
	}
	tasks := []pluginsdk.Task{{ID: "task-1"}}

	err := d.host.attachDependencies(context.Background(), tasks, []*taskmodels.Task{{ID: "task-1"}}, false, "GetTask")
	require.NoError(t, err)
	require.Empty(t, logs.All())
}

// TestAttachDependencies_NoLogWhenCapabilityDenied proves a caller lacking
// api_read:tasks is not logged as a failed derivation attempt: per the
// design, withholding for lack of capability never reaches the derivation
// at all, so there is nothing to report as having failed.
func TestAttachDependencies_NoLogWhenCapabilityDenied(t *testing.T) {
	log, logs := newObservedLogger(t)
	d := newTestDataHost(manifest.Capabilities{})
	d.host.log = log
	tasks := []pluginsdk.Task{{ID: "task-1"}}

	err := d.host.attachDependencies(context.Background(), tasks, []*taskmodels.Task{{ID: "task-1"}}, false, "GetTask")
	require.NoError(t, err)
	require.Empty(t, logs.All())
}

// TestAttachDependencies_NoLogOnFanOutRefusal proves the bounded fan-out
// refusal is not logged by this diagnostic: it is a distinct sentinel
// (ResourceExhausted before any task is serialized), never the withheld
// verdict AC-PLUGINS-TASK-DEPS-002.4 covers.
func TestAttachDependencies_NoLogOnFanOutRefusal(t *testing.T) {
	log, logs := newObservedLogger(t)
	d := newTestDataHost(manifest.Capabilities{APIRead: []string{"tasks"}})
	d.host.log = log
	d.tasks.dependencyViewsErr = taskservice.ErrDependencyFanOutExceeded
	tasks := []pluginsdk.Task{{ID: "task-1"}}

	err := d.host.attachDependencies(context.Background(), tasks, []*taskmodels.Task{{ID: "task-1"}}, true, "ListTasks")
	require.Error(t, err)
	require.Empty(t, logs.All())
}

// TestAttachDependencies_NilLoggerNeverPanics proves a bare test/production
// host with no logger wired (log is nil) degrades to no-op logging instead
// of panicking, on both the source-absent and derivation-withheld paths.
func TestAttachDependencies_NilLoggerNeverPanics(t *testing.T) {
	host := &pluginHost{capabilities: manifest.Capabilities{APIRead: []string{"tasks"}}}
	tasks := []pluginsdk.Task{{ID: "task-1"}}

	require.NotPanics(t, func() {
		_ = host.attachDependencies(context.Background(), tasks, []*taskmodels.Task{{ID: "task-1"}}, false, "GetTask")
	})
}
