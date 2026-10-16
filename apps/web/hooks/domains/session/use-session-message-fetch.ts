import { useEffect, type MutableRefObject } from "react";

import type { useAppStoreApi } from "@/components/state-provider";
import type { Message } from "@/lib/types/http";
import type { MessageHistoryStatus } from "./use-message-fetch-state";

type SessionMessageStore = ReturnType<typeof useAppStoreApi>;

export type SessionHydrationGeneration = {
  key: string;
  sessionId: string;
  readiness: Promise<void>;
};
export type SessionHydrationRef = MutableRefObject<SessionHydrationGeneration | null>;

export function useInitialMessageLoadingState(
  taskSessionId: string | null,
  messageCount: number,
  initialFetchStartRef: MutableRefObject<number | null>,
  lastFetchedSessionIdRef: MutableRefObject<string | null>,
  setIsWaitingForInitialMessages: (value: boolean) => void,
) {
  useEffect(() => {
    if (!taskSessionId) {
      initialFetchStartRef.current = null;
      lastFetchedSessionIdRef.current = null;
      setIsWaitingForInitialMessages(false);
      return;
    }
    if (messageCount > 0) {
      setIsWaitingForInitialMessages(false);
      return;
    }
    if (initialFetchStartRef.current === null) {
      initialFetchStartRef.current = Date.now();
      setIsWaitingForInitialMessages(true);
    }
  }, [
    taskSessionId,
    messageCount,
    initialFetchStartRef,
    lastFetchedSessionIdRef,
    setIsWaitingForInitialMessages,
  ]);
}

export function getHydratedMessagesForGeneration({
  hydrationRef,
  sessionId,
  readiness,
  hydrationKey,
  store,
  force = false,
}: {
  hydrationRef: SessionHydrationRef | undefined;
  sessionId: string;
  readiness: Promise<void>;
  hydrationKey: string | undefined;
  store: SessionMessageStore;
  force?: boolean;
}): Message[] | undefined {
  if (force) return undefined;
  const generation = hydrationRef?.current;
  if (
    !hydrationKey ||
    !generation ||
    generation.key !== hydrationKey ||
    generation.sessionId !== sessionId ||
    generation.readiness !== readiness
  ) {
    return undefined;
  }
  return store.getState().messages.bySession[sessionId] ?? [];
}

export function recordHydratedGeneration(
  hydrationRef: SessionHydrationRef | undefined,
  sessionId: string,
  readiness: Promise<void>,
  hydrationKey: string | undefined,
): void {
  if (hydrationRef && hydrationKey) {
    hydrationRef.current = { key: hydrationKey, sessionId, readiness };
  }
}

// Multiple lifecycle paths can hydrate the same session concurrently (for
// example, the initial mount and a visibility refresh). Keep the shared
// loading flag asserted until the last visible operation settles; an older
// request must not make a newer request look idle. Background refreshes are
// counted too but never raise the flag, because the flag drives the
// transcript's loading row.
const inFlightFetchesBySession = new Map<string, { total: number; visible: number }>();

function beginSessionFetch(sessionId: string, visible: boolean): void {
  const counts = inFlightFetchesBySession.get(sessionId) ?? { total: 0, visible: 0 };
  inFlightFetchesBySession.set(sessionId, {
    total: counts.total + 1,
    visible: counts.visible + (visible ? 1 : 0),
  });
}

/** Releases one fetch and returns how many visible fetches are still in flight. */
function endSessionFetch(sessionId: string, visible: boolean): number {
  const counts = inFlightFetchesBySession.get(sessionId) ?? { total: 1, visible: visible ? 1 : 0 };
  const next = {
    total: counts.total - 1,
    visible: Math.max(0, counts.visible - (visible ? 1 : 0)),
  };
  if (next.total > 0) inFlightFetchesBySession.set(sessionId, next);
  else inFlightFetchesBySession.delete(sessionId);
  return next.visible;
}

function hasMessagesOnScreen(store: SessionMessageStore, sessionId: string): boolean {
  return (store.getState().messages?.bySession?.[sessionId]?.length ?? 0) > 0;
}

