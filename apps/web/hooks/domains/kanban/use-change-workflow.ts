"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import type { AgentProfileOption } from "@/lib/state/slices";
import type { ExecutorProfile, Task, Workflow, WorkflowStepDTO } from "@/lib/types/http";
import { useAppStore } from "@/components/state-provider";
import { useAgentProfileOptions } from "@/components/task-create-dialog-options";
import {
  buildWorkflowAgentOverrideRows,
  type WorkflowAgentOverrideRow,
} from "@/components/task-create-dialog-workflow-agent-overrides";
import { useCompatibleAgentProfiles } from "@/hooks/domains/session/use-compatible-agent-profiles";
import {
  useChangeWorkflowCatalog,
  useChangeWorkflowDestination,
  useChangeWorkflowProfiles,
  useChangeWorkflowTask,
  type ChangeWorkflowLoadStatus,
} from "./use-change-workflow-data";
import { useChangeWorkflowSubmit } from "./use-change-workflow-submit";
import { buildWorkflowChangePayload } from "./use-change-workflow-utils";

const NO_SOURCE_STEPS: never[] = [];

export {
  buildWorkflowChangePayload,
  normalizeChangeWorkflowOverrides,
  taskMatchesWorkflowChange,
} from "./use-change-workflow-utils";
export type { ChangeWorkflowLoadStatus } from "./use-change-workflow-data";

function fixedProfileSteps(steps: readonly WorkflowStepDTO[]) {
  return steps
    .filter((step) => step.agent_profile_id || step.session_target)
    .map((step) => ({
      id: step.id,
      title: step.name,
      position: step.position,
      agent_profile_id: step.agent_profile_id,
      session_target: step.session_target,
    }));
}

function profileForTaskExecutor(
  task: Task | null,
  workspaceDefaultExecutorId: string | null | undefined,
  executors: Awaited<ReturnType<typeof import("@/lib/api").listExecutors>>["executors"],
): ExecutorProfile | null {
  if (!task) return null;
  const profiles = executors.flatMap((executor) => executor.profiles ?? []);
  if (task.primary_executor_profile_id) {
    return profiles.find((profile) => profile.id === task.primary_executor_profile_id) ?? null;
  }
  const executor = executors.find((item) => item.id === workspaceDefaultExecutorId);
  return executor?.profiles?.[0] ?? null;
}

function buildOverrideRows(
  steps: readonly WorkflowStepDTO[],
  profiles: readonly AgentProfileOption[],
  options: ReturnType<typeof useAgentProfileOptions>,
  overrides: Readonly<Record<string, string>>,
): WorkflowAgentOverrideRow[] {
  const rows = buildWorkflowAgentOverrideRows({
    steps: fixedProfileSteps(steps),
    profiles,
    replacementOptions: options,
    overrides,
  });
  const selectableIds = new Set(options.map((option) => option.value));
  return rows.map((row) => {
    const selectedProfileId = overrides[row.sourceProfileId] || row.sourceProfileId;
    return {
      ...row,
      replacementProfileId: selectedProfileId,
      replacementLabel:
        profiles.find((profile) => profile.id === selectedProfileId)?.label ?? selectedProfileId,
      replacementAvailable: selectableIds.has(selectedProfileId),
    };
  });
}

