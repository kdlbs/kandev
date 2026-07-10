import { useMemo } from "react";
import { useAppStore } from "@/components/state-provider";
import { useAllWorkflowSnapshots } from "@/hooks/domains/kanban/use-all-workflow-snapshots";
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
 * Fires `useAllWorkflowSnapshots` to populate `kanbanMulti.snapshots` for every
 * workflow in the workspace, then aggregates them (with a fallback to the
 * single active `kanban` slice for tasks that arrived via WS before their
 * snapshot resolved). Snapshots from other workspaces are filtered out so a
 * stale hydration doesn't leak across workspace switches.
 *
 * Assumes `state.workflows.items` is kept in sync with the active workspace by
 * an always-mounted caller (`useEnsureWorkspaceWorkflows` from `AppSidebar`).
 * Do not add the fetch back here — this hook only runs when the Tasks section
 * accordion is expanded, so co-locating the fetch would recreate the original
 * "sidebar stale after workspace switch" bug for collapsed-section users.
 */
export function useWorkspaceSidebarTasks(workspaceId: string | null): WorkspaceSidebarTasksResult {
  useAllWorkflowSnapshots(workspaceId);

  const snapshots = useAppStore((state) => state.kanbanMulti.snapshots);
  const isMultiLoading = useAppStore((state) => state.kanbanMulti.isLoading);
  const workflows = useAppStore((state) => state.workflows.items);
  const activeKanbanWorkflowId = useAppStore((state) => state.kanban.workflowId);
  const activeKanbanTasks = useAppStore((state) => state.kanban.tasks);
  const activeKanbanSteps = useAppStore((state) => state.kanban.steps);

  // While `workspaceId` is unresolved (initial SSR / pre-hydration), return an
  // empty scope rather than every workflow in the store — otherwise snapshots
  // from previously-active workspaces would briefly bleed into the sidebar.
  const filteredWorkflows = useMemo(
    () => (workspaceId ? workflows.filter((w) => w.workspaceId === workspaceId) : []),
    [workflows, workspaceId],
  );
  const workspaceWorkflowIds = useMemo(
    () => new Set(filteredWorkflows.map((w) => w.id)),
    [filteredWorkflows],
  );

  const scopedSnapshots = useMemo(() => {
    const result: typeof snapshots = {};
    for (const [wfId, snap] of Object.entries(snapshots)) {
      if (workspaceWorkflowIds.has(wfId)) result[wfId] = snap;
    }
    return result;
  }, [snapshots, workspaceWorkflowIds]);

  const fallbackWorkflowId =
    activeKanbanWorkflowId && workspaceWorkflowIds.has(activeKanbanWorkflowId)
      ? activeKanbanWorkflowId
      : null;

  const aggregated = useMemo(
    () =>
      aggregateSidebarTasks(
        scopedSnapshots,
        fallbackWorkflowId,
        activeKanbanTasks,
        activeKanbanSteps,
      ),
    [scopedSnapshots, fallbackWorkflowId, activeKanbanTasks, activeKanbanSteps],
  );

  const workspaceWorkflows = useMemo<TaskMoveWorkflow[]>(
    () => filteredWorkflows.map((w) => ({ id: w.id, name: w.name, hidden: w.hidden })),
    [filteredWorkflows],
  );

  // Only flash a skeleton on the very first fetch (no snapshots yet); refreshes
  // shouldn't blow away the existing list.
  const isLoading = isMultiLoading && Object.keys(scopedSnapshots).length === 0;

  return {
    ...aggregated,
    workflows: workspaceWorkflows,
    isLoading,
  };
}
