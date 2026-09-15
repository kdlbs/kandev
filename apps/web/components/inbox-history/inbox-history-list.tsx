"use client";

import { useTranslation } from "react-i18next";
import type { InboxHistoryBundle } from "@/lib/types/inbox-history";
import { InboxHistoryRow } from "./inbox-history-row";

export function InboxHistoryList({
  bundles,
  hasMore,
}: {
  bundles: readonly InboxHistoryBundle[];
  hasMore: boolean;
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
        <p className="text-xs text-muted-foreground" data-testid="inbox-history-truncated">
          {t("inboxHistory:truncatedNotice")}
        </p>
      )}
    </>
  );
}
