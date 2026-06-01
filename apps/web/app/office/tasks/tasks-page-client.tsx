"use client";

import { TasksList } from "./tasks-list";

/**
 * Client shell for the office tasks list. `TasksList` owns the full
 * fetch / filter / sort / pagination lifecycle via `usePaginatedTasks`
 * (TanStack Query `useInfiniteQuery`), with WS-driven refresh flowing
 * through the office bridge. No SSR Zustand seed is needed — the list
 * fetches on mount and the office TQ cache is the single source of truth.
 */
export function TasksPageClient() {
  return <TasksList />;
}
