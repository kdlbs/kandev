import type { Workflow, WorkflowStep } from "@/lib/types/http";
import type { WorkflowActionRecord, WorkflowLifecycleTrigger } from "./workflow-action-catalog";
import { scriptActionConfig, validateWorkflowScriptAction } from "./workflow-action-catalog";
import { areStepDraftsEqual } from "@/components/settings/workflow-card-actions";

export type WorkflowEditorIssue = {
  id: string;
  stepId: string;
  trigger?: WorkflowLifecycleTrigger;
  actionIndex?: number;
  target: "workflow" | "step" | "action";
  messageKey: string;
};

export type WorkflowStepSummary = {
  stepId: string;
  name: string;
  color: string;
  effectiveProfileId?: string;
  actionCount: number;
  actionCounts: Record<WorkflowLifecycleTrigger, number>;
  primaryDestinationId?: string;
  isDirty: boolean;
  issues: WorkflowEditorIssue[];
};

export type WorkflowEditorEdge = {
  fromStepId: string;
  toStepId: string;
  trigger: "on_turn_start" | "on_turn_complete";
};

export type WorkflowEditorViewModel = {
  stepSummaries: WorkflowStepSummary[];
  edges: WorkflowEditorEdge[];
  issues: WorkflowEditorIssue[];
  workflowDirty: boolean;
};

const SUMMARY_TRIGGERS: readonly WorkflowLifecycleTrigger[] = [
  "on_enter",
  "on_turn_start",
  "on_turn_complete",
  "on_exit",
];

export function buildWorkflowEditorViewModel(
  workflow: Workflow,
  steps: WorkflowStep[],
  savedWorkflow?: Workflow,
  savedSteps: WorkflowStep[] = [],
): WorkflowEditorViewModel {
  const orderedSteps = [...steps].sort((left, right) => left.position - right.position);
  const savedById = new Map(savedSteps.map((step) => [step.id, step]));
  const stepIds = new Set(orderedSteps.map((step) => step.id));
  const summaries = orderedSteps.map((step) =>
    buildStepSummary(workflow, step, savedById.get(step.id), orderedSteps, stepIds),
  );
  const issues = summaries.flatMap((summary) => summary.issues);
  if (!workflow.name.trim()) {
    issues.unshift({
      id: "workflow-name-required",
      stepId: "",
      target: "workflow",
      messageKey: "workflows:workflowNameIsRequired",
    });
  }
  return {
    stepSummaries: summaries,
    edges: orderedSteps.flatMap((step) => transitionEdges(step, orderedSteps, stepIds)),
    issues,
    workflowDirty: !savedWorkflow || !sameWorkflowFields(workflow, savedWorkflow),
  };
}

export function repairWorkflowEditorSelection(
  selectedStepId: string | null,
  steps: WorkflowStep[],
): string | null {
  if (steps.length === 0) return null;
  if (selectedStepId && steps.some((step) => step.id === selectedStepId)) return selectedStepId;
  return [...steps].sort((left, right) => left.position - right.position)[0]?.id ?? null;
}

function buildStepSummary(
  workflow: Workflow,
  step: WorkflowStep,
  savedStep: WorkflowStep | undefined,
  steps: WorkflowStep[],
  stepIds: Set<string>,
): WorkflowStepSummary {
  const actionCounts = Object.fromEntries(
    SUMMARY_TRIGGERS.map((trigger) => [trigger, actionsFor(step, trigger).length]),
  ) as Record<WorkflowLifecycleTrigger, number>;
  const issues = stepIssues(step, stepIds);
  return {
    stepId: step.id,
    name: step.name,
    color: step.color,
    effectiveProfileId: step.agent_profile_id ?? workflow.agent_profile_id,
    actionCount: Object.values(actionCounts).reduce((total, count) => total + count, 0),
    actionCounts,
    primaryDestinationId: primaryDestinationId(step, steps, stepIds),
    isDirty: !savedStep || !areStepDraftsEqual(step, savedStep),
    issues,
  };
}

function stepIssues(step: WorkflowStep, stepIds: Set<string>): WorkflowEditorIssue[] {
  const issues: WorkflowEditorIssue[] = [];
  for (const trigger of SUMMARY_TRIGGERS) {
    const actions = actionsFor(step, trigger);
    actions.forEach((action, actionIndex) => {
      const validation = action.type === "run_script" ? validateWorkflowScriptAction(action) : null;
      if (validation && !validation.valid) {
        issues.push({
          id: `${step.id}:${trigger}:${actionIndex}:script`,
          stepId: step.id,
          trigger,
          actionIndex,
          target: "action",
          messageKey: validation.errorKey,
        });
      }
      if (action.type === "move_to_step") {
        const target = scriptActionConfig(action)?.step_id ?? action.config?.step_id;
        if (typeof target !== "string" || !stepIds.has(target)) {
          issues.push({
            id: `${step.id}:${trigger}:${actionIndex}:target`,
            stepId: step.id,
            trigger,
            actionIndex,
            target: "action",
            messageKey: "workflows:workflowTransitionTargetRequired",
          });
        }
      }
    });
  }
  return issues;
}

function transitionEdges(
  step: WorkflowStep,
  steps: WorkflowStep[],
  stepIds: Set<string>,
): WorkflowEditorEdge[] {
  return (["on_turn_start", "on_turn_complete"] as const).flatMap((trigger) => {
    const action = actionsFor(step, trigger).find(isTransitionAction);
    const target = transitionTarget(action, step, steps);
    return target && stepIds.has(target)
      ? [{ fromStepId: step.id, toStepId: target, trigger }]
      : [];
  });
}

function primaryDestinationId(
  step: WorkflowStep,
  steps: WorkflowStep[],
  stepIds: Set<string>,
): string | undefined {
  for (const trigger of ["on_turn_complete", "on_turn_start"] as const) {
    const action = actionsFor(step, trigger).find(isTransitionAction);
    const target = transitionTarget(action, step, steps);
    if (target && stepIds.has(target)) return target;
  }
  return undefined;
}

function transitionTarget(
  action: WorkflowActionRecord | undefined,
  step: WorkflowStep,
  steps: WorkflowStep[],
): string | undefined {
  if (!action) return undefined;
  const index = steps.findIndex((candidate) => candidate.id === step.id);
  if (action.type === "move_to_next") return steps[index + 1]?.id;
  if (action.type === "move_to_previous") return steps[index - 1]?.id;
  const target = action.config?.step_id;
  return typeof target === "string" && target ? target : undefined;
}

function isTransitionAction(action: WorkflowActionRecord): boolean {
  return ["move_to_next", "move_to_previous", "move_to_step"].includes(action.type);
}

function actionsFor(step: WorkflowStep, trigger: WorkflowLifecycleTrigger): WorkflowActionRecord[] {
  return (step.events?.[trigger] ?? []) as unknown as WorkflowActionRecord[];
}

function sameWorkflowFields(left: Workflow, right: Workflow): boolean {
  return (
    left.name === right.name &&
    (left.description ?? "") === (right.description ?? "") &&
    (left.prompt ?? "") === (right.prompt ?? "") &&
    (left.agent_profile_id ?? "") === (right.agent_profile_id ?? "")
  );
}
