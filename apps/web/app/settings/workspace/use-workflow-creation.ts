"use client";

import { useState } from "react";
import { generateUUID } from "@/lib/utils";
import {
  workflowId as toWorkflowId,
  workspaceId as toWorkspaceId,
  type StepDefinition,
  type Workflow,
  type WorkflowStep,
  type WorkflowTemplate,
  type Workspace,
  normalizeWorkflowProfileSessionStartPolicy,
  normalizeWorkflowProfileSessionEndPolicy,
} from "@/lib/types/http";
import { createWorkflowDuplication } from "./workflow-duplication";

// Seeded step definitions used whenever no template supplies default steps —
// the Custom option, and any template whose `default_steps` is absent or empty.
// These names are PERSISTED as the workflow's step names, so they deliberately
// stay English — translating them would write localized values into the
// database.
// i18n-exempt: persisted workflow step names. See the comment above.
export const DEFAULT_CUSTOM_STEPS: StepDefinition[] = [
  { name: "Todo", position: 0, color: "bg-slate-500" },
  { name: "In Progress", position: 1, color: "bg-blue-500" },
  { name: "Review", position: 2, color: "bg-purple-500" },
  { name: "Done", position: 3, color: "bg-green-500" },
];

type WorkflowCreationArgs = {
  workspace: Workspace | null;
  workflowItems: Workflow[];
  workflowTemplates: WorkflowTemplate[];
  setWorkflowItems: React.Dispatch<React.SetStateAction<Workflow[]>>;
};

function newClientId(prefix: string): string {
  return `${prefix}-${generateUUID()}`;
}

function remapStepReferences<T>(value: T, mappings: Map<string, string>): T {
  if (Array.isArray(value)) return value.map((item) => remapStepReferences(item, mappings)) as T;
  if (!value || typeof value !== "object") return value;
  return Object.fromEntries(
    Object.entries(value).map(([key, item]) => [
      key,
      key === "step_id" && typeof item === "string"
        ? (mappings.get(item) ?? item)
        : remapStepReferences(item, mappings),
    ]),
  ) as T;
}

function toDraftStep(
  tempWorkflowId: string,
  definition: StepDefinition,
  definitionIds: Map<string, string>,
): WorkflowStep {
  return {
    id:
      definitionIds.get(definition.id ?? "") ??
      `temp-template-step-${tempWorkflowId}-${definition.position}`,
    workflow_id: toWorkflowId(tempWorkflowId),
    name: definition.name,
    position: definition.position,
    color: definition.color ?? "bg-slate-500",
    prompt: definition.prompt,
    events: remapStepReferences(definition.events, definitionIds),
    is_start_step: definition.is_start_step,
    show_in_command_panel: definition.show_in_command_panel,
    agent_profile_id: definition.agent_profile_id,
    profile_session_start_policy: normalizeWorkflowProfileSessionStartPolicy(
      definition.profile_session_start_policy,
    ),
    profile_session_end_policy: normalizeWorkflowProfileSessionEndPolicy(
      definition.profile_session_end_policy,
    ),
    auto_advance_requires_signal: definition.auto_advance_requires_signal,
    cancel_triggers_turn_complete: definition.cancel_triggers_turn_complete,
    wip_limit: definition.wip_limit,
    pull_from_step_id: definition.pull_from_step_id
      ? (definitionIds.get(definition.pull_from_step_id) ?? definition.pull_from_step_id)
      : definition.pull_from_step_id,
    allow_manual_move: true,
    created_at: "",
    updated_at: "",
  };
}

export function createDraftWorkflowSteps(
  tempWorkflowId: string,
  definitions: StepDefinition[],
): WorkflowStep[] {
  const definitionIds = new Map(
    definitions.flatMap((definition) =>
      definition.id
        ? [[definition.id, `temp-template-step-${tempWorkflowId}-${definition.position}`] as const]
        : [],
    ),
  );
  return definitions.map((definition) => toDraftStep(tempWorkflowId, definition, definitionIds));
}

export function useWorkflowCreation({
  workspace,
  workflowItems,
  workflowTemplates,
  setWorkflowItems,
}: WorkflowCreationArgs) {
  const [isAddWorkflowDialogOpen, setIsAddWorkflowDialogOpen] = useState(false);
  const [newWorkflowName, setNewWorkflowName] = useState("");
  const [selectedTemplateId, setSelectedTemplateId] = useState<string | null>(null);
  const [initialStepsByWorkflowId, setInitialStepsByWorkflowId] = useState(
    () => new Map<string, WorkflowStep[]>(),
  );

  const handleOpenAddWorkflowDialog = () => {
    setNewWorkflowName("");
    setSelectedTemplateId(workflowTemplates.length > 0 ? workflowTemplates[0].id : null);
    setIsAddWorkflowDialogOpen(true);
  };

  const handleCreateWorkflow = () => {
    if (!workspace) return;
    const template = selectedTemplateId
      ? workflowTemplates.find((item) => item.id === selectedTemplateId)
      : undefined;
    const tempId = newClientId("temp-workflow");
    // i18n-exempt: persisted workflow name. See the comment below.
    const workflow: Workflow = {
      id: toWorkflowId(tempId),
      workspace_id: toWorkspaceId(workspace.id),
      // Persisted workflow name; the fallback is data, not copy.
      name: newWorkflowName.trim() || template?.name || "New Workflow",
      description: template?.description,
      workflow_template_id: template?.id,
      created_at: "",
      updated_at: "",
    };
    const definitions = template?.default_steps?.length
      ? template.default_steps
      : DEFAULT_CUSTOM_STEPS;
    const steps = createDraftWorkflowSteps(tempId, definitions);

    setInitialStepsByWorkflowId((previous) => new Map(previous).set(tempId, steps));
    setWorkflowItems((previous) => [workflow, ...previous]);
    setIsAddWorkflowDialogOpen(false);
  };

  const handleDuplicateWorkflow = (source: Workflow, sourceSteps: WorkflowStep[]) => {
    const { workflow, steps } = createWorkflowDuplication(source, workflowItems, sourceSteps);
    setInitialStepsByWorkflowId((previous) => new Map(previous).set(workflow.id, steps));
    setWorkflowItems((previous) => {
      const sourceIndex = previous.findIndex((item) => item.id === source.id);
      if (sourceIndex === -1) return [...previous, workflow];
      return [...previous.slice(0, sourceIndex + 1), workflow, ...previous.slice(sourceIndex + 1)];
    });
  };

  const forgetInitialSteps = (workflowId: string) => {
    setInitialStepsByWorkflowId((previous) => {
      const next = new Map(previous);
      next.delete(workflowId);
      return next;
    });
  };

  const remapInitialSteps = (
    clientWorkflowId: string,
    persistedWorkflowId: string,
    steps: WorkflowStep[],
  ) => {
    setInitialStepsByWorkflowId((previous) => {
      const next = new Map(previous);
      next.delete(clientWorkflowId);
      next.set(persistedWorkflowId, steps);
      return next;
    });
  };

  return {
    isAddWorkflowDialogOpen,
    setIsAddWorkflowDialogOpen,
    newWorkflowName,
    setNewWorkflowName,
    selectedTemplateId,
    setSelectedTemplateId,
    createWorkflowLoading: false,
    initialStepsByWorkflowId,
    handleOpenAddWorkflowDialog,
    handleCreateWorkflow,
    handleDuplicateWorkflow,
    forgetInitialSteps,
    remapInitialSteps,
  };
}
