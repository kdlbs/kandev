import { useCallback, useEffect, useRef, useState, type MutableRefObject } from "react";
import { fetchWorkflowSnapshot } from "@/lib/api";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { copyPrimaryExecutorFields, toKanbanTask } from "@/lib/kanban/map-task";
import type { KanbanState, WorkflowSnapshotData } from "@/lib/state/slices/kanban/types";
import type { Task } from "@/lib/types/http";
import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import { isCurrentWorkspaceContext } from "@/lib/state/workspace-context";
import { pickFreshestStatusSummary } from "@/lib/task-status-summary";
import { useForegroundRefresh } from "@/hooks/use-foreground-refresh";

type KanbanTask = KanbanState["tasks"][number];
type Workflow = { id: string; name: string };
type WorkspaceContextRequest = { workspaceId: string; generation: number };

function isBootHydratedSnapshot(snapshot: WorkflowSnapshotData | undefined): boolean {
  return !!snapshot && snapshot.isPlaceholder !== true;
}

function hasNewerLivePlacement(existing: KanbanTask, fetchStart: KanbanTask | undefined): boolean {
  return (
    fetchStart !== undefined &&
    (existing.workflowId !== fetchStart.workflowId ||
      existing.workflowStepId !== fetchStart.workflowStepId ||
      existing.position !== fetchStart.position)
  );
}

function hasNewerLiveStatusSummary(
  existing: KanbanTask,
  fetchStart: KanbanTask | undefined,
): boolean {
  const existingRevision = existing.statusSummary?.revision;
  if (existingRevision === undefined) return false;
  if (!fetchStart) return true;
  return existingRevision > (fetchStart.statusSummary?.revision ?? -1);
}

function hasNewerLiveAutoStartFailed(
  existing: KanbanTask,
  fetchStart: KanbanTask | undefined,
): boolean {
  if (!fetchStart) return true;
  return existing.autoStartFailed !== fetchStart.autoStartFailed;
}

function hasNewerLiveExecutor(existing: KanbanTask, fetchStart: KanbanTask | undefined): boolean {
  if (!fetchStart) return true;
  return (
    existing.primaryExecutorId !== fetchStart.primaryExecutorId ||
    existing.primaryExecutorType !== fetchStart.primaryExecutorType ||
    existing.primaryExecutorName !== fetchStart.primaryExecutorName ||
    existing.isRemoteExecutor !== fetchStart.isRemoteExecutor
  );
}

function preserveLiveExecutorBinding(
  merged: KanbanTask,
  existing: KanbanTask,
  fetchStart: KanbanTask | undefined,
  source: Task,
): void {
  // An explicit primary-session clear is authoritative, even if a newer live
  // update changed the cached executor while this snapshot was in flight.
  if (source.primary_session_id === null || !hasNewerLiveExecutor(existing, fetchStart)) return;
  copyPrimaryExecutorFields(merged, existing);
}

function mergeFetchedTask(
  mapped: KanbanTask,
  existing: KanbanTask,
  fetchStart: KanbanTask | undefined,
  source: Task,
  statusSummaryInvalidated: boolean,
): KanbanTask {
  // Production snapshot payloads can be surfaced through read-only objects.
  // Merge into a fresh task copy before applying any live-field backfills.
  const merged = { ...mapped };

  if (hasNewerLivePlacement(existing, fetchStart)) {
    // A live task.updated/task.moved event arrived after this request
    // started. Keep the newer placement instead of moving the card
    // back to the location captured by the older snapshot response.
    merged.workflowId = existing.workflowId;
    merged.workflowStepId = existing.workflowStepId;
    merged.position = existing.position;
  }

  // Multiple mounted surfaces can refresh the same workflow at once.
  // A response that started before a live WS status update may finish
  // afterwards, so do not let an older snapshot roll the status
  // projection back to a lower revision.
  if (statusSummaryInvalidated && !hasNewerLiveStatusSummary(existing, fetchStart)) {
    merged.statusSummary = undefined;
  } else {
    const existingRevision = existing.statusSummary?.revision;
    const mappedRevision = merged.statusSummary?.revision;
    if (existingRevision !== undefined && existingRevision > (mappedRevision ?? -1)) {
      merged.statusSummary = existing.statusSummary;
    }
    // An equal revision can still carry a newer queued-prompt count. Preserve
    // the freshest status projection while keeping the revision guard above.
    merged.statusSummary = pickFreshestStatusSummary(merged.statusSummary, existing.statusSummary);
  }
  merged.primarySessionId = merged.primarySessionId || existing.primarySessionId;
  merged.primarySessionState = merged.primarySessionState || existing.primarySessionState;
  // Autopilot is immutable after creation. Keep the cached value when
  // an older or partial snapshot does not include the field.
  merged.autopilot = merged.autopilot ?? existing.autopilot;
  // A task.updated event can set or clear this marker while the snapshot is
  // in flight. Preserve that newer live value instead of rolling it back.
  if (merged.autoStartFailed === undefined || hasNewerLiveAutoStartFailed(existing, fetchStart)) {
    merged.autoStartFailed = existing.autoStartFailed;
  }
  preserveLiveExecutorBinding(merged, existing, fetchStart, source);
  return merged;
}

