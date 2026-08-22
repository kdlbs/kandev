type Step = { id: string };
type StepTask = { workflowStepId: string | null };

export function sortWorkflowStepsByPosition<T extends Step & { position: number }>(
  steps: T[],
): T[] {
  return [...steps].sort((a, b) => a.position - b.position || a.id.localeCompare(b.id));
}

export function areAllEmptyStepsAutoHidden(visibleSteps: Step[], moveTargetSteps: Step[]): boolean {
  return visibleSteps.length === 0 && moveTargetSteps.length > 0;
}

export function deriveAutoHiddenStepIds(
  steps: Step[],
  tasks: StepTask[],
  enabled: boolean,
  manuallyHiddenStepIds: string[],
): Set<string> {
  if (!enabled) return new Set();
  const occupied = new Set(tasks.map((task) => task.workflowStepId).filter(Boolean));
  const manuallyHidden = new Set(manuallyHiddenStepIds);
  return new Set(
    steps
      .filter((step) => !occupied.has(step.id) && !manuallyHidden.has(step.id))
      .map((step) => step.id),
  );
}
