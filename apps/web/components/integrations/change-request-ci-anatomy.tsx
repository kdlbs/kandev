"use client";

import { useEffect, useState, type ReactNode } from "react";
import {
  IconCheck,
  IconCircleCheck,
  IconCircleDot,
  IconCircleX,
  IconExternalLink,
  IconGitPullRequest,
  IconMessageCircle,
} from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { openExternalLink } from "@/lib/desktop/external-links";
import { t } from "@/lib/i18n";
import type { IntegrationChangeRequestPipelineState } from "./integration-change-request-status-types";

export type ChangeRequestCheckCounts = {
  passed: number;
  pending: number;
  failed: number;
};

export const CHANGE_REQUEST_CI_DESKTOP_SCROLL_CLASS =
  "max-h-[min(28rem,calc(100vh-8rem))] overflow-y-auto overscroll-contain";

export type ChangeRequestCheckRow = {
  id: string;
  label: string;
  state: IntegrationChangeRequestPipelineState;
  detail?: string;
  url?: string;
  onAddAsContext?: () => void;
};

export function ChangeRequestPopoverHeader({
  number,
  title,
  url,
  onOpenReview,
  externalLabel,
  openDetailsLabel,
}: {
  number: number | string;
  title: string;
  url?: string;
  onOpenReview?: () => void;
  externalLabel?: string;
  openDetailsLabel?: string;
}) {
  const displayTitle = `#${number} ${title || t("integrations:untitledPullRequest")}`;
  return (
    <div
      data-testid="pr-popover-header"
      className="flex items-center justify-between gap-2 border-b border-border/50 pb-2"
    >
      {onOpenReview ? (
        <button
          type="button"
          data-testid="pr-popover-title"
          className="min-w-0 cursor-pointer truncate text-left text-sm font-medium hover:underline"
          title={displayTitle}
          aria-label={openDetailsLabel ?? t("integrations:openDetails", { title: displayTitle })}
          onClick={onOpenReview}
        >
          {displayTitle}
        </button>
      ) : (
        <span
          data-testid="pr-popover-title"
          className="min-w-0 truncate text-sm font-medium"
          title={displayTitle}
        >
          {displayTitle}
        </span>
      )}
      {url ? (
        <a
          data-testid="pr-popover-pr-link"
          href={url}
          target="_blank"
          rel="noopener noreferrer"
          className="cursor-pointer text-muted-foreground hover:text-foreground"
          aria-label={externalLabel ?? t("integrations:viewPullRequestExternally", { number })}
          onClick={(event) => event.stopPropagation()}
        >
          <IconGitPullRequest className="h-3.5 w-3.5" />
        </a>
      ) : null}
    </div>
  );
}

function groupLabel(
  state: IntegrationChangeRequestPipelineState,
  labels?: Partial<Record<IntegrationChangeRequestPipelineState, string>>,
): string {
  if (labels?.[state]) return labels[state];
  if (state === "success") return t("integrations:passed");
  if (state === "pending") return t("integrations:inProgress");
  if (state === "failure") return t("integrations:failed");
  return t("integrations:other");
}

function GroupIcon({ state }: { state: IntegrationChangeRequestPipelineState }) {
  if (state === "success") return <IconCircleCheck className="h-3.5 w-3.5 text-emerald-500" />;
  if (state === "pending") {
    return <IconCircleDot className="h-3.5 w-3.5 animate-pulse text-yellow-500" />;
  }
  if (state === "failure") return <IconCircleX className="h-3.5 w-3.5 text-red-500" />;
  return <IconCircleDot className="h-3.5 w-3.5 text-muted-foreground" />;
}

function countForState(
  counts: ChangeRequestCheckCounts,
  state: IntegrationChangeRequestPipelineState,
): number {
  if (state === "success") return counts.passed;
  if (state === "pending") return counts.pending;
  if (state === "failure") return counts.failed;
  return 0;
}

