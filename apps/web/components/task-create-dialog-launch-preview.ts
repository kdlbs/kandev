import type { StepType } from "@/components/task-create-dialog-types";

const TASK_PROMPT_TOKEN = "{{task_prompt}}";

export type TaskCreateLaunchPreview = {
  stepId: string;
  stepName: string;
  stepPrompt: string;
};

type ResolveTaskCreateLaunchPreviewArgs = {
  effectiveWorkflowId: string | null;
  fetchedSteps: ReadonlyArray<StepType> | null;
  snapshotSteps?: ReadonlyArray<StepType>;
};

function byPosition(a: StepType, b: StepType): number {
  return (a.position ?? Number.MAX_SAFE_INTEGER) - (b.position ?? Number.MAX_SAFE_INTEGER);
}

function hasAutoStartAction(step: StepType): boolean {
  return step.events?.on_enter?.some((action) => action.type === "auto_start_agent") ?? false;
}

export function resolveLaunchPreviewStep(steps: ReadonlyArray<StepType>): StepType | null {
  const ordered = [...steps].sort(byPosition);
  return (
    ordered.find(hasAutoStartAction) ??
    ordered.find((step) => step.is_start_step) ??
    ordered[0] ??
    null
  );
}

export function resolveTaskCreateLaunchPreview({
  effectiveWorkflowId,
  fetchedSteps,
  snapshotSteps,
}: ResolveTaskCreateLaunchPreviewArgs): TaskCreateLaunchPreview | null {
  if (!effectiveWorkflowId) return null;

  const matchingFetchedSteps = fetchedSteps?.filter(
    (step) => step.workflowId === effectiveWorkflowId,
  );
  const steps =
    matchingFetchedSteps && matchingFetchedSteps.length > 0
      ? matchingFetchedSteps
      : (snapshotSteps ?? []);
  const step = resolveLaunchPreviewStep(steps);
  if (!step) return null;

  return {
    stepId: step.id,
    stepName: step.title,
    stepPrompt: step.prompt ?? "",
  };
}

export function composeLaunchPreviewPrompt(stepPrompt: string, taskPrompt: string): string {
  return stepPrompt.replace(TASK_PROMPT_TOKEN, taskPrompt);
}