function mergeSnapshotTasks(
  snapshotTasks: Task[],
  stepIds: Set<string>,
  snapshotAtFetchStart: WorkflowSnapshotData | undefined,
  existingSnapshot: WorkflowSnapshotData | undefined,
): KanbanTask[] {
  const taskIdsAtFetchStart = new Set((snapshotAtFetchStart?.tasks ?? []).map((t) => t.id));
  const existingById = new Map((existingSnapshot?.tasks ?? []).map((t) => [t.id, t]));
  const fetchStartById = new Map(
    (snapshotAtFetchStart?.tasks ?? []).map((task) => [task.id, task]),
  );

  const tasks = snapshotTasks
    .filter((task) => !task.is_ephemeral)
    .map((task) => {
      const mapped = mapSnapshotTask(task, stepIds);
      if (!mapped) return null;
      const existing = existingById.get(mapped.id);
      return existing
        ? mergeFetchedTask(
            mapped,
            existing,
            fetchStartById.get(mapped.id),
            task,
            task.status_summary_invalidated === true,
          )
        : mapped;
    })
    .filter((t): t is KanbanTask => t !== null);
  const snapshotTaskIds = new Set(tasks.map((t) => t.id));
  const preserveExistingPlaceholderTasks = snapshotAtFetchStart?.isPlaceholder === true;
  const inFlightCreatedTasks = (existingSnapshot?.tasks ?? []).filter(
    (task) =>
      (preserveExistingPlaceholderTasks || !taskIdsAtFetchStart.has(task.id)) &&
      !snapshotTaskIds.has(task.id) &&
      stepIds.has(task.workflowStepId),
  );

  return [...tasks, ...inFlightCreatedTasks];
}

async function fetchAndWriteSnapshot(
  wf: Workflow,
  store: StoreApi<AppState>,
  fetchGenRef: MutableRefObject<number>,
  myGen: number,
  request: WorkspaceContextRequest,
): Promise<void> {
  try {
    const snapshotAtFetchStart = store.getState().kanbanMulti.snapshots[wf.id];
    const snapshot = await fetchWorkflowSnapshot(wf.id, { cache: "no-store" });
    if (
      fetchGenRef.current !== myGen ||
      !isCurrentWorkspaceContext(store.getState(), request.workspaceId, request.generation)
    ) {
      return;
    }

    const steps = snapshot.steps.map((step) => ({
      id: step.id,
      title: step.name,
      color: step.color ?? "bg-neutral-400",
      position: step.position,
      events: step.events,
      allow_manual_move: step.allow_manual_move,
      prompt: step.prompt,
      is_start_step: step.is_start_step,
      show_in_command_panel: step.show_in_command_panel,
      agent_profile_id: step.agent_profile_id,
      wip_limit: step.wip_limit,
      pull_from_step_id: step.pull_from_step_id ?? null,
      stage_type: step.stage_type,
    }));
    const stepIds = new Set(steps.map((s) => s.id));

    const existingSnapshot = store.getState().kanbanMulti.snapshots[wf.id];
    const tasks = mergeSnapshotTasks(
      snapshot.tasks,
      stepIds,
      snapshotAtFetchStart,
      existingSnapshot,
    );

    const workflowSnapshot = {
      workflowId: wf.id,
      workflowName: wf.name,
      steps,
      tasks,
    };
    store.getState().setWorkflowSnapshot(wf.id, workflowSnapshot);
    const activeKanban = store.getState().kanban;
    if (activeKanban?.workflowId === wf.id) {
      store.getState().hydrate({
        kanban: {
          ...activeKanban,
          isLoading: false,
          steps,
          tasks: workflowSnapshot.tasks,
        },
      });
    }
  } catch (err) {
    console.error(
      `[useAllWorkflowSnapshots] Failed to fetch snapshot for workflow "${wf.name}" (${wf.id}):`,
      err,
    );
    // A failed fetch must not leave a placeholder permanently "unknown": that
    // would block the final-step ensure forever. Transition to an explicit
    // known-but-empty state so the ensure proceeds ungated (safe default).
    const current = store.getState().kanbanMulti.snapshots[wf.id];
    if (current?.isPlaceholder) {
      store.getState().setWorkflowSnapshot(wf.id, { ...current, isPlaceholder: false });
    }
  }
}

