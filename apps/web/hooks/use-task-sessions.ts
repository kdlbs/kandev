import { useCallback, useEffect, useRef } from "react";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { listTaskSessions } from "@/lib/api";
import type { AppState } from "@/lib/state/store";
import type { TaskSession } from "@/lib/types/http";
import { useForegroundRefresh } from "@/hooks/use-foreground-refresh";
import { captureTaskSessionHydrationEpochs } from "@/lib/state/slices/session/hydration-epochs";
import { stateReadScopeIdentity } from "@/lib/state/shared-resource-reads";
import { getTaskSessionReads, type TaskSessionReads } from "@/lib/state/task-session-reads";

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
  isCurrent,
  reads,
  getStoreState,
  setTaskSessionsForTask,
  setTaskSessionsError,
}: {
  taskId: string;
  force: boolean;
  isCurrent: () => boolean;
  reads: TaskSessionReads;
  getStoreState: () => AppState;
  setTaskSessionsForTask: AppState["setTaskSessionsForTask"];
  setTaskSessionsError: AppState["setTaskSessionsError"];
}): Promise<boolean> {
  const stateAtRequestStart = getStoreState();
  const sessionsAtRequestStart = storedTaskSessions(() => stateAtRequestStart, taskId);
  const sessionIdsAtRequestStart = new Set(sessionsAtRequestStart.map((session) => session.id));
  const hydrationEpochsAtRequestStart = captureTaskSessionHydrationEpochs(
    stateAtRequestStart,
    taskId,
  );
  try {
    const response = await reads.read(
      taskId,
      (signal) =>
        listTaskSessions(taskId, {
          cache: "no-store",
          init: { signal },
        }),
      { refresh: force },
    );
    if (!isCurrent()) return false;
    const fetchedSessions = response.sessions ?? [];
    const fetchedSessionIds = new Set(fetchedSessions.map((session) => session.id));
    const sessionsAddedDuringLoad = storedTaskSessions(getStoreState, taskId).filter(
      (session) => !sessionIdsAtRequestStart.has(session.id) && !fetchedSessionIds.has(session.id),
    );
    setTaskSessionsForTask(
      taskId,
      [...fetchedSessions, ...sessionsAddedDuringLoad],
      hydrationEpochsAtRequestStart,
    );
    getStoreState().reconcileWorkflowSessionFocus?.(taskId);
    return sessionsAddedDuringLoad.length > 0;
  } catch (error) {
    if (!isCurrent() || isAbortError(error)) return false;
    console.error("Failed to load task sessions:", error);
    // A failed initial request must not turn an empty list into an
    // authoritative snapshot. Reconcile existing live rows when available so
    // the activity-epoch guard still applies, then expose the retryable error.
    const currentSessions = storedTaskSessions(getStoreState, taskId);
    if (!force && currentSessions.length > 0) {
      setTaskSessionsForTask(taskId, currentSessions, hydrationEpochsAtRequestStart);
      getStoreState().reconcileWorkflowSessionFocus?.(taskId);
    }
    setTaskSessionsError(taskId, error instanceof Error ? error.message : String(error));
    return false;
  }
}

async function hydrateCurrentTaskSessions(args: Parameters<typeof hydrateTaskSessions>[0]) {
  let needsFollowUp = await hydrateTaskSessions(args);
  while (needsFollowUp && args.isCurrent()) {
    needsFollowUp = await hydrateTaskSessions({ ...args, force: true });
  }
}