const CHECK_STATES: IntegrationChangeRequestPipelineState[] = ["success", "pending", "failure"];

function ChecksProgress({
  counts,
  passRateLabel,
}: {
  counts: ChangeRequestCheckCounts;
  passRateLabel?: string;
}) {
  const total = counts.passed + counts.pending + counts.failed;
  if (total === 0) return null;
  const percent = (value: number) => (value / total) * 100;
  return (
    <div data-testid="pr-checks-progress" className="flex flex-col gap-1.5 px-1 pb-1.5 pt-1">
      <div className="flex items-center justify-between text-xs">
        <span className="font-medium text-foreground">
          {passRateLabel ?? t("integrations:passRate")}
        </span>
        <span className="tabular-nums text-muted-foreground">
          {counts.passed}/{total} ({Math.round(percent(counts.passed))}%)
        </span>
      </div>
      <div className="flex h-1.5 w-full overflow-hidden rounded-full bg-muted/70">
        <div className="h-full bg-green-500" style={{ width: `${percent(counts.passed)}%` }} />
        <div className="h-full bg-yellow-500" style={{ width: `${percent(counts.pending)}%` }} />
        <div className="h-full bg-red-500" style={{ width: `${percent(counts.failed)}%` }} />
      </div>
    </div>
  );
}

function CheckRow({ row }: { row: ChangeRequestCheckRow }) {
  return (
    <div
      data-testid="pr-workflow-row"
      data-workflow={row.label}
      data-bucket={row.state}
      className="flex cursor-pointer items-center gap-2 rounded-sm px-2 py-1 hover:bg-accent/50"
      onClick={() => row.url && void openExternalLink(row.url).catch(() => undefined)}
    >
      <span className="min-w-0 flex-1 truncate text-xs font-medium" title={row.label}>
        {row.label}
      </span>
      {row.detail ? (
        <span className="shrink-0 text-[10px] text-muted-foreground">{row.detail}</span>
      ) : null}
      {row.url ? <IconExternalLink className="h-3 w-3 shrink-0" /> : null}
      {row.state === "failure" && row.onAddAsContext ? (
        <Button
          type="button"
          size="sm"
          variant="ghost"
          className="h-5 w-5 cursor-pointer p-0"
          aria-label={t("integrations:addFailuresToChatContext", { name: row.label })}
          onClick={(event) => {
            event.stopPropagation();
            row.onAddAsContext?.();
          }}
        >
          +
        </Button>
      ) : null}
    </div>
  );
}

export function ChangeRequestChecksSection({
  counts,
  rows,
  loading,
  emptyLabel,
  passRateLabel,
  groupLabels,
}: {
  counts: ChangeRequestCheckCounts;
  rows: readonly ChangeRequestCheckRow[];
  loading?: boolean;
  emptyLabel?: string;
  passRateLabel?: string;
  groupLabels?: Partial<Record<IntegrationChangeRequestPipelineState, string>>;
}) {
  const total = counts.passed + counts.pending + counts.failed;
  if (!loading && total === 0 && rows.length === 0) {
    return (
      <div data-testid="pr-checks-section" className="flex flex-col">
        <div data-testid="pr-checks-empty" className="px-1 py-2 text-xs text-muted-foreground">
          {emptyLabel ?? t("integrations:noChecksHaveStarted")}
        </div>
      </div>
    );
  }
  return (
    <div data-testid="pr-checks-section" className="flex flex-col gap-1">
      <ChecksProgress counts={counts} passRateLabel={passRateLabel} />
      {CHECK_STATES.map((state) => {
        const stateRows = rows.filter((row) => row.state === state);
        const count = countForState(counts, state);
        if (count === 0 && stateRows.length === 0) return null;
        return (
          <div key={state} data-testid="pr-check-group" data-kind={state}>
            <div className="flex items-center justify-between gap-2 px-1 py-1">
              <div className="flex items-center gap-1.5">
                <GroupIcon state={state} />
                <span className="text-xs font-medium">{groupLabel(state, groupLabels)}</span>
              </div>
              <span className="text-xs tabular-nums text-muted-foreground">{count}</span>
            </div>
            {state !== "success" ? (
              <div className="flex flex-col pl-5">
                {stateRows.map((row) => (
                  <CheckRow key={row.id} row={row} />
                ))}
              </div>
            ) : null}
          </div>
        );
      })}
    </div>
  );
}

