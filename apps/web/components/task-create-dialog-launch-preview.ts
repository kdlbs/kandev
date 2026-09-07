import type { StepType } from "@/components/task-create-dialog-types";

const TASK_PROMPT_TOKEN = "{{task_prompt}}";

export type TaskCreateLaunchPreview = {
  stepId: string;
  stepName: string;
  stepPrompt: string;
};

export type TaskCreateLaunchIntent = "start-agent" | "plan-mode";

type ResolveTaskCreateLaunchPreviewArgs = {
  effectiveWorkflowId: string | null;
  fetchedSteps: ReadonlyArray<StepType> | null;
  snapshotSteps?: ReadonlyArray<StepType>;
  launchIntent?: TaskCreateLaunchIntent;
};

function byPosition(a: StepType, b: StepType): number {
  return (a.position ?? Number.MAX_SAFE_INTEGER) - (b.position ?? Number.MAX_SAFE_INTEGER);
}

function hasAutoStartAction(step: StepType): boolean {
  return step.events?.on_enter?.some((action) => action.type === "auto_start_agent") ?? false;
}

export function resolveLaunchPreviewStep(
  steps: ReadonlyArray<StepType>,
  launchIntent: TaskCreateLaunchIntent = "start-agent",
): StepType | null {
  const ordered = [...steps].sort(byPosition);
  if (launchIntent === "plan-mode") return ordered[0] ?? null;
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
  launchIntent = "start-agent",
}: ResolveTaskCreateLaunchPreviewArgs): TaskCreateLaunchPreview | null {
  if (!effectiveWorkflowId) return null;

  const matchingFetchedSteps = fetchedSteps?.filter(
    (step) => step.workflowId === effectiveWorkflowId,
  );
  const steps =
    fetchedSteps !== null &&
    matchingFetchedSteps &&
    (fetchedSteps.length === 0 || matchingFetchedSteps.length > 0)
      ? matchingFetchedSteps
      : (snapshotSteps ?? []);
  const step = resolveLaunchPreviewStep(steps, launchIntent);
  if (!step) return null;

  return {
    stepId: step.id,
    stepName: step.title,
    stepPrompt: step.prompt ?? "",
  };
}

export function composeLaunchPreviewPrompt(stepPrompt: string, taskPrompt: string): string {
  return stepPrompt.replace(TASK_PROMPT_TOKEN, () => taskPrompt);
}