function announceFetchStart({
  taskSessionId,
  store,
  setIsLoading,
  setHistoryStatus,
  setHistoryError,
  lastFetchedSessionIdRef,
  background,
}: Pick<
  DoFetchMessagesParams,
  | "taskSessionId"
  | "store"
  | "setIsLoading"
  | "setHistoryStatus"
  | "setHistoryError"
  | "lastFetchedSessionIdRef"
> & { background: boolean }): boolean {
  const silent = background && hasMessagesOnScreen(store, taskSessionId);
  beginSessionFetch(taskSessionId, !silent);
  if (silent) return true;
  setIsLoading(true);
  if (lastFetchedSessionIdRef.current !== taskSessionId) {
    setHistoryStatus("loading");
    setHistoryError(null);
  }
  store.getState().setMessagesLoading(taskSessionId, true);
  return false;
}

function isInactive(isActive?: () => boolean): boolean {
  return isActive !== undefined && !isActive();
}

type DoFetchMessagesParams = {
  taskSessionId: string;
  store: SessionMessageStore;
  setIsLoading: (value: boolean) => void;
  setIsWaitingForInitialMessages: (value: boolean) => void;
  setHistoryStatus: (value: MessageHistoryStatus) => void;
  setHistoryError: (value: unknown) => void;
  initialFetchStartRef: MutableRefObject<number | null>;
  lastFetchedSessionIdRef: MutableRefObject<string | null>;
  // eslint-disable-next-line max-params -- hydration guards and retry feedback share one fetch contract.
  fetchAndStoreMessages: (
    sessionId: string,
    store: SessionMessageStore,
    isActive?: () => boolean,
    hydrationRef?: SessionHydrationRef,
    hydrationKey?: string,
    onRetry?: () => void,
    options?: MessageFetchOptions,
  ) => Promise<Message[]>;
  onError?: (error: unknown) => void;
  isActive?: () => boolean;
  /** Allows the owning hook generation to fence local loading finalization. */
  canFinalizeLoading?: () => boolean;
  hydrationRef?: SessionHydrationRef;
  hydrationKey?: string;
  options?: MessageFetchOptions;
  /** Reconciles a transcript already on screen without visible loading feedback. */
  background?: boolean;
};

export type MessageFetchOptions = {
  force?: boolean;
  authoritative?: boolean;
};

export async function doFetchMessages({
  taskSessionId,
  store,
  setIsLoading,
  setIsWaitingForInitialMessages,
  setHistoryStatus,
  setHistoryError,
  initialFetchStartRef,
  lastFetchedSessionIdRef,
  fetchAndStoreMessages,
  onError,
  isActive,
  canFinalizeLoading,
  hydrationRef,
  hydrationKey,
  options,
  background = false,
}: DoFetchMessagesParams): Promise<boolean> {
  if (isInactive(isActive)) return false;
  const silent = announceFetchStart({
    taskSessionId,
    store,
    setIsLoading,
    setHistoryStatus,
    setHistoryError,
    lastFetchedSessionIdRef,
    background,
  });
  if (!silent && initialFetchStartRef.current === null) {
    initialFetchStartRef.current = Date.now();
    setIsWaitingForInitialMessages(true);
  }
  try {
    await fetchAndStoreMessages(
      taskSessionId,
      store,
      isActive,
      hydrationRef,
      hydrationKey,
      // A transient retry notice would shift a transcript that is already on screen.
      () => {
        if (!silent) setHistoryStatus("retrying");
      },
      options,
    );
    if (isInactive(isActive)) return false;
    lastFetchedSessionIdRef.current = taskSessionId;
    setHistoryStatus("ready");
    setHistoryError(null);
    setIsWaitingForInitialMessages(false);
    return true;
  } catch (error) {
    if (isInactive(isActive)) return false;
    if (onError) onError(error);
    else console.error("Failed to fetch messages:", error);
    // A failure stays visible, even for a background refresh, so Retry is offered.
    setHistoryStatus("unavailable");
    setHistoryError(error);
    return false;
  } finally {
    const active = !isInactive(isActive);
    if (endSessionFetch(taskSessionId, !silent) === 0) {
      if (!silent) store.getState().setMessagesLoading(taskSessionId, false);
      if (!canFinalizeLoading || canFinalizeLoading()) setIsLoading(false);
    }
    if (active) setIsWaitingForInitialMessages(false);
  }
}
