"use client";

import { useState } from "react";
import {
  createWorkflowAction,
  createWorkflowStepAction,
  deleteWorkflowAction,
  listWorkflowStepsAction,
} from "@/app/actions/workspaces";
import type {
  StepDefinition,
  Workflow,
  WorkflowStep,
  WorkflowTemplate,
  Workspace,
} from "@/lib/types/http";
import type { useToast } from "@/components/toast-provider";

export const DEFAULT_CUSTOM_STEPS: StepDefinition[] = [
  { name: "Todo", position: 0, color: "bg-slate-500" },
  { name: "In Progress", position: 1, color: "bg-blue-500" },
  { name: "Review", position: 2, color: "bg-purple-500" },
  { name: "Done", position: 3, color: "bg-green-500" },
];

export function buildWorkflowSteps(
  workflow: Workflow,
  definitions: StepDefinition[],
): WorkflowStep[] {
  return definitions.map((step, index) => ({
    id: `temp-step-${workflow.id}-${index}`,
    workflow_id: workflow.id,
    name: step.name,
    position: step.position ?? index,
    color: step.color ?? "bg-slate-500",
    prompt: step.prompt,
    events: step.events,
    is_start_step: step.is_start_step,
    show_in_command_panel: step.show_in_command_panel,
    allow_manual_move: true,
    created_at: "",
    updated_at: "",
  }));
}

type WorkflowCreationArgs = {
  workspace: Workspace | null;
  workflowTemplates: WorkflowTemplate[];
  setWorkflowItems: React.Dispatch<React.SetStateAction<Workflow[]>>;
  setSavedWorkflowItems: React.Dispatch<React.SetStateAction<Workflow[]>>;
  toast: ReturnType<typeof useToast>["toast"];
};

export function useWorkflowCreation({
  workspace,
  workflowTemplates,
  setWorkflowItems,
  setSavedWorkflowItems,
  toast,
}: WorkflowCreationArgs) {
  const [isAddWorkflowDialogOpen, setIsAddWorkflowDialogOpen] = useState(false);
  const [newWorkflowName, setNewWorkflowName] = useState("");
  const [selectedTemplateId, setSelectedTemplateId] = useState<string | null>(null);
  const [createWorkflowLoading, setCreateWorkflowLoading] = useState(false);
  const [initialStepsByWorkflowId, setInitialStepsByWorkflowId] = useState(
    () => new Map<string, WorkflowStep[]>(),
  );

  const handleOpenAddWorkflowDialog = () => {
    setNewWorkflowName("");
    setSelectedTemplateId(workflowTemplates.length > 0 ? workflowTemplates[0].id : null);
    setIsAddWorkflowDialogOpen(true);
  };

  const handleCreateWorkflow = async () => {
    if (!workspace) return;
    const templateName = selectedTemplateId
      ? (workflowTemplates.find((template) => template.id === selectedTemplateId)?.name ??
        "New Workflow")
      : "New Workflow";
    let createdWorkflowId: string | null = null;
    setCreateWorkflowLoading(true);
    try {
      const created = await createWorkflowAction({
        workspace_id: workspace.id,
        name: newWorkflowName.trim() || templateName,
        workflow_template_id: selectedTemplateId || undefined,
      });
      createdWorkflowId = created.id;
      const createdSteps = selectedTemplateId
        ? ((await listWorkflowStepsAction(created.id)).steps ?? [])
        : await Promise.all(
            DEFAULT_CUSTOM_STEPS.map((step) =>
              createWorkflowStepAction({
                workflow_id: created.id,
                name: step.name,
                position: step.position,
                color: step.color ?? "bg-slate-500",
              }),
            ),
          );
      setInitialStepsByWorkflowId((prev) => new Map(prev).set(created.id, createdSteps));
      setWorkflowItems((prev) =>
        prev.some((workflow) => workflow.id === created.id) ? prev : [created, ...prev],
      );
      setSavedWorkflowItems((prev) =>
        prev.some((workflow) => workflow.id === created.id) ? prev : [{ ...created }, ...prev],
      );
      setIsAddWorkflowDialogOpen(false);
    } catch (error) {
      if (createdWorkflowId) await deleteWorkflowAction(createdWorkflowId).catch(() => {});
      toast({
        title: "Failed to create workflow",
        description: error instanceof Error ? error.message : "Request failed",
        variant: "error",
      });
    } finally {
      setCreateWorkflowLoading(false);
    }
  };

  const forgetInitialSteps = (workflowId: string) => {
    setInitialStepsByWorkflowId((prev) => {
      const next = new Map(prev);
      next.delete(workflowId);
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
    createWorkflowLoading,
    initialStepsByWorkflowId,
    handleOpenAddWorkflowDialog,
    handleCreateWorkflow,
    forgetInitialSteps,
  };
}
