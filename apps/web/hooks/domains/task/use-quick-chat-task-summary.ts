"use client";

import { useEffect } from "react";
import { useAppStore } from "@/components/state-provider";
import { fetchTask } from "@/lib/api/domains/kanban-api";

/**
 * Hydrates retained status attention for an ephemeral Quick Chat task, which is
 * intentionally absent from board/overview collections. The bounded live
 * summary cache is authoritative once populated; this fills it on open and on a
 * full reload so dismissal and prompt acknowledgement see current attention.
 */
export function useQuickChatTaskSummary(
  taskId: string | null,
  hasBoardTask: boolean,
  workspaceId: string | null,
): void {
  const hasRetainedSummary = useAppStore((state) =>
    taskId
      ? Object.values(state.sidebarStatusSummaryByWorkspaceId ?? {}).some(
          (summaries) => summaries?.[taskId] != null,
        )
      : false,
  );
  const recordSidebarStatusSummary = useAppStore((state) => state.recordSidebarStatusSummary);

  useEffect(() => {
    if (!taskId || hasBoardTask || hasRetainedSummary) return;
    let active = true;
    void fetchTask(taskId, { cache: "no-store" }).then(
      (fetched) => {
        if (active && fetched.status_summary) {
          recordSidebarStatusSummary(
            fetched.workspace_id ?? workspaceId ?? "",
            taskId,
            fetched.status_summary,
          );
        }
      },
      () => undefined,
    );
    return () => {
      active = false;
    };
  }, [hasBoardTask, hasRetainedSummary, recordSidebarStatusSummary, taskId, workspaceId]);
}
