import type { TaskStatusSummary } from "./types/task-status-summary";

/**
 * True when `next` is a strictly newer reading of the same task's status
 * summary than `current`.
 *
 * The summary is a monotonically-revisioned projection delivered over two
 * independent paths: `task.status_summary.updated` WS deltas, and whole-task
 * payloads from HTTP hydration (workflow snapshots, boot payload). Both write
 * the same cache, so ordering between them is not guaranteed — a snapshot
 * request issued before a delta can land after it. Every writer must therefore
 * compare revisions rather than assume it holds the latest reading.
 *
 * An absent `next` is never newer: HTTP payloads use `omitempty`, so a missing
 * summary means "not carried by this response", not "cleared".
 */
export function isNewerStatusSummary(
  next: TaskStatusSummary | null | undefined,
  current: TaskStatusSummary | null | undefined,
): boolean {
  if (!next) return false;
  if (!current) return true;
  return next.revision > current.revision;
}

/**
 * Picks the newer of two readings of the same task's status summary.
 *
 * Used where a cached task is rebuilt from an HTTP response: taking the
 * response's summary unconditionally lets a slow response regress the cache to
 * an older revision. A settled task emits no further deltas, so that regression
 * is permanent until the next full hydrate — it surfaced as a task stuck behind
 * a "preparing" spinner in the sidebar after its turn had already finished.
 */
export function pickNewerStatusSummary(
  next: TaskStatusSummary | null | undefined,
  current: TaskStatusSummary | null | undefined,
): TaskStatusSummary | null | undefined {
  return isNewerStatusSummary(next, current) ? next : current;
}

/**
 * Returns the current task-level error only when it has not already been
 * acknowledged or dismissed for the same session and stamp.
 */
export function statusSummaryActiveErrorPreview(
  summary: TaskStatusSummary | null | undefined,
  acknowledgedAgentErrors?: Record<string, string>,
  dismissedAgentErrors?: Record<string, string>,
): string | null {
  const error = summary?.active_error;
  if (!error) return null;
  if (
    acknowledgedAgentErrors?.[error.session_id] === error.stamp ||
    dismissedAgentErrors?.[error.session_id] === error.stamp
  ) {
    return null;
  }
  return error.preview;
}
