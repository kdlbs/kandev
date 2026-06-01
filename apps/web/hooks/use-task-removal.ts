import { useCallback } from "react";
import { useQueryClient, type QueryClient } from "@tanstack/react-query";
import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import type { KanbanState } from "@/lib/state/slices";
import type { TaskSession } from "@/lib/types/http";
import { replaceTaskUrl } from "@/lib/links";
import { listTaskSessions } from "@/lib/api";
import { qk } from "@/lib/query/keys";
import type { KanbanMultiData } from "@/lib/query/query-options/kanban";
import { performLayoutSwitch } from "@/lib/state/dockview-store";

function getMultiSnapshots(queryClient: QueryClient): KanbanMultiData["snapshots"] {
  return queryClient.getQueryData<KanbanMultiData>(qk.kanban.multi())?.snapshots ?? {};
}

type TaskRemovalOptions = {
  store: StoreApi<AppState>;
  /** Whether to call performLayoutSwitch when switching sessions (desktop sidebar uses this) */
  useLayoutSwitch?: boolean;
};

type RemoveFromBoardOptions = {
  /**
   * The active task ID captured **before** the async delete API call.
   * Avoids a race with the WS "task.deleted" handler that may clear
   * activeTaskId before removeTaskFromBoard runs.
   */
  wasActiveTaskId?: string | null;
  /** The active session ID captured before the async delete API call. */
  wasActiveSessionId?: string | null;
};

/**
 * Whether a cached by-task session list is a complete REST snapshot safe to
 * reuse without a fresh fetch.
 *
 * Two failure modes make a cached entry unsafe:
 *  - missing `task_environment_id` — the env-keyed layout switch needs it.
 *  - a bridge-synthesized PARTIAL record — the session-state bridge upserts a
 *    by-task entry from agentctl WS events (which DO carry env IDs but OMIT
 *    list-only fields like `is_passthrough`). Such a partial passes the env-ID
 *    check yet lacks `is_passthrough`, so reusing it seeds the by-id observe
 *    cache with `is_passthrough: undefined`. A client-side task switch then
 *    reads a passthrough session as non-passthrough and renders chat instead of
 *    the PTY terminal. `is_passthrough` is a non-omitempty bool in the REST DTO
 *    (always present in a real list response, undefined only in bridge
 *    partials), so requiring it defined rejects partials and forces a fetch.
 */
export function cachedSessionsAreFullyHydrated(sessions: TaskSession[]): boolean {
  return (
    sessions.length === 0 ||
    sessions.every(
      (session) => !!session.task_environment_id && session.is_passthrough !== undefined,
    )
  );
}

async function loadTaskSessionsForTaskFromStore(
  queryClient: QueryClient,
  taskId: string,
): Promise<TaskSession[]> {
  const key = qk.taskSession.byTask(taskId);
  const cachedSessions = queryClient.getQueryData<{ sessions: TaskSession[] }>(key)?.sessions ?? [];
  // Reuse the cache only when it's a complete REST snapshot (env IDs present and
  // not a bridge-synthesized partial); otherwise force a fresh fetch so the
  // env-keyed layout switch AND the passthrough gating both have full records.
  if (cachedSessions.length > 0 && cachedSessionsAreFullyHydrated(cachedSessions)) {
    return cachedSessions;
  }
  try {
    const response = await listTaskSessions(taskId, { cache: "no-store" });
    const sessions = response.sessions ?? [];
    queryClient.setQueryData<{ sessions: TaskSession[]; total: number }>(key, {
      sessions,
      total: sessions.length,
    });
    return sessions;
  } catch (error) {
    console.error("Failed to load task sessions:", error);
    return [];
  }
}

function removeTaskFromSnapshots(queryClient: QueryClient, taskId: string): void {
  queryClient.setQueryData<KanbanMultiData>(qk.kanban.multi(), (prev) => {
    if (!prev) return prev;
    let changed = false;
    const snapshots: KanbanMultiData["snapshots"] = {};
    for (const [wfId, snapshot] of Object.entries(prev.snapshots)) {
      const hadTask = snapshot.tasks.some((t: KanbanState["tasks"][number]) => t.id === taskId);
      if (hadTask) {
        changed = true;
        snapshots[wfId] = {
          ...snapshot,
          tasks: snapshot.tasks.filter((t: KanbanState["tasks"][number]) => t.id !== taskId),
        };
      } else {
        snapshots[wfId] = snapshot;
      }
    }
    return changed ? { ...prev, snapshots } : prev;
  });
}

function collectRemainingTasks(queryClient: QueryClient): KanbanState["tasks"] {
  const allRemainingTasks: KanbanState["tasks"] = [];
  for (const snapshot of Object.values(getMultiSnapshots(queryClient))) {
    allRemainingTasks.push(...snapshot.tasks);
  }
  return allRemainingTasks;
}

