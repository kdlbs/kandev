"use client";

import { useState } from "react";
import { IconRefresh } from "@tabler/icons-react";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { useTranslation } from "react-i18next";
import type { TFunction } from "i18next";

type BranchRefreshButtonProps = {
  onRefresh: () => void;
  refreshing?: boolean;
  fetchedAt?: string;
  fetchError?: string;
  label?: string;
  testId?: string;
  touchTarget?: boolean;
};

export function BranchRefreshButton({
  onRefresh,
  refreshing,
  fetchedAt,
  fetchError,
  label = "branches",
  testId = "branch-refresh-button",
  touchTarget = false,
}: BranchRefreshButtonProps) {
  const { t } = useTranslation();
  // Controlled open so the tooltip only reacts to hover, not focus.
  // Radix Popover auto-focuses the first focusable child when it opens, which
  // would otherwise trigger this tooltip the moment the dropdown is opened.
  const [open, setOpen] = useState(false);
  const hasError = Boolean(fetchError);
  const tooltip = formatRefreshTooltip(t, label, fetchedAt, refreshing, fetchError);
  return (
    <Tooltip open={open} onOpenChange={setOpen}>
      <TooltipTrigger asChild>
        <button
          type="button"
          aria-label={t("common:refreshAriaLabel", { label })}
          data-testid={testId}
          onClick={(e) => {
            e.preventDefault();
            e.stopPropagation();
            onRefresh();
          }}
          onMouseEnter={() => setOpen(true)}
          onMouseLeave={() => setOpen(false)}
          disabled={refreshing}
          className={`inline-flex ${touchTarget ? "h-12 w-12" : "h-6 w-6"} items-center justify-center rounded-md hover:bg-muted/40 ${
            hasError
              ? "text-amber-500 hover:text-amber-600"
              : "text-muted-foreground hover:text-foreground"
          } ${refreshing ? "cursor-not-allowed opacity-50" : "cursor-pointer"}`}
        >
          <IconRefresh className={`h-3.5 w-3.5 ${refreshing ? "animate-spin" : ""}`} />
        </button>
      </TooltipTrigger>
      <TooltipContent>{tooltip}</TooltipContent>
    </Tooltip>
  );
}

// `label` stays untranslated: it is compared with `===` below, and callers pass
// it as a free-form noun ("branches", "repositories"). It travels through
// interpolation instead.
function formatRefreshTooltip(
  t: TFunction,
  label: string,
  fetchedAt: string | undefined,
  refreshing: boolean | undefined,
  fetchError: string | undefined,
) {
  if (refreshing) return t("common:refreshingLabel", { label });
  if (fetchError) return t("common:lastRefreshFailed", { error: fetchError });
  if (!fetchedAt) return initialRefreshTooltip(t, label);
  const date = new Date(fetchedAt);
  if (Number.isNaN(date.getTime())) return initialRefreshTooltip(t, label);
  return t("common:refreshLabelLastFetched", { label, time: date.toLocaleTimeString() });
}

function initialRefreshTooltip(t: TFunction, label: string) {
  return label === "branches"
    ? t("common:refreshBranchesGitFetch")
    : t("common:refreshAriaLabel", { label });
}
