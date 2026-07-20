"use client";

import { useEffect, useRef } from "react";
import type { Workflow, WorkflowStep } from "@/lib/types/http";
import { useToast } from "@/components/toast-provider";
import { useSerializedMutationQueue } from "./use-serialized-mutation-queue";
import type { WorkflowMutationGuardController } from "./workflow-mutation-guard";
import { applyWorkflowStepUpdates } from "./workflow-step-mutations";
import {
  createWorkflowAction,
  createWorkflowStepAction,
  updateWorkflowAction,
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
const TEMP_WORKFLOW_PREFIX = "temp-workflow-";

type WorkflowStepActionsParams = {
  workflow: Workflow;
  isNewWorkflow?: boolean;
  /**
   * Workflows synced from GitHub (`workflow.source === "github"`) are
   * read-only in the UI; the backend also rejects step mutations with a 409.
   * Gating here is defense-in-depth in case a disabled control is somehow
   * still triggered.
   */
  readOnly?: boolean;
  workflowSteps: WorkflowStep[];
  setWorkflowSteps: (updater: ((prev: WorkflowStep[]) => WorkflowStep[]) | WorkflowStep[]) => void;
  refreshWorkflowSteps?: () => Promise<void>;
  setStepToDelete: (id: string | null) => void;
  setStepTaskCount: (count: number | null) => void;
  setTargetStepForMigration: (id: string) => void;
  setStepDeleteOpen: (open: boolean) => void;
  toast: ReturnType<typeof useToast>["toast"];
  mutationGuard?: WorkflowMutationGuardController;
};

type OpenStepDeleteDialogParams = {
  stepId: string;
  taskCount: number;
  workflowSteps: WorkflowStep[];
  setStepToDelete: (id: string | null) => void;
  setStepTaskCount: (count: number | null) => void;
  setTargetStepForMigration: (id: string) => void;
  setStepDeleteOpen: (open: boolean) => void;
};

function openStepDeleteDialog({
  stepId,
  taskCount,
  workflowSteps,
  setStepToDelete,
  setStepTaskCount,
  setTargetStepForMigration,
  setStepDeleteOpen,
}: OpenStepDeleteDialogParams) {
  setStepToDelete(stepId);
  setStepTaskCount(taskCount);
  const otherSteps = workflowSteps.filter((s) => s.id !== stepId);
  setTargetStepForMigration(otherSteps.length > 0 ? otherSteps[0].id : "");
  setStepDeleteOpen(true);
}

const NEW_STEP_DEFAULTS = { name: "New Step", color: "bg-slate-500" } as const;

function createDraftStep(workflow: Workflow, position: number): WorkflowStep {
  return {
    id: `temp-step-${crypto.randomUUID()}`,
    workflow_id: workflow.id,
    ...NEW_STEP_DEFAULTS,
    position,
    allow_manual_move: true,
    created_at: "",
    updated_at: "",
  };
}

function useWorkflowStepMutationQueue(toast: WorkflowStepActionsParams["toast"]) {
  return useSerializedMutationQueue((errorTitle, error) => {
    toast({
      title: errorTitle,
      description: error instanceof Error ? error.message : FALLBACK_ERROR_MESSAGE,
      variant: "error",
    });
  });
}

function createRemoveWorkflowStepHandler(
  params: WorkflowStepActionsParams,
  guardMutation: WorkflowMutationGuardController["guardMutation"],
) {
  const {
    workflow,
    readOnly = false,
    workflowSteps,
    setWorkflowSteps,
    setStepToDelete,
    setStepTaskCount,
    setTargetStepForMigration,
    setStepDeleteOpen,
    toast,
  } = params;
  const isNewWorkflow = params.isNewWorkflow ?? workflow.id.startsWith(TEMP_WORKFLOW_PREFIX);
  return async (stepId: string) => {
    if (readOnly) return;
    const proposedSteps = workflowSteps
      .filter((step) => step.id !== stepId)
      .map((step, position) => ({ ...step, position }));
    if (isNewWorkflow) {
      setWorkflowSteps(proposedSteps);
      return;
    }
    await guardMutation({
      proposedSteps,
      operation: async () => {
        if (stepId.startsWith("temp-")) {
          setWorkflowSteps(proposedSteps);
          return;
        }
        let taskCount: number;
        try {
          ({ task_count: taskCount } = await getStepTaskCount(stepId));
        } catch (error) {
          toast({
            title: "Failed to check workflow step tasks",
            description: error instanceof Error ? error.message : FALLBACK_ERROR_MESSAGE,
            variant: "error",
          });
          return;
        }
        openStepDeleteDialog({
          stepId,
          taskCount,
          workflowSteps,
          setStepToDelete,
          setStepTaskCount,
          setTargetStepForMigration,
          setStepDeleteOpen,
        });
      },
    });
  };
}

export function useWorkflowStepActions(params: WorkflowStepActionsParams) {
  const { workflow, workflowSteps, setWorkflowSteps, toast, mutationGuard } = params;
  const isNewWorkflow = params.isNewWorkflow ?? workflow.id.startsWith(TEMP_WORKFLOW_PREFIX);
  const readOnly = params.readOnly ?? false;
  const mutationQueue = useWorkflowStepMutationQueue(toast);
  const runMutation = mutationQueue.run;
  const guardMutation: WorkflowMutationGuardController["guardMutation"] =
    mutationGuard?.guardMutation ?? (async ({ operation }) => operation());

  const handleUpdateWorkflowStep = async (stepId: string, updates: Partial<WorkflowStep>) => {
    if (readOnly) return;
    const proposedSteps = applyWorkflowStepUpdates(workflowSteps, stepId, updates);
    if (isNewWorkflow) {
      setWorkflowSteps(() => proposedSteps);
      return;
    }
    await guardMutation({
      proposedSteps,
      operation: async () => setWorkflowSteps(() => proposedSteps),
    });
  };

  const handleAddWorkflowStep = async () => {
    if (readOnly) return;
    const draftStep = createDraftStep(workflow, workflowSteps.length);
    const proposedSteps = [...workflowSteps, draftStep];
    if (isNewWorkflow) {
      setWorkflowSteps((previous) => [...previous, draftStep]);
      return;
    }
    await guardMutation({
      proposedSteps,
      operation: async () => setWorkflowSteps((previous) => [...previous, draftStep]),
    });
  };

  const handleRemoveWorkflowStep = createRemoveWorkflowStepHandler(params, guardMutation);

  const handleReorderWorkflowSteps = async (reorderedSteps: WorkflowStep[]) => {
    if (readOnly) return;
    const proposedSteps = reorderedSteps.map((step, position) => ({ ...step, position }));
    if (isNewWorkflow) {
      setWorkflowSteps(proposedSteps);
      return;
    }
    await guardMutation({
      proposedSteps,
      operation: async () => setWorkflowSteps(proposedSteps),
    });
  };

  return {
    handleUpdateWorkflowStep,
    handleAddWorkflowStep,
    handleRemoveWorkflowStep,
    handleReorderWorkflowSteps,
    status: mutationQueue.status,
    retry: mutationQueue.retry,
    runMutation,
  };
}

export type WorkflowDraftSaveProgress = {
  workflow?: Workflow;
  stepIds: Map<string, string>;
  templateStepsLoaded?: boolean;
};

export function createWorkflowDraftSaveProgress(): WorkflowDraftSaveProgress {
  return { stepIds: new Map() };
}

type PersistWorkflowDraftParams = {
  workflow: Workflow;
  draftSteps: WorkflowStep[];
  savedSteps: WorkflowStep[];
  progress: WorkflowDraftSaveProgress;
};

export async function persistWorkflowDraft({
  workflow,
  draftSteps,
  savedSteps,
  progress,
}: PersistWorkflowDraftParams): Promise<{ workflow: Workflow; steps: WorkflowStep[] }> {
  const isNewWorkflow = workflow.id.startsWith(TEMP_WORKFLOW_PREFIX);
  const persistedWorkflow = await ensurePersistedWorkflow(workflow, progress);
  const updatedWorkflow = await updateWorkflowAction(persistedWorkflow.id, {
    name: workflow.name.trim(),
    description: workflow.description ?? "",
    agent_profile_id: workflow.agent_profile_id ?? "",
  });
  progress.workflow = updatedWorkflow;
  await reconcileTemplateSteps({ workflow, draftSteps, updatedWorkflow, progress, isNewWorkflow });
  await createMissingSteps(updatedWorkflow.id, draftSteps, progress.stepIds);
  const remappedSteps = remapWorkflowDraftSteps(draftSteps, updatedWorkflow.id, progress.stepIds);
  await updateChangedSteps(remappedSteps, savedSteps);
  if (remappedSteps.length > 0) {
    await reorderWorkflowStepsAction(
      updatedWorkflow.id,
      remappedSteps.map((step) => step.id),
    );
  }
  return { workflow: updatedWorkflow, steps: remappedSteps };
}

async function ensurePersistedWorkflow(
  workflow: Workflow,
  progress: WorkflowDraftSaveProgress,
): Promise<Workflow> {
  if (progress.workflow) return progress.workflow;
  const persisted = workflow.id.startsWith(TEMP_WORKFLOW_PREFIX)
    ? await createWorkflowAction({
        workspace_id: workflow.workspace_id,
        name: workflow.name.trim(),
        description: workflow.description ?? undefined,
        workflow_template_id: workflow.workflow_template_id ?? undefined,
      })
    : workflow;
  progress.workflow = persisted;
  return persisted;
}

async function reconcileTemplateSteps({
  workflow,
  draftSteps,
  updatedWorkflow,
  progress,
  isNewWorkflow,
}: {
  workflow: Workflow;
  draftSteps: WorkflowStep[];
  updatedWorkflow: Workflow;
  progress: WorkflowDraftSaveProgress;
  isNewWorkflow: boolean;
}) {
  if (!isNewWorkflow || !workflow.workflow_template_id || progress.templateStepsLoaded) return;
  const templateSteps = (await listWorkflowStepsAction(updatedWorkflow.id)).steps ?? [];
  mapTemplateStepIds(workflow.id, draftSteps, templateSteps, progress.stepIds);
  const keptServerIds = new Set(progress.stepIds.values());
  for (const serverStep of templateSteps) {
    if (!keptServerIds.has(serverStep.id)) await deleteWorkflowStepAction(serverStep.id);
  }
  progress.templateStepsLoaded = true;
}

async function createMissingSteps(
  workflowId: string,
  draftSteps: WorkflowStep[],
  mappings: Map<string, string>,
) {
  for (const step of draftSteps) {
    if (!step.id.startsWith("temp-") || mappings.has(step.id)) continue;
    const created = await createWorkflowStepAction({
      workflow_id: workflowId,
      name: step.name,
      position: step.position,
      color: step.color,
    });
    mappings.set(step.id, created.id);
  }
}

async function updateChangedSteps(remapped: WorkflowStep[], savedSteps: WorkflowStep[]) {
  const savedById = new Map(savedSteps.map((step) => [step.id, step]));
  for (const step of remapped) {
    const saved = savedById.get(step.id);
    if (!saved || !areStepDraftsEqual(step, saved)) {
      await updateWorkflowStepAction(step.id, stepUpdatePayload(step));
    }
  }
}

function mapTemplateStepIds(
  clientWorkflowId: string,
  draftSteps: WorkflowStep[],
  serverSteps: WorkflowStep[],
  mappings: Map<string, string>,
) {
  const serverByPosition = new Map(serverSteps.map((step) => [step.position, step.id]));
  const prefix = `temp-template-step-${clientWorkflowId}-`;
  for (const step of draftSteps) {
    if (!step.id.startsWith(prefix)) continue;
    const originalPosition = Number(step.id.slice(prefix.length));
    const serverId = serverByPosition.get(originalPosition);
    if (serverId) mappings.set(step.id, serverId);
  }
}

function remapDraftStep(
  step: WorkflowStep,
  persistedWorkflowId: string,
  position: number,
  mappings: Map<string, string>,
): WorkflowStep {
  const remapId = (id: string | null | undefined) => (id ? (mappings.get(id) ?? id) : id);
  return {
    ...step,
    id: mappings.get(step.id) ?? step.id,
    workflow_id: persistedWorkflowId as WorkflowStep["workflow_id"],
    position,
    pull_from_step_id: remapId(step.pull_from_step_id),
    events: remapStepReferences(step.events, mappings),
  };
}

export function remapWorkflowDraftSteps(
  steps: WorkflowStep[],
  persistedWorkflowId: string,
  mappings: Map<string, string>,
): WorkflowStep[] {
  return steps.map((step, position) =>
    remapDraftStep(step, persistedWorkflowId, position, mappings),
  );
}

function remapStepReferences<T>(value: T, mappings: Map<string, string>): T {
  if (Array.isArray(value)) return value.map((item) => remapStepReferences(item, mappings)) as T;
  if (!value || typeof value !== "object") return value;
  const mapped = Object.entries(value).map(([key, item]) => [
    key,
    key === "step_id" && typeof item === "string"
      ? (mappings.get(item) ?? item)
      : remapStepReferences(item, mappings),
  ]);
  return Object.fromEntries(mapped) as T;
}

function stepUpdatePayload(step: WorkflowStep): Partial<WorkflowStep> {
  return {
    name: step.name,
    position: step.position,
    color: step.color,
    prompt: step.prompt ?? "",
    events: step.events ?? {},
    allow_manual_move: step.allow_manual_move ?? true,
    is_start_step: step.is_start_step ?? false,
    show_in_command_panel: step.show_in_command_panel ?? false,
    auto_archive_after_hours: step.auto_archive_after_hours ?? 0,
    agent_profile_id: step.agent_profile_id ?? "",
    auto_advance_requires_signal: step.auto_advance_requires_signal ?? false,
    wip_limit: step.wip_limit ?? 0,
    pull_from_step_id: step.pull_from_step_id ?? "",
  };
}

export function areStepDraftsEqual(left: WorkflowStep[], right: WorkflowStep[]): boolean;
export function areStepDraftsEqual(left: WorkflowStep, right: WorkflowStep): boolean;
export function areStepDraftsEqual(
  left: WorkflowStep[] | WorkflowStep,
  right: WorkflowStep[] | WorkflowStep,
): boolean {
  if (Array.isArray(left) && Array.isArray(right)) {
    if (left.length !== right.length) return false;
    return left.every((step, index) => areStepDraftsEqual(step, right[index]));
  }
  if (Array.isArray(left) || Array.isArray(right)) return false;
  return JSON.stringify(stepUpdatePayload(left)) === JSON.stringify(stepUpdatePayload(right));
}

type WorkflowDeleteHandlersParams = {
  workflow: Workflow;
  /** See `WorkflowStepActionsParams.readOnly` — defense-in-depth for synced workflows. */
  readOnly?: boolean;
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
  readOnly = false,
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
    if (readOnly) return;
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
    // A background sync can flip the workflow read-only while the delete
    // dialog is already open; re-check at confirm time.
    if (readOnly) return;
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
    if (readOnly) return;
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
    setStepDeletePending: (v: boolean) => void;
    setStepDeleteOpen: (v: boolean) => void;
    setStepToDelete: (v: string | null) => void;
  };
  refreshWorkflowSteps: () => Promise<void>;
  runMutation: (operation: () => Promise<void>, errorTitle: string) => Promise<boolean>;
};

