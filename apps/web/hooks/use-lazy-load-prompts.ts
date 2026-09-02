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
  generationBySession?: Record<string, number>;
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
  useEffect(() => {
    stateRef.current = meta;
  }, [meta]);
  const store = useAppStoreApi();

  const loadMore = useCallback(async () => {
    const { hasMore, oldestCursor, isLoadingMore } = stateRef.current;
    if (!sessionId || !hasMore || !oldestCursor || isLoadingMore) return 0;
    const generation = store.getState().messagePrompts.generationBySession?.[sessionId] ?? 0;
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
      if (isCurrentPromptState(current, sessionId, generation)) {
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

  return { loadMore, hasMore: meta.hasMore, isLoadingMore: meta.isLoadingMore };
}
