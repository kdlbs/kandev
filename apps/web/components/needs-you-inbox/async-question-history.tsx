"use client";

import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { AsyncQuestionRow } from "@/components/needs-you-inbox/async-question-row";
import {
  useAsyncQuestionHistory,
  type AsyncQuestionHistoryState,
} from "@/hooks/domains/needs-you-inbox/use-async-question-history";

function AsyncQuestionHistoryView({
  state,
  onRetry,
  onLoadMore,
}: {
  state: AsyncQuestionHistoryState;
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

/**
 * Read-only terminal async questions for the existing History tab. The
 * workspace-scoped hook rejects stale first-page and load-more responses.
 */
export function AsyncQuestionHistory({
  workspaceId,
  refreshTick,
}: {
  workspaceId: string | null;
  refreshTick: number;
}) {
  const { state, retry, loadMore } = useAsyncQuestionHistory(workspaceId, refreshTick);
  if (!workspaceId) return null;
  return (
    <AsyncQuestionHistoryView state={state} onRetry={retry} onLoadMore={() => void loadMore()} />
  );
}
