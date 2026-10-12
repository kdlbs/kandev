import { toKanbanTask } from "@/lib/kanban/map-task";
import type { AppState } from "@/lib/state/store";
import { reconcileTaskOverviewRead } from "@/lib/state/slices/task-overview-merge";
import type { WorkflowSnapshot } from "@/lib/types/http";

// Map workflow snapshot to kanban state on workspace switch.
export function mapSnapshotToKanban(snapshot: WorkflowSnapshot, newWorkflowId: string) {
  const stepIds = new Set(snapshot.steps.map((step) => step.id));
  const tasks = snapshot.tasks
    .filter((task) => !task.is_ephemeral && stepIds.has(task.workflow_step_id))
    .map(toKanbanTask);
  return {
    workflowId: newWorkflowId,
    isLoading: false,
    steps: snapshot.steps.map((step) => ({
      id: step.id,
      title: step.name,
      color: step.color,
      position: step.position,
      events: step.events,
      // Preserve optional step capabilities until the next full reload.
      allow_manual_move: step.allow_manual_move,
      auto_advance_requires_signal: step.auto_advance_requires_signal,
      prompt: step.prompt,
      is_start_step: step.is_start_step,
      show_in_command_panel: step.show_in_command_panel,
      agent_profile_id: step.agent_profile_id,
    })),
    tasks,
    taskCoverage: snapshot.task_coverage && {
      ...snapshot.task_coverage,
      complete: snapshot.task_coverage.complete && tasks.length === snapshot.task_coverage.total,
    },
  };
}

export function sortByUpdatedAtDesc<T extends { updated_at?: string | null }>(items: T[]): T[] {
  return [...items].sort((a, b) => {
    const aDate = a.updated_at ? new Date(a.updated_at).getTime() : 0;
    const bDate = b.updated_at ? new Date(b.updated_at).getTime() : 0;
    return bDate - aDate;
  });
}

export function reconcileMobileSnapshot(
  state: AppState,
  snapshot: ReturnType<typeof mapSnapshotToKanban>,
  readId: string | undefined,
) {
  const tasks = state.taskOverview
    ? reconcileTaskOverviewRead(state.taskOverview, snapshot.tasks, readId, true)
    : snapshot.tasks;
  if (!tasks) return null;
  const members = tasks.filter(
    (task) => task.workflowId === snapshot.workflowId && !task.isArchived,
  );
  return {
    ...snapshot,
    tasks: members,
    taskCoverage: snapshot.taskCoverage && { ...snapshot.taskCoverage, total: members.length },
  };
}
