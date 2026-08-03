type StepLike = { id: string };
type TaskLike = { workflowStepId?: string };

/** First step that currently has tasks, or 0 when the board is empty. */
export function getInitialColumnIndex(steps: StepLike[], tasks: TaskLike[]): number {
  if (steps.length === 0) return 0;
  const idx = steps.findIndex((step) => tasks.some((t) => t.workflowStepId === step.id));
  return idx !== -1 ? idx : 0;
}

/**
 * Resolve the mobile column index from a stored workflow-step id.
 * Falls back to the first occupied step when the id is missing or stale.
 */
export function resolveMobileColumnIndex(
  steps: StepLike[],
  tasks: TaskLike[],
  storedStepId: string | undefined,
): number {
  if (steps.length === 0) return 0;
  if (storedStepId) {
    const storedIndex = steps.findIndex((step) => step.id === storedStepId);
    if (storedIndex !== -1) return storedIndex;
  }
  return getInitialColumnIndex(steps, tasks);
}
