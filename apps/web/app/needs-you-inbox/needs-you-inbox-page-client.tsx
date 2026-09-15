"use client";

import { useCallback } from "react";
import { useTranslation } from "react-i18next";
import { TabsContent } from "@kandev/ui/tabs";
import { PageShell } from "@/components/page-shell";
import { useAppStore } from "@/components/state-provider";
import { usePathname, useRouter, useSearchParams } from "@/lib/routing/client-router";
import {
  selectNeedsYouInboxBundles,
  selectNeedsYouInboxCount,
  selectNeedsYouInboxHasMore,
  selectNeedsYouInboxHiddenCount,
  selectNeedsYouInboxLastAppliedOk,
  selectNeedsYouInboxRevision,
  selectNeedsYouInboxStatus,
} from "@/lib/state/slices/needs-you-inbox/selectors";
import {
  selectFailedInboxCount,
  selectFailedInboxCountIsKnown,
  selectFailedInboxTruncated,
} from "@/lib/state/slices/failed-inbox/selectors";
import type { NeedsYouInboxReadStatus } from "@/lib/state/slices/needs-you-inbox/types";
import type { ClarificationInboxBundle } from "@/lib/types/clarification-inbox";
import { NeedsYouInboxRow } from "@/components/needs-you-inbox/needs-you-inbox-row";
import { NeedsYouInboxEmptyState } from "@/components/needs-you-inbox/needs-you-inbox-empty-state";
import { NeedsYouInboxErrorState } from "@/components/needs-you-inbox/needs-you-inbox-error-state";
import { NeedsYouInboxHiddenPanel } from "@/components/needs-you-inbox/needs-you-inbox-hidden-panel";
import { InboxTabStrip } from "@/components/needs-you-inbox/inbox-tab-strip";
import { FailedInboxTabPanel } from "@/components/needs-you-inbox/failed-inbox-tab-panel";
import { useFailedInboxController } from "@/hooks/domains/failed-inbox/use-failed-inbox-controller";
import { buildInboxTabHref, resolveInboxTab, type InboxTab } from "@/lib/failed-inbox/inbox-tab";
import { useInboxHistoryController } from "@/hooks/domains/inbox-history/use-inbox-history-controller";
import {
  selectInboxHistoryBundles,
  selectInboxHistoryCount,
  selectInboxHistoryHasMore,
  selectInboxHistoryStatus,
} from "@/lib/state/slices/inbox-history/selectors";
import { InboxHistoryList } from "@/components/inbox-history/inbox-history-list";
import { InboxHistoryEmptyState } from "@/components/inbox-history/inbox-history-empty-state";
import { InboxHistoryErrorState } from "@/components/inbox-history/inbox-history-error-state";

type ViewMode = "error" | "loading" | "empty" | "list";

