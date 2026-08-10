import { useCallback, useEffect, useRef, useState, type MutableRefObject } from "react";
import { fetchWorkflowSnapshot } from "@/lib/api";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { toKanbanTask } from "@/lib/kanban/map-task";
import type { KanbanState, WorkflowSnapshotData } from "@/lib/state/slices/kanban/types";
import type { Task } from "@/lib/types/http";
import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import { isCurrentWorkspaceContext } from "@/lib/state/workspace-context";
import { useForegroundRefresh } from "@/hooks/use-foreground-refresh";

type KanbanTask = KanbanState["tasks"][number];
type Workflow = { id: string; name: string };
type WorkspaceContextRequest = { workspaceId: string; generation: number };

function isBootHydratedSnapshot(snapshot: WorkflowSnapshotData | undefined): boolean {
  return !!snapshot && snapshot.isPlaceholder !== true;
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
    const taskIdsAtFetchStart = new Set((snapshotAtFetchStart?.tasks ?? []).map((t) => t.id));
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

    // Preserve runtime fields (e.g., primarySessionId) from existing snapshot
    // tasks when the fresh API response omits them (backend uses omitempty).
    const existingSnapshot = store.getState().kanbanMulti.snapshots[wf.id];
    const existingById = new Map((existingSnapshot?.tasks ?? []).map((t) => [t.id, t]));

    const tasks: KanbanTask[] = snapshot.tasks
      .filter((task) => !task.is_ephemeral)
      .map((task) => {
        const mapped = mapSnapshotTask(task, stepIds);
        if (!mapped) return null;
        const existing = existingById.get(mapped.id);
        if (existing) {
          mapped.primarySessionId = mapped.primarySessionId || existing.primarySessionId;
          mapped.primarySessionState = mapped.primarySessionState || existing.primarySessionState;
          // Autopilot is immutable after creation. Keep the cached value when
          // an older or partial snapshot does not include the field.
          mapped.autopilot = mapped.autopilot ?? existing.autopilot;
        }
        return mapped;
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

    const workflowSnapshot = {
      workflowId: wf.id,
      workflowName: wf.name,
      steps,
      tasks: [...tasks, ...inFlightCreatedTasks],
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
