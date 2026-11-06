import { useCallback, useEffect, useRef, useState } from "react";
import { listAgentQuestions } from "@/lib/api/domains/agent-questions-api";
import type { ClarificationInboxAsyncQuestion } from "@/lib/types/clarification-inbox";

export type AsyncQuestionHistoryState = {
  status: "loading" | "ready" | "error";
  questions: ClarificationInboxAsyncQuestion[];
  hasMore: boolean;
  nextCursor?: string;
  loadingMore?: boolean;
  loadMoreFailed?: boolean;
};

const initialState: AsyncQuestionHistoryState = {
  status: "loading",
  questions: [],
  hasMore: false,
};

/**
 * Workspace-scoped history read. Late first-page or load-more responses are
 * rejected by workspace and generation identity so a workspace switch or
 * refresh cannot append stale rows or replace the current cursor.
 */
export function useAsyncQuestionHistory(workspaceId: string | null, refreshTick: number) {
  const [state, setState] = useState<AsyncQuestionHistoryState>(initialState);
  const [retryTick, setRetryTick] = useState(0);
  const generationRef = useRef(0);
  const workspaceRef = useRef(workspaceId);
  workspaceRef.current = workspaceId;

  const load = useCallback(
    async (signal: AbortSignal) => {
      if (!workspaceId) {
        setState({ status: "ready", questions: [], hasMore: false });
        return;
      }
      const generation = ++generationRef.current;
      setState((current) => ({ ...current, status: "loading" }));
      try {
        const page = await listAgentQuestions(
          workspaceId,
          { status: "history", limit: 50 },
          { init: { signal } },
        );
        if (
          signal.aborted ||
          workspaceRef.current !== workspaceId ||
          generationRef.current !== generation
        ) {
          return;
        }
        setState({
          status: "ready",
          questions: page.questions,
          hasMore: page.has_more,
          nextCursor: page.next_cursor,
        });
      } catch {
        if (
          signal.aborted ||
          workspaceRef.current !== workspaceId ||
          generationRef.current !== generation
        ) {
          return;
        }
        setState({ status: "error", questions: [], hasMore: false });
      }
    },
    [workspaceId],
  );

  useEffect(() => {
    const controller = new AbortController();
    void load(controller.signal);
    return () => controller.abort();
  }, [load, refreshTick, retryTick]);

  const retry = useCallback(() => setRetryTick((value) => value + 1), []);

  const loadMore = useCallback(async () => {
    if (!workspaceId || !state.nextCursor || state.loadingMore) return;
    const generation = generationRef.current;
    const cursor = state.nextCursor;
    setState((current) => ({ ...current, loadingMore: true, loadMoreFailed: false }));
    try {
      const page = await listAgentQuestions(workspaceId, {
        status: "history",
        limit: 50,
        cursor,
      });
      if (workspaceRef.current !== workspaceId || generationRef.current !== generation) {
        return;
      }
      setState((current) => ({
        ...current,
        status: "ready",
        questions: [...current.questions, ...page.questions],
        hasMore: page.has_more,
        nextCursor: page.next_cursor,
        loadingMore: false,
      }));
    } catch {
      if (workspaceRef.current !== workspaceId || generationRef.current !== generation) {
        return;
      }
      setState((current) => ({ ...current, loadingMore: false, loadMoreFailed: true }));
    }
  }, [state.loadingMore, state.nextCursor, workspaceId]);

  return { state, retry, loadMore };
}
