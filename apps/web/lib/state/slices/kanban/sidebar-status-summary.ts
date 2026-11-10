import type { TaskStatusSummary } from "@/lib/types/task-status-summary";
import {
  isNewerStatusSummary,
  MAX_SIDEBAR_STATUS_SUMMARIES,
  MAX_SIDEBAR_SUMMARY_WORKSPACES,
} from "@/lib/task-status-summary";
import type { KanbanSliceState } from "./types";

/** Retains one live task summary under the same bounded cache the sidebar
 * summary event handler uses. Ephemeral Quick Chat tasks are not board rows but
 * still need an authoritative summary for dismissal and acknowledgement. */
export function recordRetainedSidebarStatusSummary(
  draft: KanbanSliceState,
  workspaceId: string,
  taskId: string,
  summary: TaskStatusSummary,
): void {
  if (!workspaceId || !taskId) return;
  const currentByTaskId = draft.sidebarStatusSummaryByWorkspaceId[workspaceId] ?? {};
  if (!isNewerStatusSummary(summary, currentByTaskId[taskId])) return;
  const nextByTaskId = { ...currentByTaskId, [taskId]: summary };
  const overflow = Object.keys(nextByTaskId).length - MAX_SIDEBAR_STATUS_SUMMARIES;
  for (const oldestTaskId of Object.keys(nextByTaskId).slice(0, Math.max(0, overflow))) {
    delete nextByTaskId[oldestTaskId];
  }
  draft.sidebarStatusSummaryByWorkspaceId[workspaceId] = nextByTaskId;
  const workspaceIds = Object.keys(draft.sidebarStatusSummaryByWorkspaceId);
  for (const oldestWorkspaceId of workspaceIds.slice(
    0,
    Math.max(0, workspaceIds.length - MAX_SIDEBAR_SUMMARY_WORKSPACES),
  )) {
    if (oldestWorkspaceId !== workspaceId) {
      delete draft.sidebarStatusSummaryByWorkspaceId[oldestWorkspaceId];
    }
  }
}
