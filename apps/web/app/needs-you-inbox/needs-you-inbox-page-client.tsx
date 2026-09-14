"use client";

import { useCallback } from "react";
import { useTranslation } from "react-i18next";
import { PageShell } from "@/components/page-shell";
import { useAppStore } from "@/components/state-provider";
import { usePathname, useRouter, useSearchParams } from "@/lib/routing/client-router";
import {
  selectNeedsYouInboxBundles,
  selectNeedsYouInboxCount,
  selectNeedsYouInboxHasMore,
  selectNeedsYouInboxHiddenCount,
  selectNeedsYouInboxRevision,
  selectNeedsYouInboxStatus,
} from "@/lib/state/slices/needs-you-inbox/selectors";
import {
  selectFailedInboxCount,
  selectFailedInboxCountIsKnown,
  selectFailedInboxTruncated,
} from "@/lib/state/slices/failed-inbox/selectors";
import type { ClarificationInboxBundle } from "@/lib/types/clarification-inbox";
import { NeedsYouInboxRow } from "@/components/needs-you-inbox/needs-you-inbox-row";
import { NeedsYouInboxEmptyState } from "@/components/needs-you-inbox/needs-you-inbox-empty-state";
import { NeedsYouInboxErrorState } from "@/components/needs-you-inbox/needs-you-inbox-error-state";
import { NeedsYouInboxHiddenPanel } from "@/components/needs-you-inbox/needs-you-inbox-hidden-panel";
import { InboxTabStrip } from "@/components/needs-you-inbox/inbox-tab-strip";
import { FailedInboxTabPanel } from "@/components/needs-you-inbox/failed-inbox-tab-panel";
import { useFailedInboxController } from "@/hooks/domains/failed-inbox/use-failed-inbox-controller";
import { buildInboxTabHref, resolveInboxTab, type InboxTab } from "@/lib/failed-inbox/inbox-tab";

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

function NeedsYouInboxTabContent({ retry }: { retry: () => void }) {
  const { t } = useTranslation();
  const status = useAppStore(selectNeedsYouInboxStatus);
  const bundles = useAppStore(selectNeedsYouInboxBundles);
  const hiddenCount = useAppStore(selectNeedsYouInboxHiddenCount);
  const listRevision = useAppStore(selectNeedsYouInboxRevision);
  const hasMore = useAppStore(selectNeedsYouInboxHasMore);
  const failedCount = useAppStore(selectFailedInboxCount);
  const failedCountKnown = useAppStore(selectFailedInboxCountIsKnown);

  const viewMode = resolveViewMode(status, bundles.length, hiddenCount, hasMore);

  return (
    <>
      {viewMode === "error" && <NeedsYouInboxErrorState onRetry={retry} />}
      {viewMode === "loading" && (
        <p className="text-sm text-muted-foreground" role="status" aria-live="polite">
          {t("common:loading")}
        </p>
      )}
      {viewMode === "empty" && (
        <NeedsYouInboxEmptyState
          hiddenCount={hiddenCount}
          listRevision={listRevision}
          failedCount={failedCount}
          failedCountKnown={failedCountKnown}
        />
      )}
      {viewMode === "list" && (
        <NeedsYouInboxList
          bundles={bundles}
          hasMore={hasMore}
          hiddenCount={hiddenCount}
          listRevision={listRevision}
        />
      )}
    </>
  );
}

// v1 renders no tab strip and no in-page title (design-01#Components). This
// capability adds the tab strip; the page title still belongs to the app top
// bar, which is PageShell's, so this route mounts the same chrome every other
// top-level route does rather than an unlabelled bare div -- that chrome also
// carries the phone nav trigger (design-03#D4).
export function NeedsYouInboxPageClient() {
  const { t } = useTranslation();
  const bumpRefreshTick = useAppStore((s) => s.bumpNeedsYouInboxRefreshTick);
  const needsYouCount = useAppStore(selectNeedsYouInboxCount);
  const needsYouHasMore = useAppStore(selectNeedsYouInboxHasMore);
  const failedCount = useAppStore(selectFailedInboxCount);
  const failedCountKnown = useAppStore(selectFailedInboxCountIsKnown);
  const failedTruncated = useAppStore(selectFailedInboxTruncated);

  const pathname = usePathname();
  const searchParams = useSearchParams();
  const router = useRouter();
  const selectedTab = resolveInboxTab(searchParams);

  // Mounted only here, never app-wide (design-01#Control-flow): the Failed
  // bucket's own refresh triggers, independent of the Needs-you controller
  // mounted at the app shell.
  useFailedInboxController(selectedTab);

  const retry = useCallback(() => bumpRefreshTick(), [bumpRefreshTick]);
  const selectTab = useCallback(
    (tab: InboxTab) => router.replace(buildInboxTabHref(pathname, tab, searchParams)),
    [router, pathname, searchParams],
  );

  return (
    <PageShell title={t("sidebar:inbox")} contentClassName="space-y-4 p-6">
      <InboxTabStrip
        selectedTab={selectedTab}
        onSelectTab={selectTab}
        needsYouCount={needsYouCount}
        needsYouHasMore={needsYouHasMore}
        failedCount={failedCountKnown ? failedCount : undefined}
        failedTruncated={failedTruncated}
      />
      {selectedTab === "failed" ? (
        <FailedInboxTabPanel />
      ) : (
        <NeedsYouInboxTabContent retry={retry} />
      )}
    </PageShell>
  );
}
