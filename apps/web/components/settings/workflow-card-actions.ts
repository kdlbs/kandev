"use client";

import { useEffect } from "react";
import type { Workflow, WorkflowStep } from "@/lib/types/http";
import { useToast } from "@/components/toast-provider";
import { useRequest } from "@/lib/http/use-request";
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
import type { WorkflowMutationGuardController } from "./workflow-mutation-guard";
import {
  addLocalStep,
  addRemoteStep,
  applyWorkflowStepUpdates,
  newWorkflowStep,
  updateRemoteWorkflowStep,
} from "./workflow-step-mutations";

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
  mutationGuard: WorkflowMutationGuardController;
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
  mutationGuard,
}: WorkflowStepActionsParams) {
  const handleUpdateWorkflowStep = async (stepId: string, updates: Partial<WorkflowStep>) => {
    if (isNewWorkflow) {
      setWorkflowSteps((prev) => applyWorkflowStepUpdates(prev, stepId, updates));
      return;
    }
    const proposedSteps = applyWorkflowStepUpdates(workflowSteps, stepId, updates);
    await mutationGuard.guardMutation({
      proposedSteps,
      operation: () => updateRemoteWorkflowStep({ stepId, updates, refreshWorkflowSteps, toast }),
    });
  };

  const handleAddWorkflowStep = async () => {
    if (isNewWorkflow) {
      addLocalStep(workflow, setWorkflowSteps);
      return;
    }
    const proposedSteps = [
      ...workflowSteps,
      newWorkflowStep(workflow, workflowSteps.length, `proposed-step-${workflow.id}`),
    ];
    await mutationGuard.guardMutation({
      proposedSteps,
      operation: () => addRemoteStep(workflow, workflowSteps.length, refreshWorkflowSteps, toast),
    });
  };

  const handleRemoveWorkflowStep = async (stepId: string) => {
    if (isNewWorkflow) {
      setWorkflowSteps((prev) =>
        prev.filter((s) => s.id !== stepId).map((s, i) => ({ ...s, position: i })),
      );
      return;
    }
    const proposedSteps = workflowSteps
      .filter((step) => step.id !== stepId)
      .map((step, position) => ({ ...step, position }));
    await mutationGuard.guardMutation({
      proposedSteps,
      operation: () =>
        removeWorkflowStep({
          stepId,
          workflowSteps,
          refreshWorkflowSteps,
          setStepToDelete,
          setStepTaskCount,
          setTargetStepForMigration,
          setStepDeleteOpen,
          toast,
        }),
    });
  };

  const handleReorderWorkflowSteps = async (reorderedSteps: WorkflowStep[]) => {
    const proposedSteps = reorderedSteps.map((step, position) => ({ ...step, position }));
    if (isNewWorkflow) {
      setWorkflowSteps(proposedSteps);
      return;
    }
    await mutationGuard.guardMutation({
      proposedSteps,
      operation: async () => {
        setWorkflowSteps(proposedSteps);
        try {
          await reorderWorkflowStepsAction(
            workflow.id,
            proposedSteps.map((step) => step.id),
          );
        } catch (error) {
          toast({
            title: "Failed to reorder workflow steps",
            description: error instanceof Error ? error.message : FALLBACK_ERROR_MESSAGE,
            variant: "error",
          });
          await refreshWorkflowSteps();
        }
      },
    });
  };

  return {
    handleUpdateWorkflowStep,
    handleAddWorkflowStep,
    handleRemoveWorkflowStep,
    handleReorderWorkflowSteps,
    mutationGuard,
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

/**
 * Compare user step edits against backend steps for reconciliation.
 * NOTE: We intentionally do NOT compare events here. The backend creates steps
 * with properly remapped step_id references (template aliases → real UUIDs).
 * If we compared events, the template aliases would overwrite the backend's UUIDs.
 */
function remapPullFromStepID(
  pullFromStepID: string | null | undefined,
  stepIDByDraftID: Map<string, string>,
) {
  if (!pullFromStepID) return "";
  return stepIDByDraftID.get(pullFromStepID) ?? pullFromStepID;
}

function diffStepUpdates(
  userStep: WorkflowStep,
  backendStep: WorkflowStep,
  stepIDByDraftID: Map<string, string>,
): Partial<WorkflowStep> {
  const updates: Partial<WorkflowStep> = {};
  if (userStep.name !== backendStep.name) updates.name = userStep.name;
  if (userStep.color !== backendStep.color) updates.color = userStep.color;
  if (userStep.prompt !== backendStep.prompt) updates.prompt = userStep.prompt;
  if (userStep.is_start_step !== backendStep.is_start_step)
    updates.is_start_step = userStep.is_start_step;
  if (userStep.allow_manual_move !== backendStep.allow_manual_move)
    updates.allow_manual_move = userStep.allow_manual_move;
  if ((userStep.wip_limit ?? 0) !== (backendStep.wip_limit ?? 0))
    updates.wip_limit = userStep.wip_limit ?? 0;
  const pullFromStepID = remapPullFromStepID(userStep.pull_from_step_id, stepIDByDraftID);
  if (pullFromStepID !== (backendStep.pull_from_step_id ?? ""))
    updates.pull_from_step_id = pullFromStepID;
  // Events are NOT compared - backend has correct step_id UUIDs, user has template aliases
  return updates;
}

function stepPayload(workflowId: string, step: WorkflowStep) {
  return {
    workflow_id: workflowId,
    name: step.name,
    position: step.position,
    color: step.color,
    prompt: step.prompt,
    events: step.events,
    is_start_step: step.is_start_step,
    allow_manual_move: step.allow_manual_move,
    wip_limit: step.wip_limit ?? 0,
    pull_from_step_id: step.pull_from_step_id ?? "",
  };
}

function stepPayloadWithoutPullSource(workflowId: string, step: WorkflowStep) {
  return { ...stepPayload(workflowId, step), pull_from_step_id: "" };
}

async function createStepsThenRemapPullSources(
  workflowId: string,
  steps: WorkflowStep[],
  existingStepIDByDraftID = new Map<string, string>(),
) {
  const createdByDraftID = new Map(existingStepIDByDraftID);
  const addedStepIDByDraftID = new Map<string, string>();
  const createdByPosition = new Map<number, string>();
  for (const step of steps) {
    const created = await createWorkflowStepAction(stepPayloadWithoutPullSource(workflowId, step));
    createdByDraftID.set(step.id, created.id);
    addedStepIDByDraftID.set(step.id, created.id);
    createdByPosition.set(step.position, created.id);
  }
  for (const step of steps) {
    const pullFromStepID = remapPullFromStepID(step.pull_from_step_id, createdByDraftID);
    if (!pullFromStepID) continue;
    const createdID = addedStepIDByDraftID.get(step.id) ?? createdByPosition.get(step.position);
    if (createdID) await updateWorkflowStepAction(createdID, { pull_from_step_id: pullFromStepID });
  }
  return addedStepIDByDraftID;
}

async function reconcileTemplateSteps(
  createdId: string,
  userSteps: WorkflowStep[],
  templateStepCount: number,
) {
  const { steps: backendSteps = [] } = await listWorkflowStepsAction(createdId);
  const stepIDByDraftID = new Map<string, string>();
  for (const backendStep of backendSteps) {
    const userStep = userSteps.find((s) => s.position === backendStep.position);
    if (userStep) stepIDByDraftID.set(userStep.id, backendStep.id);
  }

  // Create added steps first so template-step updates can remap pull sources
  // that point at a newly-added step.
  const addedSteps = userSteps.filter((step) => step.position >= templateStepCount);
  const addedStepIDByDraftID = await createStepsThenRemapPullSources(
    createdId,
    addedSteps,
    stepIDByDraftID,
  );
  for (const [draftID, createdID] of addedStepIDByDraftID) {
    stepIDByDraftID.set(draftID, createdID);
  }

  // Reconcile user edits (name, color, etc.) with backend steps.
  // We do NOT touch events - the backend has correct step_id UUIDs.
  for (const backendStep of backendSteps) {
    const userStep = userSteps.find((s) => s.position === backendStep.position);
    if (!userStep) continue;
    const updates = diffStepUpdates(userStep, backendStep, stepIDByDraftID);
    if (Object.keys(updates).length > 0) await updateWorkflowStepAction(backendStep.id, updates);
  }
}

type WorkflowSaveActionsParams = {
  workflow: Workflow;
  isNewWorkflow: boolean;
  workflowSteps: WorkflowStep[];
  templateStepCount: number;
  onSaveWorkflow: () => Promise<unknown>;
  onWorkflowCreated?: (created: Workflow) => void;
  toast: ReturnType<typeof useToast>["toast"];
  mutationGuard: WorkflowMutationGuardController;
};

export function useWorkflowSaveActions({
  workflow,
  isNewWorkflow,
  workflowSteps,
  templateStepCount,
  onSaveWorkflow,
  onWorkflowCreated,
  toast,
  mutationGuard,
}: WorkflowSaveActionsParams) {
  const saveWorkflowRequest = useRequest(onSaveWorkflow);

  const saveNewWorkflowRequest = useRequest(async () => {
    const templateId = workflow.workflow_template_id;
    const created = await createWorkflowAction({
      workspace_id: workflow.workspace_id,
      name: workflow.name.trim() || "New Workflow",
      workflow_template_id: templateId || undefined,
    });

    if (templateId) {
      // Backend creates template steps with remapped step_id references.
      // Reconcile user edits and additions on top.
      await reconcileTemplateSteps(created.id, workflowSteps, templateStepCount);
    } else {
      await createStepsThenRemapPullSources(created.id, workflowSteps);
    }

    onWorkflowCreated?.(created);
  });

  const activeSaveRequest = isNewWorkflow ? saveNewWorkflowRequest : saveWorkflowRequest;

  const runSaveWorkflow = async () => {
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

  const handleSaveWorkflow = async () => {
    if (!isNewWorkflow) {
      await runSaveWorkflow();
      return;
    }
    await mutationGuard.guardMutation({
      baselineSteps: [],
      proposedSteps: workflowSteps,
      intent: "create",
      operation: runSaveWorkflow,
    });
  };

  return { activeSaveRequest, handleSaveWorkflow, mutationGuard };
}

type WorkflowExportActionsParams = {
  workflowId: string;
  setExportYaml: (yaml: string) => void;
  setExportOpen: (open: boolean) => void;
  toast: ReturnType<typeof useToast>["toast"];
};

export async function handleExportWorkflow({
  workflowId,
  setExportYaml,
  setExportOpen,
  toast,
}: WorkflowExportActionsParams) {
  try {
    const yamlText = await exportWorkflowAction(workflowId);
    setExportYaml(yamlText);
    setExportOpen(true);
  } catch (error) {
    toast({
      title: "Failed to export workflow",
      description: error instanceof Error ? error.message : FALLBACK_ERROR_MESSAGE,
      variant: "error",
    });
  }
}
