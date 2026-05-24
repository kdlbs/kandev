"use client";

import { useAppStore } from "@/components/state-provider";
import { findTaskInSnapshots } from "@/lib/kanban/find-task";
import type { KanbanState } from "@/lib/state/slices/kanban/types";

type Task = KanbanState["tasks"][number];

/**
 * Read-only lookup of a task by ID across all loaded workflow snapshots.
 *
 * Migration note: during Wave 3 we read primarily from the Zustand kanban
 * slices (still populated by the old WS handlers) so that cross-domain
 * consumers (SenderTaskBadge, PRRowTaskIndicator, etc.) that are not yet
 * migrated continue to work without a QueryClientProvider in their tests.
 *
 * The hook intentionally preserves the original Zustand-based signature
 * so that no cross-domain refactors are needed in this wave. Wave 6
 * (cleanup) will switch this to pure TQ reads once all consumers are
 * migrated.
 *
 * Preserved signature: `useTaskById(taskId)`.
 */
export function useTaskById(taskId: string | null | undefined): Task | null {
  return useAppStore((state) => {
    if (!taskId) return null;
    const fromActive = state.kanban.tasks.find((item: Task) => item.id === taskId);
    if (fromActive) return fromActive;
    return findTaskInSnapshots(taskId, state.kanbanMulti.snapshots);
  });
}
