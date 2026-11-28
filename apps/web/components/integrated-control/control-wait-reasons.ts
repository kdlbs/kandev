import type { NativeDashboard } from "@/lib/types/control-native";

/** A recorded reason live work is not executing. */
export const WAIT_REASONS = ["input", "blocked", "approval", "hold"] as const;
export type WaitReason = (typeof WAIT_REASONS)[number];

/**
 * Recorded counts per reason. A null count means the read cannot tell that
 * reason apart; it is never reported as a measured zero.
 */
export type WaitReasonCounts = Record<WaitReason, number | null>;

type TaskStage = NonNullable<NativeDashboard["task_stages"]>[number];

/** Task states that are finished or already failed, so they are not waiting. */
const NON_WAITING_STATES = new Set(["COMPLETED", "CANCELLED", "FAILED"]);

function isWaiting(state: string): boolean {
  return !NON_WAITING_STATES.has(state.toUpperCase());
}

function stageType(group: TaskStage): string {
  return group.stage_type.toLowerCase();
}

function isCount(value: number): boolean {
  return Number.isSafeInteger(value) && value >= 0;
}

function sumStages(groups: TaskStage[], match: (group: TaskStage) => boolean): number | null {
  let total = 0;
  for (const group of groups) {
    if (!match(group)) continue;
    if (!isCount(group.count)) return null;
    total += group.count;
  }
  return total;
}

function histogramBlocked(tasks: NativeDashboard["tasks"]): number | null {
  const value = tasks.BLOCKED ?? 0;
  return isCount(value) ? value : null;
}

/**
 * Splits recorded waiting work by reason. Input wait counts unfinished tasks
 * whose deterministic primary session waits for input. Blocked is the recorded
 * BLOCKED task state; approval and hold are workflow-stage natures that exclude
 * BLOCKED tasks, so one task is not counted twice across those reasons. Without
 * recorded stage groups, approval and hold are unknown, never zero. An invalid
 * recorded count makes that reason unknown instead of a partial sum.
 *
 * Input wait and the stage reasons are independent observations: one task can
 * appear under both, so these counts are not a partition of waiting work.
 */
export function waitReasonCounts(data?: NativeDashboard): WaitReasonCounts | undefined {
  if (!data) return undefined;
  const stages = data.task_stages;
  return {
    input: isCount(data.waiting_input_tasks) ? data.waiting_input_tasks : null,
    blocked: stages
      ? sumStages(stages, (group) => group.state.toUpperCase() === "BLOCKED")
      : histogramBlocked(data.tasks),
    approval: stages
      ? sumStages(
          stages,
          (group) =>
            isWaiting(group.state) &&
            group.state.toUpperCase() !== "BLOCKED" &&
            stageType(group) === "approval",
        )
      : null,
    hold: stages
      ? sumStages(
          stages,
          (group) =>
            isWaiting(group.state) &&
            group.state.toUpperCase() !== "BLOCKED" &&
            stageType(group) === "hold",
        )
      : null,
  };
}
