"use client";

import { useCallback, useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { listAgentQuestions } from "@/lib/api/domains/agent-questions-api";
import { AsyncQuestionRow } from "@/components/needs-you-inbox/async-question-row";
import type { ClarificationInboxAsyncQuestion } from "@/lib/types/clarification-inbox";

type ReadState = {
  status: "loading" | "ready" | "error";
  questions: ClarificationInboxAsyncQuestion[];
  hasMore: boolean;
  nextCursor?: string;
  loadingMore?: boolean;
  loadMoreFailed?: boolean;
};

/**
 * Read-only terminal async questions for the existing History tab. Answered
 * and withdrawn requests keep their outcome and reason; they are never
 * actionable.
 */
export function AsyncQuestionHistory({
  workspaceId,
  refreshTick,
}: {
  workspaceId: string | null;
  refreshTick: number;
}) {
  const [state, setState] = useState<ReadState>({
    status: "loading",
    questions: [],
    hasMore: false,
  });
  const [retryTick, setRetryTick] = useState(0);

  const load = useCallback(
    async (signal: AbortSignal) => {
      if (!workspaceId) {
        setState({ status: "ready", questions: [], hasMore: false });
        return;
      }
      setState((current) => ({ ...current, status: "loading" }));
      try {
        const page = await listAgentQuestions(
          workspaceId,
          { status: "history", limit: 50 },
          { init: { signal } },
        );
        if (signal.aborted) return;
        setState({
          status: "ready",
          questions: page.questions,
          hasMore: page.has_more,
          nextCursor: page.next_cursor,
        });
      } catch {
        if (signal.aborted) return;
        setState({ status: "error", questions: [], hasMore: false });
      }
    },
    [workspaceId],
  );

  const loadMore = useCallback(async () => {
    if (!workspaceId || !state.nextCursor || state.loadingMore) return;
    setState((current) => ({ ...current, loadingMore: true, loadMoreFailed: false }));
    try {
      const page = await listAgentQuestions(workspaceId, {
        status: "history",
        limit: 50,
        cursor: state.nextCursor,
      });
      setState((current) => ({
        ...current,
        status: "ready",
        questions: [...current.questions, ...page.questions],
        hasMore: page.has_more,
        nextCursor: page.next_cursor,
        loadingMore: false,
      }));
    } catch {
      setState((current) => ({ ...current, loadingMore: false, loadMoreFailed: true }));
    }
  }, [state.loadingMore, state.nextCursor, workspaceId]);

  useEffect(() => {
    const controller = new AbortController();
    void load(controller.signal);
    return () => controller.abort();
  }, [load, refreshTick, retryTick]);

  if (!workspaceId) return null;
  return (
    <AsyncQuestionHistoryView
      state={state}
      onRetry={() => setRetryTick((value) => value + 1)}
      onLoadMore={() => void loadMore()}
    />
  );
}

function AsyncQuestionHistoryView({
  state,
  onRetry,
  onLoadMore,
}: {
  state: ReadState;
  onRetry: () => void;
  onLoadMore: () => void;
}) {
  const { t } = useTranslation();
  if (state.status === "error") {
    return (
      <div className="flex items-center justify-between gap-2 text-sm text-muted-foreground">
        <span>{t("needsYouInbox:asyncHistoryReadFailed")}</span>
        <Button
          type="button"
          variant="outline"
          size="sm"
          className="cursor-pointer"
          onClick={onRetry}
        >
          {t("needsYouInbox:retry")}
        </Button>
      </div>
    );
  }
  if (state.questions.length === 0) return null;
  return (
    <section className="space-y-2" data-testid="async-question-history">
      <h3 className="text-sm font-semibold">{t("needsYouInbox:asyncHistoryTitle")}</h3>
      <div className="overflow-hidden rounded-lg border border-border divide-y divide-border">
        {state.questions.map((question) => (
          <AsyncQuestionRow key={question.question_id} question={question} onSettled={() => {}} />
        ))}
      </div>
      {state.hasMore && (
        <Button
          type="button"
          variant="outline"
          size="sm"
          className="cursor-pointer"
          disabled={state.loadingMore}
          onClick={onLoadMore}
          data-testid="async-question-history-load-more"
        >
          {state.loadingMore
            ? t("needsYouInbox:asyncSubmitting")
            : t("needsYouInbox:asyncLoadMore")}
        </Button>
      )}
      {state.loadMoreFailed && (
        <p className="text-xs text-destructive">{t("needsYouInbox:asyncHistoryReadFailed")}</p>
      )}
    </section>
  );
}
