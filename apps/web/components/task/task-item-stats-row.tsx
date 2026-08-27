"use client";

import { IconClockHour4, IconMail } from "@tabler/icons-react";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { Fragment, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { useAppStore } from "@/components/state-provider";
import { isDebugUI } from "@/lib/config";
import type { SessionPollMode } from "@/lib/state/slices/session-runtime/types";
import { cn, formatRelativeTime } from "@/lib/utils";
import type { WipQueueStatus } from "@/lib/kanban/wip-queue";
import {
  SIDEBAR_TASK_ROW_DETAIL_KEYS,
  type SidebarTaskRowDetail,
} from "@/lib/state/slices/ui/sidebar-task-row-presentation";

/** Debug-overlay-only poll-mode legend. Labels resolve through `t()` so the
 * pseudo-locale stays complete; the row itself is gated behind `isDebugUI()`. */
const POLL_MODE_CONFIG: Record<
  SessionPollMode,
  { letter: string; color: string; labelKey: string }
> = {
  fast: { letter: "F", color: "text-emerald-500", labelKey: "sidebar:pollModeFastLabel" },
  slow: { letter: "S", color: "text-yellow-500", labelKey: "sidebar:pollModeSlowLabel" },
  paused: {
    letter: "P",
    color: "text-muted-foreground/40",
    labelKey: "sidebar:pollModePausedLabel",
  },
};

/**
 * The repository a task belongs to. The one element of the stats row allowed to
 * shrink: a long `owner/repo` truncates rather than pushing the PR number and
 * badges out of the row, and `title` keeps the full name reachable on hover.
 */
function RepositoryLabel({ label }: { label?: string }) {
  if (!label) return null;
  return (
    <span
      data-testid="sidebar-task-repository"
      title={label}
      className="min-w-0 truncate text-muted-foreground/50"
    >
      {label}
    </span>
  );
}

function buildDetailNodes({
  updatedAt,
  repositoryLabel,
  prInfo,
  hasRelativeTime,
  hasRepository,
  hasPullRequestNumber,
}: {
  updatedAt?: string;
  repositoryLabel?: string;
  prInfo?: { number: number; state: string; aggregateState?: string };
  hasRelativeTime: boolean;
  hasRepository: boolean;
  hasPullRequestNumber: boolean;
}): Record<SidebarTaskRowDetail, ReactNode> {
  return {
    relative_time:
      hasRelativeTime && updatedAt ? (
        <span
          data-testid="sidebar-task-time"
          data-time-value={updatedAt}
          className="shrink-0 text-muted-foreground/50"
        >
          {formatRelativeTime(updatedAt)}
        </span>
      ) : null,
    repository: hasRepository ? <RepositoryLabel label={repositoryLabel} /> : null,
    pull_request_number:
      hasPullRequestNumber && prInfo ? (
        <span className="shrink-0 text-muted-foreground/50">#{prInfo.number}</span>
      ) : null,
  };
}

function QueuedPromptCount({ count }: { count?: number }) {
  const { t } = useTranslation();
  if (!count) return null;
  const label = t("sidebar:queuedPromptCount", { count });
  return (
    <span
      data-testid="sidebar-task-queued-count"
      aria-label={label}
      title={label}
      className="inline-flex shrink-0 items-center gap-0.5 text-muted-foreground/60"
    >
      <IconMail className="h-3 w-3" aria-hidden="true" />
      {count}
    </span>
  );
}

function WipQueueIndicator({ wipQueue }: { wipQueue?: WipQueueStatus }) {
  const { t } = useTranslation();
  if (!wipQueue) return null;
  const label = t("sidebar:wipQueuePosition", {
    position: wipQueue.position,
    total: wipQueue.total,
    step: wipQueue.destinationTitle,
  });
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span
          data-testid="sidebar-task-wip-queue"
          tabIndex={0}
          aria-label={label}
          title={label}
          className="inline-flex shrink-0 items-center text-muted-foreground/60 outline-none focus-visible:ring-1 focus-visible:ring-ring"
        >
          <IconClockHour4 className="h-3.5 w-3.5" aria-hidden="true" />
        </span>
      </TooltipTrigger>
      <TooltipContent side="right">{label}</TooltipContent>
    </Tooltip>
  );
}

