"use client";

import { IconAlertTriangle } from "@tabler/icons-react";
import { Progress } from "@kandev/ui/progress";
import { cn } from "@/lib/utils";

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
  if (state === "idle" || state === "ready") return null;

  if (state === "error") {
    return (
      <div
        data-testid="voice-model-load-indicator"
        data-state="error"
        className="flex items-center gap-1 text-xs text-destructive"
        role="status"
        aria-live="polite"
      >
        <IconAlertTriangle className="h-3.5 w-3.5 shrink-0" />
        <span className="hidden sm:inline">Voice model failed to load</span>
      </div>
    );
  }

  const pct = clampPercent(progress);
  return (
    <div
      data-testid="voice-model-load-indicator"
      data-state="loading"
      className={cn("flex items-center gap-1.5 w-32 max-w-[8rem]")}
      role="status"
      aria-live="polite"
      aria-label={`Downloading ${modelLabel}, ${pct} percent`}
    >
      <div className="flex flex-col gap-0.5 min-w-0 flex-1">
        <span className="hidden sm:inline text-[10px] leading-none text-muted-foreground truncate">
          Downloading {modelLabel}… {pct}%
        </span>
        <Progress value={pct} className="h-1" />
      </div>
    </div>
  );
}
