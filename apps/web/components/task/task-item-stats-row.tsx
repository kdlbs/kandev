"use client";

import { IconMail } from "@tabler/icons-react";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { useTranslation } from "react-i18next";
import { useAppStore } from "@/components/state-provider";
import { isDebugUI } from "@/lib/config";
import type { SessionPollMode } from "@/lib/state/slices/session-runtime/types";
import { cn, formatRelativeTime } from "@/lib/utils";

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
 * The sidebar row's metadata line: relative last-update time, PR number,
 * queued-prompt mail badge, and (debug UI only) the session poll-mode letter.
 */
export function TaskItemStatsRow({
  updatedAt,
  prInfo,
  primarySessionId,
  queuedCount,
}: {
  updatedAt?: string;
  prInfo?: { number: number; state: string; aggregateState?: string };
  primarySessionId?: string | null;
  queuedCount?: number;
}) {
  const { t } = useTranslation();
  const pollMode = useAppStore((s) =>
    isDebugUI() && primarySessionId
      ? (s.sessionPollMode.bySessionId[primarySessionId] ?? null)
      : null,
  );

  if (!updatedAt && !prInfo && !pollMode && !queuedCount) return null;

  const modeConfig = pollMode ? POLL_MODE_CONFIG[pollMode] : null;
  const modeLabel = modeConfig ? t(modeConfig.labelKey) : "";

  return (
    <span className="flex items-center gap-1.5 text-[11px]">
      {updatedAt && (
        <span className="text-muted-foreground/50">{formatRelativeTime(updatedAt)}</span>
      )}
      {prInfo && <span className="text-muted-foreground/50">#{prInfo.number}</span>}
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
