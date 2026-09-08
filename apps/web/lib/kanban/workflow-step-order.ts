type Step = { id: string };

export function sortWorkflowStepsByPosition<T extends Step & { position: number }>(
  steps: T[],
): T[] {
  return [...steps].sort((a, b) => a.position - b.position || a.id.localeCompare(b.id));
}
