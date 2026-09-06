import { useCallback, useEffect, useRef } from "react";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { listTaskSessionMessages } from "@/lib/api/domains/session-api";

const OLDER_PROMPT_PAGE_LIMIT = 20;
const EMPTY_PROMPT_META = {
  isLoading: false,
  isLoadingMore: false,
  hasMore: false,
  oldestCursor: null,
};

type PromptRequestState = {
  bySession: Record<string, unknown>;
  metaBySession: Record<string, { isLoading: boolean } | undefined>;
  generationBySession?: Record<string, number>;
  refreshGenerationBySession?: Record<string, number>;
};

function isCurrentPromptState(state: PromptRequestState, sessionId: string, generation: number) {
  return (
    (state.generationBySession?.[sessionId] ?? 0) === generation &&
    state.bySession[sessionId] !== undefined
  );
}

/** Loads older prompt pages independently of transcript pagination. */
export function useLazyLoadPrompts(sessionId: string | null) {
  const meta = useAppStore((state) =>
    sessionId
      ? (state.messagePrompts.metaBySession[sessionId] ?? EMPTY_PROMPT_META)
      : EMPTY_PROMPT_META,
  );
  const stateRef = useRef(meta);
  const requestGenerationRef = useRef<{
    sessionId: string;
    generation: number;
    refreshGeneration: number;
  } | null>(null);
  useEffect(() => {
    stateRef.current = meta;
  }, [meta]);
  const store = useAppStoreApi();

  // eslint-disable-next-line complexity -- request freshness guards keep session pagination race-free.
  const loadMore = useCallback(async () => {
    if (!sessionId) return 0;
    const { hasMore, oldestCursor, isLoading, isLoadingMore } = stateRef.current;
    if (!hasMore || !oldestCursor || isLoading || isLoadingMore) return 0;
    const generation = store.getState().messagePrompts.generationBySession?.[sessionId] ?? 0;
    const refreshGeneration =
      store.getState().messagePrompts.refreshGenerationBySession?.[sessionId] ?? 0;
    requestGenerationRef.current = { sessionId, generation, refreshGeneration };
    store.getState().setPromptMessagesLoadingMore(sessionId, true);
    try {
      const response = await listTaskSessionMessages(sessionId, {
        author_type: "user",
        before: oldestCursor,
        limit: OLDER_PROMPT_PAGE_LIMIT,
        sort: "desc",
      });
      const rows = [...(response.messages ?? [])].reverse();
      const current = store.getState().messagePrompts;
      if (
        isCurrentPromptState(current, sessionId, generation) &&
        (current.refreshGenerationBySession?.[sessionId] ?? 0) === refreshGeneration &&
        !current.metaBySession[sessionId]?.isLoading
      ) {
        store.getState().prependPromptMessages(sessionId, rows, {
          hasMore: response.has_more ?? false,
          oldestCursor: response.cursor ?? oldestCursor,
        });
      }
      return rows.length;
    } finally {
      const current = store.getState().messagePrompts;
      if (isCurrentPromptState(current, sessionId, generation)) {
        store.getState().setPromptMessagesLoadingMore(sessionId, false);
      }
    }
  }, [sessionId, store]);

  const isRequestCurrent = useCallback(() => {
    const request = requestGenerationRef.current;
    return (
      request !== null &&
      request.sessionId === sessionId &&
      request.refreshGeneration ===
        (store.getState().messagePrompts.refreshGenerationBySession?.[sessionId ?? ""] ?? 0) &&
      isCurrentPromptState(store.getState().messagePrompts, sessionId ?? "", request.generation)
    );
  }, [sessionId, store]);
  return { loadMore, hasMore: meta.hasMore, isLoadingMore: meta.isLoadingMore, isRequestCurrent };
}
