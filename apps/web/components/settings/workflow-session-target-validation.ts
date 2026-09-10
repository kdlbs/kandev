import type { WorkflowStep } from "@/lib/types/http";

export type WorkflowSessionTargetIssueKind =
  | "missing-source"
  | "not-earlier"
  | "missing-profile"
  | "indirect-source";

export type WorkflowSessionTargetIssue = {
  kind: WorkflowSessionTargetIssueKind;
  sourceStepId?: string;
};

export function getWorkflowSessionTargetIssue(
  step: WorkflowStep,
  steps: readonly WorkflowStep[],
): WorkflowSessionTargetIssue | undefined {
  const target = step.session_target;
  if (!target || target.kind !== "step") return undefined;

  const source = steps.find((candidate) => candidate.id === target.step_id);
  if (!source) return { kind: "missing-source", sourceStepId: target.step_id };
  if (source.position >= step.position) {
    return { kind: "not-earlier", sourceStepId: source.id };
  }
  if (!source.agent_profile_id) {
    return { kind: "missing-profile", sourceStepId: source.id };
  }
  if (source.session_target) {
    return { kind: "indirect-source", sourceStepId: source.id };
  }
  return undefined;
}

export function hasInvalidWorkflowSessionTargets(steps: readonly WorkflowStep[]): boolean {
  return steps.some((step) => getWorkflowSessionTargetIssue(step, steps) !== undefined);
}