function useChangeWorkflowDraftState({
  task,
  workflows,
  profiles,
  executors,
  workspaceId,
  selectedWorkflowId,
  selectedStepId,
  overrides,
  snapshotSteps,
  taskStatus,
  profilesStatus,
  workflowsStatus,
  snapshotStatus,
  open,
}: {
  task: Task | null;
  workflows: Workflow[];
  profiles: AgentProfileOption[];
  executors: Awaited<ReturnType<typeof import("@/lib/api").listExecutors>>["executors"];
  workspaceId: string | null;
  selectedWorkflowId: string;
  selectedStepId: string;
  overrides: Record<string, string>;
  snapshotSteps: WorkflowStepDTO[];
  taskStatus: ChangeWorkflowLoadStatus;
  profilesStatus: ChangeWorkflowLoadStatus;
  workflowsStatus: ChangeWorkflowLoadStatus;
  snapshotStatus: ChangeWorkflowLoadStatus;
  open: boolean;
}) {
  const workspace = useAppStore((state) =>
    state.workspaces.items.find((item) => item.id === workspaceId),
  );
  const selectedWorkflow = workflows.find((workflow) => workflow.id === selectedWorkflowId) ?? null;
  const executorProfile = profileForTaskExecutor(task, workspace?.default_executor_id, executors);
  const compatibleProfiles = useCompatibleAgentProfiles(profiles, executorProfile);
  const profileOptions = useAgentProfileOptions(compatibleProfiles);
  const rows = useMemo(
    () => buildOverrideRows(snapshotSteps, profiles, profileOptions, overrides),
    [snapshotSteps, profiles, profileOptions, overrides],
  );
  const workflowChange = useMemo(
    () => (task ? buildWorkflowChangePayload(task, overrides) : undefined),
    [task, overrides],
  );
  const sourceSteps = useAppStore((state) => {
    if (!task) return NO_SOURCE_STEPS;
    if (state.kanban.workflowId === task.workflow_id) return state.kanban.steps;
    return state.kanbanMulti.snapshots[task.workflow_id]?.steps ?? NO_SOURCE_STEPS;
  });
  const sourceWorkflowName = workflows.find((workflow) => workflow.id === task?.workflow_id)?.name;
  const sourceStepName = sourceSteps.find((step) => step.id === task?.workflow_step_id)?.title;
  const destinations = workflows.filter((workflow) => workflow.id !== task?.workflow_id);
  const selectableIds = new Set(profileOptions.map((option) => option.value));
  const invalidRows =
    profilesStatus === "success"
      ? rows.filter((row) => !selectableIds.has(row.replacementProfileId))
      : [];
  const canSubmit = Boolean(
    open &&
    task &&
    taskStatus === "success" &&
    workflowsStatus === "success" &&
    profilesStatus === "success" &&
    snapshotStatus === "success" &&
    selectedWorkflow &&
    selectedWorkflow.id !== task.workflow_id &&
    snapshotSteps.some((step) => step.id === selectedStepId) &&
    invalidRows.length === 0,
  );
  return {
    selectedWorkflow,
    destinations,
    profileOptions,
    rows,
    workflowChange,
    sourceWorkflowName,
    sourceStepName,
    canSubmit,
  };
}

