// host_data_dependencies.go attaches the dependency projection (blocked,
// blocked_reason, depends_on, blocks, depends_on_truncated,
// blocks_truncated, start_when_unblocked) onto Task DTOs, for both the gRPC
// and canvas surfaces. It reuses internal/task/service's already fail-closed
// derivation (BuildDependencyViews/BuildDependencyViewsBounded) rather than
// reimplementing any of its withheld-verdict or fan-out-bound logic.
package plugins

import (
	"context"

	taskmodels "github.com/kandev/kandev/internal/task/models"
	taskservice "github.com/kandev/kandev/internal/task/service"
	"github.com/kandev/kandev/pkg/pluginsdk"
)

// attachDependencies fills the seven dependency-projection fields on tasks in
// place. models[i] must be the raw model backing tasks[i] (same order, same
// length) — StartWhenUnblocked reads the stored auto-start intent directly
// off models[i].
//
// A caller without api_read:tasks never reaches real dependency data through
// this path, even when it holds some other capability (e.g. api_write:tasks)
// that let it reach a Task DTO in the first place: every task is stamped with
// the withheld verdict instead of deriving anything.
//
// Unlike attachPullRequests, a derivation failure is not left at zero value:
// BuildDependencyViews/Bounded already substitute the withheld verdict
// (blocked: true, blocked_reason: "unknown") for every requested id on any
// read failure, so this only copies whatever the derivation returned. The
// one failure this function itself can return is the bounded fan-out
// refusal, translated to a single gRPC ResourceExhausted error so a caller
// never partially serializes a bounded batch around it.
func (h *pluginHost) attachDependencies(ctx context.Context, tasks []pluginsdk.Task, models []*taskmodels.Task, bounded bool) error {
	if len(tasks) == 0 {
		return nil
	}
	if !h.capabilities.CanRead(resourceTasks) {
		withholdDependencies(tasks)
		return nil
	}
	if h.taskData == nil {
		withholdDependencies(tasks)
		return nil
	}
	var views map[string]taskservice.DependencyView
	if bounded {
		v, err := h.taskData.BuildDependencyViewsBounded(ctx, models)
		if err != nil {
			return resourceExhausted(err.Error())
		}
		views = v
	} else {
		views = h.taskData.BuildDependencyViews(ctx, models)
	}
	for i := range tasks {
		view := views[tasks[i].ID]
		tasks[i].Blocked = view.Blocked
		tasks[i].BlockedReason = view.BlockedReason
		tasks[i].DependsOn = dependencyRefsToDTOs(view.DependsOn, true)
		tasks[i].Blocks = dependencyRefsToDTOs(view.Blocks, false)
		tasks[i].DependsOnTruncated = view.DependsOnTruncated
		tasks[i].BlocksTruncated = view.BlocksTruncated
		withheld := view.BlockedReason == taskservice.BlockedReasonUnknown
		tasks[i].StartWhenUnblocked = !withheld && taskmodels.HasStartWhenUnblockedIntent(models[i])
	}
	return nil
}

// withholdDependencies stamps every task with the fail-closed withheld
// verdict in place, for a caller that lacks api_read:tasks. It is the same
// verdict a derivation failure produces, so a write-only caller cannot
// distinguish "withheld for lack of capability" from "withheld because
// derivation failed."
func withholdDependencies(tasks []pluginsdk.Task) {
	for i := range tasks {
		tasks[i].Blocked = true
		tasks[i].BlockedReason = taskservice.BlockedReasonUnknown
		tasks[i].DependsOn = []pluginsdk.TaskDependencyRef{}
		tasks[i].Blocks = []pluginsdk.TaskDependencyRef{}
		tasks[i].DependsOnTruncated = false
		tasks[i].BlocksTruncated = false
		tasks[i].StartWhenUnblocked = false
	}
}

// dependencyRefsToDTOs converts a derived edge list to its DTO form,
// preserving order. Status carries DependencyStatusForTask's verdict for a
// predecessor (includeStatus true); a dependent's own status describes its
// own progress, not readiness to unblock this task, so it is dropped
// (includeStatus false). Never nil: depends_on/blocks always serialize as
// [], never as an absent or null field.
func dependencyRefsToDTOs(refs []taskservice.DependencyRef, includeStatus bool) []pluginsdk.TaskDependencyRef {
	out := make([]pluginsdk.TaskDependencyRef, len(refs))
	for i, ref := range refs {
		status := ref.Status
		if !includeStatus {
			status = ""
		}
		out[i] = pluginsdk.TaskDependencyRef{
			ID: ref.ID, Title: ref.Title, State: string(ref.State), Status: status,
			WorkspaceID: ref.WorkspaceID,
		}
	}
	return out
}
