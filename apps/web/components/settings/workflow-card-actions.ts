"use client";

import { useEffect } from "react";
import type { Workflow, WorkflowStep } from "@/lib/types/http";
import { useToast } from "@/components/toast-provider";
import { useRequest } from "@/lib/http/use-request";
import { generateUUID } from "@/lib/utils";
import {
  createWorkflowAction,
  createWorkflowStepAction,
  updateWorkflowStepAction,
  deleteWorkflowStepAction,
  reorderWorkflowStepsAction,
  listWorkflowStepsAction,
  getStepTaskCount,
  getWorkflowTaskCount,
  exportWorkflowAction,
  bulkMoveTasks,
} from "@/app/actions/workspaces";

const FALLBACK_ERROR_MESSAGE = "Request failed";

type WorkflowStepActionsParams = {
  workflow: Workflow;
  isNewWorkflow: boolean;
  workflowSteps: WorkflowStep[];
  setWorkflowSteps: (updater: ((prev: WorkflowStep[]) => WorkflowStep[]) | WorkflowStep[]) => void;
  refreshWorkflowSteps: () => Promise<void>;
  setStepToDelete: (id: string | null) => void;
  setStepTaskCount: (count: number | null) => void;
  setTargetStepForMigration: (id: string) => void;
  setStepDeleteOpen: (open: boolean) => void;
  toast: ReturnType<typeof useToast>["toast"];
};

type RemoveStepParams = {
  stepId: string;
  workflowSteps: WorkflowStep[];
  refreshWorkflowSteps: () => Promise<void>;
  setStepToDelete: (id: string | null) => void;
  setStepTaskCount: (count: number | null) => void;
  setTargetStepForMigration: (id: string) => void;
  setStepDeleteOpen: (open: boolean) => void;
  toast: ReturnType<typeof useToast>["toast"];
};

async function removeWorkflowStep({
  stepId,
  workflowSteps,
  refreshWorkflowSteps,
  setStepToDelete,
  setStepTaskCount,
  setTargetStepForMigration,
  setStepDeleteOpen,
  toast,
}: RemoveStepParams) {
  try {
    const { task_count } = await getStepTaskCount(stepId);
    if (task_count === 0) {
      await deleteWorkflowStepAction(stepId);
      await refreshWorkflowSteps();
      return;
    }
    setStepToDelete(stepId);
    setStepTaskCount(task_count);
    const otherSteps = workflowSteps.filter((s) => s.id !== stepId);
    setTargetStepForMigration(otherSteps.length > 0 ? otherSteps[0].id : "");
    setStepDeleteOpen(true);
  } catch (error) {
    toast({
      title: "Failed to check step tasks",
      description: error instanceof Error ? error.message : FALLBACK_ERROR_MESSAGE,
      variant: "error",
    });
  }
}

const NEW_STEP_DEFAULTS = { name: "New Step", color: "bg-slate-500" } as const;

function addLocalStep(
  workflow: Workflow,
  setWorkflowSteps: WorkflowStepActionsParams["setWorkflowSteps"],
) {
  setWorkflowSteps((prev) => [
    ...prev,
    {
      id: `temp-step-${generateUUID()}`,
      workflow_id: workflow.id,
      ...NEW_STEP_DEFAULTS,
      position: prev.length,
      allow_manual_move: true,
      created_at: "",
      updated_at: "",
    },
  ]);
}

