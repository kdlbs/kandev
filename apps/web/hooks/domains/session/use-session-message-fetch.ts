import type { MutableRefObject } from "react";

import type { useAppStoreApi } from "@/components/state-provider";
import type { Message } from "@/lib/types/http";

type SessionMessageStore = ReturnType<typeof useAppStoreApi>;

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
  autoBackfillUntilUserMessage: (sessionId: string, store: SessionMessageStore) => Promise<void>;
  hasUserOrAgentMessage: (messages: Message[]) => boolean;
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
  autoBackfillUntilUserMessage,
  hasUserOrAgentMessage,
  onError,
  isActive,
}: DoFetchMessagesParams): Promise<void> {
  if (isActive && !isActive()) return;
  setIsLoading(true);
  store.getState().setMessagesLoading(taskSessionId, true);
  if (initialFetchStartRef.current === null) {
    initialFetchStartRef.current = Date.now();
    setIsWaitingForInitialMessages(true);
  }
  try {
    const fetched = await fetchAndStoreMessages(taskSessionId, store, isActive);
    if (isActive && !isActive()) return;
    lastFetchedSessionIdRef.current = taskSessionId;
    if (fetched.length > 0) setIsWaitingForInitialMessages(false);
    if (fetched.length > 0 && !hasUserOrAgentMessage(fetched)) {
      await autoBackfillUntilUserMessage(taskSessionId, store);
    }
  } catch (error) {
    if (isActive && !isActive()) return;
    if (onError) onError(error);
    else console.error("Failed to fetch messages:", error);
    store.getState().setMessages(taskSessionId, []);
    lastFetchedSessionIdRef.current = taskSessionId;
  } finally {
    store.getState().setMessagesLoading(taskSessionId, false);
    if (isActive && !isActive()) return;
    setIsLoading(false);
  }
}