function mapSnapshotTask(task: Task, stepIds: Set<string>): KanbanTask | null {
  if (!task.workflow_step_id || !stepIds.has(task.workflow_step_id)) return null;
  return toKanbanTask(task);
}

export function useAllWorkflowSnapshots(workspaceId: string | null) {
  const store = useAppStoreApi();
  const connectionStatus = useAppStore((state) => state.connection.status);
  const workflows = useAppStore((state) => state.workflows.items);
  const lastFetchedRef = useRef<string>("");
  const lastWorkspaceIdRef = useRef<string | null>(null);
  const fetchGenRef = useRef(0);
  const refreshResolversRef = useRef<Array<{ generation: number; resolve: () => void }>>([]);
  const [refreshNonce, setRefreshNonce] = useState(0);
  const resolveRefreshes = useCallback((generation: number) => {
    const ready = refreshResolversRef.current.filter((entry) => entry.generation === generation);
    refreshResolversRef.current = refreshResolversRef.current.filter(
      (entry) => entry.generation !== generation,
    );
    ready.forEach(({ resolve }) => resolve());
  }, []);
  const refresh = useCallback(
    () =>
      new Promise<void>((resolve) => {
        refreshResolversRef.current.push({ generation: fetchGenRef.current, resolve });
        setRefreshNonce((current) => current + 1);
      }),
    [],
  );

  useForegroundRefresh(refresh, Boolean(workspaceId), workspaceId);

  useEffect(() => {
    // Skip clear on initial mount to preserve SSR-hydrated snapshots.
    if (lastWorkspaceIdRef.current !== workspaceId) {
      if (lastWorkspaceIdRef.current !== null) {
        resolveRefreshes(fetchGenRef.current);
        store.getState().clearKanbanMulti();
        lastFetchedRef.current = "";
        fetchGenRef.current += 1;
      }
      lastWorkspaceIdRef.current = workspaceId;
    }

    if (!workspaceId) {
      resolveRefreshes(fetchGenRef.current);
      return;
    }

    const workspaceWorkflows = workflows.filter((w) => w.workspaceId === workspaceId);
    if (workspaceWorkflows.length === 0) {
      resolveRefreshes(fetchGenRef.current);
      return;
    }

    // Deduplicate: skip if same set of workflow IDs already fetched for this connection status
    const key =
      workspaceWorkflows
        .map((w) => w.id)
        .sort()
        .join(",") +
      ":" +
      connectionStatus +
      ":" +
      refreshNonce;
    if (lastFetchedRef.current === key) {
      resolveRefreshes(fetchGenRef.current);
      return;
    }
    if (
      lastFetchedRef.current === "" &&
      workspaceWorkflows.every((wf) =>
        isBootHydratedSnapshot(store.getState().kanbanMulti.snapshots[wf.id]),
      )
    ) {
      lastFetchedRef.current = key;
      resolveRefreshes(fetchGenRef.current);
      return;
    }
    lastFetchedRef.current = key;

    const myGen = fetchGenRef.current;
    const request = {
      workspaceId,
      generation: store.getState().workspaceContextGeneration,
    };
    store.getState().setKanbanMultiLoading(true);

    Promise.all(
      workspaceWorkflows.map((wf) => fetchAndWriteSnapshot(wf, store, fetchGenRef, myGen, request)),
    ).finally(() => {
      if (
        fetchGenRef.current !== myGen ||
        !isCurrentWorkspaceContext(store.getState(), request.workspaceId, request.generation)
      ) {
        return;
      }
      store.getState().setKanbanMultiLoading(false);
      resolveRefreshes(myGen);
    });
  }, [workspaceId, workflows, connectionStatus, refreshNonce, resolveRefreshes, store]);

  return { refresh };
}
