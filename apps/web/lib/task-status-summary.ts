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
 * Picks the reading to keep when a cached task is rebuilt from an HTTP
 * response. Rejects only a *strictly older* response.
 *
 * Taking the response unconditionally lets a slow response regress the cache to
 * an older revision. A settled task emits no further deltas, so that regression
 * is permanent until the next full hydrate — it surfaced as a task stuck behind
 * a "preparing" spinner in the sidebar after its turn had already finished.
 *
 * Equal revisions must still take the response, because not every field is
 * covered by the revision: `buildTaskDTOsWithSessionInfo` re-stamps
 * `queued_prompt_count` onto the loaded summary from a fresh queue read without
 * incrementing it (the initial-load backstop for queue mutations the projector
 * has not observed, e.g. after a restart). Preferring the cached copy at an
 * equal revision would pin a stale queued badge until the next projector event.
 * The revision-owned fields are identical at equal revisions, so taking the
 * response cannot reintroduce the staleness above.
 */
export function pickFreshestStatusSummary(
  next: TaskStatusSummary | null | undefined,
  current: TaskStatusSummary | null | undefined,
): TaskStatusSummary | null | undefined {
  if (!next) return current;
  if (!current) return next;
  return next.revision >= current.revision ? next : current;
}

/** Select the newest detail/live reading without allowing an HTTP response to regress it. */
export function selectTaskStatusSummary(
  detail: TaskStatusSummary | null | undefined,
  live: Array<TaskStatusSummary | null | undefined>,
): TaskStatusSummary | null | undefined {
  return live.reduce((current, candidate) => pickFreshestStatusSummary(candidate, current), detail);
}

/**
 * Returns the current task-level error only when it has not already been
 * acknowledged or dismissed for the same session and stamp. Task-owned
 * errors have no session key and stay visible until the backend clears them.
 */
export function statusSummaryActiveErrorPreview(
  summary: TaskStatusSummary | null | undefined,
  acknowledgedAgentErrors?: Record<string, string>,
  dismissedAgentErrors?: Record<string, string>,
): string | null {
  const error = summary?.active_error;
  if (!error) return null;
  if (
    error.session_id &&
    (acknowledgedAgentErrors?.[error.session_id] === error.stamp ||
      dismissedAgentErrors?.[error.session_id] === error.stamp)
  ) {
    return null;
  }
  return error.preview;
}
