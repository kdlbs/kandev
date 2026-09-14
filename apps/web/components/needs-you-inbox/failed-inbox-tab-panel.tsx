"use client";

import { useTranslation } from "react-i18next";
import { useAppStore } from "@/components/state-provider";
import {
  selectFailedInboxRows,
  selectFailedInboxStatus,
  selectFailedInboxTruncated,
} from "@/lib/state/slices/failed-inbox/selectors";
import { FailedInboxRow } from "./failed-inbox-row";
import { FailedInboxEmptyState } from "./failed-inbox-empty-state";
import { FailedInboxErrorState } from "./failed-inbox-error-state";

type ViewMode = "error" | "loading" | "empty" | "list";

function resolveViewMode(status: string, rowCount: number): ViewMode {
  if (status === "error") return "error";
  if ((status === "loading" || status === "idle") && rowCount === 0) return "loading";
  if (rowCount === 0) return "empty";
  return "list";
}

export function FailedInboxTabPanel() {
  const { t } = useTranslation();
  const status = useAppStore(selectFailedInboxStatus);
  const rows = useAppStore(selectFailedInboxRows);
  const truncated = useAppStore(selectFailedInboxTruncated);

  const viewMode = resolveViewMode(status, rows.length);

  return (
    <div className="space-y-4">
      {viewMode === "error" && <FailedInboxErrorState />}
      {viewMode === "loading" && (
        <p className="text-sm text-muted-foreground" role="status" aria-live="polite">
          {t("common:loading")}
        </p>
      )}
      {viewMode === "empty" && <FailedInboxEmptyState />}
      {viewMode === "list" && (
        <>
          <div className="overflow-hidden rounded-lg border border-border divide-y divide-border">
            {rows.map((row) => (
              <FailedInboxRow key={row.task_id} row={row} />
            ))}
          </div>
          {truncated && (
            <p className="text-xs text-muted-foreground" data-testid="failed-inbox-truncated">
              {t("failedInbox:truncatedNotice")}
            </p>
          )}
        </>
      )}
    </div>
  );
}
