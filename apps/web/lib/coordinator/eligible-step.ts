// TypeScript copy of the eligible-step walk
// (apps/backend/internal/coordinator/eligibility.go's EligibleStep), for the
// Edit form's step filter (docs/specs/coordinator/system-design/
// proposal-cards.md#cards "Edit form options"). This client filter is only a
// convenience: the approve route re-validates and is authoritative.

export type EligibleStepNode = {
  id: string;
  isStart: boolean;
  allowManualMove: boolean;
  autoStartOnEnter: boolean;
  pullFromStepId: string | null;
};

function feedsInto(
  byId: Map<string, EligibleStepNode>,
  fromStepId: string | null,
  target: string,
): boolean {
  const visited = new Set<string>();
  let current = fromStepId;
  while (current) {
    if (current === target) return true;
    if (visited.has(current)) return false;
    visited.add(current);
    const step = byId.get(current);
    if (!step) return false;
    current = step.pullFromStepId;
  }
  return false;
}

/**
 * Whether stepId is an eligible placement for a coordinator-approved task:
 * the workflow's start step or a step that allows manual moves, the step
 * itself does not auto-start an agent on enter, and it is not a feeder —
 * directly or through a chain of pullFromStepId links — of any step that
 * does. steps is the workflow's full step graph, not just the candidate
 * step. An unknown stepId is ineligible.
 */
export function eligibleStep(steps: EligibleStepNode[], stepId: string): boolean {
  const byId = new Map(steps.map((step) => [step.id, step]));
  const candidate = byId.get(stepId);
  if (!candidate) return false;
  if (candidate.autoStartOnEnter) return false;
  if (!candidate.isStart && !candidate.allowManualMove) return false;
  for (const step of steps) {
    if (step.autoStartOnEnter && feedsInto(byId, step.pullFromStepId, stepId)) return false;
  }
  return true;
}
