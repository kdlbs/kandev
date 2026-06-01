"use client";

import { useMemo } from "react";
import { useQuery } from "@tanstack/react-query";
import { multiKanbanQueryOptions } from "@/lib/query/query-options/kanban";
import { useAppStore } from "@/components/state-provider";
import type { KanbanState } from "@/lib/state/slices/kanban/types";

type KanbanTask = KanbanState["tasks"][number];
type KanbanStep = KanbanState["steps"][number];

/**
 * Returns all tasks across every workflow snapshot in the active workspace,
 * read from the TanStack Query `qk.kanban.multi()` cache so WS events update
 * the UI without a hard refresh.
 */
export function useAllKanbanTasks(): KanbanTask[] {
  const workspaceId = useAppStore((s) => s.workspaces.activeId);
  const { data } = useQuery({
    ...multiKanbanQueryOptions(workspaceId ?? ""),
    enabled: !!workspaceId,
  });
  return useMemo(() => {
    if (!data) return [];
    const tasks: KanbanTask[] = [];
    for (const snap of Object.values(data.snapshots)) {
      for (const t of snap.tasks) tasks.push(t);
    }
    return tasks;
  }, [data]);
}

/**
 * Returns the steps for the *contextually active* workflow, reading the matching
 * snapshot from the TanStack Query cache.
 *
 * Resolution mirrors the deleted Zustand `kanban.workflowId`: prefer the
 * workflow that owns the active task (so the task page's stepper / command panel
 * show the task's own workflow even when the homepage filter is "All Workflows"
 * or a different workflow), and only fall back to the homepage filter
 * (`workflows.activeId`) when there is no active-task context. Using the filter
 * alone — as a naive port did — broke the task page: a task created in a
 * non-filtered workflow rendered an empty stepper.
 *
 * Falls back to an empty array when nothing has loaded yet.
 */
export function useActiveWorkflowSteps(): KanbanStep[] {
  const workspaceId = useAppStore((s) => s.workspaces.activeId);
  const filterWorkflowId = useAppStore((s) => s.workflows.activeId);
  const activeTaskId = useAppStore((s) => s.tasks.activeTaskId);
  const { data } = useQuery({
    ...multiKanbanQueryOptions(workspaceId ?? ""),
    enabled: !!workspaceId,
  });
  return useMemo(() => {
    if (!data) return [];
    if (activeTaskId) {
      const owning = Object.values(data.snapshots).find((snap) =>
        snap.tasks.some((t) => t.id === activeTaskId),
      );
      if (owning) return owning.steps;
    }
    if (filterWorkflowId) return data.snapshots[filterWorkflowId]?.steps ?? [];
    return [];
  }, [data, filterWorkflowId, activeTaskId]);
}

/**
 * Returns all snapshots for the active workspace.
 *
 * Useful for components that need the {tasks, steps, workflowName} per-workflow
 * structure for cross-workflow lookups.
 */
export function useKanbanSnapshots(): Record<
  string,
  import("@/lib/state/slices/kanban/types").WorkflowSnapshotData
> {
  const workspaceId = useAppStore((s) => s.workspaces.activeId);
  const { data } = useQuery({
    ...multiKanbanQueryOptions(workspaceId ?? ""),
    enabled: !!workspaceId,
  });
  return data?.snapshots ?? EMPTY_SNAPSHOTS;
}

const EMPTY_SNAPSHOTS: Record<
  string,
  import("@/lib/state/slices/kanban/types").WorkflowSnapshotData
> = {};
