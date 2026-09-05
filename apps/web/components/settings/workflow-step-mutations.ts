import type { Workflow, WorkflowStep } from "@/lib/types/http";
import type { useToast } from "@/components/toast-provider";
import { t } from "@/lib/i18n";
import { generateUUID } from "@/lib/utils";
import { createWorkflowStepAction, updateWorkflowStepAction } from "@/app/actions/workspaces";
import type {
  WorkflowActionRecord,
  WorkflowLifecycleTrigger,
} from "@/lib/workflows/workflow-action-catalog";

// See `workflow-card-actions.ts`: both fields are persisted verbatim, so the
// seeded step name deliberately stays English.
// i18n-exempt: persisted workflow step name, same contract as DEFAULT_CUSTOM_STEPS.
const NEW_STEP_DEFAULTS = { name: "New Step", color: "bg-slate-500" } as const;

function fallbackErrorMessage(error: unknown): string {
  return error instanceof Error ? error.message : t("common:requestFailed");
}

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

export function removeLocalStep(stepId: string, setWorkflowSteps: WorkflowStepsSetter) {
  setWorkflowSteps((previous) =>
    previous.filter((step) => step.id !== stepId).map((step, position) => ({ ...step, position })),
  );
}

export async function addRemoteStep(
  workflow: Workflow,
  stepCount: number,
  setWorkflowSteps: WorkflowStepsSetter,
  toast: Toast,
) {
  try {
    const created = await createWorkflowStepAction({
      workflow_id: workflow.id,
      ...NEW_STEP_DEFAULTS,
      position: stepCount,
    });
    setWorkflowSteps((previous) => [...previous, created]);
  } catch (error) {
    toast({
      title: t("workflows:failedToAddWorkflowStep"),
      description: fallbackErrorMessage(error),
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

export function addWorkflowAction(
  step: WorkflowStep,
  trigger: WorkflowLifecycleTrigger,
  action: WorkflowActionRecord,
): WorkflowStep {
  const actions = workflowActions(step, trigger);
  return withWorkflowActions(step, trigger, [...actions, cloneAction(action)]);
}

export function updateWorkflowAction(
  step: WorkflowStep,
  trigger: WorkflowLifecycleTrigger,
  actionIndex: number,
  updates: Partial<WorkflowActionRecord>,
): WorkflowStep {
  const actions = workflowActions(step, trigger);
  if (!actions[actionIndex]) return step;
  const current = actions[actionIndex];
  const next = {
    ...current,
    ...updates,
    ...(updates.config ? { config: { ...(current.config ?? {}), ...updates.config } } : {}),
  };
  return withWorkflowActions(
    step,
    trigger,
    actions.map((action, index) => (index === actionIndex ? next : action)),
  );
}

export function removeWorkflowAction(
  step: WorkflowStep,
  trigger: WorkflowLifecycleTrigger,
  actionIndex: number,
): WorkflowStep {
  return withWorkflowActions(
    step,
    trigger,
    workflowActions(step, trigger).filter((_action, index) => index !== actionIndex),
  );
}

export function moveWorkflowAction(
  step: WorkflowStep,
  trigger: WorkflowLifecycleTrigger,
  fromIndex: number,
  toIndex: number,
): WorkflowStep {
  const actions = workflowActions(step, trigger);
  if (
    fromIndex < 0 ||
    fromIndex >= actions.length ||
    toIndex < 0 ||
    toIndex >= actions.length ||
    fromIndex === toIndex
  ) {
    return step;
  }
  const next = [...actions];
  const [moved] = next.splice(fromIndex, 1);
  next.splice(toIndex, 0, moved);
  return withWorkflowActions(step, trigger, next);
}

export function repairWorkflowActionSelection(
  selectedIndex: number | null,
  actionCount: number,
): number | null {
  if (actionCount <= 0 || selectedIndex === null) return null;
  return Math.min(selectedIndex, actionCount - 1);
}

function workflowActions(
  step: WorkflowStep,
  trigger: WorkflowLifecycleTrigger,
): WorkflowActionRecord[] {
  return (step.events?.[trigger] ?? []) as unknown as WorkflowActionRecord[];
}

function withWorkflowActions(
  step: WorkflowStep,
  trigger: WorkflowLifecycleTrigger,
  actions: WorkflowActionRecord[],
): WorkflowStep {
  return {
    ...step,
    events: {
      ...(step.events ?? {}),
      [trigger]: actions,
    },
  };
}

function cloneAction(action: WorkflowActionRecord): WorkflowActionRecord {
  return {
    ...action,
    ...(action.config ? { config: { ...action.config } } : {}),
  };
}

export async function updateRemoteWorkflowStep({
  stepId,
  updates,
  setWorkflowSteps,
  toast,
}: {
  stepId: string;
  updates: Partial<WorkflowStep>;
  setWorkflowSteps: WorkflowStepsSetter;
  toast: Toast;
}) {
  try {
    const updated = await updateWorkflowStepAction(stepId, updates);
    setWorkflowSteps((previous) => applyWorkflowStepUpdates(previous, stepId, updated));
  } catch (error) {
    toast({
      title: t("workflows:failedToUpdateWorkflowStep"),
      description: fallbackErrorMessage(error),
      variant: "error",
    });
  }
}
