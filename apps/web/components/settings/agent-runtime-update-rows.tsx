"use client";

import { useTranslation } from "react-i18next";
import type { ReactNode } from "react";
import { IconLoader2 } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Checkbox } from "@kandev/ui/checkbox";
import type {
  RuntimeBatchTarget,
  RuntimePreviewFailure,
  RuntimeUpdateBatchState,
} from "@/lib/agents/runtime-update-batch";
import type { RuntimeUpdateManagerRow } from "@/lib/agents/runtime-update-manager";
import { RuntimeGuidanceAction } from "./agent-runtime-policies";

export type RuntimeUpdateBulkProps = {
  batch: RuntimeUpdateBatchState;
  reviewRows: RuntimeBatchTarget[];
  reviewKeys: ReadonlySet<string>;
  previewFailures: Record<string, RuntimePreviewFailure>;
  reviewLoading: boolean;
  selection: ReadonlySet<string>;
  allSelected: boolean;
  onToggleSelected: (key: string, selected: boolean) => void;
  onSelectAll: (selected: boolean) => void;
  onStart: (eligibleKeys: ReadonlySet<string>) => void;
  onRetryFailed: () => void;
  onRefreshStatus: () => void;
  onRetryPreviews: () => void;
};

export type RuntimeRowListProps = {
  rows: RuntimeUpdateManagerRow[];
  isMobile: boolean;
  onOpenRuntime: (agentName: string, opener?: HTMLElement) => void;
  individualErrors: Record<string, string | undefined>;
  bulk?: RuntimeUpdateBulkProps;
};

function GuidanceRowActions({
  row,
  manage,
  onOpenRuntime,
}: {
  row: RuntimeUpdateManagerRow;
  manage: ReactNode;
  onOpenRuntime: (agentName: string, opener?: HTMLElement) => void;
}) {
  const { t } = useTranslation();
  return (
    <div className="flex flex-wrap items-center gap-2">
      {row.individualActionable && manage}
      <RuntimeGuidanceAction status={row.status} />
      <Button
        type="button"
        variant="ghost"
        className="min-h-7 cursor-pointer max-md:min-h-11 [@media(pointer:coarse)]:min-h-11"
        onClick={(event) =>
          onOpenRuntime(row.status.agent_name, event.currentTarget as HTMLElement)
        }
        data-testid={`runtime-details-${row.status.agent_name}`}
      >
        {t("agents:runtimeReviewDetails")}
      </Button>
    </div>
  );
}

function RuntimeRowActions({
  row,
  error,
  onOpenRuntime,
}: {
  row: RuntimeUpdateManagerRow;
  error?: string;
  onOpenRuntime: (agentName: string, opener?: HTMLElement) => void;
}) {
  const { t } = useTranslation();
  const { status } = row;
  const manage = (
    <Button
      type="button"
      variant="outline"
      className="min-h-7 cursor-pointer max-md:min-h-11 [@media(pointer:coarse)]:min-h-11"
      onClick={(event) => onOpenRuntime(status.agent_name, event.currentTarget as HTMLElement)}
      data-testid={`runtime-manage-versions-${status.agent_name}`}
    >
      {t(status.managed_fallback ? "agents:runtimeFallbackManage" : "agents:runtimeManage")}
    </Button>
  );
  if (row.group === "external" || row.group === "unavailable") {
    return <GuidanceRowActions row={row} manage={manage} onOpenRuntime={onOpenRuntime} />;
  }
  return (
    <div className="flex flex-wrap items-center gap-2">
      {row.group === "available" && (
        <Button
          type="button"
          className="min-h-7 cursor-pointer max-md:min-h-11 [@media(pointer:coarse)]:min-h-11"
          disabled={row.busy || row.dirtyPolicy}
          onClick={(event) => onOpenRuntime(status.agent_name, event.currentTarget as HTMLElement)}
          data-testid={`runtime-update-${status.agent_name}`}
        >
          {status.update_mode === "self_update"
            ? t("agents:runtimeReviewRuntimeUpdate")
            : t("agents:updateRuntime")}
        </Button>
      )}
      {manage}
      {error && (
        <p className="w-full text-xs text-destructive" role="alert">
          {t("agents:unableToStartUpdate", { error })}
        </p>
      )}
    </div>
  );
}

function rowDescription(row: RuntimeUpdateManagerRow): string | null {
  if (row.busy) return "agents:runtimeMaintenanceInProgress";
  if (row.dirtyPolicy) return "agents:runtimePolicyDirtyBlocked";
  if (row.status.check_state === "unknown") return "agents:runtimeCheckUnknown";
  if (row.status.check_state === "up_to_date") return "agents:runtimeUpToDate";
  return null;
}

const PREVIEW_FAILURE_KEYS: Record<RuntimePreviewFailure["code"], string> = {
  request_failed: "agents:runtimeUpdatePreviewRequestFailed",
  review_unavailable: "agents:runtimeUpdatePreviewUnavailable",
  runtime_changed: "agents:runtimeUpdatePreviewChanged",
  individual_required: "agents:runtimeUpdatePreviewIndividual",
};

