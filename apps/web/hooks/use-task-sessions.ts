import { useCallback, useEffect, useRef } from "react";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { listTaskSessions } from "@/lib/api";
import type { AppState } from "@/lib/state/store";
import type { TaskSession } from "@/lib/types/http";
import { useForegroundRefresh } from "@/hooks/use-foreground-refresh";
import { captureTaskSessionActivityEpochs } from "@/lib/state/slices/session/activity-epochs";

const EMPTY_SESSIONS: TaskSession[] = [];

function storedTaskSessions(getStoreState: () => AppState, taskId: string) {
  return getStoreState().taskSessionsByTask.itemsByTaskId[taskId] ?? EMPTY_SESSIONS;
}

function resolveForcedReloadWaiters(waitersRef: { current: Array<() => void> }) {
  const waiters = waitersRef.current.splice(0);
  waiters.forEach((resolve) => resolve());
}

async function hydrateTaskSessions({
  taskId,
  force,
  getStoreState,
  setTaskSessionsForTask,
  setTaskSessionsError,
}: {
  taskId: string;
  force: boolean;
  getStoreState: () => AppState;
  setTaskSessionsForTask: AppState["setTaskSessionsForTask"];
  setTaskSessionsError: AppState["setTaskSessionsError"];
}): Promise<boolean> {
  const stateAtRequestStart = getStoreState();
  const sessionsAtRequestStart = storedTaskSessions(() => stateAtRequestStart, taskId);
  const sessionIdsAtRequestStart = new Set(sessionsAtRequestStart.map((session) => session.id));
  const activityEpochsAtRequestStart = captureTaskSessionActivityEpochs(
    stateAtRequestStart,
    taskId,
  );
  try {
    const response = await listTaskSessions(taskId, { cache: "no-store" });
    const fetchedSessions = response.sessions ?? [];
    const fetchedSessionIds = new Set(fetchedSessions.map((session) => session.id));
    const sessionsAddedDuringLoad = storedTaskSessions(getStoreState, taskId).filter(
      (session) => !sessionIdsAtRequestStart.has(session.id) && !fetchedSessionIds.has(session.id),
    );
    setTaskSessionsForTask(
      taskId,
      [...fetchedSessions, ...sessionsAddedDuringLoad],
      activityEpochsAtRequestStart,
    );
    return sessionsAddedDuringLoad.length > 0;
  } catch (error) {
    console.error("Failed to load task sessions:", error);
    // A failed initial request must not turn an empty list into an
    // authoritative snapshot. Reconcile existing live rows when available so
    // the activity-epoch guard still applies, then expose the retryable error.
    const currentSessions = storedTaskSessions(getStoreState, taskId);
    if (!force && currentSessions.length > 0) {
      setTaskSessionsForTask(taskId, currentSessions, activityEpochsAtRequestStart);
    }
    setTaskSessionsError(taskId, error instanceof Error ? error.message : String(error));
    return false;
  }
}

function useTaskSessionState(taskId: string | null) {
  const sessions = useAppStore((state) =>
    taskId ? (state.taskSessionsByTask.itemsByTaskId[taskId] ?? EMPTY_SESSIONS) : EMPTY_SESSIONS,
  );
  const isLoading = useAppStore((state) =>
    taskId ? (state.taskSessionsByTask.loadingByTaskId[taskId] ?? false) : false,
  );
  const isLoaded = useAppStore((state) =>
    taskId ? (state.taskSessionsByTask.loadedByTaskId[taskId] ?? false) : false,
  );
  const error = useAppStore((state) =>
    taskId ? (state.taskSessionsByTask.errorByTaskId?.[taskId] ?? null) : null,
  );
  const connectionStatus = useAppStore((state) => state.connection.status);
  return { sessions, isLoading, isLoaded, error, connectionStatus };
}

export function useTaskSessions(taskId: string | null) {
  const getStoreState = useAppStoreApi().getState;
  const { sessions, isLoading, isLoaded, error, connectionStatus } = useTaskSessionState(taskId);
  const setTaskSessionsForTask = useAppStore((state) => state.setTaskSessionsForTask);
  const setTaskSessionsError = useAppStore((state) => state.setTaskSessionsError);
  const setTaskSessionsLoading = useAppStore((state) => state.setTaskSessionsLoading);
  const pendingForcedReloadRef = useRef(false);
  const pendingForcedReloadWaitersRef = useRef<Array<() => void>>([]);
  const requestInFlightRef = useRef(false);

  const loadSessions = useCallback(
    async (force = false) => {
      if (!taskId) return;
      if (isLoading || requestInFlightRef.current) {
        if (force) {
          pendingForcedReloadRef.current = true;
          return new Promise<void>((resolve) => {
            pendingForcedReloadWaitersRef.current.push(resolve);
          });
        }
        return;
      }
      if (!force && isLoaded) return;
      requestInFlightRef.current = true;
      setTaskSessionsLoading(taskId, true);
      try {
        const needsFollowUp = await hydrateTaskSessions({
          taskId,
          force,
          getStoreState,
          setTaskSessionsForTask,
          setTaskSessionsError,
        });
        if (needsFollowUp) pendingForcedReloadRef.current = true;
      } finally {
        requestInFlightRef.current = false;
        setTaskSessionsLoading(taskId, false);
        if (force && !pendingForcedReloadRef.current) {
          resolveForcedReloadWaiters(pendingForcedReloadWaitersRef);
        }
      }
    },
    [
      getStoreState,
      isLoaded,
      isLoading,
      setTaskSessionsError,
      setTaskSessionsForTask,
      setTaskSessionsLoading,
      taskId,
    ],
  );

  useEffect(() => {
    if (!taskId) return;
    if (isLoaded || isLoading || error) return;
    loadSessions();
  }, [error, isLoaded, isLoading, loadSessions, taskId]);

  useEffect(() => {
    pendingForcedReloadRef.current = false;
    resolveForcedReloadWaiters(pendingForcedReloadWaitersRef);
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

  return { sessions, isLoading, isLoaded, error, loadSessions };
}