function switchToSessionForTask(params: {
  store: StoreApi<AppState>;
  nextTask: KanbanState["tasks"][number];
  sessionId: string;
  oldEnvId: string | null;
  useLayoutSwitch: boolean;
}): void {
  const { store, nextTask, sessionId, oldEnvId, useLayoutSwitch } = params;
  store.getState().setActiveSession(nextTask.id, sessionId);
  if (!useLayoutSwitch) return;
  const newEnvId = store.getState().environmentIdBySessionId[sessionId] ?? null;
  if (newEnvId) performLayoutSwitch(oldEnvId, newEnvId, sessionId);
}

async function switchToNextTask(params: {
  store: StoreApi<AppState>;
  nextTask: KanbanState["tasks"][number];
  oldEnvId: string | null;
  useLayoutSwitch: boolean;
  loadTaskSessionsForTask: (taskId: string) => Promise<TaskSession[]>;
}): Promise<void> {
  const { store, nextTask, oldEnvId, useLayoutSwitch, loadTaskSessionsForTask } = params;
  if (nextTask.primarySessionId) {
    if (useLayoutSwitch && !store.getState().environmentIdBySessionId[nextTask.primarySessionId]) {
      await loadTaskSessionsForTask(nextTask.id);
    }
    switchToSessionForTask({
      store,
      nextTask,
      sessionId: nextTask.primarySessionId,
      oldEnvId,
      useLayoutSwitch,
    });
    replaceTaskUrl(nextTask.id);
    return;
  }

  const sessions = await loadTaskSessionsForTask(nextTask.id);
  const sessionId = sessions[0]?.id ?? null;
  if (sessionId) {
    switchToSessionForTask({ store, nextTask, sessionId, oldEnvId, useLayoutSwitch });
  } else {
    store.getState().setActiveTask(nextTask.id);
  }
  replaceTaskUrl(nextTask.id);
}

function resolveOldEnvId(store: StoreApi<AppState>, opts?: RemoveFromBoardOptions): string | null {
  const oldSessionId =
    opts?.wasActiveSessionId !== undefined
      ? opts.wasActiveSessionId
      : store.getState().tasks.activeSessionId;
  return oldSessionId ? (store.getState().environmentIdBySessionId[oldSessionId] ?? null) : null;
}

function resolveActiveTaskId(
  store: StoreApi<AppState>,
  opts?: RemoveFromBoardOptions,
): string | null {
  return opts?.wasActiveTaskId !== undefined
    ? opts.wasActiveTaskId
    : store.getState().tasks.activeTaskId;
}

/**
 * Hook that provides shared logic for removing a task from the kanban board
 * (after archive or delete) and switching to the next available task.
 *
 * Used by both TaskSessionSidebar and SessionTaskSwitcherSheet.
 */
export function useTaskRemoval({ store, useLayoutSwitch = false }: TaskRemovalOptions) {
  const queryClient = useQueryClient();
  const loadTaskSessionsForTask = useCallback(
    (taskId: string) => loadTaskSessionsForTaskFromStore(queryClient, taskId),
    [queryClient],
  );

  /**
   * Remove a task from the kanban board state (both single and multi snapshots)
   * and switch to the next available task if the removed task was active.
   *
   * Pass `opts.wasActiveTaskId` / `opts.wasActiveSessionId` when calling after
   * an async API call (e.g. deleteTaskById) — the WS "task.deleted" handler may
   * clear activeTaskId before this function runs.
   */
  const removeTaskFromBoard = useCallback(
    async (taskId: string, opts?: RemoveFromBoardOptions) => {
      removeTaskFromSnapshots(queryClient, taskId);
      const allRemainingTasks = collectRemainingTasks(queryClient);

      // Use the caller-provided active task ID (captured before the async API
      // call) to avoid the race with the WS handler that may have already
      // cleared it.  Fall back to the current store value for callers that
      // don't provide it (e.g. archive, which doesn't go through the API).
      const activeTaskId = resolveActiveTaskId(store, opts);
      if (activeTaskId !== taskId) return;

      const oldEnvId = resolveOldEnvId(store, opts);
      if (allRemainingTasks.length > 0) {
        await switchToNextTask({
          store,
          nextTask: allRemainingTasks[0],
          oldEnvId,
          useLayoutSwitch,
          loadTaskSessionsForTask,
        });
      } else {
        window.location.href = "/";
      }
    },
    [store, queryClient, useLayoutSwitch, loadTaskSessionsForTask],
  );

  return { removeTaskFromBoard, loadTaskSessionsForTask };
}
