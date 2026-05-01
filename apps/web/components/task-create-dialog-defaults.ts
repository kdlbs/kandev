import type { DialogComputedArgs, StepType } from "@/components/task-create-dialog-types";
import { computeEffectiveStepId } from "@/components/task-create-dialog-helpers";

export function computeSingleWorkflowFallbackId(
  selectedWorkflowId: string | null,
  workflowId: string | null,
  workflows: DialogComputedArgs["workflows"],
): string | null {
  if (selectedWorkflowId || workflowId || workflows.length !== 1) return null;
  return workflows[0]?.id ?? null;
}

export function computeSnapshotDefaultStepId(
  workflowId: string | null,
  snapshots: DialogComputedArgs["snapshots"],
): string | null {
  if (!workflowId) return null;
  const steps = snapshots[workflowId]?.steps ?? [];
  const startStep = steps.find((step) => step.is_start_step);
  if (startStep) return startStep.id;
  return [...steps].sort((a, b) => a.position - b.position)[0]?.id ?? null;
}

type ComputeDialogDefaultStepIdArgs = {
  selectedWorkflowId: string | null;
  workflowId: string | null;
  fetchedSteps: StepType[] | null;
  defaultStepId: string | null;
  effectiveWorkflowId: string | null;
  snapshots: DialogComputedArgs["snapshots"];
};

export function computeDialogDefaultStepId({
  selectedWorkflowId,
  workflowId,
  fetchedSteps,
  defaultStepId,
  effectiveWorkflowId,
  snapshots,
}: ComputeDialogDefaultStepIdArgs): string | null {
  const switchedWorkflowWithoutFetchedSteps =
    Boolean(selectedWorkflowId) && selectedWorkflowId !== workflowId && !fetchedSteps;
  const computedStepId = switchedWorkflowWithoutFetchedSteps
    ? null
    : computeEffectiveStepId(selectedWorkflowId, workflowId, fetchedSteps, defaultStepId);

  return computedStepId ?? computeSnapshotDefaultStepId(effectiveWorkflowId, snapshots);
}
