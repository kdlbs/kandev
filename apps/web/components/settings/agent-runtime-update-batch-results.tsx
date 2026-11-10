"use client";

import { useTranslation } from "react-i18next";
import { IconAlertTriangle, IconLoader2 } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import {
  batchCounts,
  type RuntimeBatchRow,
  type RuntimeUpdateBatchState,
} from "@/lib/agents/runtime-update-batch";

function phaseLabelKey(row: RuntimeBatchRow): string {
  switch (row.phase) {
    case "submitting":
      return "agents:runtimeUpdateStatusSubmitting";
    case "queued":
      return "agents:runtimeUpdateStatusQueued";
    case "running":
      return "agents:runtimeUpdateStatusRunning";
    case "succeeded":
      return "agents:runtimeUpdateStatusSucceeded";
    case "unconfirmed":
      return "agents:runtimeUpdateStatusUnconfirmed";
    case "failed":
      return "agents:runtimeUpdateStatusFailed";
    default:
      return "agents:runtimeUpdateStatusQueued";
  }
}

function rowCauseKey(row: RuntimeBatchRow): string | null {
  if (row.phase !== "failed" && row.phase !== "unconfirmed") return null;
  if (row.errorCode === "review_changed") return "agents:runtimeUpdateReviewChanged";
  if (row.errorCode === "maintenance") return "agents:runtimeUpdateMaintenanceConflict";
  if (row.errorCode === "authority") return "agents:runtimeUpdateAuthorityChanged";
  if (row.phase === "unconfirmed") return "agents:runtimeUpdateResultUnconfirmed";
  return null;
}

function BatchRow({
  row,
  onOpenRuntime,
}: {
  row: RuntimeBatchRow;
  onOpenRuntime: (agentName: string) => void;
}) {
  const { t } = useTranslation();
  const causeKey = rowCauseKey(row);
  const active = row.phase === "submitting" || row.phase === "queued" || row.phase === "running";
  return (
    <li
      className="flex min-w-0 flex-col gap-1 py-2 md:flex-row md:items-start md:justify-between"
      data-testid={`runtime-batch-row-${row.agentName}`}
    >
      <div className="min-w-0 space-y-1">
        <div className="flex flex-wrap items-baseline gap-x-3 gap-y-1">
          <span className="text-sm font-semibold">{row.displayName}</span>
          <span className="text-xs text-muted-foreground">
            {row.currentVersion} → {row.targetVersion}
          </span>
        </div>
        <p
          className={
            row.phase === "failed" || row.phase === "unconfirmed"
              ? "text-xs text-destructive"
              : "flex items-center gap-1.5 text-xs text-muted-foreground"
          }
          role={row.phase === "failed" ? "alert" : "status"}
        >
          {active && <IconLoader2 className="size-3.5 shrink-0 animate-spin" />}
          {t(phaseLabelKey(row))}
        </p>
        {causeKey && <p className="text-xs text-muted-foreground">{t(causeKey)}</p>}
        {row.error && (
          <p className="flex items-start gap-1.5 text-xs text-destructive">
            <IconAlertTriangle className="mt-0.5 size-3.5 shrink-0" />
            {row.error}
          </p>
        )}
        {row.refreshError && (
          <p className="text-xs text-amber-600 dark:text-amber-400">
            {t("agents:runtimeUpdatedRefreshFailed", { error: row.refreshError })}
          </p>
        )}
      </div>
      <Button
        type="button"
        variant="ghost"
        size="sm"
        className="cursor-pointer self-start max-md:min-h-11 [@media(pointer:coarse)]:min-h-11"
        onClick={() => onOpenRuntime(row.agentName)}
        data-testid={`runtime-batch-details-${row.agentName}`}
      >
        {t("agents:runtimeUpdateDetails")}
      </Button>
    </li>
  );
}

/**
 * Live per-runtime outcomes for one approved bulk run. Accepted or queued is
 * progress, never success; an unconfirmed POST never becomes a retry.
 */
export function RuntimeUpdateBatchResults({
  batch,
  onRetryFailed,
  onRefreshStatus,
  onOpenRuntime,
  onClose,
  onReviewAgain,
  showClose = true,
}: {
  batch: RuntimeUpdateBatchState;
  onRetryFailed: () => void;
  onRefreshStatus: () => void;
  onOpenRuntime: (agentName: string) => void;
  onClose: () => void;
  onReviewAgain: () => void;
  showClose?: boolean;
}) {
  const { t } = useTranslation();
  const counts = batchCounts(batch);
  const rows = batch.order.map((key) => batch.rows[key]).filter(Boolean);
  const summary =
    counts.pending > 0
      ? t("agents:runtimeUpdateBatchProgress", {
          updated: counts.succeeded,
          failed: counts.failed,
          pending: counts.pending,
        })
      : t("agents:runtimeUpdateBatchComplete", {
          updated: counts.succeeded,
          failed: counts.failed,
        });
  return (
    <div className="space-y-3" data-testid="runtime-batch-results">
      <ul className="divide-y">
        {rows.map((row) => (
          <BatchRow key={row.key} row={row} onOpenRuntime={onOpenRuntime} />
        ))}
      </ul>
      {counts.unconfirmed > 0 && (
        <p className="rounded-md border border-amber-500/30 bg-amber-500/5 p-2 text-xs text-amber-700 dark:text-amber-400">
          {t("agents:runtimeUpdateResultUnconfirmed")}
        </p>
      )}
      <div className="flex flex-wrap items-center justify-between gap-2">
        <p className="text-sm" role="status" data-testid="runtime-batch-summary">
          {summary}
        </p>
        <div className="flex flex-wrap gap-2">
          {counts.unconfirmed > 0 && (
            <Button
              type="button"
              variant="outline"
              size="sm"
              className="cursor-pointer max-md:min-h-11 [@media(pointer:coarse)]:min-h-11"
              onClick={onRefreshStatus}
              data-testid="runtime-batch-refresh"
            >
              {t("agents:runtimeUpdateRefreshStatus")}
            </Button>
          )}
          {batch.status === "complete" && (
            <Button
              type="button"
              variant="outline"
              size="sm"
              className="cursor-pointer max-md:min-h-11 [@media(pointer:coarse)]:min-h-11"
              onClick={onReviewAgain}
              data-testid="runtime-batch-review-again"
            >
              {t("agents:runtimeReviewUpdates")}
            </Button>
          )}
          {counts.failed > 0 && batch.status !== "running" && (
            <Button
              type="button"
              size="sm"
              className="cursor-pointer max-md:min-h-11 [@media(pointer:coarse)]:min-h-11"
              onClick={onRetryFailed}
              data-testid="runtime-batch-retry-failed"
            >
              {t("agents:runtimeUpdateRetryFailed", { count: counts.failed })}
            </Button>
          )}
          {showClose && (
            <Button
              type="button"
              variant="outline"
              size="sm"
              className="cursor-pointer max-md:min-h-11 [@media(pointer:coarse)]:min-h-11"
              onClick={onClose}
              data-testid="runtime-batch-close"
            >
              {t("common:close")}
            </Button>
          )}
        </div>
      </div>
    </div>
  );
}
