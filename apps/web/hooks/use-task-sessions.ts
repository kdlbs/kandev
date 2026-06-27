import { useCallback, useEffect, useRef } from "react";
import { useAppStore } from "@/components/state-provider";
import { listTaskSessions } from "@/lib/api";
import type { TaskSession } from "@/lib/types/http";

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

  const loadSessions = useCallback(
    async (force = false) => {
      if (!taskId) return;
      if (isLoading) {
        if (force) pendingForcedReloadRef.current = true;
        return;
      }
      if (!force && isLoaded) return;
      setTaskSessionsLoading(taskId, true);
      try {
        const response = await listTaskSessions(taskId, { cache: "no-store" });
        const sessions = response.sessions ?? [];
        setTaskSessionsForTask(taskId, sessions);
      } catch {
        setTaskSessionsForTask(taskId, []);
      } finally {
        setTaskSessionsLoading(taskId, false);
      }
    },
    [isLoaded, isLoading, setTaskSessionsForTask, setTaskSessionsLoading, taskId],
  );

  useEffect(() => {
    if (!taskId) return;
    if (connectionStatus !== "connected") return;
    if (isLoaded || isLoading) return;
    loadSessions();
  }, [connectionStatus, isLoaded, isLoading, loadSessions, taskId]);

  useEffect(() => {
    pendingForcedReloadRef.current = false;
  }, [taskId]);

  useEffect(() => {
    if (!taskId || isLoading || connectionStatus !== "connected") return;
    if (!pendingForcedReloadRef.current) return;
    pendingForcedReloadRef.current = false;
    void loadSessions(true);
  }, [connectionStatus, isLoading, loadSessions, taskId]);

  const previousConnectionStatusRef = useRef(connectionStatus);
  useEffect(() => {
    const previous = previousConnectionStatusRef.current;
    previousConnectionStatusRef.current = connectionStatus;
    if (!taskId) return;
    if (connectionStatus !== "connected" || previous === "connected") return;
    if (!isLoaded) return;
    void loadSessions(true);
  }, [connectionStatus, isLoaded, isLoading, loadSessions, taskId]);

  useEffect(() => {
    if (!taskId) return;
    const refetchOnVisible = () => {
      if (document.visibilityState !== "visible") return;
      if (connectionStatus !== "connected") return;
      if (!isLoaded) return;
      void loadSessions(true);
    };
    document.addEventListener("visibilitychange", refetchOnVisible);
    return () => document.removeEventListener("visibilitychange", refetchOnVisible);
  }, [connectionStatus, isLoaded, isLoading, loadSessions, taskId]);

  return { sessions, isLoading, isLoaded, loadSessions };
}
