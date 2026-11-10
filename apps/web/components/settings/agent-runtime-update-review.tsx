"use client";

import { useTranslation } from "react-i18next";
import { IconLoader2 } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Checkbox } from "@kandev/ui/checkbox";
import { CopilotRuntimeCompatibilityNotice } from "./copilot-runtime-compatibility";
import type { RuntimeUpdateBulkProps } from "./agent-runtime-update-rows";

export function RuntimeReviewHeader({
  bulk,
  loading,
}: {
  bulk: RuntimeUpdateBulkProps;
  loading: boolean;
}) {
  const { t } = useTranslation();
  const failures = Object.keys(bulk.previewFailures).length;
  return (
    <div className="space-y-2">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <label className="flex min-h-11 cursor-pointer items-center gap-2 text-sm md:min-h-7 [@media(pointer:coarse)]:min-h-11">
          <Checkbox
            checked={bulk.allSelected}
            disabled={bulk.reviewKeys.size === 0 || loading || bulk.reviewLoading}
            onCheckedChange={(checked) => bulk.onSelectAll(checked === true)}
            data-testid="runtime-bulk-select-all"
          />
          {t("agents:runtimeUpdateSelectAllLabel", { count: bulk.reviewKeys.size })}
        </label>
        {bulk.reviewLoading && (
          <span className="flex items-center gap-1.5 text-xs text-muted-foreground" role="status">
            <IconLoader2 className="size-3.5 animate-spin" />
            {t("agents:runtimeUpdateReviewing")}
          </span>
        )}
      </div>
      {failures > 0 && (
        <div
          className="space-y-2 rounded-md border border-amber-500/30 bg-amber-500/5 p-2"
          role="alert"
        >
          <p className="text-xs text-amber-700 dark:text-amber-400">
            {t("agents:runtimeUpdatePreviewFailed", { count: failures })}
          </p>
          <Button
            type="button"
            variant="outline"
            size="sm"
            className="cursor-pointer max-md:min-h-11 [@media(pointer:coarse)]:min-h-11"
            onClick={bulk.onRetryPreviews}
            data-testid="runtime-bulk-retry-preview"
          >
            {t("agents:runtimeRetryRefresh")}
          </Button>
        </div>
      )}
    </div>
  );
}

export function RuntimeReviewNotices({ bulk }: { bulk: RuntimeUpdateBulkProps }) {
  const selected = bulk.reviewRows.filter(
    (row) => bulk.selection.has(row.key) && row.compatibility,
  );
  if (selected.length === 0) return null;
  return (
    <div className="space-y-1">
      {selected.map((row) => (
        <CopilotRuntimeCompatibilityNotice
          key={row.key}
          agentName={row.agentName}
          compatibility={row.compatibility}
        />
      ))}
    </div>
  );
}
