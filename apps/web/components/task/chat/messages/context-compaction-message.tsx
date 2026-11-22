"use client";

import { memo, useState, type ReactNode } from "react";
import {
  IconCheck,
  IconChevronDown,
  IconChevronRight,
  IconClock,
  IconX,
} from "@tabler/icons-react";
import { GridSpinner } from "@/components/grid-spinner";
import { MemoizedMarkdown } from "@/components/shared/memoized-markdown";
import { formatNumber } from "@/lib/i18n/formats";
import { cn } from "@/lib/utils";
import type { Message } from "@/lib/types/http";
import type { ContextCompactionPayload, ToolCallMetadata } from "@/components/task/chat/types";
import { useTranslation } from "react-i18next";

type ContextCompactionMessageProps = {
  comment: Message;
  isContainingTurnActive?: boolean;
  worktreePath?: string;
  onOpenFile?: (path: string) => void;
};

type ContextCompactionStatus = "in_progress" | "completed" | "failed" | "cancelled";

type CompactionHeaderProps = {
  commentId: string;
  statusLabel: string;
  tokenCounts: string;
  trigger?: string;
  duration: string;
  hasError: boolean;
  hasExpandableContent: boolean;
  expanded: boolean;
  statusIcon: ReactNode;
  onToggle: () => void;
};

type CompactionHeaderContentProps = Pick<
  CompactionHeaderProps,
  | "statusLabel"
  | "tokenCounts"
  | "trigger"
  | "duration"
  | "statusIcon"
  | "hasExpandableContent"
  | "expanded"
>;
type CompactionMetadataProps = Pick<CompactionHeaderProps, "tokenCounts" | "trigger" | "duration">;

export function isContextCompactionEffectivelyActive(
  payload: ContextCompactionPayload | undefined,
  isContainingTurnActive: boolean,
): boolean {
  return payload?.status === "in_progress" && isContainingTurnActive;
}

function normalizedStatus(status: string | undefined): ContextCompactionStatus {
  if (status === "completed" || status === "failed" || status === "cancelled") return status;
  return "in_progress";
}

function statusLabel(status: ContextCompactionStatus, active: boolean, t: (key: string) => string) {
  if (active) return t("task:contextCompactionActive");
  if (status === "in_progress") return t("task:contextCompactionUnconfirmed");
  if (status === "completed") return t("task:contextCompactionCompleted");
  if (status === "failed") return t("task:contextCompactionFailed");
  return t("task:contextCompactionCancelled");
}

function statusIcon(status: ContextCompactionStatus, active: boolean) {
  if (active) {
    return (
      <span data-testid="context-compaction-active-indicator">
        <GridSpinner className="text-muted-foreground" />
      </span>
    );
  }
  if (status === "completed")
    return <IconCheck aria-hidden className="h-3.5 w-3.5 text-green-500" />;
  if (status === "failed" || status === "cancelled") {
    return <IconX aria-hidden className="h-3.5 w-3.5 text-destructive" />;
  }
  return <IconClock aria-hidden className="h-3.5 w-3.5 text-muted-foreground" />;
}

function formatDuration(durationMs: number): string {
  if (durationMs < 1000) return `${durationMs}ms`;
  return `${(durationMs / 1000).toFixed(1)}s`;
}

function toggleLabel(expanded: boolean, hasError: boolean, t: (key: string) => string): string {
  if (expanded) return t("task:contextCompactionHideDetails");
  if (hasError) return t("task:contextCompactionShowError");
  return t("task:contextCompactionShowSummary");
}

function CompactionChevron({ expanded }: { expanded: boolean }) {
  if (expanded) {
    return (
      <IconChevronDown aria-hidden className="h-3.5 w-3.5 shrink-0 text-muted-foreground/60" />
    );
  }
  return <IconChevronRight aria-hidden className="h-3.5 w-3.5 shrink-0 text-muted-foreground/60" />;
}

function CompactionMetadata({ tokenCounts, trigger, duration }: CompactionMetadataProps) {
  const { t } = useTranslation();
  if (!tokenCounts && !trigger && !duration) return null;
  return (
    <div className="flex max-w-full flex-wrap items-center gap-x-2 gap-y-1 pl-5 sm:ml-auto sm:pl-0">
      {tokenCounts && (
        <span
          className="text-xs tabular-nums text-muted-foreground"
          data-testid="context-compaction-token-counts"
        >
          {tokenCounts}
        </span>
      )}
      {trigger && (
        <span
          aria-label={t("task:contextCompactionTrigger", { trigger })}
          className="rounded bg-muted px-1.5 py-0.5 text-[10px] text-muted-foreground"
          data-testid="context-compaction-trigger"
        >
          {trigger}
        </span>
      )}
      {duration && (
        <span
          aria-label={t("task:contextCompactionDuration", { duration })}
          className="text-xs text-muted-foreground"
          data-testid="context-compaction-duration"
        >
          {duration}
        </span>
      )}
    </div>
  );
}

