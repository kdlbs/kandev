import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { listTasksByWorkspace } from "@/lib/api/domains/kanban-api";
import { toKanbanTask } from "@/lib/kanban/map-task";
import type { Task } from "@/lib/types/http";
import type { KanbanState } from "@/lib/state/slices/kanban/types";
import { isCurrentWorkspaceContext } from "@/lib/state/workspace-context";
import { useForegroundRefresh } from "@/hooks/use-foreground-refresh";
import { t } from "@/lib/i18n";

const ARCHIVED_PAGE_SIZE = 100;
const EMPTY_ARCHIVED_TASKS: KanbanState["tasks"] = [];

export async function loadSidebarArchivedTasks(workspaceId: string): Promise<Task[]> {
  const tasks: Task[] = [];
  let page = 1;

  while (true) {
    const response = await listTasksByWorkspace(workspaceId, {
      page,
      pageSize: ARCHIVED_PAGE_SIZE,
      onlyArchived: true,
    });
    tasks.push(...response.tasks);
    if (response.tasks.length === 0 || tasks.length >= response.total) {
      return tasks;
    }
    page += 1;
  }
}

export function useSidebarArchivedTasks(workspaceId: string | null, enabled: boolean) {
  const store = useAppStoreApi();
  const items = useAppStore((state) =>
    workspaceId
      ? (state.sidebarArchivedTasks?.itemsByWorkspaceId[workspaceId] ?? EMPTY_ARCHIVED_TASKS)
      : EMPTY_ARCHIVED_TASKS,
  );
  const isLoading = useAppStore((state) =>
    workspaceId ? (state.sidebarArchivedTasks?.loadingByWorkspaceId[workspaceId] ?? false) : false,
  );
  const error = useAppStore((state) =>
    workspaceId ? (state.sidebarArchivedTasks?.errorByWorkspaceId[workspaceId] ?? null) : null,
  );
  const loaded = useAppStore((state) =>
    workspaceId ? (state.sidebarArchivedTasks?.loadedByWorkspaceId[workspaceId] ?? false) : false,
  );
  const [refreshNonce, setRefreshNonce] = useState(0);
  const requestGenerationRef = useRef(0);
  const lastRequestKeyRef = useRef("");

  const refresh = useCallback(() => {
    setRefreshNonce((current) => current + 1);
  }, []);

  useForegroundRefresh(refresh, enabled && Boolean(workspaceId), workspaceId);

  useEffect(() => {
    if (!enabled || !workspaceId) return;
    const requestKey = `${workspaceId}:${refreshNonce}`;
    if (lastRequestKeyRef.current === requestKey) return;

    const currentState = store.getState();
    if (currentState.sidebarArchivedTasks?.loadingByWorkspaceId[workspaceId]) return;
    if (loaded && refreshNonce === 0) {
      lastRequestKeyRef.current = requestKey;
      return;
    }

    lastRequestKeyRef.current = requestKey;
    const requestGeneration = ++requestGenerationRef.current;
    const workspaceGeneration = currentState.workspaceContextGeneration;
    const archivedRevision =
      currentState.sidebarArchivedTasks?.revisionByWorkspaceId[workspaceId] ?? 0;
    currentState.setSidebarArchivedTasksLoading(workspaceId, true);
    currentState.setSidebarArchivedTasksError(workspaceId, null);

    void (async () => {
      try {
        const response = await loadSidebarArchivedTasks(workspaceId);
        const latest = store.getState();
        if (
          requestGenerationRef.current !== requestGeneration ||
          !isCurrentWorkspaceContext(latest, workspaceId, workspaceGeneration)
        ) {
          return;
        }
        const applied = latest.setSidebarArchivedTasks(
          workspaceId,
          response.map((task) => toKanbanTask(task)),
          archivedRevision,
        );
        if (!applied) refresh();
      } catch (loadError) {
        const latest = store.getState();
        if (
          requestGenerationRef.current !== requestGeneration ||
          !isCurrentWorkspaceContext(latest, workspaceId, workspaceGeneration)
        ) {
          return;
        }
        latest.setSidebarArchivedTasksError(
          workspaceId,
          loadError instanceof Error ? loadError.message : t("sidebar:failedToLoadArchivedTasks"),
        );
      } finally {
        const latest = store.getState();
        if (requestGenerationRef.current === requestGeneration) {
          latest.setSidebarArchivedTasksLoading(workspaceId, false);
        }
      }
    })();
  }, [enabled, loaded, refreshNonce, store, workspaceId]);

  return useMemo(
    () => ({
      tasks: items,
      isLoading: enabled && isLoading && items.length === 0,
      isRefreshing: enabled && isLoading && items.length > 0,
      error: enabled ? error : null,
      refresh,
    }),
    [enabled, error, isLoading, items, refresh],
  );
}
