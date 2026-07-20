import { useCallback, useEffect, useRef } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useAppStore } from "@/components/state-provider";
import { taskSessionsQueryOptions } from "@/lib/query/query-options";
import type { TaskSession } from "@/lib/types/http";

const EMPTY_SESSIONS: TaskSession[] = [];

function resolvePendingForcedReloadWaiters(waiters: Array<() => void>) {
  waiters.splice(0).forEach((resolve) => resolve());
}

export function useTaskSessions(taskId: string | null) {
  const queryClient = useQueryClient();
  const storeSessions = useAppStore((state) =>
    taskId ? (state.taskSessionsByTask.itemsByTaskId[taskId] ?? EMPTY_SESSIONS) : EMPTY_SESSIONS,
  );
  const storeIsLoading = useAppStore((state) =>
    taskId ? (state.taskSessionsByTask.loadingByTaskId[taskId] ?? false) : false,
  );
  const storeIsLoaded = useAppStore((state) =>
    taskId ? (state.taskSessionsByTask.loadedByTaskId[taskId] ?? false) : false,
  );
  const setTaskSessionsForTask = useAppStore((state) => state.setTaskSessionsForTask);
  const setTaskSessionsLoading = useAppStore((state) => state.setTaskSessionsLoading);
  const connectionStatus = useAppStore((state) => state.connection.status);
  const sessionsQuery = useQuery({
    ...taskSessionsQueryOptions(taskId ?? ""),
    enabled: Boolean(taskId && !storeIsLoaded && !storeIsLoading),
  });
  const pendingForcedReloadRef = useRef(false);
  const pendingForcedReloadWaitersRef = useRef<Array<() => void>>([]);
  const sessions = taskId ? (sessionsQuery.data?.sessions ?? storeSessions) : EMPTY_SESSIONS;
  const isLoading = taskId ? sessionsQuery.isFetching || storeIsLoading : false;
  const isLoaded = taskId ? sessionsQuery.isSuccess || storeIsLoaded : false;

  useEffect(() => {
    if (taskId && sessionsQuery.data)
      setTaskSessionsForTask(taskId, sessionsQuery.data.sessions ?? []);
  }, [sessionsQuery.data, setTaskSessionsForTask, taskId]);

  useEffect(() => {
    if (taskId) setTaskSessionsLoading(taskId, sessionsQuery.isFetching);
  }, [sessionsQuery.isFetching, setTaskSessionsLoading, taskId]);

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
      try {
        setTaskSessionsLoading(taskId, true);
        await queryClient.fetchQuery({ ...taskSessionsQueryOptions(taskId), staleTime: 0 });
      } catch (error) {
        console.error("Failed to load task sessions:", error);
        if (!force) setTaskSessionsForTask(taskId, []);
      } finally {
        setTaskSessionsLoading(taskId, false);
        if (force && !pendingForcedReloadRef.current) {
          resolvePendingForcedReloadWaiters(pendingForcedReloadWaitersRef.current);
        }
      }
    },
    [isLoaded, isLoading, queryClient, setTaskSessionsForTask, setTaskSessionsLoading, taskId],
  );

  useEffect(() => {
    if (!taskId) return;
    if (isLoaded || isLoading) return;
    loadSessions();
  }, [isLoaded, isLoading, loadSessions, taskId]);

  useEffect(() => {
    pendingForcedReloadRef.current = false;
    resolvePendingForcedReloadWaiters(pendingForcedReloadWaitersRef.current);
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

  useEffect(() => {
    if (!taskId) return;
    const refetchOnVisible = () => {
      if (document.visibilityState !== "visible") return;
      if (!isLoaded) {
        if (isLoading) void loadSessions(true);
        return;
      }
      void loadSessions(true);
    };
    document.addEventListener("visibilitychange", refetchOnVisible);
    return () => document.removeEventListener("visibilitychange", refetchOnVisible);
  }, [isLoaded, isLoading, loadSessions, taskId]);

  return { sessions, isLoading, isLoaded, loadSessions };
}
