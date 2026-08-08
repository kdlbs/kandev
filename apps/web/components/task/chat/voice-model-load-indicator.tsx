"use client";

import { IconAlertTriangle } from "@tabler/icons-react";
import { Progress } from "@kandev/ui/progress";
import { useTranslation } from "react-i18next";

export type VoiceModelLoadIndicatorProps = {
  state: "idle" | "loading" | "ready" | "error";
  /** 0–1 fraction; clamped before rendering. */
  progress: number;
  modelLabel: string;
};

function clampPercent(progress: number): number {
  if (!Number.isFinite(progress)) return 0;
  const pct = Math.round(progress * 100);
  if (pct < 0) return 0;
  if (pct > 100) return 100;
  return pct;
}

export function VoiceModelLoadIndicator({
  state,
  progress,
  modelLabel,
}: VoiceModelLoadIndicatorProps) {
  const { t } = useTranslation();
  if (state === "idle" || state === "ready") return null;

  const pct = clampPercent(progress);

  // Single stable role="status" wrapper so AT announces the loading→error
  // transition (a newly-inserted live region would be silently ignored).
  // <Progress> (Radix) already renders its own role="progressbar" — the
  // wrapper div intentionally has no role to avoid nested progressbar nodes.
  return (
    <div
      data-testid="voice-model-load-indicator"
      data-state={state}
      role="status"
      aria-label={state === "error" ? t("task:failedToLoad", { modelLabel }) : undefined}
      className={
        state === "error"
          ? "flex items-center gap-1 text-xs text-destructive w-32"
          : "flex items-center gap-1.5 w-32"
      }
    >
      {state === "error" ? (
        <>
          <IconAlertTriangle className="h-3.5 w-3.5 shrink-0" />
          <span className="hidden sm:inline">{t("task:failedToLoad", { modelLabel })}</span>
        </>
      ) : (
        <div className="flex flex-col gap-0.5 min-w-0 flex-1">
          <span className="hidden sm:inline text-[10px] leading-none text-muted-foreground truncate">
            {t("task:downloadingModelPercent", { modelLabel, pct })}
          </span>
          <Progress
            value={pct}
            className="h-1"
            aria-label={t("task:downloadingPercent", { modelLabel, pct })}
          />
        </div>
      )}
    </div>
  );
}
