"use client";

import { useCallback } from "react";
import { useTranslation } from "react-i18next";
import { useAppStore } from "@/components/state-provider";
import {
  selectNeedsYouInboxBundles,
  selectNeedsYouInboxHasMore,
  selectNeedsYouInboxHiddenCount,
  selectNeedsYouInboxStatus,
} from "@/lib/state/slices/needs-you-inbox/selectors";
import type { ClarificationInboxBundle } from "@/lib/types/clarification-inbox";
import { NeedsYouInboxRow } from "@/components/needs-you-inbox/needs-you-inbox-row";
import { NeedsYouInboxEmptyState } from "@/components/needs-you-inbox/needs-you-inbox-empty-state";
import { NeedsYouInboxErrorState } from "@/components/needs-you-inbox/needs-you-inbox-error-state";
import { NeedsYouInboxHiddenPanel } from "@/components/needs-you-inbox/needs-you-inbox-hidden-panel";

type ViewMode = "error" | "loading" | "empty" | "list";

// A page reporting truncation while listing zero rows means enrichment
// emptied a page the query had filled, so this resolves to the retryable
// "error" state rather than "empty" (design-01#Data-and-contracts).
function resolveViewMode(
  status: string,
  bundleCount: number,
  hiddenCount: number,
  hasMore: boolean,
): ViewMode {
  if (status === "error" || (bundleCount === 0 && hasMore)) return "error";
  if (status === "loading" && bundleCount === 0 && hiddenCount === 0) return "loading";
  if (bundleCount === 0) return "empty";
  return "list";
}

function NeedsYouInboxList({
  bundles,
  hasMore,
  hiddenCount,
}: {
  bundles: readonly ClarificationInboxBundle[];
  hasMore: boolean;
  hiddenCount: number;
}) {
  const { t } = useTranslation();
  return (
    <>
      <div className="overflow-hidden rounded-lg border border-border divide-y divide-border">
        {bundles.map((bundle) => (
          <NeedsYouInboxRow key={bundle.pending_id} bundle={bundle} />
        ))}
      </div>
      {hasMore && (
        <p className="text-xs text-muted-foreground" data-testid="needs-you-inbox-truncated">
          {t("needsYouInbox:truncatedNotice")}
        </p>
      )}
      {hiddenCount > 0 && <NeedsYouInboxHiddenPanel hiddenCount={hiddenCount} />}
    </>
  );
}

// v1 renders no tab strip and no page title (design-01#Components): the
// layout starts at the toolbar with p-6 space-y-4 and the app top bar owns
// the title.
export function NeedsYouInboxPageClient() {
  const { t } = useTranslation();
  const status = useAppStore(selectNeedsYouInboxStatus);
  const bundles = useAppStore(selectNeedsYouInboxBundles);
  const hiddenCount = useAppStore(selectNeedsYouInboxHiddenCount);
  const hasMore = useAppStore(selectNeedsYouInboxHasMore);
  const bumpRefreshTick = useAppStore((s) => s.bumpNeedsYouInboxRefreshTick);
  const retry = useCallback(() => bumpRefreshTick(), [bumpRefreshTick]);

  const viewMode = resolveViewMode(status, bundles.length, hiddenCount, hasMore);

  return (
    <div className="flex min-h-0 flex-1 flex-col space-y-4 overflow-y-auto p-6">
      {viewMode === "error" && <NeedsYouInboxErrorState onRetry={retry} />}
      {viewMode === "loading" && (
        <p className="text-sm text-muted-foreground" role="status" aria-live="polite">
          {t("common:loading")}
        </p>
      )}
      {viewMode === "empty" && <NeedsYouInboxEmptyState hiddenCount={hiddenCount} />}
      {viewMode === "list" && (
        <NeedsYouInboxList bundles={bundles} hasMore={hasMore} hiddenCount={hiddenCount} />
      )}
    </div>
  );
}
