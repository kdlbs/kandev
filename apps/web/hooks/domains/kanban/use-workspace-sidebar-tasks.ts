import { useMemo, useRef } from "react";
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

type SidebarTask = AggregatedSidebarTasks["allTasks"][number];

function shallowTaskEqual(previous: SidebarTask, next: SidebarTask): boolean {
  const previousKeys = Object.keys(previous) as Array<keyof SidebarTask>;
  const nextKeys = Object.keys(next) as Array<keyof SidebarTask>;
  return (
    previousKeys.length === nextKeys.length &&
    nextKeys.every((key) => Object.is(previous[key], next[key]))
  );
}

function reuseUnchangedTasks(previous: SidebarTask[], next: SidebarTask[]): SidebarTask[] {
  // Aggregation stamps `_workflowId` onto fresh objects. Restore the store's
  // per-task structural sharing so downstream view models can stay granular.
  const previousById = new Map(previous.map((task) => [task.id, task]));
  let changed = previous.length !== next.length;
  const shared = next.map((task, index) => {
    const prior = previousById.get(task.id);
    const value = prior && shallowTaskEqual(prior, task) ? prior : task;
    if (value !== previous[index]) changed = true;
    return value;
  });
  return changed ? shared : previous;
}

function reuseReferenceArray<T>(previous: T[], next: T[]): T[] {
  return previous.length === next.length && previous.every((value, index) => value === next[index])
    ? previous
    : next;
}

function reuseStepRecord(
  previous: AggregatedSidebarTasks["stepsByWorkflowId"],
  next: AggregatedSidebarTasks["stepsByWorkflowId"],
): AggregatedSidebarTasks["stepsByWorkflowId"] {
  const previousKeys = Object.keys(previous);
  const nextKeys = Object.keys(next);
  let changed = previousKeys.length !== nextKeys.length;
  const shared: AggregatedSidebarTasks["stepsByWorkflowId"] = {};
  for (const workflowId of nextKeys) {
    const steps = reuseReferenceArray(previous[workflowId] ?? [], next[workflowId]);
    shared[workflowId] = steps;
    if (steps !== previous[workflowId]) changed = true;
  }
  return changed ? shared : previous;
}

function useSharedStepMetadata(aggregated: AggregatedSidebarTasks) {
  const previousAllStepsRef = useRef<AggregatedSidebarTasks["allSteps"]>([]);
  const allSteps = reuseReferenceArray(previousAllStepsRef.current, aggregated.allSteps);
  previousAllStepsRef.current = allSteps;
  const previousStepsByWorkflowRef = useRef<AggregatedSidebarTasks["stepsByWorkflowId"]>({});
  const stepsByWorkflowId = reuseStepRecord(
    previousStepsByWorkflowRef.current,
    aggregated.stepsByWorkflowId,
  );
  previousStepsByWorkflowRef.current = stepsByWorkflowId;
  return { allSteps, stepsByWorkflowId };
}

function buildWipQueueByTaskId(
  allTasks: SidebarTask[],
  allSteps: AggregatedSidebarTasks["allSteps"],
  stepsByWorkflowId: AggregatedSidebarTasks["stepsByWorkflowId"],
): Map<string, WipQueueStatus> {
  const result = new Map<string, WipQueueStatus>();
  const activeTasks = allTasks.filter((task) => task.isArchived !== true);
  const destinationStepIds = new Set(
    activeTasks.map((task) => task.queuedForStepId).filter((stepId): stepId is string => !!stepId),
  );
  for (const stepId of destinationStepIds) {
    for (const entry of getDestinationQueue(activeTasks, stepId)) {
      const task = entry.task;
      const stepTitle =
        stepsByWorkflowId[task._workflowId]?.find((step) => step.id === stepId)?.title ??
        allSteps.find((step) => step.id === stepId)?.title ??
        stepId;
      result.set(task.id, {
        position: entry.position,
        total: entry.total,
        destinationTitle: stepTitle,
      });
    }
  }
  return result;
}

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

  const { allSteps, stepsByWorkflowId } = useSharedStepMetadata(aggregated);

  const previousTasksRef = useRef<SidebarTask[]>([]);
  const allTasks = useMemo(() => {
    const merged = mergeSidebarArchivedTasks(
      aggregated.allTasks,
      archived.tasks,
      workspaceId,
      needsArchivedTasks,
    );
    const shared = reuseUnchangedTasks(previousTasksRef.current, merged);
    previousTasksRef.current = shared;
    return shared;
  }, [aggregated.allTasks, archived.tasks, needsArchivedTasks, workspaceId]);

  const wipQueueByTaskId = useMemo(
    () => buildWipQueueByTaskId(allTasks, allSteps, stepsByWorkflowId),
    [allTasks, allSteps, stepsByWorkflowId],
  );

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
    allSteps,
    stepsByWorkflowId,
    wipQueueByTaskId,
    workflows: workspaceWorkflows,
    isLoading,
    archivedError: archived.error,
    retryArchivedTasks: archived.refresh,
  };
}
