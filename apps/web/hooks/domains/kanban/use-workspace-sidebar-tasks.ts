import { useMemo } from "react";
import { useAppStore } from "@/components/state-provider";
import { useAllWorkflowSnapshots } from "@/hooks/domains/kanban/use-all-workflow-snapshots";
import { useSidebarArchivedTasks } from "@/hooks/domains/kanban/use-sidebar-archived-tasks";
import { viewRequiresArchivedTasks } from "@/lib/sidebar/apply-view";
import {
  aggregateSidebarTasks,
  type AggregatedSidebarTasks,
} from "@/components/task/task-session-sidebar-aggregate";
import type { TaskMoveWorkflow } from "@/components/task/task-move-context-menu";
import type { KanbanState } from "@/lib/state/slices/kanban/types";
import { getDestinationQueue, type WipQueueStatus } from "@/lib/kanban/wip-queue";

export type WorkspaceSidebarTasksResult = AggregatedSidebarTasks & {
  workflows: TaskMoveWorkflow[];
  wipQueueByTaskId: Map<string, WipQueueStatus>;
  isLoading: boolean;
  archivedError: string | null;
  retryArchivedTasks: () => void;
};

export function mergeSidebarArchivedTasks(
  activeTasks: AggregatedSidebarTasks["allTasks"],
  archivedTasks: KanbanState["tasks"],
  workspaceId: string | null,
  enabled: boolean,
): AggregatedSidebarTasks["allTasks"] {
  if (!enabled || !workspaceId || archivedTasks.length === 0) return activeTasks;
  const seen = new Set(activeTasks.map((task) => task.id));
  const archived = archivedTasks
    .filter((task) => task.workspaceId === workspaceId && task.isArchived)
    .filter((task) => {
      if (seen.has(task.id)) return false;
      seen.add(task.id);
      return true;
    })
    .map((task) => ({ ...task, _workflowId: task.workflowId ?? "" }));
  return archived.length > 0 ? [...activeTasks, ...archived] : activeTasks;
}

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

  const sidebarViews = useAppStore((state) => state.sidebarViews);
  const effectiveView = useMemo(() => {
    const active =
      sidebarViews?.views.find((view) => view.id === sidebarViews.activeViewId) ??
      sidebarViews?.views[0];
    if (!active) return undefined;
    const draft = sidebarViews.draft;
    if (!draft || draft.baseViewId !== active.id) return active;
    return { ...active, filters: draft.filters, sort: draft.sort, group: draft.group };
  }, [sidebarViews]);
  const needsArchivedTasks = viewRequiresArchivedTasks(effectiveView);
  const archived = useSidebarArchivedTasks(workspaceId, needsArchivedTasks);

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

  const allTasks = useMemo(() => {
    return mergeSidebarArchivedTasks(
      aggregated.allTasks,
      archived.tasks,
      workspaceId,
      needsArchivedTasks,
    );
  }, [aggregated.allTasks, archived.tasks, needsArchivedTasks, workspaceId]);

  const wipQueueByTaskId = useMemo(() => {
    const result = new Map<string, WipQueueStatus>();
    const destinationStepIds = new Set(
      allTasks.map((task) => task.queuedForStepId).filter((stepId): stepId is string => !!stepId),
    );
    for (const stepId of destinationStepIds) {
      for (const entry of getDestinationQueue(allTasks, stepId)) {
        const task = entry.task;
        const stepTitle =
          aggregated.stepsByWorkflowId[task._workflowId]?.find((step) => step.id === stepId)
            ?.title ??
          aggregated.allSteps.find((step) => step.id === stepId)?.title ??
          stepId;
        result.set(task.id, {
          position: entry.position,
          total: entry.total,
          destinationTitle: stepTitle,
        });
      }
    }
    return result;
  }, [allTasks, aggregated.allSteps, aggregated.stepsByWorkflowId]);

  const workspaceWorkflows = useMemo<TaskMoveWorkflow[]>(
    () => filteredWorkflows.map((w) => ({ id: w.id, name: w.name, hidden: w.hidden })),
    [filteredWorkflows],
  );

  // Only flash a skeleton on the very first fetch (no snapshots yet); refreshes
  // shouldn't blow away the existing list.
  const isLoading =
    (isMultiLoading && Object.keys(scopedSnapshots).length === 0) || archived.isLoading;

  return {
    ...aggregated,
    allTasks,
    wipQueueByTaskId,
    workflows: workspaceWorkflows,
    isLoading,
    archivedError: archived.error,
    retryArchivedTasks: archived.refresh,
  };
}
