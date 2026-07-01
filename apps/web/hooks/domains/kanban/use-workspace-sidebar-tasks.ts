import { useMemo } from "react";
import { useAllWorkflowSnapshots } from "@/hooks/domains/kanban/use-all-workflow-snapshots";
import { useCachedWorkflows } from "@/hooks/use-workflow-cache";
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
 * Fires `useAllWorkflowSnapshots` for every workflow in the workspace, then
 * aggregates Query-owned snapshots. Snapshots from other workspaces are
 * filtered out so stale hydration doesn't leak across workspace switches.
 *
 * Assumes the active workspace workflow query is kept warm by an always-mounted
 * caller (`useEnsureWorkspaceWorkflows` from `AppSidebar`). Do not add the fetch
 * back here — this hook only runs when the Tasks section accordion is expanded,
 * so co-locating the fetch would recreate the collapsed-section staleness bug.
 */
export function useWorkspaceSidebarTasks(workspaceId: string | null): WorkspaceSidebarTasksResult {
  const { snapshots, isLoading: snapshotsLoading } = useAllWorkflowSnapshots(workspaceId);

  const workflows = useCachedWorkflows(workspaceId);

  // While `workspaceId` is unresolved (initial SSR / pre-hydration), return an
  // empty scope rather than every workflow in the store — otherwise snapshots
  // from previously-active workspaces would briefly bleed into the sidebar.
  const filteredWorkflows = useMemo(() => (workspaceId ? workflows : []), [workflows, workspaceId]);
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

  const aggregated = useMemo(() => aggregateSidebarTasks(scopedSnapshots), [scopedSnapshots]);

  const workspaceWorkflows = useMemo<TaskMoveWorkflow[]>(
    () => filteredWorkflows.map((w) => ({ id: w.id, name: w.name, hidden: w.hidden })),
    [filteredWorkflows],
  );

  // Only flash a skeleton on the very first fetch (no snapshots yet); refreshes
  // shouldn't blow away the existing list.
  const isLoading = snapshotsLoading && Object.keys(scopedSnapshots).length === 0;

  return {
    ...aggregated,
    workflows: workspaceWorkflows,
    isLoading,
  };
}
