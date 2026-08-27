import type { MutableRefObject } from "react";

import type { useAppStoreApi } from "@/components/state-provider";
import type { Message } from "@/lib/types/http";

type SessionMessageStore = ReturnType<typeof useAppStoreApi>;

// Multiple lifecycle paths can hydrate the same session concurrently (for
// example, the initial mount and a visibility refresh). Keep the shared
// loading flag asserted until the last operation settles; an older request
// must not make a newer request look idle.
const inFlightFetchesBySession = new Map<string, number>();

function beginSessionFetch(sessionId: string): void {
  inFlightFetchesBySession.set(sessionId, (inFlightFetchesBySession.get(sessionId) ?? 0) + 1);
}

function endSessionFetch(sessionId: string): boolean {
  const remaining = (inFlightFetchesBySession.get(sessionId) ?? 1) - 1;
  if (remaining > 0) {
    inFlightFetchesBySession.set(sessionId, remaining);
    return false;
  }
  inFlightFetchesBySession.delete(sessionId);
  return true;
}

function isInactive(isActive?: () => boolean): boolean {
  return isActive !== undefined && !isActive();
}

type DoFetchMessagesParams = {
  taskSessionId: string;
  store: SessionMessageStore;
  setIsLoading: (value: boolean) => void;
  setIsWaitingForInitialMessages: (value: boolean) => void;
  initialFetchStartRef: MutableRefObject<number | null>;
  lastFetchedSessionIdRef: MutableRefObject<string | null>;
  fetchAndStoreMessages: (
    sessionId: string,
    store: SessionMessageStore,
    isActive?: () => boolean,
  ) => Promise<Message[]>;
  onError?: (error: unknown) => void;
  isActive?: () => boolean;
};

export async function doFetchMessages({
  taskSessionId,
  store,
  setIsLoading,
  setIsWaitingForInitialMessages,
  initialFetchStartRef,
  lastFetchedSessionIdRef,
  fetchAndStoreMessages,
  onError,
  isActive,
}: DoFetchMessagesParams): Promise<void> {
  if (isInactive(isActive)) return;
  beginSessionFetch(taskSessionId);
  setIsLoading(true);
  store.getState().setMessagesLoading(taskSessionId, true);
  if (initialFetchStartRef.current === null) {
    initialFetchStartRef.current = Date.now();
    setIsWaitingForInitialMessages(true);
  }
  try {
    const fetched = await fetchAndStoreMessages(taskSessionId, store, isActive);
    if (isInactive(isActive)) return;
    lastFetchedSessionIdRef.current = taskSessionId;
    if (fetched.length > 0) setIsWaitingForInitialMessages(false);
  } catch (error) {
    if (isInactive(isActive)) return;
    if (onError) onError(error);
    else console.error("Failed to fetch messages:", error);
    store.getState().setMessages(taskSessionId, []);
    lastFetchedSessionIdRef.current = taskSessionId;
  } finally {
    if (endSessionFetch(taskSessionId)) {
      store.getState().setMessagesLoading(taskSessionId, false);
    }
    if (isInactive(isActive)) return;
    setIsLoading(false);
  }
}
