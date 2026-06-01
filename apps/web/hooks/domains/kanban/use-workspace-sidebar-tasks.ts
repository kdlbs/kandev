"use client";

import { useMemo } from "react";
import { useQuery } from "@tanstack/react-query";
import { multiKanbanQueryOptions } from "@/lib/query/query-options/kanban";
import { useAllWorkflowSnapshots } from "@/hooks/domains/kanban/use-all-workflow-snapshots";
import { useWorkflowItems } from "@/hooks/domains/kanban/use-kanban-snapshots";
import {
  aggregateSidebarTasks,
  type AggregatedSidebarTasks,
} from "@/components/task/task-session-sidebar-aggregate";
import type { TaskMoveWorkflow } from "@/components/task/task-move-context-menu";

export type WorkspaceSidebarTasksResult = AggregatedSidebarTasks & {
  workflows: TaskMoveWorkflow[];
  isLoading: boolean;
};

/**
 * Shared data source for the desktop sidebar and the mobile task-switcher sheet.
 *
 * Reads from the TanStack Query `qk.kanban.multi()` cache (populated by
 * `useAllWorkflowSnapshots` and kept fresh by the kanban bridge). Aggregates
 * snapshots from every workflow in the workspace, scoped so that stale
 * snapshots from other workspaces never leak in.
 *
 * Falls back to the active single-workflow `kanban` Zustand slice (still
 * populated by the old WS handler) for tasks that arrive via WS before
 * their snapshot is in the TQ cache — this keeps the sidebar coherent during
 * the incremental migration period.
 */
export function useWorkspaceSidebarTasks(workspaceId: string | null): WorkspaceSidebarTasksResult {
  // Trigger TQ fetch for this workspace's snapshots
  useAllWorkflowSnapshots(workspaceId);

  // TQ cache — primary source
  const { data: multiData, isFetching } = useQuery({
    ...multiKanbanQueryOptions(workspaceId ?? ""),
    enabled: !!workspaceId,
  });

  const workflows = useWorkflowItems(workspaceId);

  const filteredWorkflows = useMemo(
    () => (workspaceId ? workflows.filter((w) => w.workspaceId === workspaceId) : []),
    [workflows, workspaceId],
  );

  const workspaceWorkflowIds = useMemo(
    () => new Set(filteredWorkflows.map((w) => w.id)),
    [filteredWorkflows],
  );

  // TQ snapshots, scoped to this workspace
  const scopedSnapshots = useMemo(() => {
    if (!multiData) return {};
    const result: typeof multiData.snapshots = {};
    for (const [wfId, snap] of Object.entries(multiData.snapshots)) {
      if (workspaceWorkflowIds.has(wfId)) result[wfId] = snap;
    }
    return result;
  }, [multiData, workspaceWorkflowIds]);

  const aggregated = useMemo(() => aggregateSidebarTasks(scopedSnapshots), [scopedSnapshots]);

  const workspaceWorkflows = useMemo<TaskMoveWorkflow[]>(
    () => filteredWorkflows.map((w) => ({ id: w.id, name: w.name, hidden: w.hidden })),
    [filteredWorkflows],
  );

  // Only flash a skeleton on the very first fetch (no snapshots yet).
  const isLoading = isFetching && Object.keys(scopedSnapshots).length === 0;

  return {
    ...aggregated,
    workflows: workspaceWorkflows,
    isLoading,
  };
}
