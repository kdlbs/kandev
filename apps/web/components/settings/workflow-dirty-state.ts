import type { Workflow, WorkflowStep } from "@/lib/types/http";
import { areStepDraftsEqual } from "./workflow-card-actions";

function valuesEqual(left: unknown, right: unknown): boolean {
  if (Object.is(left, right)) return true;
  return JSON.stringify(left) === JSON.stringify(right);
}

export function isWorkflowFieldDirty(
  draft: Workflow,
  saved: Workflow | undefined,
  field: "name" | "description" | "agent_profile_id",
): boolean {
  if (!saved) return true;
  return !valuesEqual(draft[field] ?? "", saved[field] ?? "");
}

export function isWorkflowStepDirty(draft: WorkflowStep, saved: WorkflowStep | undefined): boolean {
  return !saved || !areStepDraftsEqual(draft, saved);
}

export function isWorkflowStepValueDirty(
  draft: WorkflowStep,
  saved: WorkflowStep | undefined,
  select: (step: WorkflowStep) => unknown,
): boolean {
  return !saved || !valuesEqual(select(draft), select(saved));
}
