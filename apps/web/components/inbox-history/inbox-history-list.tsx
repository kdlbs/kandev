"use client";

import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import type { InboxHistoryBundle } from "@/lib/types/inbox-history";
import { InboxHistoryRow } from "./inbox-history-row";

export function InboxHistoryList({
  bundles,
  hasMore,
  isLoadingMore,
  loadMoreError,
  onLoadMore,
}: {
  bundles: readonly InboxHistoryBundle[];
  hasMore: boolean;
  isLoadingMore: boolean;
  loadMoreError: boolean;
  onLoadMore: () => void;
}) {
  const { t } = useTranslation();
  return (
    <>
      <div className="overflow-hidden rounded-lg border border-border divide-y divide-border">
        {bundles.map((bundle) => (
          <InboxHistoryRow key={bundle.pending_id} bundle={bundle} />
        ))}
      </div>
      {hasMore && (
        <div className="flex flex-col items-start gap-2 pt-2">
          <p className="text-xs text-muted-foreground" data-testid="inbox-history-truncated">
            {t("inboxHistory:truncatedNotice")}
          </p>
          <Button
            type="button"
            variant="outline"
            size="sm"
            className="max-md:min-h-11 [@media(pointer:coarse)]:min-h-11"
            onClick={onLoadMore}
            disabled={isLoadingMore}
            aria-busy={isLoadingMore}
            data-testid="inbox-history-load-more"
          >
            {isLoadingMore ? t("common:loading") : t("inboxHistory:loadMore")}
          </Button>
          {loadMoreError && (
            <p
              className="text-xs text-destructive"
              role="alert"
              data-testid="inbox-history-load-more-error"
            >
              {t("inboxHistory:loadMoreError")}
            </p>
          )}
        </div>
      )}
    </>
  );
}
