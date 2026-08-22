"use client";

import { useMemo } from "react";
import { useOptionalAppStore } from "@/components/state-provider";
import { selectTaskStatusSummary } from "@/lib/task-status-summary";
import type { TaskStatusSummary } from "@/lib/types/task-status-summary";

type StatusSummaryTask = {
  id: string;
  statusSummary?: TaskStatusSummary | null;
};

export type TaskStatusSummaryState = {
  kanban: { tasks: StatusSummaryTask[] };
  kanbanMulti: { snapshots: Record<string, { tasks: StatusSummaryTask[] }> };
  sidebarArchivedTasks: {
    itemsByWorkspaceId: Record<string, StatusSummaryTask[]>;
  };
};

export function collectTaskStatusSummaryCandidates(
  state: TaskStatusSummaryState,
  taskId: string,
): Array<TaskStatusSummary | null | undefined> {
  const candidates: Array<TaskStatusSummary | null | undefined> = [];
  const addCandidate = (candidate: StatusSummaryTask) => {
    if (candidate.id === taskId) candidates.push(candidate.statusSummary);
  };
  state.kanban.tasks.forEach(addCandidate);
  Object.values(state.kanbanMulti.snapshots).forEach((snapshot) =>
    snapshot.tasks.forEach(addCandidate),
  );
  Object.values(state.sidebarArchivedTasks.itemsByWorkspaceId).forEach((items) =>
    items.forEach(addCandidate),
  );
  return candidates;
}

export function useTaskStatusSummary(
  taskId: string | null | undefined,
  detail: TaskStatusSummary | null | undefined,
): TaskStatusSummary | null | undefined {
  const live = useOptionalAppStore((state) => {
    if (!taskId) return null;
    return selectTaskStatusSummary(undefined, collectTaskStatusSummaryCandidates(state, taskId));
  }, null);

  return useMemo(() => selectTaskStatusSummary(detail, [live]), [detail, live]);
}
