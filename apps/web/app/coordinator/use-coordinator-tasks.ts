"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import { useAppStore } from "@/components/state-provider";
import { useAllWorkflowSnapshots } from "@/hooks/domains/kanban/use-all-workflow-snapshots";
import { fetchWorkflowSnapshot, listWorkflows } from "@/lib/api/domains/kanban-api";
import type { AttentionTask } from "@/lib/coordinator/attention";
import type { AppState } from "@/lib/state/store";
import type { Task } from "@/lib/types/http";

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
  /** Re-fetches this input. */
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

function toAttentionTask(task: Task): AttentionTask {
  return {
    id: task.id,
    title: task.title,
    identifier: task.identifier,
    state: task.state,
    workflowStepId: task.workflow_step_id,
    isArchived: task.archived_at != null,
    updatedAt: task.updated_at,
    statusSummary: task.status_summary,
  };
}

/**
 * Reads tasks from the board's shared cache (`state.workflows.items`,
 * `kanbanMulti.snapshots`), kept current by `task.status_summary.updated` and
 * the task WebSocket handlers (docs/specs/coordinator/system-design/
 * needs-you.md#inputs). That cache only ever describes whichever workspace is
 * globally active (`useEnsureWorkspaceWorkflows`, mounted once in the
 * sidebar), so callers must only use this when `workspaceId` already is the
 * active workspace — pass `null` otherwise so it stays an inert no-op.
 */
// eslint-disable-next-line max-lines-per-function -- one hook owns snapshot flattening, error/load-time tracking, and retry
function useCoordinatorTasksFromActiveCache(workspaceId: string | null): UseCoordinatorTasksResult {
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

type DirectTasksState = {
  tasks: AttentionTask[];
  stepNameByTaskId: Map<string, string>;
  workflowNameById: Map<string, string>;
  stepNameByWorkflowStep: Map<string, string>;
  error: boolean;
  loadedAt: number | undefined;
};

function emptyDirectState(): DirectTasksState {
  return {
    tasks: [],
    stepNameByTaskId: new Map(),
    workflowNameById: new Map(),
    stepNameByWorkflowStep: new Map(),
    error: false,
    loadedAt: undefined,
  };
}

async function fetchDirectTasksState(workspaceId: string): Promise<DirectTasksState> {
  const { workflows } = await listWorkflows(workspaceId);
  const loaded = await Promise.all(
    workflows.map(async (workflow) => ({
      workflow,
      snapshot: await fetchWorkflowSnapshot(workflow.id, { cache: "no-store" }),
    })),
  );

  const tasks: AttentionTask[] = [];
  const stepNameByTaskId = new Map<string, string>();
  const workflowNameById = new Map<string, string>();
  const stepNameByWorkflowStep = new Map<string, string>();
  for (const { workflow, snapshot } of loaded) {
    workflowNameById.set(workflow.id, workflow.name);
    const stepNameById = new Map(snapshot.steps.map((step) => [step.id, step.name]));
    for (const [stepId, name] of stepNameById) {
      stepNameByWorkflowStep.set(workflowStepKey(workflow.id, stepId), name);
    }
    for (const task of snapshot.tasks) {
      if (task.is_ephemeral) continue;
      const stepName = stepNameById.get(task.workflow_step_id);
      if (!stepName) continue;
      tasks.push(toAttentionTask(task));
      stepNameByTaskId.set(task.id, stepName);
    }
  }
  return {
    tasks,
    stepNameByTaskId,
    workflowNameById,
    stepNameByWorkflowStep,
    error: false,
    loadedAt: Date.now(),
  };
}

/**
 * Self-contained fetch for a workspace that is NOT the globally active one
 * (a coordinator deep-link or cold load into another workspace). Reads
 * `listWorkflows`/`fetchWorkflowSnapshot` directly into local hook state
 * instead of the shared, single-active-workspace `workflows.items` /
 * `kanbanMulti.snapshots` caches: writing this workspace's data into those
 * would either be invisible (filtered out because `workflows.items` only
 * ever holds the active workspace's rows) or overwrite whatever the active
 * workspace's own board view is showing (build round 4's fixed class of bug).
 * Trade-off: no live WebSocket updates on this path, only mount and `retry()`
 * — acceptable because it only applies to the non-active-workspace edge case;
 * the common case (route workspace already active) stays on
 * `useCoordinatorTasksFromActiveCache` and keeps full WS liveness.
 */
function useCoordinatorTasksDirect(workspaceId: string | null): UseCoordinatorTasksResult {
  const [state, setState] = useState<DirectTasksState>(emptyDirectState);
  const requestRef = useRef(0);
  const [retryNonce, setRetryNonce] = useState(0);

  useEffect(() => {
    if (!workspaceId) {
      setState(emptyDirectState());
      return;
    }
    const requestId = ++requestRef.current;
    fetchDirectTasksState(workspaceId)
      .then((next) => {
        if (requestRef.current !== requestId) return;
        setState(next);
      })
      .catch(() => {
        if (requestRef.current !== requestId) return;
        setState((prev) => ({ ...prev, error: true }));
      });
  }, [workspaceId, retryNonce]);

  return {
    tasks: state.tasks,
    stepNameByTaskId: state.stepNameByTaskId,
    workflowNameById: state.workflowNameById,
    stepNameByWorkflowStep: state.stepNameByWorkflowStep,
    error: state.error,
    loadedAt: state.loadedAt,
    retry: () => setRetryNonce((n) => n + 1),
  };
}

/**
 * Tasks input for the Needs you / Queue screens, for the route's own
 * `workspaceId` regardless of which workspace is globally active
 * (docs/specs/coordinator/system-design/needs-you.md#inputs,
 * #failure-and-recovery). See `useCoordinatorTasksDirect` for why the
 * non-active-workspace path cannot simply read the shared board cache.
 */
export function useCoordinatorTasks(workspaceId: string | null): UseCoordinatorTasksResult {
  const activeId = useAppStore((state) => state.workspaces.activeId);
  const isActiveWorkspace = workspaceId !== null && workspaceId === activeId;

  const liveResult = useCoordinatorTasksFromActiveCache(isActiveWorkspace ? workspaceId : null);
  const directResult = useCoordinatorTasksDirect(isActiveWorkspace ? null : workspaceId);

  return isActiveWorkspace ? liveResult : directResult;
}
