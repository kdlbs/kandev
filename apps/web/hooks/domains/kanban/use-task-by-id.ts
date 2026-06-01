"use client";

import { useMemo } from "react";
import { useQuery } from "@tanstack/react-query";
import { useAppStore } from "@/components/state-provider";
import { multiKanbanQueryOptions } from "@/lib/query/query-options/kanban";
import { findTaskInSnapshots } from "@/lib/kanban/find-task";
import type { KanbanState } from "@/lib/state/slices/kanban/types";

type Task = KanbanState["tasks"][number];

/**
 * Read-only lookup of a task by ID across all loaded workflow snapshots.
 *
 * Reads from the TanStack Query `qk.kanban.multi()` cache (single source of
 * truth, populated by the kanban bridge + SSR seed).
 *
 * Preserved signature: `useTaskById(taskId)`.
 */
export function useTaskById(taskId: string | null | undefined): Task | null {
  const workspaceId = useAppStore((s) => s.workspaces.activeId);
  const { data } = useQuery({
    ...multiKanbanQueryOptions(workspaceId ?? ""),
    enabled: !!workspaceId,
  });
  return useMemo(() => {
    if (!taskId || !data) return null;
    return findTaskInSnapshots(taskId, data.snapshots) ?? null;
  }, [taskId, data]);
}
