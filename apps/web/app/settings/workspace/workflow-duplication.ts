import { generateUUID } from "@/lib/utils";
import {
  normalizeWorkflowProfileSessionStartPolicy,
  normalizeWorkflowProfileSessionEndPolicy,
  workflowId as toWorkflowId,
  type Workflow,
  type WorkflowStep,
} from "@/lib/types/http";

const COPY_SUFFIX_PATTERN = /^(.*) \(copy(?: (\d+))?\)$/;

export function getWorkflowCopyName(
  sourceName: string,
  existingWorkflows: readonly Pick<Workflow, "name">[],
): string {
  const baseName = sourceName.match(COPY_SUFFIX_PATTERN)?.[1] ?? sourceName;
  const existingNames = new Set(existingWorkflows.map((workflow) => workflow.name));

  for (let suffix = 1; ; suffix += 1) {
    const candidate = `${baseName} (copy${suffix === 1 ? "" : ` ${suffix}`})`;
    if (!existingNames.has(candidate)) return candidate;
  }
}

function remapStepReferences<T>(value: T, stepIds: ReadonlyMap<string, string>): T {
  if (Array.isArray(value)) return value.map((item) => remapStepReferences(item, stepIds)) as T;
  if (!value || typeof value !== "object") return value;

  return Object.fromEntries(
    Object.entries(value).map(([key, item]) => [
      key,
      key === "step_id" && typeof item === "string"
        ? (stepIds.get(item) ?? item)
        : remapStepReferences(item, stepIds),
    ]),
  ) as T;
}

function temporaryWorkflowId(): string {
  return `temp-workflow-${generateUUID()}`;
}

function temporaryStepId(): string {
  return `temp-step-${generateUUID()}`;
}

function cloneWorkflow(source: Workflow, workflows: readonly Workflow[], id: string): Workflow {
  return {
    id: toWorkflowId(id),
    workspace_id: source.workspace_id,
    name: getWorkflowCopyName(source.name, workflows),
    description: source.description,
    prompt: source.prompt,
    agent_profile_id: source.agent_profile_id,
    created_at: "",
    updated_at: "",
  };
}

function cloneWorkflowStep(
  source: WorkflowStep,
  workflowId: WorkflowStep["workflow_id"],
  stepId: string,
  stepIds: ReadonlyMap<string, string>,
): WorkflowStep {
  const remapStepId = (id: string | null | undefined) =>
    id == null ? id : (stepIds.get(id) ?? id);

  return {
    id: stepId,
    workflow_id: workflowId,
    name: source.name,
    position: source.position,
    color: source.color,
    prompt: source.prompt,
    events: remapStepReferences(source.events, stepIds),
    allow_manual_move: source.allow_manual_move,
    is_start_step: source.is_start_step,
    show_in_command_panel: source.show_in_command_panel,
    auto_archive_after_hours: source.auto_archive_after_hours,
    agent_profile_id: source.agent_profile_id,
    profile_session_start_policy: normalizeWorkflowProfileSessionStartPolicy(
      source.profile_session_start_policy,
    ),
    profile_session_end_policy: normalizeWorkflowProfileSessionEndPolicy(
      source.profile_session_end_policy,
    ),
    auto_advance_requires_signal: source.auto_advance_requires_signal,
    cancel_triggers_turn_complete: source.cancel_triggers_turn_complete,
    wip_limit: source.wip_limit,
    pull_from_step_id: remapStepId(source.pull_from_step_id),
    stage_type: source.stage_type,
    created_at: "",
    updated_at: "",
  };
}

export function createWorkflowDuplication(
  source: Workflow,
  workflows: readonly Workflow[],
  sourceSteps: readonly WorkflowStep[],
): { workflow: Workflow; steps: WorkflowStep[] } {
  const clientWorkflowId = temporaryWorkflowId();
  const stepIds = new Map(sourceSteps.map((step) => [step.id, temporaryStepId()]));
  const workflow = cloneWorkflow(source, workflows, clientWorkflowId);
  const steps = sourceSteps.map((step) =>
    cloneWorkflowStep(step, workflow.id, stepIds.get(step.id)!, stepIds),
  );

  return { workflow, steps };
}
