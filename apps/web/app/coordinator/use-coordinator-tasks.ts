"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import { useAppStore } from "@/components/state-provider";
import { useAllWorkflowSnapshots } from "@/hooks/domains/kanban/use-all-workflow-snapshots";
import type { AttentionTask } from "@/lib/coordinator/attention";
import type { AppState } from "@/lib/state/store";

export type UseCoordinatorTasksResult = {
  /** Every open task of the workspace, across every loaded workflow (Adoption decision 2). */
  tasks: AttentionTask[];
  /** The name of each task's current step, keyed by task id. Absent for an unknown step id (Adoption decision 3). */
  stepNameByTaskId: Map<string, string>;
  /** The name of each loaded workflow of the workspace, keyed by workflow id (for a proposal's target). */
  workflowNameById: Map<string, string>;
  /** The name of a workflow's step, keyed by `${workflowId}:${stepId}` (for a proposal's target). */
  stepNameByWorkflowStep: Map<string, string>;
  /** Set while the workspace's tasks failed to load (docs/specs/coordinator/system-design/needs-you.md#failure-and-recovery). */
  error: boolean;
  /** The time the screen last saw a successful tasks read complete. Undefined before the first success. */
  loadedAt: number | undefined;
  /** Re-fetches only the workflows that failed to load. */
  retry: () => void;
};

function workflowStepKey(workflowId: string, stepId: string): string {
  return `${workflowId}:${stepId}`;
}

function matchesWorkspace(
  read: AppState["workspaceContextRead"],
  workspaceId: string | null,
): boolean {
  return read?.workspaceId === workspaceId;
}

/**
 * Flattens `kanbanMulti.snapshots` for a workspace's tasks input (docs/specs/
 * coordinator/system-design/needs-you.md#inputs, #failure-and-recovery).
 * Never fetches directly: `useAllWorkflowSnapshots` owns that, keyed by the
 * always-mounted `state.workflows.items` for the workspace.
 */
// eslint-disable-next-line max-lines-per-function -- one hook owns snapshot flattening, error/load-time tracking, and retry
export function useCoordinatorTasks(workspaceId: string | null): UseCoordinatorTasksResult {
  useAllWorkflowSnapshots(workspaceId);
  const requestWorkspaceContextRefresh = useAppStore(
    (state) => state.requestWorkspaceContextRefresh,
  );

  const workflows = useAppStore((state) => state.workflows.items);
  const snapshots = useAppStore((state) => state.kanbanMulti.snapshots);
  const workspaceContextRead = useAppStore((state) => state.workspaceContextRead);

  const workspaceWorkflowIds = useMemo(
    () =>
      new Set(
        workflows.filter((workflow) => workflow.workspaceId === workspaceId).map((w) => w.id),
      ),
    [workflows, workspaceId],
  );

  const { tasks, stepNameByTaskId, workflowNameById, stepNameByWorkflowStep } = useMemo(() => {
    const flattened: AttentionTask[] = [];
    const stepNames = new Map<string, string>();
    const workflowNames = new Map<string, string>();
    const workflowStepNames = new Map<string, string>();
    for (const workflowId of workspaceWorkflowIds) {
      const snapshot = snapshots[workflowId];
      if (!snapshot) continue;
      const workflow = workflows.find((w) => w.id === workflowId);
      if (workflow) workflowNames.set(workflowId, workflow.name);
      const stepNameById = new Map(snapshot.steps.map((step) => [step.id, step.title]));
      for (const [stepId, title] of stepNameById) {
        workflowStepNames.set(workflowStepKey(workflowId, stepId), title);
      }
      for (const task of snapshot.tasks) {
        flattened.push({
          id: task.id,
          title: task.title,
          identifier: task.identifier,
          state: task.state,
          workflowStepId: task.workflowStepId,
          isArchived: task.isArchived,
          updatedAt: task.updatedAt,
          statusSummary: task.statusSummary,
        });
        const stepName = task.workflowStepId ? stepNameById.get(task.workflowStepId) : undefined;
        if (stepName) stepNames.set(task.id, stepName);
      }
    }
    return {
      tasks: flattened,
      stepNameByTaskId: stepNames,
      workflowNameById: workflowNames,
      stepNameByWorkflowStep: workflowStepNames,
    };
  }, [snapshots, workspaceWorkflowIds, workflows]);

  const matches = matchesWorkspace(workspaceContextRead, workspaceId);
  const error = matches && workspaceContextRead.snapshotError !== null;
  const pending = matches && workspaceContextRead.snapshotPending;
  const requestId = matches ? workspaceContextRead.snapshotRequestId : null;

  const [loadedAt, setLoadedAt] = useState<number | undefined>(undefined);
  const lastRecordedRequestIdRef = useRef<string | null>(null);

  useEffect(() => {
    if (!matches || pending || error || requestId === null) return;
    if (lastRecordedRequestIdRef.current === requestId) return;
    lastRecordedRequestIdRef.current = requestId;
    setLoadedAt(Date.now());
  }, [matches, pending, error, requestId]);

  useEffect(() => {
    if (!matches) {
      lastRecordedRequestIdRef.current = null;
      setLoadedAt(undefined);
    }
  }, [matches]);

  return {
    tasks,
    stepNameByTaskId,
    workflowNameById,
    stepNameByWorkflowStep,
    error,
    loadedAt,
    retry: () => {
      requestWorkspaceContextRefresh?.();
    },
  };
}