async function addRemoteStep(
  workflow: Workflow,
  stepCount: number,
  refreshWorkflowSteps: () => Promise<void>,
  toast: WorkflowStepActionsParams["toast"],
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

export function useWorkflowStepActions({
  workflow,
  isNewWorkflow,
  workflowSteps,
  setWorkflowSteps,
  refreshWorkflowSteps,
  setStepToDelete,
  setStepTaskCount,
  setTargetStepForMigration,
  setStepDeleteOpen,
  toast,
}: WorkflowStepActionsParams) {
  const handleUpdateWorkflowStep = async (stepId: string, updates: Partial<WorkflowStep>) => {
    if (isNewWorkflow) {
      setWorkflowSteps((prev) => prev.map((s) => (s.id === stepId ? { ...s, ...updates } : s)));
      return;
    }
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
  };

  const handleAddWorkflowStep = async () => {
    if (isNewWorkflow) {
      addLocalStep(workflow, setWorkflowSteps);
      return;
    }
    await addRemoteStep(workflow, workflowSteps.length, refreshWorkflowSteps, toast);
  };

  const handleRemoveWorkflowStep = async (stepId: string) => {
    if (isNewWorkflow) {
      setWorkflowSteps((prev) =>
        prev.filter((s) => s.id !== stepId).map((s, i) => ({ ...s, position: i })),
      );
      return;
    }
    await removeWorkflowStep({
      stepId,
      workflowSteps,
      refreshWorkflowSteps,
      setStepToDelete,
      setStepTaskCount,
      setTargetStepForMigration,
      setStepDeleteOpen,
      toast,
    });
  };

  const handleReorderWorkflowSteps = async (reorderedSteps: WorkflowStep[]) => {
    setWorkflowSteps(reorderedSteps);
    if (isNewWorkflow) return;
    try {
      await reorderWorkflowStepsAction(
        workflow.id,
        reorderedSteps.map((s) => s.id),
      );
    } catch (error) {
      toast({
        title: "Failed to reorder workflow steps",
        description: error instanceof Error ? error.message : FALLBACK_ERROR_MESSAGE,
        variant: "error",
      });
      await refreshWorkflowSteps();
    }
  };

  return {
    handleUpdateWorkflowStep,
    handleAddWorkflowStep,
    handleRemoveWorkflowStep,
    handleReorderWorkflowSteps,
  };
}

type WorkflowDeleteHandlersParams = {
  workflow: Workflow;
  isNewWorkflow: boolean;
  otherWorkflows: Workflow[];
  wfDel: {
    setDeleteOpen: (v: boolean) => void;
    setWorkflowTaskCount: (v: number | null) => void;
    setWorkflowDeleteLoading: (v: boolean) => void;
    setTargetWorkflowId: (v: string) => void;
    setTargetWorkflowSteps: (v: WorkflowStep[]) => void;
    setTargetStepId: (v: string) => void;
    targetWorkflowId: string;
    targetStepId: string;
    setMigrateLoading: (v: boolean) => void;
  };
  deleteWorkflowRun: () => Promise<unknown>;
  toast: ReturnType<typeof useToast>["toast"];
};

export function useWorkflowDeleteHandlers({
  workflow,
  isNewWorkflow,
  otherWorkflows,
  wfDel,
  deleteWorkflowRun,
  toast,
}: WorkflowDeleteHandlersParams) {
  useEffect(() => {
    if (!wfDel.targetWorkflowId) {
      wfDel.setTargetWorkflowSteps([]);
      wfDel.setTargetStepId("");
      return;
    }
    let cancelled = false;
    listWorkflowStepsAction(wfDel.targetWorkflowId)
      .then((res) => {
        if (!cancelled) {
          const steps = res.steps ?? [];
          wfDel.setTargetWorkflowSteps(steps);
          wfDel.setTargetStepId(steps.length > 0 ? steps[0].id : "");
        }
      })
      .catch(() => {
        if (!cancelled) wfDel.setTargetWorkflowSteps([]);
      });
    return () => {
      cancelled = true;
    };
  }, [wfDel.targetWorkflowId]); // eslint-disable-line react-hooks/exhaustive-deps

  const handleDeleteWorkflowClick = async () => {
    if (isNewWorkflow) {
      wfDel.setWorkflowTaskCount(0);
      wfDel.setDeleteOpen(true);
      return;
    }
    wfDel.setWorkflowDeleteLoading(true);
    try {
      const { task_count } = await getWorkflowTaskCount(workflow.id);
      wfDel.setWorkflowTaskCount(task_count);
      if (task_count > 0 && otherWorkflows.length > 0)
        wfDel.setTargetWorkflowId(otherWorkflows[0].id);
      wfDel.setDeleteOpen(true);
    } catch (error) {
      toast({
        title: "Failed to check workflow tasks",
        description: error instanceof Error ? error.message : FALLBACK_ERROR_MESSAGE,
        variant: "error",
      });
    } finally {
      wfDel.setWorkflowDeleteLoading(false);
    }
  };

  const handleDeleteWorkflow = async () => {
    try {
      await deleteWorkflowRun();
      wfDel.setDeleteOpen(false);
    } catch (error) {
      toast({
        title: "Failed to delete workflow",
        description: error instanceof Error ? error.message : FALLBACK_ERROR_MESSAGE,
        variant: "error",
      });
    }
  };

  const handleMigrateAndDeleteWorkflow = async () => {
    if (!wfDel.targetWorkflowId || !wfDel.targetStepId) return;
    wfDel.setMigrateLoading(true);
    try {
      await bulkMoveTasks({
        source_workflow_id: workflow.id,
        target_workflow_id: wfDel.targetWorkflowId,
        target_step_id: wfDel.targetStepId,
      });
      await deleteWorkflowRun();
      wfDel.setDeleteOpen(false);
    } catch (error) {
      toast({
        title: "Failed to migrate tasks",
        description: error instanceof Error ? error.message : FALLBACK_ERROR_MESSAGE,
        variant: "error",
      });
    } finally {
      wfDel.setMigrateLoading(false);
    }
  };

  return { handleDeleteWorkflowClick, handleDeleteWorkflow, handleMigrateAndDeleteWorkflow };
}

type StepDeleteHandlersParams = {
  workflow: Workflow;
  stepDel: {
    stepToDelete: string | null;
    targetStepForMigration: string;
    setStepMigrateLoading: (v: boolean) => void;
    setStepDeleteOpen: (v: boolean) => void;
    setStepToDelete: (v: string | null) => void;
  };
  refreshWorkflowSteps: () => Promise<void>;
  toast: ReturnType<typeof useToast>["toast"];
};

export function useStepDeleteHandlers({
  workflow,
  stepDel,
  refreshWorkflowSteps,
  toast,
}: StepDeleteHandlersParams) {
  const handleMigrateAndDeleteStep = async () => {
    if (!stepDel.stepToDelete || !stepDel.targetStepForMigration) return;
    stepDel.setStepMigrateLoading(true);
    try {
      await bulkMoveTasks({
        source_workflow_id: workflow.id,
        source_step_id: stepDel.stepToDelete,
        target_workflow_id: workflow.id,
        target_step_id: stepDel.targetStepForMigration,
      });
      await deleteWorkflowStepAction(stepDel.stepToDelete);
      await refreshWorkflowSteps();
      stepDel.setStepDeleteOpen(false);
      stepDel.setStepToDelete(null);
    } catch (error) {
      toast({
        title: "Failed to migrate tasks",
        description: error instanceof Error ? error.message : FALLBACK_ERROR_MESSAGE,
        variant: "error",
      });
    } finally {
      stepDel.setStepMigrateLoading(false);
    }
  };

  const handleDeleteStepAndTasks = async () => {
    if (!stepDel.stepToDelete) return;
    stepDel.setStepMigrateLoading(true);
    try {
      await deleteWorkflowStepAction(stepDel.stepToDelete);
      await refreshWorkflowSteps();
      stepDel.setStepDeleteOpen(false);
      stepDel.setStepToDelete(null);
    } catch (error) {
      toast({
        title: "Failed to delete step",
        description: error instanceof Error ? error.message : FALLBACK_ERROR_MESSAGE,
        variant: "error",
      });
    } finally {
      stepDel.setStepMigrateLoading(false);
    }
  };

  return { handleMigrateAndDeleteStep, handleDeleteStepAndTasks };
}

type WorkflowSaveActionsParams = {
  workflow: Workflow;
  isNewWorkflow: boolean;
  workflowSteps: WorkflowStep[];
  onSaveWorkflow: () => Promise<unknown>;
  onWorkflowCreated?: (created: Workflow) => void;
  toast: ReturnType<typeof useToast>["toast"];
};

export function useWorkflowSaveActions({
  workflow,
  isNewWorkflow,
  workflowSteps,
  onSaveWorkflow,
  onWorkflowCreated,
  toast,
}: WorkflowSaveActionsParams) {
  const saveWorkflowRequest = useRequest(onSaveWorkflow);

  const saveNewWorkflowRequest = useRequest(async () => {
    const created = await createWorkflowAction({
      workspace_id: workflow.workspace_id,
      name: workflow.name.trim() || "New Workflow",
    });
    for (const step of workflowSteps) {
      await createWorkflowStepAction({
        workflow_id: created.id,
        name: step.name,
        position: step.position,
        color: step.color,
        prompt: step.prompt,
        events: step.events,
        is_start_step: step.is_start_step,
        allow_manual_move: step.allow_manual_move,
      });
    }
    onWorkflowCreated?.(created);
  });

  const activeSaveRequest = isNewWorkflow ? saveNewWorkflowRequest : saveWorkflowRequest;

  const handleSaveWorkflow = async () => {
    try {
      if (isNewWorkflow) await saveNewWorkflowRequest.run();
      else await saveWorkflowRequest.run();
    } catch (error) {
      toast({
        title: "Failed to save workflow changes",
        description: error instanceof Error ? error.message : FALLBACK_ERROR_MESSAGE,
        variant: "error",
      });
    }
  };

  return { activeSaveRequest, handleSaveWorkflow };
}

type WorkflowExportActionsParams = {
  workflowId: string;
  setExportJson: (json: string) => void;
  setExportOpen: (open: boolean) => void;
  toast: ReturnType<typeof useToast>["toast"];
};

export async function handleExportWorkflow({
  workflowId,
  setExportJson,
  setExportOpen,
  toast,
}: WorkflowExportActionsParams) {
  try {
    const data = await exportWorkflowAction(workflowId);
    setExportJson(JSON.stringify(data, null, 2));
    setExportOpen(true);
  } catch (error) {
    toast({
      title: "Failed to export workflow",
      description: error instanceof Error ? error.message : FALLBACK_ERROR_MESSAGE,
      variant: "error",
    });
  }
}
