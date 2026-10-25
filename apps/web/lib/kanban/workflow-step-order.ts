type Step = { id: string };

export function sortWorkflowStepsByPosition<T extends Step & { position: number }>(
  steps: T[],
): T[] {
  return [...steps].sort((a, b) => a.position - b.position || a.id.localeCompare(b.id));
}

export type StepOrderRevisionSource = { id: string; order_revision?: number };

/**
 * Seeds a stepId->revision map from freshly-fetched steps, keeping the
 * larger revision on conflict. A hydration response can resolve after a
 * fresher WS-driven `task.reordered` already advanced the recorded
 * revision (REQ-TASKS-KANBAN-TASK-REORDERING-001.25/.37); taking the max
 * keeps that later value from being rolled back to a stale one.
 */
export function mergeStepOrderRevisions(
  current: Record<string, number>,
  steps: readonly StepOrderRevisionSource[] | undefined,
): Record<string, number> {
  if (!steps || steps.length === 0) return current;
  let changed = false;
  const next = { ...current };
  for (const step of steps) {
    const revision = step.order_revision;
    if (revision === undefined) continue;
    const existing = next[step.id];
    if (existing === undefined || revision > existing) {
      next[step.id] = revision;
      changed = true;
    }
  }
  return changed ? next : current;
}
