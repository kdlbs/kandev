"use client";

import { useCallback } from "react";
import { useTranslation } from "react-i18next";
import { PageShell } from "@/components/page-shell";
import { useAppStore } from "@/components/state-provider";
import {
  selectNeedsYouInboxBundles,
  selectNeedsYouInboxHasMore,
  selectNeedsYouInboxHiddenCount,
  selectNeedsYouInboxRevision,
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
// "error" state rather than "empty" (design-01#Data-and-contracts) -- but
// only once a read has actually applied (`appliedGeneration > 0`): a boot
// seed's truncation flag arrives before any read settles, and must not flash
// as an error.
//
// `status === "loading"` covers both the very first read and every
// background refresh after it, and by itself can't tell them apart -- gate
// the spinner on `appliedGeneration === 0` too, so a refresh over an already
// settled (possibly empty) inbox keeps its settled view instead of reverting
// to the first-load spinner.
function resolveViewMode(
  status: string,
  bundleCount: number,
  hiddenCount: number,
  hasMore: boolean,
  appliedGeneration: number,
): ViewMode {
  if (status === "error" || (bundleCount === 0 && hasMore && appliedGeneration !== 0)) {
    return "error";
  }
  if (status === "loading" && bundleCount === 0 && hiddenCount === 0 && appliedGeneration === 0) {
    return "loading";
  }
  if (bundleCount === 0) return "empty";
  return "list";
}

function NeedsYouInboxList({
  bundles,
  hasMore,
  hiddenCount,
  listRevision,
}: {
  bundles: readonly ClarificationInboxBundle[];
  hasMore: boolean;
  hiddenCount: number;
  listRevision: number;
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
      {hiddenCount > 0 && (
        <NeedsYouInboxHiddenPanel hiddenCount={hiddenCount} listRevision={listRevision} />
      )}
    </>
  );
}

// v1 renders no tab strip and no in-page title (design-01#Components). The
// title belongs to the app top bar, which is PageShell's, so this route mounts
// the same chrome every other top-level route does rather than an unlabelled
// bare div -- that chrome also carries the phone nav trigger (design-03#D4).
export function NeedsYouInboxPageClient() {
  const { t } = useTranslation();
  const status = useAppStore(selectNeedsYouInboxStatus);
  const bundles = useAppStore(selectNeedsYouInboxBundles);
  const hiddenCount = useAppStore(selectNeedsYouInboxHiddenCount);
  const listRevision = useAppStore(selectNeedsYouInboxRevision);
  const hasMore = useAppStore(selectNeedsYouInboxHasMore);
  const bumpRefreshTick = useAppStore((s) => s.bumpNeedsYouInboxRefreshTick);
  const retry = useCallback(() => bumpRefreshTick(), [bumpRefreshTick]);

  const viewMode = resolveViewMode(status, bundles.length, hiddenCount, hasMore, listRevision);

  return (
    <PageShell title={t("sidebar:inbox")} contentClassName="space-y-4 p-6">
      {viewMode === "error" && <NeedsYouInboxErrorState onRetry={retry} />}
      {viewMode === "loading" && (
        <p className="text-sm text-muted-foreground" role="status" aria-live="polite">
          {t("common:loading")}
        </p>
      )}
      {viewMode === "empty" && (
        <NeedsYouInboxEmptyState hiddenCount={hiddenCount} listRevision={listRevision} />
      )}
      {viewMode === "list" && (
        <NeedsYouInboxList
          bundles={bundles}
          hasMore={hasMore}
          hiddenCount={hiddenCount}
          listRevision={listRevision}
        />
      )}
    </PageShell>
  );
}
