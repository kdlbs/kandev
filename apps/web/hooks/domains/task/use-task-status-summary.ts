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
  taskOverview: { byId: Record<string, StatusSummaryTask> };
  kanban: { tasks: StatusSummaryTask[] };
  kanbanMulti: { snapshots: Record<string, { tasks: StatusSummaryTask[] }> };
  sidebarArchivedTasks: {
    itemsByWorkspaceId: Record<string, StatusSummaryTask[]>;
  };
  sidebarStatusSummaryByWorkspaceId?: Record<string, Record<string, TaskStatusSummary>>;
};

export function collectTaskStatusSummaryCandidates(
  state: TaskStatusSummaryState,
  taskId: string,
): Array<TaskStatusSummary | null | undefined> {
  const candidates: Array<TaskStatusSummary | null | undefined> = [];
  const addCandidate = (candidate: StatusSummaryTask) => {
    if (candidate.id === taskId) candidates.push(candidate.statusSummary);
  };
  // Focused test harnesses and early boot may hydrate only part of the store;
  // a missing collection is an empty candidate source, not a crash.
  const canonical = state.taskOverview?.byId?.[taskId];
  if (canonical) addCandidate(canonical);
  state.kanban?.tasks?.forEach(addCandidate);
  Object.values(state.kanbanMulti?.snapshots ?? {}).forEach((snapshot) =>
    snapshot.tasks?.forEach(addCandidate),
  );
  Object.values(state.sidebarArchivedTasks?.itemsByWorkspaceId ?? {}).forEach((items) =>
    items?.forEach(addCandidate),
  );
  // Ephemeral Quick Chat tasks are intentionally absent from board/overview
  // collections. The bounded live summary cache is their retained authoritative
  // source after a summary event, and is also hydrated on reopen.
  Object.values(state.sidebarStatusSummaryByWorkspaceId ?? {}).forEach((summaries) => {
    const candidate = summaries?.[taskId];
    if (candidate) candidates.push(candidate);
  });
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
