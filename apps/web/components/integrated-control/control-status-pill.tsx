import { useTranslation } from "react-i18next";
import { cn } from "@/lib/utils";
import type { AttentionKind } from "./control-attention";
import type { TaskSignal, WorkspaceStatus } from "./control-signals";

export type PillState = WorkspaceStatus | TaskSignal | AttentionKind;
export type ControlTone = "error" | "delayed" | "running" | "info" | "idle";

/** Literal classes on the theme's semantic tokens, whole so Tailwind can see them. */
const TONE_SURFACE: Record<ControlTone, string> = {
  error: "border-destructive/40 bg-destructive/10 text-destructive",
  delayed: "border-warning/40 bg-warning/10 text-warning",
  running: "border-success/40 bg-success/10 text-success",
  info: "border-info/40 bg-info/10 text-info",
  idle: "border-border bg-muted text-muted-foreground",
};
const TONE_TEXT: Record<ControlTone, string> = {
  error: "text-destructive",
  delayed: "text-warning",
  running: "text-success",
  info: "text-info",
  idle: "text-muted-foreground",
};

export function toneSurfaceClass(tone: ControlTone): string {
  return TONE_SURFACE[tone];
}
export function toneTextClass(tone: ControlTone): string {
  return TONE_TEXT[tone];
}

const TONES: Record<PillState, ControlTone> = {
  question: "delayed",
  stalled: "error",
  repeat: "error",
  failed: "delayed",
  capacity: "delayed",
  config: "idle",
  active: "running",
  running: "running",
  waiting: "info",
  idle: "idle",
  noSession: "idle",
  other: "idle",
  held: "info",
  paused: "info",
  queued: "info",
  unknown: "idle",
  pending: "info",
};

const LABEL_KEYS: Record<PillState, string> = {
  question: "sidebar:controlSummary.state.question",
  stalled: "sidebar:controlSummary.state.stalled",
  repeat: "sidebar:controlSummary.state.repeat",
  failed: "sidebar:controlSummary.state.failed",
  capacity: "sidebar:controlSummary.state.capacity",
  config: "sidebar:controlSummary.state.config",
  active: "sidebar:controlSummary.state.active",
  running: "sidebar:controlSummary.state.running",
  waiting: "sidebar:controlSummary.state.waiting",
  idle: "sidebar:controlSummary.state.idle",
  noSession: "sidebar:controlSummary.state.noSession",
  other: "sidebar:controlSummary.state.other",
  held: "sidebar:controlSummary.stage.held",
  paused: "sidebar:controlSummary.state.paused",
  queued: "sidebar:controlSummary.state.queued",
  unknown: "common:unknown",
  pending: "sidebar:controlSummary.state.pending",
};

export function pillTone(state: PillState): ControlTone {
  return TONES[state];
}

export function pillLabelKey(state: PillState): string {
  return LABEL_KEYS[state];
}

/** A small state chip whose color comes from the shared status tones. */
export function ControlStatusPill({ state, className }: { state: PillState; className?: string }) {
  const { t } = useTranslation();
  return (
    <span
      className={cn(
        "inline-flex shrink-0 items-center whitespace-nowrap rounded border px-1.5 py-0.5 text-[11px] font-medium",
        TONE_SURFACE[TONES[state]],
        className,
      )}
      data-state={state}
    >
      {t(LABEL_KEYS[state])}
    </span>
  );
}