function rowBulkIssue(
  row: RuntimeUpdateManagerRow,
  bulk: RuntimeUpdateBulkProps | undefined,
  t: (key: string, values?: Record<string, unknown>) => string,
): string | undefined {
  if (!bulk || row.group !== "available" || !row.bulkEligible) return undefined;
  const failure = bulk.previewFailures[row.key];
  if (failure) {
    return failure.code === "request_failed" && failure.message
      ? t("agents:runtimeUpdatePreviewRequestError", { error: failure.message })
      : t(PREVIEW_FAILURE_KEYS[failure.code]);
  }
  if (bulk.reviewLoading) return t("agents:runtimeUpdateReviewing");
  return undefined;
}

function rowBulkSelection(row: RuntimeUpdateManagerRow, bulk: RuntimeUpdateBulkProps | undefined) {
  if (!bulk || row.group !== "available" || !row.bulkEligible) return undefined;
  const selectable = bulk.reviewKeys.has(row.key);
  return {
    checked: bulk.selection.has(row.key),
    disabled:
      !selectable ||
      Boolean(bulk.previewFailures[row.key]) ||
      bulk.reviewLoading ||
      bulk.batch.status === "running",
    onToggle: (selected: boolean) => bulk.onToggleSelected(row.key, selected),
  };
}

function RuntimeOverviewRow({
  row,
  isMobile,
  onOpenRuntime,
  error,
  bulkSelection,
  bulkIssue,
}: {
  row: RuntimeUpdateManagerRow;
  isMobile: boolean;
  onOpenRuntime: (agentName: string, opener?: HTMLElement) => void;
  error?: string;
  bulkSelection?: {
    checked: boolean;
    disabled: boolean;
    onToggle: (selected: boolean) => void;
  };
  bulkIssue?: string;
}) {
  const { t } = useTranslation();
  const { status } = row;
  const description = rowDescription(row);
  const versions =
    status.check_state === "update_available"
      ? `${status.effective_version || t("agents:runtimeUnknown")} → ${status.latest_version || t("agents:runtimeUnknown")}`
      : t("agents:runtimeEffectiveShort", {
          version: status.effective_version || t("agents:runtimeUnknown"),
        });
  return (
    <li
      className="flex min-w-0 flex-col gap-2 py-3 md:flex-row md:items-center md:justify-between"
      data-testid={`runtime-manager-row-${status.agent_name}`}
    >
      <div className="flex min-w-0 items-start gap-2">
        {bulkSelection && (
          <label className="flex min-h-11 min-w-11 cursor-pointer items-center justify-center md:min-h-7 md:min-w-7 [@media(pointer:coarse)]:min-h-11 [@media(pointer:coarse)]:min-w-11">
            <Checkbox
              checked={bulkSelection.checked}
              disabled={bulkSelection.disabled}
              onCheckedChange={(checked) => bulkSelection.onToggle(checked === true)}
              aria-label={t("agents:runtimeUpdateSelectionRow", {
                agent: status.display_name || status.agent_name,
              })}
              data-testid={`runtime-bulk-select-${status.agent_name}`}
            />
          </label>
        )}
        <div className="min-w-0 space-y-1">
          <div className="flex flex-wrap items-baseline gap-x-3 gap-y-1">
            <span className="text-sm font-semibold">
              {status.display_name || status.agent_name}
            </span>
            <span className="text-xs text-muted-foreground">{versions}</span>
          </div>
          {description && <p className="text-xs text-muted-foreground">{t(description)}</p>}
          {bulkIssue && (
            <p className="break-words text-xs text-amber-600 dark:text-amber-400">{bulkIssue}</p>
          )}
          {row.busy && (
            <p className="flex items-center gap-1.5 text-xs text-muted-foreground" role="status">
              <IconLoader2 className="size-3.5 animate-spin" />
              {t("agents:runtimeMaintenanceInProgress")}
            </p>
          )}
        </div>
      </div>
      <div className={isMobile ? "min-w-0" : "shrink-0"}>
        <RuntimeRowActions row={row} error={error} onOpenRuntime={onOpenRuntime} />
      </div>
    </li>
  );
}

export function RuntimeRowList({ rows, bulk, ...props }: RuntimeRowListProps) {
  const { t } = useTranslation();
  return (
    <ul className="divide-y">
      {rows.map((row) => (
        <RuntimeOverviewRow
          key={row.key}
          row={row}
          isMobile={props.isMobile}
          onOpenRuntime={props.onOpenRuntime}
          error={props.individualErrors[row.key]}
          bulkIssue={rowBulkIssue(row, bulk, t)}
          bulkSelection={rowBulkSelection(row, bulk)}
        />
      ))}
    </ul>
  );
}
