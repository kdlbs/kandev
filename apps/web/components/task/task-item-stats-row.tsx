"use client";

import { useAppStore } from "@/components/state-provider";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { cn, formatRelativeTime } from "@/lib/utils";
import { isDebugUI } from "@/lib/config";
import type { SessionPollMode } from "@/lib/state/slices/session-runtime/types";
import { useTranslation } from "react-i18next";

/** Debug-overlay-only poll-mode legend. `label` stays English on purpose: the
 * row is gated behind `isDebugUI()`, so these are developer diagnostics rather
 * than user copy — and a SCREAMING_CASE const is invisible to
 * `i18next/no-literal-string`, so nothing would have flagged it either way. */
const POLL_MODE_CONFIG: Record<SessionPollMode, { letter: string; color: string; label: string }> =
  {
    fast: { letter: "F", color: "text-emerald-500", label: "focused, 2s polling" },
    slow: { letter: "S", color: "text-yellow-500", label: "subscribed, 30s polling" },
    paused: { letter: "P", color: "text-muted-foreground/40", label: "no subscribers" },
  };

export function TaskItemStatsRow({
  updatedAt,
  prInfo,
  primarySessionId,
}: {
  updatedAt?: string;
  prInfo?: { number: number; state: string; aggregateState?: string };
  primarySessionId?: string | null;
}) {
  const { t } = useTranslation();
  const pollMode = useAppStore((s) =>
    isDebugUI() && primarySessionId
      ? (s.sessionPollMode.bySessionId[primarySessionId] ?? null)
      : null,
  );

  if (!updatedAt && !prInfo && !pollMode) return null;

  const modeConfig = pollMode ? POLL_MODE_CONFIG[pollMode] : null;

  return (
    <span className="flex items-center gap-1.5 text-[11px]">
      {updatedAt && (
        <span className="text-muted-foreground/50">{formatRelativeTime(updatedAt)}</span>
      )}
      {prInfo && <span className="text-muted-foreground/50">#{prInfo.number}</span>}
      {modeConfig && (
        <Tooltip>
          <TooltipTrigger asChild>
            <span className={cn("font-mono text-[10px] font-semibold", modeConfig.color)}>
              {modeConfig.letter}
            </span>
          </TooltipTrigger>
          <TooltipContent side="right">
            {t("task:gitPollMode", { mode: pollMode, label: modeConfig.label })}
          </TooltipContent>
        </Tooltip>
      )}
    </span>
  );
}
