import type {
  OverviewEventKind,
  OverviewQueueStatus,
  OverviewStatus,
} from "@/lib/types/control-overview";

/**
 * The overview's status vocabulary. Each tone names one color family in the
 * theme tokens, so a state is never spelled as a raw palette color at the call
 * site: a status dot, a chip, and body text for the same state all read from
 * this one list.
 *
 * `error` covers the queue's `undeliverable` as well, because both mean the
 * message cannot get where it is going.
 */
export type OverviewStatusTone =
  | "error"
  | "stalled"
  | "delayed"
  | "running"
  | "idle"
  | "hold"
  | "info";

type ToneClasses = {
  /** The small dot that precedes a name. */
  dot: string;
  /** Status text on a card or table row. */
  text: string;
  /** A badge or chip: tinted fill, tinted border, readable label. */
  badge: string;
  /** A filled surface that keeps a readable label on it. */
  surface: string;
};

/**
 * Literal class strings, kept whole rather than assembled from a color name so
 * Tailwind's static extraction sees every utility it has to generate.
 */
const TONE_CLASSES: Record<OverviewStatusTone, ToneClasses> = {
  error: {
    dot: "bg-status-error",
    text: "text-status-error-text",
    badge: "border-status-error/30 bg-status-error/10 text-status-error-text",
    surface: "border-status-error/30 bg-status-error/10 text-status-error-text",
  },
  stalled: {
    dot: "bg-status-stalled",
    text: "text-status-stalled-text",
    badge: "border-status-stalled/30 bg-status-stalled/10 text-status-stalled-text",
    surface: "border-status-stalled/30 bg-status-stalled/10 text-status-stalled-text",
  },
  delayed: {
    dot: "bg-status-delayed",
    text: "text-status-delayed-text",
    badge: "border-status-delayed/30 bg-status-delayed/10 text-status-delayed-text",
    surface: "border-status-delayed/30 bg-status-delayed/10 text-status-delayed-text",
  },
  running: {
    dot: "bg-status-running",
    text: "text-status-running-text",
    badge: "border-status-running/30 bg-status-running/10 text-status-running-text",
    surface: "border-status-running/30 bg-status-running/10 text-status-running-text",
  },
  idle: {
    dot: "bg-status-idle",
    text: "text-status-idle-text",
    badge: "border-border bg-muted text-status-idle-text",
    surface: "border-border bg-muted text-status-idle-text",
  },
  hold: {
    dot: "bg-status-hold",
    text: "text-status-hold-text",
    badge: "border-status-hold/30 bg-status-hold/10 text-status-hold-text",
    surface: "border-status-hold/30 bg-status-hold/10 text-status-hold-text",
  },
  info: {
    dot: "bg-status-info",
    text: "text-status-info-text",
    badge: "border-status-info/30 bg-status-info/10 text-status-info-text",
    surface: "border-status-info/30 bg-status-info/10 text-status-info-text",
  },
};

const OVERVIEW_STATUS_TONES: Record<OverviewStatus, OverviewStatusTone> = {
  error: "error",
  stalled: "stalled",
  delayed: "delayed",
  running: "running",
  waiting: "idle",
  blocked: "hold",
};

const QUEUE_STATUS_TONES: Record<OverviewQueueStatus, OverviewStatusTone> = {
  undeliverable: "error",
  delayed: "delayed",
  waiting: "idle",
};

/**
 * What each last-24-hours event kind means for a reader, so a completion and a
 * failure do not look alike. `info` is the fallback for a kind this screen has
 * no color for yet: it reads as neutral rather than as a state that was never
 * reported.
 */
const EVENT_KIND_TONES: Record<OverviewEventKind, OverviewStatusTone> = {
  task_created: "info",
  server_started: "info",
  task_completed: "running",
  session_failed: "error",
  automation_run: "info",
  // A block is trouble and its clearing is the resolution, so the pair reads as
  // the failure and its recovery rather than as two neutral facts.
  model_blocked: "stalled",
  model_unblocked: "running",
  pr_merged: "running",
  automation_failed: "error",
  owner_decision: "hold",
  step_move: "info",
};

/** The states the overview classifies, in the order the legend reads. */
export const OVERVIEW_STATUS_LEGEND: OverviewStatus[] = [
  "error",
  "stalled",
  "delayed",
  "running",
  "waiting",
  "blocked",
];

/**
 * The tone for a task or queue state. An absent status is read as `idle`: a
 * row with no classification is not a row in trouble.
 */
export function statusToneName(
  status: OverviewStatus | OverviewQueueStatus | undefined,
): OverviewStatusTone {
  if (!status) return "idle";
  if (status in QUEUE_STATUS_TONES) {
    return QUEUE_STATUS_TONES[status as OverviewQueueStatus];
  }
  return OVERVIEW_STATUS_TONES[status as OverviewStatus] ?? "idle";
}

/** The tone for a last-24-hours event kind, falling back to `info`. */
export function eventToneName(kind: OverviewEventKind | undefined): OverviewStatusTone {
  return (kind && EVENT_KIND_TONES[kind]) || "info";
}

export function statusDotClass(tone: OverviewStatusTone): string {
  return TONE_CLASSES[tone].dot;
}

export function statusTextClass(tone: OverviewStatusTone): string {
  return TONE_CLASSES[tone].text;
}

export function statusBadgeClass(tone: OverviewStatusTone): string {
  return TONE_CLASSES[tone].badge;
}

export function statusSurfaceClass(tone: OverviewStatusTone): string {
  return TONE_CLASSES[tone].surface;
}
