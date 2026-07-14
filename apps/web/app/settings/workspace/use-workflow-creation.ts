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

type WorkflowCreationArgs = {
  workspace: Workspace | null;
  workflowTemplates: WorkflowTemplate[];
  setWorkflowItems: React.Dispatch<React.SetStateAction<Workflow[]>>;
  setSavedWorkflowItems: React.Dispatch<React.SetStateAction<Workflow[]>>;
  toast: ReturnType<typeof useToast>["toast"];
};

async function createInitialWorkflowSteps(workflow: Workflow, templateId: string | null) {
  if (templateId) {
    return (await listWorkflowStepsAction(workflow.id)).steps ?? [];
  }
  return Promise.all(
    DEFAULT_CUSTOM_STEPS.map((step) =>
      createWorkflowStepAction({
        workflow_id: workflow.id,
        name: step.name,
        position: step.position,
        color: step.color ?? "bg-slate-500",
      }),
    ),
  );
}

function addCreatedWorkflow(
  workflow: Workflow,
  setWorkflowItems: WorkflowCreationArgs["setWorkflowItems"],
  setSavedWorkflowItems: WorkflowCreationArgs["setSavedWorkflowItems"],
) {
  setWorkflowItems((prev) =>
    prev.some((item) => item.id === workflow.id) ? prev : [workflow, ...prev],
  );
  setSavedWorkflowItems((prev) =>
    prev.some((item) => item.id === workflow.id) ? prev : [{ ...workflow }, ...prev],
  );
}

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
    let createdWorkflow: Workflow | null = null;
    setCreateWorkflowLoading(true);
    try {
      const created = await createWorkflowAction({
        workspace_id: workspace.id,
        name: newWorkflowName.trim() || templateName,
        workflow_template_id: selectedTemplateId || undefined,
      });
      createdWorkflow = created;
      const createdSteps = await createInitialWorkflowSteps(created, selectedTemplateId);
      setInitialStepsByWorkflowId((prev) => new Map(prev).set(created.id, createdSteps));
      addCreatedWorkflow(created, setWorkflowItems, setSavedWorkflowItems);
      setIsAddWorkflowDialogOpen(false);
    } catch (error) {
      if (createdWorkflow) {
        const workflowToRecover = createdWorkflow;
        try {
          await deleteWorkflowAction(workflowToRecover.id);
        } catch (cleanupError) {
          addCreatedWorkflow(workflowToRecover, setWorkflowItems, setSavedWorkflowItems);
          setIsAddWorkflowDialogOpen(false);
          toast({
            title: "Workflow created with setup errors",
            description: `Initial setup failed and cleanup also failed: ${
              cleanupError instanceof Error ? cleanupError.message : "Request failed"
            }. The workflow was kept so you can retry or delete it.`,
            variant: "error",
          });
          return;
        }
      }
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
