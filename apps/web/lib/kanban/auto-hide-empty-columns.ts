type Step = { id: string };
type StepTask = { workflowStepId: string | null };

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
