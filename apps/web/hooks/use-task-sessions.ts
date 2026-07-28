import { useCallback, useEffect, useRef } from "react";
import { useAppStore } from "@/components/state-provider";
import { listTaskSessions } from "@/lib/api";
import type { TaskSession } from "@/lib/types/http";
import { useForegroundRefresh } from "@/hooks/use-foreground-refresh";

const EMPTY_SESSIONS: TaskSession[] = [];

export function useTaskSessions(taskId: string | null) {
  const sessions = useAppStore((state) =>
    taskId ? (state.taskSessionsByTask.itemsByTaskId[taskId] ?? EMPTY_SESSIONS) : EMPTY_SESSIONS,
  );
  const isLoading = useAppStore((state) =>
    taskId ? (state.taskSessionsByTask.loadingByTaskId[taskId] ?? false) : false,
  );
  const isLoaded = useAppStore((state) =>
    taskId ? (state.taskSessionsByTask.loadedByTaskId[taskId] ?? false) : false,
  );
  const setTaskSessionsForTask = useAppStore((state) => state.setTaskSessionsForTask);
  const setTaskSessionsLoading = useAppStore((state) => state.setTaskSessionsLoading);
  const connectionStatus = useAppStore((state) => state.connection.status);
  const pendingForcedReloadRef = useRef(false);
  const pendingForcedReloadWaitersRef = useRef<Array<() => void>>([]);

  const loadSessions = useCallback(
    async (force = false) => {
      if (!taskId) return;
      if (isLoading) {
        if (force) {
          pendingForcedReloadRef.current = true;
          return new Promise<void>((resolve) => {
            pendingForcedReloadWaitersRef.current.push(resolve);
          });
        }
        return;
      }
      if (!force && isLoaded) return;
      setTaskSessionsLoading(taskId, true);
      try {
        const response = await listTaskSessions(taskId, { cache: "no-store" });
        const sessions = response.sessions ?? [];
        setTaskSessionsForTask(taskId, sessions);
      } catch (error) {
        console.error("Failed to load task sessions:", error);
        if (!force) setTaskSessionsForTask(taskId, []);
      } finally {
        setTaskSessionsLoading(taskId, false);
        if (force && !pendingForcedReloadRef.current) {
          const waiters = pendingForcedReloadWaitersRef.current.splice(0);
          waiters.forEach((resolve) => resolve());
        }
      }
    },
    [isLoaded, isLoading, setTaskSessionsForTask, setTaskSessionsLoading, taskId],
  );

  useEffect(() => {
    if (!taskId) return;
    if (isLoaded || isLoading) return;
    loadSessions();
  }, [isLoaded, isLoading, loadSessions, taskId]);

  useEffect(() => {
    pendingForcedReloadRef.current = false;
    const waiters = pendingForcedReloadWaitersRef.current.splice(0);
    waiters.forEach((resolve) => resolve());
  }, [taskId]);

  useEffect(() => {
    if (!taskId || isLoading) return;
    if (!pendingForcedReloadRef.current) return;
    pendingForcedReloadRef.current = false;
    void loadSessions(true);
  }, [isLoading, loadSessions, taskId]);

  const previousConnectionStatusRef = useRef(connectionStatus);
  useEffect(() => {
    const previous = previousConnectionStatusRef.current;
    previousConnectionStatusRef.current = connectionStatus;
    if (!taskId) return;
    if (connectionStatus !== "connected" || previous === "connected") return;
    if (!isLoaded) {
      if (isLoading) void loadSessions(true);
      return;
    }
    void loadSessions(true);
  }, [connectionStatus, isLoaded, isLoading, loadSessions, taskId]);

  useForegroundRefresh(
    () => {
      if (!taskId) return;
      if (!isLoaded) {
        if (isLoading) void loadSessions(true);
        return;
      }
      void loadSessions(true);
    },
    Boolean(taskId),
    taskId,
  );

  return { sessions, isLoading, isLoaded, loadSessions };
}
