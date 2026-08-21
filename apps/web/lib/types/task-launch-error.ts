export const TASK_LAUNCH_RECOVERY_ACTIONS = [
  "retry_default",
  "pick_base_branch",
  "mark_review_done",
] as const;

export type TaskLaunchRecoveryAction = (typeof TASK_LAUNCH_RECOVERY_ACTIONS)[number];

/** Keep recovery controls bounded and in the order declared by the backend. */
export function normalizeTaskLaunchRecoveryActions(value: unknown): TaskLaunchRecoveryAction[] {
  if (!Array.isArray(value)) return [];
  const seen = new Set<TaskLaunchRecoveryAction>();
  const actions: TaskLaunchRecoveryAction[] = [];
  for (const candidate of value) {
    if (
      typeof candidate !== "string" ||
      !TASK_LAUNCH_RECOVERY_ACTIONS.includes(candidate as TaskLaunchRecoveryAction)
    ) {
      continue;
    }
    const action = candidate as TaskLaunchRecoveryAction;
    if (seen.has(action)) continue;
    seen.add(action);
    actions.push(action);
    if (actions.length === 3) break;
  }
  return actions;
}
