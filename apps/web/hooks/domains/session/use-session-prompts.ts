import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { listTaskSessionMessages } from "@/lib/api/domains/session-api";
import type { Message } from "@/lib/types/http";
import { getWebSocketClient } from "@/lib/ws/connection";
const EMPTY_PROMPTS: Message[] = [];
/** Stable empty prompt metadata fallback reused by both prompt selectors. */
const EMPTY_PROMPT_META = {
  isLoading: false,
  isLoadingMore: false,
  hasMore: false,
  oldestCursor: null,
};

type PromptListResponse = {
  messages?: Message[];
  has_more?: boolean;
  cursor?: string | null;
};

type PromptRequest = { promise: Promise<PromptListResponse> };
const inFlightPromptRequests = new Map<string, PromptRequest>();

/** Joins concurrent initial prompt reads and waits for subscription readiness. */
function requestPromptMessages(
  sessionId: string,
  readiness: Promise<unknown> | null,
  generation: number,
): Promise<PromptListResponse> {
  // A removed session can be recreated with the same ID. Keep the old
  // request isolated so the new session does not join a stale snapshot.
  const requestKey = `${sessionId}\u0000${generation}`;
  const existing = inFlightPromptRequests.get(requestKey);
  if (existing) return existing.promise;
  const promise = (readiness ?? Promise.resolve()).then(() =>
    listTaskSessionMessages(sessionId, { author_type: "user", limit: 20, sort: "desc" }),
  );
  const entry = { promise };
  inFlightPromptRequests.set(requestKey, entry);
  void promise.then(
    () => {
      if (inFlightPromptRequests.get(requestKey) === entry)
        inFlightPromptRequests.delete(requestKey);
    },
    () => {
      if (inFlightPromptRequests.get(requestKey) === entry)
        inFlightPromptRequests.delete(requestKey);
    },
  );
  return promise;
}
export type UseSessionPromptsResult = {
  prompts: Message[];
  isLoading: boolean;
  hasMore: boolean;
  oldestCursor: string | null;
  isLoadingMore: boolean;
  fetchFailed: boolean;
  retryPrompts: () => void;
};

/** Loads the prompt-only window without initializing the transcript cache. */
export function useSessionPrompts(sessionId: string | null): UseSessionPromptsResult {
  const prompts = useAppStore((state) =>
    sessionId ? (state.messagePrompts.bySession[sessionId] ?? EMPTY_PROMPTS) : EMPTY_PROMPTS,
  );
  const meta = useAppStore((state) =>
    sessionId
      ? (state.messagePrompts.metaBySession[sessionId] ?? EMPTY_PROMPT_META)
      : EMPTY_PROMPT_META,
  );
  const store = useAppStoreApi();
  const generation = useAppStore((state) =>
    sessionId ? (state.messagePrompts.generationBySession?.[sessionId] ?? 0) : 0,
  );
  const connectionStatus = useAppStore((state) => state.connection.status);
  const readinessRef = useRef<Promise<unknown> | null>(null);
  const [fetchFailed, setFetchFailed] = useState(false);
  const [retryVersion, setRetryVersion] = useState(0);
  const retryPrompts = useCallback(() => setRetryVersion((version) => version + 1), []);

  useEffect(() => {
    if (!sessionId || connectionStatus !== "connected") return;
    const client = getWebSocketClient();
    if (!client) return;
    const subscription = client.subscribeSessionWithReady(sessionId);
    readinessRef.current = subscription.ready;
    return () => {
      readinessRef.current = null;
      subscription.unsubscribe();
    };
  }, [connectionStatus, sessionId]);

  useEffect(() => {
    if (!sessionId) return;
    let current = true;
    setFetchFailed(false);
    const generation = store.getState().messagePrompts.generationBySession?.[sessionId] ?? 0;
    store.getState().setPromptMessagesLoading(sessionId, true);
    void requestPromptMessages(sessionId, readinessRef.current, generation)
      .then((response) => {
        if (!current) return;
        const promptState = store.getState().messagePrompts;
        if ((promptState.generationBySession?.[sessionId] ?? 0) !== generation) return;
        store
          .getState()
          .replacePromptMessages(sessionId, [...(response.messages ?? [])].reverse(), {
            hasMore: response.has_more ?? false,
            oldestCursor: response.cursor ?? null,
          });
      })
      .catch(() => {
        const promptState = store.getState().messagePrompts;
        if (current && (promptState.generationBySession?.[sessionId] ?? 0) === generation) {
          setFetchFailed(true);
        }
      })
      .finally(() => {
        if (!current) return;
        const promptState = store.getState().messagePrompts;
        if ((promptState.generationBySession?.[sessionId] ?? 0) === generation) {
          store.getState().setPromptMessagesLoading(sessionId, false);
        }
      });
    return () => {
      current = false;
    };
  }, [connectionStatus, generation, retryVersion, sessionId, store]);

  return useMemo(
    () => ({
      prompts,
      isLoading: meta.isLoading,
      hasMore: meta.hasMore,
      oldestCursor: meta.oldestCursor,
      isLoadingMore: meta.isLoadingMore,
      fetchFailed,
      retryPrompts,
    }),
    [
      fetchFailed,
      meta.hasMore,
      meta.isLoading,
      meta.isLoadingMore,
      meta.oldestCursor,
      prompts,
      retryPrompts,
    ],
  );
}