function CompactionHeaderContent({
  statusLabel: label,
  tokenCounts,
  trigger,
  duration,
  statusIcon: icon,
  hasExpandableContent,
  expanded,
}: CompactionHeaderContentProps) {
  return (
    <div className="flex w-full min-w-0 flex-col gap-1 sm:flex-row sm:items-center">
      <div className="flex min-w-0 items-center gap-2">
        {hasExpandableContent && <CompactionChevron expanded={expanded} />}
        <span
          className="min-w-0 flex-1 text-xs font-mono text-muted-foreground"
          data-testid="context-compaction-status"
          aria-live="polite"
        >
          {label}
        </span>
        <span className="shrink-0">{icon}</span>
      </div>
      <CompactionMetadata tokenCounts={tokenCounts} trigger={trigger} duration={duration} />
    </div>
  );
}

function CompactionHeader(props: CompactionHeaderProps) {
  const { t } = useTranslation();
  const content = <CompactionHeaderContent {...props} />;
  const className = "flex min-h-11 w-full items-center rounded px-2 py-1 text-left";
  if (!props.hasExpandableContent) {
    return <div className={cn(className, "cursor-default")}>{content}</div>;
  }
  return (
    <button
      type="button"
      aria-expanded={props.expanded}
      aria-controls={`context-compaction-details-${props.commentId}`}
      aria-label={toggleLabel(props.expanded, props.hasError, t)}
      onClick={props.onToggle}
      className={cn(
        className,
        "hover:bg-muted/50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring",
      )}
    >
      {content}
    </button>
  );
}

type CompactionDetailsProps = {
  comment: Message;
  error: string;
  hasError: boolean;
  summary: string;
  hasSummary: boolean;
  worktreePath?: string;
  onOpenFile?: (path: string) => void;
};

function CompactionDetails({
  comment,
  error,
  hasError,
  summary,
  hasSummary,
  worktreePath,
  onOpenFile,
}: CompactionDetailsProps) {
  const { t } = useTranslation();
  return (
    <div
      id={`context-compaction-details-${comment.id}`}
      className="ml-7 mt-2 space-y-2 border-l-2 border-border/30 pl-4 text-xs"
    >
      {hasError && (
        <div
          className="break-words whitespace-pre-wrap text-destructive"
          data-testid="context-compaction-error"
        >
          {error}
        </div>
      )}
      {hasSummary && (
        <div className="space-y-1" data-testid="context-compaction-summary">
          <div className="font-medium text-muted-foreground">
            {t("task:contextCompactionSummary")}
          </div>
          <div className="markdown-body max-w-none overflow-hidden break-words text-xs text-foreground/80 [&>*]:my-1 [&>p]:my-1 [&>ul]:my-1 [&>ol]:my-1">
            <MemoizedMarkdown
              content={summary}
              taskId={comment.task_id}
              worktreePath={worktreePath}
              onOpenFile={onOpenFile}
            />
          </div>
        </div>
      )}
    </div>
  );
}

export const ContextCompactionMessage = memo(function ContextCompactionMessage({
  comment,
  isContainingTurnActive = false,
  worktreePath,
  onOpenFile,
}: ContextCompactionMessageProps) {
  const { t } = useTranslation();
  const [expanded, setExpanded] = useState(false);
  const metadata = comment.metadata as ToolCallMetadata | undefined;
  const payload = metadata?.normalized?.context_compaction;
  if (!payload) return null;

  const status = normalizedStatus(payload.status);
  const active = isContextCompactionEffectivelyActive(payload, isContainingTurnActive);
  const summary = payload.summary?.slice(0, 64 * 1024) ?? "";
  const error = payload.error ?? "";
  const hasSummary = summary.length > 0;
  const hasError = status === "failed" && error.length > 0;
  const hasExpandableContent = hasSummary || hasError;
  const tokenCounts =
    typeof payload.pre_tokens === "number" && typeof payload.post_tokens === "number"
      ? t("task:contextCompactionTokenCounts", {
          preTokens: formatNumber(payload.pre_tokens),
          postTokens: formatNumber(payload.post_tokens),
        })
      : "";
  const duration =
    typeof payload.duration_ms === "number" ? formatDuration(payload.duration_ms) : "";

  return (
    <div className="w-full" data-testid="context-compaction-message">
      <CompactionHeader
        commentId={comment.id}
        statusLabel={statusLabel(status, active, t)}
        tokenCounts={tokenCounts}
        trigger={payload.trigger}
        duration={duration}
        hasError={hasError}
        hasExpandableContent={hasExpandableContent}
        expanded={expanded}
        statusIcon={statusIcon(status, active)}
        onToggle={() => setExpanded((value) => !value)}
      />
      {expanded && hasExpandableContent ? (
        <CompactionDetails
          comment={comment}
          error={error}
          hasError={hasError}
          summary={summary}
          hasSummary={hasSummary}
          worktreePath={worktreePath}
          onOpenFile={onOpenFile}
        />
      ) : null}
    </div>
  );
});
