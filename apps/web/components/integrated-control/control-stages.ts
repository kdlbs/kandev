import type { NativeDashboard } from "@/lib/types/control-native";

/** Workflow nature is display-only: it grants no admission or execution. */
export const TASK_STAGES = ["work", "review", "held", "completed", "unknown"] as const;
export type TaskStage = (typeof TASK_STAGES)[number];
export type TaskStageCounts = Record<TaskStage, number>;

export function taskStage(task: { state: string; stage_type: string }): TaskStage | null {
  const state = task.state?.toUpperCase();
  if (state === "COMPLETED") return "completed";
  if (state === "CANCELLED" || state === "FAILED") return null;
  if (state === "BLOCKED" || task.stage_type === "hold" || task.stage_type === "approval")
    return "held";
  if (task.stage_type === "review") return "review";
  if (task.stage_type === "work") return "work";
  return "unknown";
}

export function emptyStageCounts(): TaskStageCounts {
  return { work: 0, review: 0, held: 0, completed: 0, unknown: 0 };
}

/** A state histogram cannot establish a workflow stage or readiness. */
export function stageCountsFromStates(states: NativeDashboard["tasks"]): TaskStageCounts {
  return stageCountsFromGroups(
    Object.entries(states).map(([state, count]) => ({ state, stage_type: "", count })),
  );
}

function stageCountsFromGroups(
  groups: ReadonlyArray<{ state: string; stage_type: string; count: number }>,
): TaskStageCounts {
  const counts = emptyStageCounts();
  for (const group of groups) {
    if (!Number.isSafeInteger(group.count) || group.count < 0) continue;
    const stage = taskStage(group);
    if (stage) counts[stage] += group.count;
  }
  return counts;
}

export function dashboardStageCounts(data: NativeDashboard): TaskStageCounts {
  return data.task_stages
    ? stageCountsFromGroups(data.task_stages)
    : stageCountsFromStates(data.tasks);
}

/** Preserve each configured step's name even when its standard nature is custom. */
export function workflowStepCounts(data: NativeDashboard | undefined, stage: TaskStage) {
  const steps = new Map<string, { id: string; name: string; count: number }>();
  for (const group of data?.task_stages ?? []) {
    if (taskStage(group) !== stage) continue;
    const previous = steps.get(group.step_id);
    steps.set(group.step_id, {
      id: group.step_id,
      name: group.step_name,
      count: (previous?.count ?? 0) + group.count,
    });
  }
  return [...steps.values()];
}
