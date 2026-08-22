"use client";

import { IconClockHour4, IconMail } from "@tabler/icons-react";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { useTranslation } from "react-i18next";
import { useAppStore } from "@/components/state-provider";
import { isDebugUI } from "@/lib/config";
import type { SessionPollMode } from "@/lib/state/slices/session-runtime/types";
import { cn, formatRelativeTime } from "@/lib/utils";
import type { WipQueueStatus } from "@/lib/kanban/wip-queue";

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
}: {
  updatedAt?: string;
  /** Repository the task belongs to; already resolved to a slug or name. */
  repositoryLabel?: string;
  prInfo?: { number: number; state: string; aggregateState?: string };
  primarySessionId?: string | null;
  queuedCount?: number;
  wipQueue?: WipQueueStatus;
}) {
  const { t } = useTranslation();
  const pollMode = useAppStore((s) =>
    isDebugUI() && primarySessionId
      ? (s.sessionPollMode.bySessionId[primarySessionId] ?? null)
      : null,
  );

  if (!updatedAt && !repositoryLabel && !prInfo && !pollMode && !queuedCount && !wipQueue) {
    return null;
  }

  const modeConfig = pollMode ? POLL_MODE_CONFIG[pollMode] : null;
  const modeLabel = modeConfig ? t(modeConfig.labelKey) : "";
  const wipQueueLabel = wipQueue
    ? t("sidebar:wipQueuePosition", {
        position: wipQueue.position,
        total: wipQueue.total,
        step: wipQueue.destinationTitle,
      })
    : "";

  return (
    <span className="flex min-w-0 items-center gap-1.5 text-[11px]">
      {updatedAt && (
        <span
          data-testid="sidebar-task-time"
          data-time-value={updatedAt}
          className="shrink-0 text-muted-foreground/50"
        >
          {formatRelativeTime(updatedAt)}
        </span>
      )}
      <RepositoryLabel label={repositoryLabel} />
      {prInfo && <span className="shrink-0 text-muted-foreground/50">#{prInfo.number}</span>}
      {queuedCount ? (
        <span
          data-testid="sidebar-task-queued-count"
          aria-label={t("sidebar:queuedPromptCount", { count: queuedCount })}
          title={t("sidebar:queuedPromptCount", { count: queuedCount })}
          className="inline-flex shrink-0 items-center gap-0.5 text-muted-foreground/60"
        >
          <IconMail className="h-3 w-3" aria-hidden="true" />
          {queuedCount}
        </span>
      ) : null}
      {wipQueue ? (
        <Tooltip>
          <TooltipTrigger asChild>
            <span
              data-testid="sidebar-task-wip-queue"
              tabIndex={0}
              aria-label={wipQueueLabel}
              title={wipQueueLabel}
              className="inline-flex shrink-0 items-center text-muted-foreground/60 outline-none focus-visible:ring-1 focus-visible:ring-ring"
            >
              <IconClockHour4 className="h-3.5 w-3.5" aria-hidden="true" />
            </span>
          </TooltipTrigger>
          <TooltipContent side="right">{wipQueueLabel}</TooltipContent>
        </Tooltip>
      ) : null}
      {modeConfig && (
        <Tooltip>
          <TooltipTrigger asChild>
            <span className={cn("font-mono text-[10px] font-semibold", modeConfig.color)}>
              {modeConfig.letter}
            </span>
          </TooltipTrigger>
          <TooltipContent side="right">
            {t("task:gitPollMode", { mode: pollMode, label: modeLabel })}
          </TooltipContent>
        </Tooltip>
      )}
    </span>
  );
}
