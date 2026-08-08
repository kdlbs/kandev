import type { DialogComputedArgs, StepType } from "@/components/task-create-dialog-types";
import { computeEffectiveStepId } from "@/components/task-create-dialog-helpers";

type WorkflowCandidate = {
  id: string;
  workspaceId?: string;
  hidden?: boolean;
};

export type ResolveEffectiveTaskCreateWorkflowIdArgs = {
  workspaceId: string | null;
  lockedWorkflowId: string | null;
  manualWorkflowId: string | null;
  lastUsedWorkflowId: string | null;
  contextWorkflowId: string | null;
  workflows: WorkflowCandidate[];
};

function isVisibleWorkflow(workflow: WorkflowCandidate | undefined, workspaceId: string | null) {
  return Boolean(
    workflow &&
    !workflow.hidden &&
    (!workspaceId || !workflow.workspaceId || workflow.workspaceId === workspaceId),
  );
}

export function resolveEffectiveTaskCreateWorkflowId({
  workspaceId,
  lockedWorkflowId,
  manualWorkflowId,
  lastUsedWorkflowId,
  contextWorkflowId,
  workflows,
}: ResolveEffectiveTaskCreateWorkflowIdArgs): string | null {
  if (lockedWorkflowId) return lockedWorkflowId;

  const visibleWorkflows = workflows.filter((workflow) => isVisibleWorkflow(workflow, workspaceId));
  const visibleWorkflowIDs = new Set(visibleWorkflows.map((workflow) => workflow.id));
  if (manualWorkflowId && visibleWorkflowIDs.has(manualWorkflowId)) return manualWorkflowId;
  if (lastUsedWorkflowId && visibleWorkflowIDs.has(lastUsedWorkflowId)) {
    return lastUsedWorkflowId;
  }
  if (contextWorkflowId && visibleWorkflowIDs.has(contextWorkflowId)) return contextWorkflowId;
  return visibleWorkflows.length === 1 ? (visibleWorkflows[0]?.id ?? null) : null;
}

export function resolveTaskCreateWorkflowContext(
  workflowId: string | null,
  defaultStepId: string | null,
  workflows: Array<{ id: string; hidden?: boolean }>,
  allowHiddenWorkflow: boolean,
): { workflowId: string | null; defaultStepId: string | null } {
  const workflow = workflows.find((item) => item.id === workflowId);
  if (allowHiddenWorkflow || !workflowId || (workflow && !workflow.hidden)) {
    return { workflowId, defaultStepId };
  }
  return { workflowId: null, defaultStepId: null };
}

export function computeSingleWorkflowFallbackId(
  selectedWorkflowId: string | null,
  workflowId: string | null,
  workflows: Array<{ id: string; hidden?: boolean }>,
): string | null {
  const visibleWorkflows = workflows.filter((workflow) => !workflow.hidden);
  if (selectedWorkflowId || workflowId || visibleWorkflows.length !== 1) return null;
  return visibleWorkflows[0]?.id ?? null;
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
  const matchingFetchedSteps = fetchedSteps?.filter(
    (step) => step.workflowId === effectiveWorkflowId,
  );
  if (effectiveWorkflowId && effectiveWorkflowId !== workflowId) {
    const fetchedStartStep = matchingFetchedSteps?.find((step) => step.is_start_step);
    const fetchedFirstStep = matchingFetchedSteps
      ? [...matchingFetchedSteps].sort((a, b) => (a.position ?? 0) - (b.position ?? 0))[0]
      : null;
    return (
      fetchedStartStep?.id ??
      fetchedFirstStep?.id ??
      computeSnapshotDefaultStepId(effectiveWorkflowId, snapshots)
    );
  }

  const switchedWorkflowWithoutFetchedSteps =
    Boolean(selectedWorkflowId) && selectedWorkflowId !== workflowId && !matchingFetchedSteps;
  const computedStepId = switchedWorkflowWithoutFetchedSteps
    ? null
    : computeEffectiveStepId(
        selectedWorkflowId,
        workflowId,
        matchingFetchedSteps ?? null,
        defaultStepId,
      );

  return computedStepId ?? computeSnapshotDefaultStepId(effectiveWorkflowId, snapshots);
}