export function ChangeRequestReviewRow({
  state,
  approved,
  required,
  requested,
}: {
  state: "approved" | "changes_requested" | "pending";
  approved: number;
  required?: number;
  requested?: number;
}) {
  const { label, icon } = reviewPresentation(state);
  const count = `${approved}${required == null ? "" : ` / ${required}`}${requested ? ` · ${t("integrations:requestedCount", { count: requested })}` : ""}`;
  return (
    <div
      data-testid="pr-review-row"
      className="flex items-center justify-between gap-2 px-1 py-1 text-xs"
    >
      <div className="flex min-w-0 items-center gap-1.5">
        {icon}
        <span className="truncate">{label}</span>
      </div>
      <span className="shrink-0 tabular-nums text-muted-foreground">{count}</span>
    </div>
  );
}

function reviewPresentation(state: "approved" | "changes_requested" | "pending") {
  if (state === "approved") {
    return {
      label: t("integrations:approved"),
      icon: <IconCheck className="h-3.5 w-3.5 text-emerald-500" />,
    };
  }
  if (state === "changes_requested") {
    return {
      label: t("integrations:changesRequested"),
      icon: <IconCircleX className="h-3.5 w-3.5 text-red-500" />,
    };
  }
  return {
    label: t("integrations:awaitingReview"),
    icon: <IconCircleDot className="h-3.5 w-3.5 text-muted-foreground" />,
  };
}

export function ChangeRequestCommentsRow({ count, label }: { count: number; label?: string }) {
  if (count <= 0) return null;
  return (
    <div data-testid="pr-comments-row" className="flex items-center gap-1.5 px-1 py-1 text-xs">
      <IconMessageCircle className="h-3.5 w-3.5 text-muted-foreground" />
      <span>{label ?? t("integrations:unresolvedComments", { count })}</span>
    </div>
  );
}

export function ChangeRequestPopoverFooter({
  updatedAt,
  formatElapsed,
}: {
  updatedAt?: number;
  formatElapsed?: (seconds: number) => string;
}) {
  const [now, setNow] = useState(updatedAt);
  useEffect(() => {
    if (updatedAt == null) return;
    const timer = setInterval(() => setNow(Date.now()), 10_000);
    return () => clearInterval(timer);
  }, [updatedAt]);
  if (updatedAt == null) return null;
  const seconds = Math.max(0, Math.floor(((now ?? updatedAt) - updatedAt) / 1000));
  const elapsed = formatElapsed ? formatElapsed(seconds) : defaultElapsed(seconds);
  return (
    <div
      data-testid="pr-popover-footer"
      className="flex items-center justify-end border-t border-border/50 pt-1.5"
    >
      <span
        data-testid="pr-popover-updated-at"
        className="text-[10px] tabular-nums text-muted-foreground"
      >
        {elapsed}
      </span>
    </div>
  );
}

function defaultElapsed(seconds: number): string {
  if (seconds === 0) return t("integrations:updatedJustNow");
  const elapsed = seconds < 60 ? `${seconds}s` : `${Math.floor(seconds / 60)}m`;
  return t("integrations:updatedAgo", { elapsed });
}

export function ChangeRequestCIPopoverFrame({ children }: { children: ReactNode }) {
  return (
    <div
      data-testid="pr-topbar-popover-inner"
      className="flex flex-col gap-2"
      onClick={(event) => event.stopPropagation()}
    >
      {children}
    </div>
  );
}
