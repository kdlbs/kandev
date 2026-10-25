"use client";

import { useTranslation } from "react-i18next";
import { useAppStore } from "@/components/state-provider";
import { NeedsYouInboxHiddenPanel } from "./needs-you-inbox-hidden-panel";

// An empty queue is a normal state, not an achievement: no icon, no
// congratulation, and the workspace is named so the sentence is never read as a
// claim about the whole instance (design-03#D2). Hidden bundles are disclosed
// only when the operator actually has some (AC .20).
export function NeedsYouInboxEmptyState({
  hiddenCount,
  listRevision = 0,
  failedCount,
  failedCountKnown = false,
}: {
  hiddenCount: number;
  listRevision?: number;
  /** The Failed tab's own count, read only for the AC-UI-INBOX-FAILED-001.23
   * clause below -- never written from here (design-01#Count-separation). */
  failedCount?: number;
  failedCountKnown?: boolean;
}) {
  const { t } = useTranslation();
  const workspaceName = useAppStore(
    (s) => s.workspaces?.items?.find((w) => w.id === s.workspaces.activeId)?.name,
  );
  return (
    <div className="space-y-4">
      <div
        className="rounded-lg border border-border p-8 text-center"
        data-testid="needs-you-inbox-empty"
      >
        <p className="text-sm font-medium">
          {workspaceName
            ? t("needsYouInbox:emptyTitle", { workspace: workspaceName })
            : t("needsYouInbox:emptyTitleUnnamedWorkspace")}
        </p>
        <p className="mt-1 text-xs text-muted-foreground">{t("needsYouInbox:emptyDescription")}</p>
        {/* AC-UI-INBOX-FAILED-001.23: omitted while the failed count is not
            known (no read applied yet, or the last one failed), so this never
            asserts presence or absence it cannot back up. */}
        {failedCountKnown && (
          <p className="mt-1 text-xs text-muted-foreground">
            {failedCount && failedCount > 0
              ? t("needsYouInbox:failedClauseSome", { count: failedCount })
              : t("needsYouInbox:failedClauseNone")}
          </p>
        )}
      </div>
      {hiddenCount > 0 && (
        <NeedsYouInboxHiddenPanel hiddenCount={hiddenCount} listRevision={listRevision} />
      )}
    </div>
  );
}
