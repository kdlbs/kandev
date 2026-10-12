import type { AppState } from "@/lib/state/store";
import { mergeTaskUpdate } from "./task-merge";
import type { TaskEventPayload } from "./task-archive-cache";
import { recordTaskOverviewChange } from "@/lib/state/slices/task-overview-merge";
import type { TaskOverviewPatch } from "@/lib/state/slices/task-overview-types";
import { taskOverviewPatch } from "@/lib/state/slices/task-overview-patch";

export function applyTaskOverviewEvent(state: AppState, payload: TaskEventPayload): AppState {
  if (!state.taskOverview || payload.is_ephemeral) return state;
  const id = payload.task_id ?? payload.id;
  if (!id || (payload.workspace_id && payload.workspace_id !== state.workspaces.activeId))
    return state;
  const previous = state.taskOverview.byId[id];
  const patch = taskOverviewPatch(payload);
  if (previous) {
    const task = mergeTaskUpdate(previous, { ...previous, ...patch }, payload);
    for (const key of Object.keys(patch) as Array<keyof TaskOverviewPatch>) {
      if (key === "statusSummary") continue;
      Object.assign(patch, { [key]: task[key] });
    }
  }
  return {
    ...invalidateUnknownWorkflow(state, payload),
    taskOverview: recordTaskOverviewChange(state.taskOverview, id, patch),
  };
}

export function applyTaskOverviewPatch(
  state: AppState,
  id: string,
  patch: TaskOverviewPatch | null,
  workspaceId?: string,
): AppState {
  if (!state.taskOverview) return state;
  if (workspaceId && workspaceId !== state.workspaces.activeId) return state;
  return { ...state, taskOverview: recordTaskOverviewChange(state.taskOverview, id, patch) };
}

function invalidateUnknownWorkflow(state: AppState, payload: TaskEventPayload): AppState {
  const coverage = state.workflows.taskWorkflowCoverage;
  if (
    !coverage?.complete ||
    !Object.hasOwn(payload, "workflow_id") ||
    coverage.workflow_ids.includes(payload.workflow_id ?? "")
  )
    return state;
  const workflowId = payload.workflow_id ?? "";
  const snapshot = state.kanbanMulti.snapshots[workflowId];
  const covered =
    snapshot?.taskCoverage?.complete &&
    snapshot.taskCoverage.workspace_id === coverage.workspace_id;
  return {
    ...state,
    workflows: {
      ...state.workflows,
      taskWorkflowCoverage: {
        ...coverage,
        complete: Boolean(covered),
        workflow_ids: covered ? [...coverage.workflow_ids, workflowId] : coverage.workflow_ids,
      },
    },
  };
}
