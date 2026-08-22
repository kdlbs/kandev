import { useEffect, useRef } from "react";
import { fetchWorkflowSnapshot } from "@/lib/api";
import { snapshotToState } from "@/lib/ssr/mapper";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { isCurrentWorkspaceContext } from "@/lib/state/workspace-context";
import type { KanbanState } from "@/lib/state/slices/kanban/types";

type KanbanTask = KanbanState["tasks"][number];

function preserveLiveAutoStartFailed(
  snapshotTasks: KanbanTask[],
  fetchStartTasks: KanbanTask[],
  currentTasks: KanbanTask[],
) {
  const fetchStartByID = new Map(fetchStartTasks.map((task) => [task.id, task]));
  const currentByID = new Map(currentTasks.map((task) => [task.id, task]));
  return snapshotTasks.map((task) => {
    const current = currentByID.get(task.id);
    if (!current) return task;
    const fetchStart = fetchStartByID.get(task.id);
    if (
      task.autoStartFailed === undefined ||
      fetchStart === undefined ||
      current.autoStartFailed !== fetchStart.autoStartFailed
    ) {
      return { ...task, autoStartFailed: current.autoStartFailed };
    }
    return task;
  });
}

export function useWorkflowSnapshot(workflowId: string | null) {
  const store = useAppStoreApi();
  const connectionStatus = useAppStore((state) => state.connection.status);
  const skippedInitialHydratedRef = useRef(false);

  useEffect(() => {
    if (!workflowId) return;
    let cancelled = false;
    const requestState = store.getState();
    const requestedWorkspaceId = requestState.workspaces.activeId;
    const requestedGeneration = requestState.workspaceContextGeneration;
    const existing = requestState.kanban;
    if (
      !skippedInitialHydratedRef.current &&
      existing.workflowId === workflowId &&
      (existing.steps.length > 0 || existing.tasks.length > 0)
    ) {
      skippedInitialHydratedRef.current = true;
      return;
    }
    const setLoading = store.getState().kanban.workflowId !== workflowId;
    if (setLoading) {
      store.setState((state) => ({ ...state, kanban: { ...state.kanban, isLoading: true } }));
    }
    fetchWorkflowSnapshot(workflowId, { cache: "no-store" })
      .then((snapshot) => {
        if (
          cancelled ||
          !isCurrentWorkspaceContext(store.getState(), requestedWorkspaceId, requestedGeneration)
        ) {
          return;
        }
        const nextState = snapshotToState(snapshot);
        if (nextState.kanban) {
          const currentTasks = store.getState().kanban.tasks;
          const fetchedAtStartTasks = existing.tasks;
          nextState.kanban.tasks = preserveLiveAutoStartFailed(
            nextState.kanban.tasks,
            fetchedAtStartTasks,
            currentTasks,
          );
        }
        store.getState().hydrate(nextState);
      })
      .catch((error) => {
        // Suppress superseded-fetch noise; retry happens on WS reconnect.
        if (
          cancelled ||
          !isCurrentWorkspaceContext(store.getState(), requestedWorkspaceId, requestedGeneration)
        ) {
          return;
        }
        console.warn("[useWorkflowSnapshot] failed to load snapshot:", error);
      })
      .finally(() => {
        // Only clear the flag this effect raised; skip when cancelled or when a concurrent caller owns it.
        if (
          cancelled ||
          !setLoading ||
          !isCurrentWorkspaceContext(store.getState(), requestedWorkspaceId, requestedGeneration)
        ) {
          return;
        }
        store.setState((state) => ({ ...state, kanban: { ...state.kanban, isLoading: false } }));
      });
    return () => {
      cancelled = true;
    };
  }, [workflowId, store, connectionStatus]);
}
