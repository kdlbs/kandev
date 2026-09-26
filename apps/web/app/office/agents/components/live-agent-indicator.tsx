"use client";

import { cn } from "@/lib/utils";
import { useTranslation } from "react-i18next";

type LiveAgentIndicatorProps = {
  count: number;
  className?: string;
};

/**
 * Pulsing emerald dot + "{N} live" badge shown next to an agent in the
 * sidebar when one or more of its task sessions are actively working
 * (RUNNING / WAITING_FOR_INPUT). Returns null when count is 0 so callers
 * can fall back to a static status dot without layout shift.
 *
 * Styling matches the existing live indicators in `agent-cards-panel`
 * and `execution-indicator` (emerald-400/500, animate-ping halo).
 */
export function LiveAgentIndicator({ count, className }: LiveAgentIndicatorProps) {
  const { t } = useTranslation();
  if (count <= 0) return null;
  return (
    <div
      className={cn("flex items-center gap-1.5", className)}
      // `count` + `_one`/`_other`, never an "s" passed as a value: the plural
      // rule belongs in the catalog, not at this call site.
      aria-label={t("office:activeSessionCount", { count })}
    >
      <span className="relative flex h-2 w-2 shrink-0">
        <span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-emerald-400 opacity-75" />
        <span className="relative inline-flex h-2 w-2 rounded-full bg-emerald-500" />
      </span>
      <span className="text-[11px] font-medium text-emerald-500">
        {t("office:countLive", { count })}
      </span>
    </div>
  );
}
