"use client";

import { useCallback } from "react";
import { useTranslation } from "react-i18next";
import { PageShell } from "@/components/page-shell";
import { useAppStore } from "@/components/state-provider";
import {
  selectNeedsYouInboxBundles,
  selectNeedsYouInboxHasMore,
  selectNeedsYouInboxHiddenCount,
  selectNeedsYouInboxLastAppliedOk,
  selectNeedsYouInboxRevision,
  selectNeedsYouInboxStatus,
} from "@/lib/state/slices/needs-you-inbox/selectors";
import type { NeedsYouInboxReadStatus } from "@/lib/state/slices/needs-you-inbox/types";
import type { ClarificationInboxBundle } from "@/lib/types/clarification-inbox";
import { NeedsYouInboxRow } from "@/components/needs-you-inbox/needs-you-inbox-row";
import { NeedsYouInboxEmptyState } from "@/components/needs-you-inbox/needs-you-inbox-empty-state";
import { NeedsYouInboxErrorState } from "@/components/needs-you-inbox/needs-you-inbox-error-state";
import { NeedsYouInboxHiddenPanel } from "@/components/needs-you-inbox/needs-you-inbox-hidden-panel";

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
  const lastAppliedOk = useAppStore(selectNeedsYouInboxLastAppliedOk);
  const hasActiveWorkspace = useAppStore((s) => s.workspaces.activeId !== null);
  const bumpRefreshTick = useAppStore((s) => s.bumpNeedsYouInboxRefreshTick);
  const retry = useCallback(() => bumpRefreshTick(), [bumpRefreshTick]);

  const viewMode = resolveViewMode(
    status,
    bundles.length,
    hasMore,
    lastAppliedOk,
    hasActiveWorkspace,
  );

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
