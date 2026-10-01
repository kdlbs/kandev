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
  /** The time this input first completed successfully for the current workspace. Undefined before the first success. */
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

/** Every repository of a task, from its repository list or its primary repository id; `[]` is none. */
export function taskRepositoryIds(task: {
  repositories?: ReadonlyArray<{ repository_id: string }>;
  repositoryId?: string;
}): string[] {
  if (task.repositories && task.repositories.length > 0) {
    return task.repositories.map((repository) => repository.repository_id);
  }
  return task.repositoryId ? [task.repositoryId] : [];
}

function toAttentionTask(task: Task, workflowId: string): AttentionTask {
  return {
    id: task.id,
    workflowId,
    repositoryIds: taskRepositoryIds(task),
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
          workflowId,
          repositoryIds: taskRepositoryIds(task),
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

  const [loadedAt, setLoadedAt] = useState<number | undefined>(undefined);
  // Tracks the workspace id `loadedAt` was last recorded for, not a request
  // id: a boot-hydrated snapshot resolves without ever setting
  // `snapshotPending`/`snapshotRequestId` (`useAllWorkflowSnapshots` skips
  // the fetch entirely when every workflow's snapshot is already
  // boot-hydrated), and `snapshotRequestId` is nulled back out by the shared
  // slice once a live fetch succeeds (`nextWorkspaceContextRequestId`), so
  // neither can serve as a "just completed" signal. Recording once per
  // workspace id the first time the read settles into `matches && !pending
  // && !error` covers both the boot-hydrated and the live-fetch case.
  const loadedWorkspaceIdRef = useRef<string | null>(null);

  useEffect(() => {
    if (!matches) {
      if (loadedWorkspaceIdRef.current !== null) {
        loadedWorkspaceIdRef.current = null;
        setLoadedAt(undefined);
      }
      return;
    }
    if (pending || error) return;
    if (loadedWorkspaceIdRef.current === workspaceId) return;
    loadedWorkspaceIdRef.current = workspaceId;
    setLoadedAt(Date.now());
  }, [matches, pending, error, workspaceId]);

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
  // Settled independently, not `Promise.all`: the tasks input's documented
  // failure contract (docs/specs/coordinator/system-design/needs-you.md
  // #failure-and-recovery) requires a partial failure to still show the
  // workflows that loaded, matching `useAllWorkflowSnapshots`'s per-workflow
  // independence on the active-cache path.
  const settled = await Promise.allSettled(
    workflows.map(async (workflow) => ({
      workflow,
      snapshot: await fetchWorkflowSnapshot(workflow.id, { cache: "no-store" }),
    })),
  );

  const tasks: AttentionTask[] = [];
  const stepNameByTaskId = new Map<string, string>();
  const workflowNameById = new Map<string, string>();
  const stepNameByWorkflowStep = new Map<string, string>();
  let error = false;
  for (const outcome of settled) {
    if (outcome.status === "rejected") {
      error = true;
      continue;
    }
    const { workflow, snapshot } = outcome.value;
    workflowNameById.set(workflow.id, workflow.name);
    const stepNameById = new Map(snapshot.steps.map((step) => [step.id, step.name]));
    for (const [stepId, name] of stepNameById) {
      stepNameByWorkflowStep.set(workflowStepKey(workflow.id, stepId), name);
    }
    for (const task of snapshot.tasks) {
      if (task.is_ephemeral) continue;
      const stepName = stepNameById.get(task.workflow_step_id);
      if (!stepName) continue;
      tasks.push(toAttentionTask(task, workflow.id));
      stepNameByTaskId.set(task.id, stepName);
    }
  }
  return {
    tasks,
    stepNameByTaskId,
    workflowNameById,
    stepNameByWorkflowStep,
    error,
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
  const previousWorkspaceIdRef = useRef(workspaceId);

  useEffect(() => {
    // Reset only on a genuine `workspaceId` transition, not just when it goes
    // null: this effect's own instance can persist across a route change from
    // one non-active workspace straight to another (no `key` remounts
    // `CoordinatorRoute`), so a stale prior workspace's tasks must never
    // survive into the next one, even transiently (mirrors
    // `use-coordinator-list.ts`'s reset-before-fetch pattern). The effect also
    // re-runs on a `retryNonce`-only change (a same-workspace `retry()`),
    // which must NOT reset: that would discard a prior successful load before
    // the retried fetch even starts, so a retry whose own fetch then fails
    // would lose data the failure/recovery contract requires it to keep.
    const workspaceChanged = previousWorkspaceIdRef.current !== workspaceId;
    previousWorkspaceIdRef.current = workspaceId;
    if (workspaceChanged) setState(emptyDirectState());
    if (!workspaceId) return;
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
