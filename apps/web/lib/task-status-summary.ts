import type { TaskStatusSummary } from "./types/task-status-summary";

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
