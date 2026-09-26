import type { RunStatus } from "@/lib/types/automation";

export type RunStatusFilter = RunStatus | "all";

export const ALL_STATUSES: RunStatusFilter = "all";

/**
 * In this feed the outcome prose is the content and status is metadata, so a
 * run wears a small coloured dot plus a word rather than the loud badge the
 * per-automation table uses.
 */
export const RUN_STATUS_LABEL_KEY: Record<RunStatus, string> = {
  triggered: "automations:runTriggered",
  task_created: "automations:runRunning",
  succeeded: "automations:runSucceeded",
  failed: "automations:runFailed",
  skipped: "automations:runSkipped",
  archived: "automations:runArchived",
  cancelled: "automations:runCancelled",
};

export const RUN_STATUS_DOT: Record<RunStatus, string> = {
  triggered: "bg-muted-foreground",
  task_created: "bg-blue-500",
  succeeded: "bg-emerald-500",
  failed: "bg-red-500",
  skipped: "bg-muted-foreground/50",
  archived: "bg-muted-foreground/50",
  cancelled: "bg-amber-500",
};

/**
 * Filter options carrying catalog keys rather than copy — this is a plain
 * module, so the label is resolved by whoever renders the control.
 */
export const STATUS_FILTER_OPTIONS: { value: RunStatusFilter; labelKey: string }[] = [
  { value: ALL_STATUSES, labelKey: "automations:runAll" },
  // Every status the backend can hand us is offered. A status that renders in
  // the feed but cannot be filtered leaves the reader unable to narrow to it.
  { value: "triggered", labelKey: RUN_STATUS_LABEL_KEY.triggered },
  { value: "task_created", labelKey: RUN_STATUS_LABEL_KEY.task_created },
  { value: "succeeded", labelKey: RUN_STATUS_LABEL_KEY.succeeded },
  { value: "failed", labelKey: RUN_STATUS_LABEL_KEY.failed },
  // Kept in the list even though a skipped run says almost nothing: a schedule
  // turned away by the concurrency cap writes one of these and nothing else, so
  // without the filter a jammed automation is indistinguishable from an idle one.
  { value: "skipped", labelKey: RUN_STATUS_LABEL_KEY.skipped },
  { value: "archived", labelKey: RUN_STATUS_LABEL_KEY.archived },
  { value: "cancelled", labelKey: RUN_STATUS_LABEL_KEY.cancelled },
];

/**
 * The statuses that mean the run has not finished. Every other status is
 * terminal, including the two derived at read time (archived, cancelled).
 */
export const OPEN_RUN_STATUSES: RunStatus[] = ["triggered", "task_created"];

export function isOpenRun(status: RunStatus): boolean {
  return OPEN_RUN_STATUSES.includes(status);
}

/**
 * Split runs into what is happening and what already happened.
 *
 * The detail view groups rather than filters: a run in flight is the single
 * most interesting thing on the page, and a filter would hide it behind a
 * control the user has to know to press.
 */
export function groupRunsByState<T extends { status: RunStatus }>(
  runs: T[],
): { running: T[]; completed: T[] } {
  const running: T[] = [];
  const completed: T[] = [];
  for (const run of runs) {
    if (isOpenRun(run.status)) running.push(run);
    else completed.push(run);
  }
  return { running, completed };
}

/** Catalog key for a status, falling back to `triggered` for an unknown one. */
export function statusLabelKey(status: RunStatus): string {
  return RUN_STATUS_LABEL_KEY[status] ?? RUN_STATUS_LABEL_KEY.triggered;
}

export function statusDotClass(status: RunStatus): string {
  return RUN_STATUS_DOT[status] ?? RUN_STATUS_DOT.triggered;
}