export function useStepDeleteHandlers({
  workflow,
  stepDel,
  refreshWorkflowSteps,
  runMutation,
}: StepDeleteHandlersParams) {
  const deletePendingRef = useRef(false);

  const runStepDelete = async (operation: () => Promise<void>, errorTitle: string) => {
    if (deletePendingRef.current) return;
    let mutationCompleted = false;
    deletePendingRef.current = true;
    stepDel.setStepDeletePending(true);
    try {
      const saved = await runMutation(async () => {
        stepDel.setStepMigrateLoading(true);
        try {
          if (!mutationCompleted) {
            await operation();
            mutationCompleted = true;
          }
          await refreshWorkflowSteps();
          stepDel.setStepToDelete(null);
          stepDel.setStepDeleteOpen(false);
        } finally {
          stepDel.setStepMigrateLoading(false);
        }
      }, errorTitle);
      if (!saved) {
        stepDel.setStepToDelete(null);
        stepDel.setStepDeleteOpen(false);
      }
    } finally {
      deletePendingRef.current = false;
      stepDel.setStepDeletePending(false);
    }
  };

  const handleMigrateAndDeleteStep = async () => {
    if (!stepDel.stepToDelete || !stepDel.targetStepForMigration) return;
    await runStepDelete(async () => {
      await bulkMoveTasks({
        source_workflow_id: workflow.id,
        source_step_id: stepDel.stepToDelete!,
        target_workflow_id: workflow.id,
        target_step_id: stepDel.targetStepForMigration,
      });
      await deleteWorkflowStepAction(stepDel.stepToDelete!);
    }, "Failed to migrate tasks");
  };

  const handleDeleteStepAndTasks = async () => {
    if (!stepDel.stepToDelete) return;
    await runStepDelete(
      () => deleteWorkflowStepAction(stepDel.stepToDelete!),
      "Failed to delete step",
    );
  };

  return { handleMigrateAndDeleteStep, handleDeleteStepAndTasks };
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