function isAbortError(error: unknown): boolean {
  return error instanceof Error && error.name === "AbortError";
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

function useTaskSessionReadOwnership({
  store,
  reads,
  taskId,
  setTaskSessionsLoading,
  requestInFlightRef,
  pendingForcedReloadRef,
  pendingForcedReloadWaitersRef,
}: {
  store: ReturnType<typeof useAppStoreApi>;
  reads: TaskSessionReads;
  taskId: string | null;
  setTaskSessionsLoading: AppState["setTaskSessionsLoading"];
  requestInFlightRef: { current: boolean };
  pendingForcedReloadRef: { current: boolean };
  pendingForcedReloadWaitersRef: { current: Array<() => void> };
}) {
  const ownershipGenerationRef = useRef(0);

  useEffect(() => {
    if (!taskId) return;
    const release = reads.retain(taskId);
    return () => {
      ownershipGenerationRef.current++;
      requestInFlightRef.current = false;
      pendingForcedReloadRef.current = false;
      resolveForcedReloadWaiters(pendingForcedReloadWaitersRef);
      release();
      if (!getTaskSessionReads(store).isReading(taskId)) {
        setTaskSessionsLoading(taskId, false);
      }
    };
  }, [
    pendingForcedReloadRef,
    pendingForcedReloadWaitersRef,
    reads,
    requestInFlightRef,
    setTaskSessionsLoading,
    store,
    taskId,
  ]);

  return ownershipGenerationRef;
}

function useTaskSessionReconnect(
  taskId: string | null,
  connectionStatus: string,
  loadSessions: (force?: boolean) => Promise<void>,
) {
  const previousConnectionStatusRef = useRef(connectionStatus);
  useEffect(() => {
    const previous = previousConnectionStatusRef.current;
    previousConnectionStatusRef.current = connectionStatus;
    if (!taskId) return;
    if (connectionStatus !== "connected" || previous === "connected") return;
    void loadSessions(true);
  }, [connectionStatus, loadSessions, taskId]);
}

function observeStoredMembership(
  observed: { current: { taskId: string | null; key: string; loaded: boolean } },
  getState: () => AppState,
  taskId: string,
) {
  observed.current = {
    taskId,
    key: JSON.stringify(
      storedTaskSessions(getState, taskId)
        .map((session) => session.id)
        .sort(),
    ),
    loaded: getState().taskSessionsByTask.loadedByTaskId[taskId] ?? false,
  };
}

function useTaskSessionLoadEffects({
  taskId,
  isLoaded,
  isLoading,
  error,
  loadSessions,
  membershipKey,
  observedMembershipRef,
  pendingForcedReloadRef,
  pendingForcedReloadWaitersRef,
}: {
  taskId: string | null;
  isLoaded: boolean;
  isLoading: boolean;
  error: string | null;
  loadSessions: (force?: boolean) => Promise<void>;
  membershipKey: string;
  observedMembershipRef: { current: { taskId: string | null; key: string; loaded: boolean } };
  pendingForcedReloadRef: { current: boolean };
  pendingForcedReloadWaitersRef: { current: Array<() => void> };
}) {
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

  useEffect(() => {
    const previous = observedMembershipRef.current;
    observedMembershipRef.current = { taskId, key: membershipKey, loaded: isLoaded };
    if (
      previous.taskId === taskId &&
      previous.loaded &&
      isLoaded &&
      previous.key !== membershipKey
    ) {
      void loadSessions(true);
    }
  }, [taskId, membershipKey, isLoaded, loadSessions]);
}

export function useTaskSessions(taskId: string | null) {
  const store = useAppStoreApi();
  useAppStore(stateReadScopeIdentity);
  const getStoreState = store.getState;
  const reads = getTaskSessionReads(store);
  const { sessions, isLoading, isLoaded, error, connectionStatus } = useTaskSessionState(taskId);
  const setTaskSessionsForTask = useAppStore((state) => state.setTaskSessionsForTask);
  const setTaskSessionsError = useAppStore((state) => state.setTaskSessionsError);
  const setTaskSessionsLoading = useAppStore((state) => state.setTaskSessionsLoading);
  const pendingForcedReloadRef = useRef(false);
  const pendingForcedReloadWaitersRef = useRef<Array<() => void>>([]);
  const requestInFlightRef = useRef(false);
  const ownershipGenerationRef = useTaskSessionReadOwnership({
    store,
    reads,
    taskId,
    setTaskSessionsLoading,
    requestInFlightRef,
    pendingForcedReloadRef,
    pendingForcedReloadWaitersRef,
  });

  const membershipKey = JSON.stringify(sessions.map((session) => session.id).sort());
  const observedMembershipRef = useRef({ taskId, key: membershipKey, loaded: isLoaded });

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
      const generation = ownershipGenerationRef.current;
      const isCurrent = () =>
        generation === ownershipGenerationRef.current && reads.isCurrent(taskId);
      requestInFlightRef.current = true;
      setTaskSessionsLoading(taskId, true);
      try {
        await hydrateCurrentTaskSessions({
          taskId,
          force,
          isCurrent: () => reads.isCurrent(taskId),
          reads,
          getStoreState,
          setTaskSessionsForTask,
          setTaskSessionsError,
        });
      } finally {
        if (isCurrent()) {
          observeStoredMembership(observedMembershipRef, getStoreState, taskId);
          requestInFlightRef.current = false;
          if (force && !pendingForcedReloadRef.current) {
            resolveForcedReloadWaiters(pendingForcedReloadWaitersRef);
          }
        }
        if (reads.isCurrent(taskId) && !reads.isReading(taskId)) {
          setTaskSessionsLoading(taskId, false);
        }
      }
    },
    [
      getStoreState,
      isLoaded,
      isLoading,
      reads,
      setTaskSessionsError,
      setTaskSessionsForTask,
      setTaskSessionsLoading,
      taskId,
    ],
  );

  useTaskSessionLoadEffects({
    taskId,
    isLoaded,
    isLoading,
    error,
    loadSessions,
    membershipKey,
    observedMembershipRef,
    pendingForcedReloadRef,
    pendingForcedReloadWaitersRef,
  });

  useTaskSessionReconnect(taskId, connectionStatus, loadSessions);

  useForegroundRefresh(
    () => {
      if (!taskId) return;
      void loadSessions(true);
    },
    Boolean(taskId),
    taskId,
  );

  return { sessions, isLoading, isLoaded, error, loadSessions };
}