// `status` alone can't distinguish a refresh after success from one after a
// failure, since `beginNeedsYouInboxRead` overwrites it to "loading" without
// touching what preceded it; `lastAppliedOk` carries that distinction and
// gates the loading view. `hasActiveWorkspace` short-circuits to the empty
// view, since with no active workspace no controller trigger ever applies a
// response and `lastAppliedOk` can never flip. A truncated page with zero
// rows is enrichment having emptied a filled page, not an empty inbox, so it
// resolves to the retryable error view instead.
function resolveViewMode(
  status: NeedsYouInboxReadStatus,
  bundleCount: number,
  hasMore: boolean,
  lastAppliedOk: boolean,
  hasActiveWorkspace: boolean,
): ViewMode {
  if (!hasActiveWorkspace) return "empty";
  if ((status === "loading" || status === "idle") && !lastAppliedOk) {
    return "loading";
  }
  if (status === "error") return "error";
  if (bundleCount === 0 && hasMore) return "error";
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

// "Needs you" is the tab strip's default-selected tab, and this content,
// count and behavior are unchanged by the strip's presence.
function NeedsYouInboxTabContent({ retry }: { retry: () => void }) {
  const { t } = useTranslation();
  const status = useAppStore(selectNeedsYouInboxStatus);
  const bundles = useAppStore(selectNeedsYouInboxBundles);
  const hiddenCount = useAppStore(selectNeedsYouInboxHiddenCount);
  const listRevision = useAppStore(selectNeedsYouInboxRevision);
  const hasMore = useAppStore(selectNeedsYouInboxHasMore);
  const failedCount = useAppStore(selectFailedInboxCount);
  const failedCountKnown = useAppStore(selectFailedInboxCountIsKnown);
  const lastAppliedOk = useAppStore(selectNeedsYouInboxLastAppliedOk);
  const hasActiveWorkspace = useAppStore((s) => s.workspaces.activeId !== null);

  const viewMode = resolveViewMode(
    status,
    bundles.length,
    hasMore,
    lastAppliedOk,
    hasActiveWorkspace,
  );

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

// A truncated-to-zero page (bundleCount === 0 with hasMore) still resolves
// to the retryable error state, matching the Needs-you sibling's own
// handling of the same shape.
function resolveHistoryViewMode(status: string, bundleCount: number, hasMore: boolean): ViewMode {
  if (status === "error" || (bundleCount === 0 && hasMore)) return "error";
  if (status === "loading" && bundleCount === 0) return "loading";
  if (bundleCount === 0) return "empty";
  return "list";
}

// The History tab's own read-only content, driven by its own isolated
// slice -- never the Needs-you slice or its refresh trigger. Takes
// `refresh` from the parent rather than calling useInboxHistoryController
// itself, so the controller (and its single read) is mounted exactly once
// per page, not once per tab activation.
function InboxHistoryTabContent({ refresh }: { refresh: (workspaceId: string) => Promise<void> }) {
  const { t } = useTranslation();
  const status = useAppStore(selectInboxHistoryStatus);
  const bundles = useAppStore(selectInboxHistoryBundles);
  const hasMore = useAppStore(selectInboxHistoryHasMore);
  const workspaceId = useAppStore((s) => s.workspaces.activeId);
  const retry = useCallback(() => {
    if (workspaceId) void refresh(workspaceId);
  }, [refresh, workspaceId]);

  const viewMode = resolveHistoryViewMode(status, bundles.length, hasMore);

  return (
    <>
      {viewMode === "error" && <InboxHistoryErrorState onRetry={retry} />}
      {viewMode === "loading" && (
        <p className="text-sm text-muted-foreground" role="status" aria-live="polite">
          {t("common:loading")}
        </p>
      )}
      {viewMode === "empty" && <InboxHistoryEmptyState />}
      {viewMode === "list" && <InboxHistoryList bundles={bundles} hasMore={hasMore} />}
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
  // The History tab's own bundle count, populated the moment the Inbox page
  // opens, without requiring a click into the tab.
  const historyCount = useAppStore(selectInboxHistoryCount);

  const pathname = usePathname();
  const searchParams = useSearchParams();
  const router = useRouter();
  const selectedTab = resolveInboxTab(searchParams);

  // Mounted only here, never app-wide (design-01#Control-flow): the Failed
  // bucket's own refresh triggers, independent of the Needs-you controller
  // mounted at the app shell.
  useFailedInboxController(selectedTab);
  // The History tab's own single read, driven by its own isolated slice --
  // mounted once at the page level (not gated by which tab is active), never
  // the Needs-you slice or its refresh trigger.
  const refreshHistory = useInboxHistoryController();

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
        historyCount={historyCount}
      >
        <TabsContent value="needs-you">
          <NeedsYouInboxTabContent retry={retry} />
        </TabsContent>
        <TabsContent value="failed">
          <FailedInboxTabPanel />
        </TabsContent>
        <TabsContent value="history">
          <InboxHistoryTabContent refresh={refreshHistory} />
        </TabsContent>
      </InboxTabStrip>
    </PageShell>
  );
}
