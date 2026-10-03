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

type PromptRequest = { promise: Promise<PromptListResponse>; readiness: Promise<unknown> | null };
const inFlightPromptRequests = new Map<string, PromptRequest>();

/** Joins concurrent initial prompt reads and waits for subscription readiness. */
function requestPromptMessages(
  sessionId: string,
  readiness: Promise<unknown> | null,
  generation: number,
  refreshGeneration: number,
): Promise<PromptListResponse> {
  // A removed session can be recreated with the same ID. Keep the old
  // request isolated so the new session does not join a stale snapshot.
  const requestKey = `${sessionId}\u0000${generation}\u0000${refreshGeneration}`;
  const existing = inFlightPromptRequests.get(requestKey);
  if (existing && existing.readiness === readiness) return existing.promise;
  const promise = (readiness ?? Promise.resolve()).then(() =>
    listTaskSessionMessages(sessionId, { author_type: "user", limit: 20, sort: "desc" }),
  );
  const entry = { promise, readiness };
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
function usePromptSubscriptionReadiness(sessionId: string | null, connectionStatus: string) {
  const readinessRef = useRef<Promise<unknown> | null>(null);
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
  return readinessRef;
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
export function useSessionPrompts(
  sessionId: string | null,
  { firstLoadOnly = false }: { firstLoadOnly?: boolean } = {},
): UseSessionPromptsResult {
  const prompts = useAppStore((state) =>
    sessionId ? (state.messagePrompts.bySession[sessionId] ?? EMPTY_PROMPTS) : EMPTY_PROMPTS,
  );
  const meta = useAppStore(
    (state) => (sessionId && state.messagePrompts.metaBySession[sessionId]) || EMPTY_PROMPT_META,
  );
  const store = useAppStoreApi();
  const generation = useAppStore((state) =>
    sessionId ? (state.messagePrompts.generationBySession?.[sessionId] ?? 0) : 0,
  );
  const connectionStatus = useAppStore((state) => state.connection.status);
  const readinessRef = usePromptSubscriptionReadiness(sessionId, connectionStatus);
  const [fetchFailed, setFetchFailed] = useState(false);
  const [firstLoadLoading, setFirstLoadLoading] = useState(false);
  const [retryVersion, setRetryVersion] = useState(0);
  const retryPrompts = useCallback(() => setRetryVersion((version) => version + 1), []);

  useEffect(() => {
    if (!sessionId) return;
    if (firstLoadOnly && store.getState().messagePrompts.authoritativeBySession?.[sessionId])
      return;
    let current = true;
    const generation = store.getState().messagePrompts.generationBySession?.[sessionId] ?? 0;
    setFetchFailed(false);
    if (firstLoadOnly) setFirstLoadLoading(true);
    else store.getState().setPromptMessagesLoading(sessionId, true);
    const refreshGeneration =
      store.getState().messagePrompts.refreshGenerationBySession?.[sessionId] ?? 0;
    void requestPromptMessages(sessionId, readinessRef.current, generation, refreshGeneration)
      .then((response) => {
        if (!current) return;
        const promptState = store.getState().messagePrompts;
        if ((promptState.generationBySession?.[sessionId] ?? 0) !== generation) return;
        if (
          !firstLoadOnly &&
          (promptState.refreshGenerationBySession?.[sessionId] ?? 0) !== refreshGeneration
        ) {
          setRetryVersion((version) => version + 1);
          return;
        }
        const rows = [...(response.messages ?? [])].reverse();
        const metadata = {
          hasMore: response.has_more ?? false,
          oldestCursor: response.cursor ?? null,
        };
        if (firstLoadOnly)
          store.getState().installAuthoritativePromptMessages(sessionId, rows, metadata);
        else store.getState().replacePromptMessages(sessionId, rows, metadata);
      })
      .catch(() => {
        const promptState = store.getState().messagePrompts;
        if (current && (promptState.generationBySession?.[sessionId] ?? 0) === generation) {
          if (
            !firstLoadOnly &&
            (promptState.refreshGenerationBySession?.[sessionId] ?? 0) !== refreshGeneration
          ) {
            setRetryVersion((version) => version + 1);
          } else {
            setFetchFailed(true);
          }
        }
      })
      .finally(() => {
        if (!current) return;
        const promptState = store.getState().messagePrompts;
        if ((promptState.generationBySession?.[sessionId] ?? 0) === generation) {
          if (firstLoadOnly) setFirstLoadLoading(false);
          else store.getState().setPromptMessagesLoading(sessionId, false);
        }
      });
    return () => {
      current = false;
    };
  }, [connectionStatus, firstLoadOnly, generation, retryVersion, sessionId, store]);

  return useMemo(
    () => ({
      prompts,
      isLoading: firstLoadOnly ? firstLoadLoading : meta.isLoading,
      hasMore: meta.hasMore,
      oldestCursor: meta.oldestCursor,
      isLoadingMore: meta.isLoadingMore,
      fetchFailed,
      retryPrompts,
    }),
    [
      firstLoadLoading,
      firstLoadOnly,
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
