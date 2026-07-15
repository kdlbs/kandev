import type { Workflow, WorkflowStep } from "@/lib/types/http";
import type { useToast } from "@/components/toast-provider";
import { generateUUID } from "@/lib/utils";
import { createWorkflowStepAction, updateWorkflowStepAction } from "@/app/actions/workspaces";

const FALLBACK_ERROR_MESSAGE = "Request failed";
const NEW_STEP_DEFAULTS = { name: "New Step", color: "bg-slate-500" } as const;

type WorkflowStepsSetter = (
  updater: ((previous: WorkflowStep[]) => WorkflowStep[]) | WorkflowStep[],
) => void;
type Toast = ReturnType<typeof useToast>["toast"];

export function newWorkflowStep(workflow: Workflow, position: number, id: string): WorkflowStep {
  return {
    id,
    workflow_id: workflow.id,
    ...NEW_STEP_DEFAULTS,
    position,
    allow_manual_move: true,
    created_at: "",
    updated_at: "",
  };
}

export function addLocalStep(workflow: Workflow, setWorkflowSteps: WorkflowStepsSetter) {
  setWorkflowSteps((previous) => [
    ...previous,
    newWorkflowStep(workflow, previous.length, `temp-step-${generateUUID()}`),
  ]);
}

export async function addRemoteStep(
  workflow: Workflow,
  stepCount: number,
  refreshWorkflowSteps: () => Promise<void>,
  toast: Toast,
) {
  try {
    await createWorkflowStepAction({
      workflow_id: workflow.id,
      ...NEW_STEP_DEFAULTS,
      position: stepCount,
    });
    await refreshWorkflowSteps();
  } catch (error) {
    toast({
      title: "Failed to add workflow step",
      description: error instanceof Error ? error.message : FALLBACK_ERROR_MESSAGE,
      variant: "error",
    });
  }
}

export function applyWorkflowStepUpdates(
  steps: WorkflowStep[],
  stepId: string,
  updates: Partial<WorkflowStep>,
): WorkflowStep[] {
  const isSettingStartStep = updates.is_start_step === true;
  return steps.map((step) => {
    if (step.id === stepId) return { ...step, ...updates };
    if (isSettingStartStep) return { ...step, is_start_step: false };
    return step;
  });
}

export async function updateRemoteWorkflowStep({
  stepId,
  updates,
  refreshWorkflowSteps,
  toast,
}: {
  stepId: string;
  updates: Partial<WorkflowStep>;
  refreshWorkflowSteps: () => Promise<void>;
  toast: Toast;
}) {
  try {
    await updateWorkflowStepAction(stepId, updates);
    await refreshWorkflowSteps();
  } catch (error) {
    toast({
      title: "Failed to update workflow step",
      description: error instanceof Error ? error.message : FALLBACK_ERROR_MESSAGE,
      variant: "error",
    });
  }
}