function PollModeIndicator({
  pollMode,
  config,
}: {
  pollMode: SessionPollMode | null;
  config: (typeof POLL_MODE_CONFIG)[SessionPollMode] | null;
}) {
  const { t } = useTranslation();
  if (!pollMode || !config) return null;
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span className={cn("font-mono text-[10px] font-semibold", config.color)}>
          {config.letter}
        </span>
      </TooltipTrigger>
      <TooltipContent side="right">
        {t("task:gitPollMode", { mode: pollMode, label: t(config.labelKey) })}
      </TooltipContent>
    </Tooltip>
  );
}

function hasStatsToRender({
  hasRelativeTime,
  hasRepository,
  hasPullRequestNumber,
  pollMode,
  queuedCount,
  wipQueue,
}: {
  hasRelativeTime: boolean;
  hasRepository: boolean;
  hasPullRequestNumber: boolean;
  pollMode: SessionPollMode | null;
  queuedCount?: number;
  wipQueue?: WipQueueStatus;
}): boolean {
  return Boolean(
    hasRelativeTime || hasRepository || hasPullRequestNumber || pollMode || queuedCount || wipQueue,
  );
}

/**
 * The sidebar row's metadata line: relative last-update time, repository, PR number,
 * queued-prompt mail badge, WIP queue icon, and (debug UI only) the session
 * poll-mode letter.
 */
export function TaskItemStatsRow({
  updatedAt,
  repositoryLabel,
  prInfo,
  primarySessionId,
  queuedCount,
  wipQueue,
  detailOrder = [...SIDEBAR_TASK_ROW_DETAIL_KEYS],
  showRelativeTime = true,
  showRepository = true,
  showPullRequestNumber = true,
}: {
  updatedAt?: string;
  /** Repository the task belongs to; already resolved to a slug or name. */
  repositoryLabel?: string;
  prInfo?: { number: number; state: string; aggregateState?: string };
  primarySessionId?: string | null;
  queuedCount?: number;
  wipQueue?: WipQueueStatus;
  detailOrder?: SidebarTaskRowDetail[];
  showRelativeTime?: boolean;
  showRepository?: boolean;
  showPullRequestNumber?: boolean;
}) {
  const pollMode = useAppStore((s) =>
    isDebugUI() && primarySessionId
      ? (s.sessionPollMode.bySessionId[primarySessionId] ?? null)
      : null,
  );

  const hasRelativeTime = showRelativeTime && !!updatedAt;
  const hasRepository = showRepository && !!repositoryLabel;
  const hasPullRequestNumber = showPullRequestNumber && !!prInfo;
  if (
    !hasStatsToRender({
      hasRelativeTime,
      hasRepository,
      hasPullRequestNumber,
      pollMode,
      queuedCount,
      wipQueue,
    })
  ) {
    return null;
  }

  const detailNodes = buildDetailNodes({
    updatedAt,
    repositoryLabel,
    prInfo,
    hasRelativeTime,
    hasRepository,
    hasPullRequestNumber,
  });

  return (
    <span className="flex min-w-0 items-center gap-1.5 text-[11px]">
      {detailOrder.map((detail) =>
        detailNodes[detail] ? <Fragment key={detail}>{detailNodes[detail]}</Fragment> : null,
      )}
      <QueuedPromptCount count={queuedCount} />
      <WipQueueIndicator wipQueue={wipQueue} />
      <PollModeIndicator
        pollMode={pollMode}
        config={pollMode ? POLL_MODE_CONFIG[pollMode] : null}
      />
    </span>
  );
}