export function useChangeWorkflow({
  open,
  taskId,
  workspaceId,
  onOpenChange,
  onSuccess,
}: {
  open: boolean;
  taskId: string | null;
  workspaceId: string | null;
  onOpenChange: (open: boolean) => void;
  onSuccess?: () => void;
}) {
  const taskData = useChangeWorkflowTask({ open, taskId, workspaceId });
  const workflowData = useChangeWorkflowCatalog(open, workspaceId);
  const profileData = useChangeWorkflowProfiles(open, workspaceId);
  const destination = useChangeWorkflowDestination(open);
  const [sourceChanged, setSourceChanged] = useState(false);
  const draft = useChangeWorkflowDraftState({
    task: taskData.task,
    workflows: workflowData.workflows,
    profiles: profileData.profiles,
    executors: profileData.executors,
    workspaceId,
    selectedWorkflowId: destination.selectedWorkflowId,
    selectedStepId: destination.selectedStepId,
    overrides: destination.overrides,
    snapshotSteps: destination.snapshot?.steps ?? [],
    taskStatus: taskData.status,
    profilesStatus: profileData.status,
    workflowsStatus: workflowData.status,
    snapshotStatus: destination.status,
    open,
  });
  const refreshTask = useCallback(async () => {
    const refreshed = await taskData.refreshTask();
    if (refreshed) setSourceChanged(true);
    return refreshed;
  }, [taskData.refreshTask]);
  const submitState = useChangeWorkflowSubmit({
    open,
    canSubmit: draft.canSubmit,
    task: taskData.task,
    selectedWorkflow: draft.selectedWorkflow,
    selectedStepId: destination.selectedStepId,
    workflowChange: draft.workflowChange,
    refreshTask,
    onOpenChange,
    onSuccess,
  });

  const setOverride = useCallback(
    (sourceId: string, replacementId: string) => {
      destination.setOverride(sourceId, replacementId);
      submitState.clearSubmitError();
    },
    [destination.setOverride, submitState.clearSubmitError],
  );
  const setSelectedStepId = useCallback(
    (stepId: string) => {
      destination.chooseStep(stepId);
      submitState.clearSubmitError();
    },
    [destination.chooseStep, submitState.clearSubmitError],
  );
  const changeWorkflow = useCallback(
    (workflowId: string) => {
      destination.changeWorkflow(workflowId);
      submitState.clearSubmitError();
    },
    [destination.changeWorkflow, submitState.clearSubmitError],
  );

  useEffect(() => {
    if (open) setSourceChanged(false);
  }, [open, taskId]);

  return createChangeWorkflowViewModel({
    taskData,
    workflowData,
    profileData,
    destination,
    sourceChanged,
    draft,
    submitState,
    setOverride,
    setSelectedStepId,
    changeWorkflow,
  });
}

function createChangeWorkflowViewModel({
  taskData,
  workflowData,
  profileData,
  destination,
  sourceChanged,
  draft,
  submitState,
  setOverride,
  setSelectedStepId,
  changeWorkflow,
}: {
  taskData: ReturnType<typeof useChangeWorkflowTask>;
  workflowData: ReturnType<typeof useChangeWorkflowCatalog>;
  profileData: ReturnType<typeof useChangeWorkflowProfiles>;
  destination: ReturnType<typeof useChangeWorkflowDestination>;
  sourceChanged: boolean;
  draft: ReturnType<typeof useChangeWorkflowDraftState>;
  submitState: ReturnType<typeof useChangeWorkflowSubmit>;
  setOverride: (sourceId: string, replacementId: string) => void;
  setSelectedStepId: (stepId: string) => void;
  changeWorkflow: (workflowId: string) => void;
}) {
  return {
    task: taskData.task,
    taskStatus: taskData.status,
    taskError: taskData.error,
    workflows: workflowData.workflows,
    workflowsStatus: workflowData.status,
    workflowsError: workflowData.error,
    destinations: draft.destinations,
    selectedWorkflowId: destination.selectedWorkflowId,
    selectedWorkflow: draft.selectedWorkflow,
    changeWorkflow,
    snapshot: destination.snapshot,
    snapshotStatus: destination.status,
    snapshotError: destination.error,
    retrySnapshot: destination.retry,
    profiles: profileData.profiles,
    profilesStatus: profileData.status,
    profilesError: profileData.error,
    profileOptions: draft.profileOptions,
    rows: draft.rows,
    setOverride,
    overrides: destination.overrides,
    selectedStepId: destination.selectedStepId,
    setSelectedStepId,
    sourceWorkflowName: draft.sourceWorkflowName,
    sourceStepName: draft.sourceStepName,
    sourceChanged,
    uncertainResult: submitState.uncertainResult,
    submitError: submitState.submitError,
    sourceProfileErrorId: submitState.sourceProfileErrorId,
    isSubmitting: submitState.isSubmitting,
    canSubmit: draft.canSubmit && !submitState.isSubmitting && !submitState.uncertainResult,
    submit: submitState.submit,
    retryTask: submitState.retryAfterRefresh,
    retryWorkflows: workflowData.retry,
    retryProfiles: profileData.retry,
    workflowChange: draft.workflowChange,
  };
}
