import { matchesSidebarClause } from "@/lib/sidebar/sidebar-local-filter";
import { sidebarCandidate } from "@/lib/sidebar/sidebar-local-projection";
import type { SidebarTaskQuery } from "@/lib/types/http";
import type { TaskOverview } from "@/lib/state/slices/task-overview-types";
import { useMemo, useRef } from "react";
import { useAppStore } from "@/components/state-provider";
import { useSidebarTaskPage } from "@/hooks/domains/kanban/use-sidebar-task-page";
import { useSidebarStoreTasks } from "@/hooks/domains/kanban/use-sidebar-store-tasks";
import { toKanbanTask } from "@/lib/kanban/map-task";
import type { AggregatedSidebarTasks } from "@/components/task/task-session-sidebar-aggregate";
import type { TaskMoveWorkflow } from "@/components/task/task-move-context-menu";
import type { WorkspaceContextReadError } from "@/lib/state/slices/kanban/types";
import type { AppState } from "@/lib/state/store";
import { getDestinationQueue, type WipQueueStatus } from "@/lib/kanban/wip-queue";
import type { TaskStatusSummary } from "@/lib/types/task-status-summary";
import { pickFreshestStatusSummary } from "@/lib/task-status-summary";
import { useShallow } from "zustand/react/shallow";

export type WorkspaceSidebarTasksResult = AggregatedSidebarTasks & {
  pendingArchiveTaskIds: ReadonlySet<string>;
  workflows: TaskMoveWorkflow[];
  wipQueueByTaskId: Map<string, WipQueueStatus>;
  isLoading: boolean;
  archivedError: string | null;
  retryArchivedTasks: () => void;
  page: ReturnType<typeof useSidebarTaskPage>;
  pageEntries:
    | NonNullable<ReturnType<typeof useSidebarTaskPage>["response"]>["entries"]
    | undefined;
  workspaceContextError: WorkspaceContextReadError | null;
  workspaceContextPending: boolean;
  workspaceContextAccessDenied: boolean;
  retryWorkspaceContext: (() => void) | undefined;
};

const NOOP_REFRESH = () => {};
const EMPTY_PAGE_ENTRIES: NonNullable<WorkspaceSidebarTasksResult["pageEntries"]> = [];

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

function buildWipQueueByTaskId(
  entries: WorkspaceSidebarTasksResult["pageEntries"],
  allSteps: AggregatedSidebarTasks["allSteps"],
  stepsByWorkflowId: AggregatedSidebarTasks["stepsByWorkflowId"],
  allTasks: SidebarTask[],
): Map<string, WipQueueStatus> {
  if (!entries) return buildStoreWipQueue(allTasks, allSteps);
  const result = new Map<string, WipQueueStatus>();
  const taskById = new Map(allTasks.map((task) => [task.id, task]));
  for (const entry of entries) {
    if (
      entry.kind !== "task" ||
      !entry.task_id ||
      !entry.wip_queue_position ||
      !entry.wip_queue_total
    ) {
      continue;
    }
    const task = taskById.get(entry.task_id);
    if (!task) continue;
    const stepId = task.queuedForStepId;
    if (!stepId) continue;
    const stepTitle =
      entry.workflow_step_name ??
      stepsByWorkflowId[task.workflowId]?.find((step) => step.id === stepId)?.title ??
      allSteps.find((step) => step.id === stepId)?.title ??
      stepId;
    result.set(task.id, {
      position: entry.wip_queue_position,
      total: entry.wip_queue_total,
      destinationTitle: stepTitle,
    });
  }
  return result;
}

function buildStoreWipQueue(allTasks: SidebarTask[], allSteps: AggregatedSidebarTasks["allSteps"]) {
  const result = new Map<string, WipQueueStatus>();
  for (const stepId of new Set(allTasks.map((task) => task.queuedForStepId))) {
    if (!stepId) continue;
    for (const entry of getDestinationQueue(allTasks, stepId)) {
      result.set(entry.task.id, {
        position: entry.position,
        total: entry.total,
        destinationTitle: allSteps.find((step) => step.id === stepId)?.title ?? stepId,
      });
    }
  }
  return result;
}

function matchesWorkspaceContextRead(
  read: AppState["workspaceContextRead"],
  generation: number,
  workspaceId: string | null,
): boolean {
  return read?.workspaceId === workspaceId && read?.generation === generation;
}

function workspaceContextErrors(
  read: AppState["workspaceContextRead"],
): WorkspaceContextReadError[] {
  return [...Object.values(read?.errors ?? {}), read?.snapshotError ?? null].filter(
    (error): error is WorkspaceContextReadError => error !== null,
  );
}

function workspaceContextIsPending(read: AppState["workspaceContextRead"]): boolean {
  const pendingCollection = Object.entries(read?.pending ?? {}).some(
    ([collection, pending]) =>
      pending &&
      (read?.requestIds === undefined ||
        read.requestIds[collection as keyof typeof read.requestIds] !== null),
  );
  const pendingSnapshot =
    read?.snapshotPending === true &&
    (read.snapshotRequestId === undefined || read.snapshotRequestId !== null);
  return pendingCollection || pendingSnapshot;
}

function getWorkspaceContextStatus(
  workspaceContextRead: AppState["workspaceContextRead"],
  workspaceContextGeneration: number,
  workspaceId: string | null,
) {
  const matches = matchesWorkspaceContextRead(
    workspaceContextRead,
    workspaceContextGeneration,
    workspaceId,
  );
  const errors = matches ? workspaceContextErrors(workspaceContextRead) : [];
  return {
    error: errors.find((error) => error === "access_denied") ?? errors[0] ?? null,
    accessDenied: errors.includes("access_denied"),
    pending: matches && workspaceContextIsPending(workspaceContextRead),
    canRetry: !matches || workspaceContextRead.snapshotError !== "access_denied",
  };
}

function projectSidebarTasks(
  entries: NonNullable<ReturnType<typeof useSidebarTaskPage>["response"]>["entries"],
  byId: AppState["taskOverview"]["byId"],
  summaries: Record<string, TaskStatusSummary>,
  filters: SidebarTaskQuery["filters"],
  activeTaskOnly: boolean,
) {
  return entries.flatMap((entry): SidebarTask[] => {
    if (entry.kind !== "task") return [];
    const task = byId[entry.task_id ?? ""] ?? (entry.task ? toKanbanTask(entry.task) : undefined);
    if (!task || (!activeTaskOnly && !eligibleArchiveMembership(task, filters))) return [];
    return [
      {
        ...task,
        _workflowId: task.workflowId,
        statusSummary: pickFreshestStatusSummary(task.statusSummary, summaries[task.id]),
      },
    ];
  });
}

function eligibleArchiveMembership(task: TaskOverview, filters: SidebarTaskQuery["filters"]) {
  if (!sidebarCandidate(task)) return false;
  const clauses = filters.filter((clause) => clause.dimension === "archived");
  return clauses.length
    ? clauses.every((clause) => matchesSidebarClause(String(task.isArchived === true), clause))
    : !task.isArchived;
}

function useWorkspaceWorkflowMetadata(workspaceId: string | null) {
  const snapshots = useAppStore((state) => state.kanbanMulti.snapshots);
  const workflows = useAppStore((state) => state.workflows.items);
  const activeKanbanWorkflowId = useAppStore((state) => state.kanban.workflowId);
  const activeKanbanSteps = useAppStore((state) => state.kanban.steps);
  const filteredWorkflows = useMemo(
    () => (workspaceId ? workflows.filter((workflow) => workflow.workspaceId === workspaceId) : []),
    [workflows, workspaceId],
  );
  const workspaceWorkflowIds = useMemo(
    () => new Set(filteredWorkflows.map((workflow) => workflow.id)),
    [filteredWorkflows],
  );
  const scopedSnapshots = useMemo(() => {
    const result: typeof snapshots = {};
    for (const [workflowId, snapshot] of Object.entries(snapshots)) {
      if (workspaceWorkflowIds.has(workflowId)) result[workflowId] = snapshot;
    }
    return result;
  }, [snapshots, workspaceWorkflowIds]);
  const stepsByWorkflowId = useMemo<AggregatedSidebarTasks["stepsByWorkflowId"]>(() => {
    const result: AggregatedSidebarTasks["stepsByWorkflowId"] = {};
    for (const [workflowId, snapshot] of Object.entries(scopedSnapshots)) {
      result[workflowId] = [...snapshot.steps].sort((a, b) => a.position - b.position);
    }
    if (
      activeKanbanWorkflowId &&
      workspaceWorkflowIds.has(activeKanbanWorkflowId) &&
      activeKanbanSteps.length > 0
    ) {
      result[activeKanbanWorkflowId] = [...activeKanbanSteps].sort(
        (a, b) => a.position - b.position,
      );
    }
    return result;
  }, [activeKanbanSteps, activeKanbanWorkflowId, scopedSnapshots, workspaceWorkflowIds]);
  const allSteps = useMemo(() => {
    const stepById = new Map<
      AggregatedSidebarTasks["allSteps"][number]["id"],
      AggregatedSidebarTasks["allSteps"][number]
    >();
    for (const steps of Object.values(stepsByWorkflowId)) {
      for (const step of steps) stepById.set(step.id, step);
    }
    return [...stepById.values()].sort((a, b) => a.position - b.position);
  }, [stepsByWorkflowId]);
  return { filteredWorkflows, stepsByWorkflowId, allSteps };
}

function sidebarSourceEntries(
  response: SidebarTaskPageResponse | null,
  activeTaskId: string | null,
  activeTaskOnly: boolean,
): SidebarTaskPageResponse["entries"] {
  if (!activeTaskOnly) return response?.entries ?? EMPTY_PAGE_ENTRIES;
  return activeTaskId ? [{ kind: "task", task_id: activeTaskId }] : EMPTY_PAGE_ENTRIES;
}

function useSidebarPageTasks(
  workspaceId: string | null,
  page: ReturnType<typeof useSidebarTaskPage>,
  activeTaskOnly: boolean,
) {
  const activeTaskId = useAppStore((state) => state.tasks.activeTaskId);
  const pageEntries = useMemo(
    () => sidebarSourceEntries(page.response, activeTaskId, activeTaskOnly),
    [activeTaskOnly, activeTaskId, page.response],
  );
  const byId = useAppStore((state) => state.taskOverview.byId);
  const pageTaskIds = useMemo(
    () => pageEntries.flatMap((entry) => (entry.task_id ? [entry.task_id] : [])),
    [pageEntries],
  );
  const statusSummaryByTaskId = useAppStore(
    useShallow((state) => {
      const workspaceSummaries = state.sidebarStatusSummaryByWorkspaceId?.[workspaceId ?? ""] ?? {};
      return Object.fromEntries(
        pageTaskIds.flatMap((taskId) =>
          workspaceSummaries[taskId] ? [[taskId, workspaceSummaries[taskId]]] : [],
        ),
      );
    }),
  );
  const nextPageTasks = useMemo(
    () =>
      projectSidebarTasks(
        pageEntries,
        byId,
        statusSummaryByTaskId,
        page.view.filters,
        activeTaskOnly,
      ),
    [pageEntries, statusSummaryByTaskId, byId, page.view.filters, activeTaskOnly],
  );
  const previousTasksRef = useRef<SidebarTask[]>([]);
  const allTasks = useMemo(() => {
    const tasks = reuseUnchangedTasks(previousTasksRef.current, nextPageTasks);
    previousTasksRef.current = tasks;
    return tasks;
  }, [nextPageTasks]);

  return { pageEntries, allTasks };
}

/**
 * Complete inventories reuse the board's task state; other views use a
 * bounded server page. This hook never fetches workflow snapshots for the sidebar.
 * Command hosts and hidden navigation read the active record without retaining a page.
 */
export function useWorkspaceSidebarTasks(
  workspaceId: string | null,
  activeTaskOnly = false,
): WorkspaceSidebarTasksResult {
  const pageWorkspaceId = activeTaskOnly ? null : workspaceId;
  const storeTasks = useSidebarStoreTasks(pageWorkspaceId);
  const page = useSidebarTaskPage(
    pageWorkspaceId,
    !activeTaskOnly && storeTasks === null,
    storeTasks,
  );
  const taskRemoval = useAppStore((state) => state.taskRemoval);
  const { filteredWorkflows, stepsByWorkflowId, allSteps } =
    useWorkspaceWorkflowMetadata(workspaceId);
  const workspaceContextGeneration = useAppStore((state) => state.workspaceContextGeneration ?? 0);
  const workspaceContextRead = useAppStore((state) => state.workspaceContextRead);
  const retryWorkspaceContext = useAppStore(
    (state) => state.requestWorkspaceContextRefresh ?? NOOP_REFRESH,
  );

  const { pageEntries, allTasks } = useSidebarPageTasks(workspaceId, page, activeTaskOnly);

  const pendingArchiveTaskIds = useMemo(() => {
    const pending = new Set<string>();
    const activeTaskIds = new Set(
      allTasks.filter((task) => task.isArchived !== true).map((task) => task.id),
    );
    for (const [taskId, token] of Object.entries(taskRemoval.pendingTokenByTaskId)) {
      const operation = taskRemoval.operationsByToken[token];
      if (
        activeTaskIds.has(taskId) &&
        operation?.action === "archive" &&
        operation.workspaceId === workspaceId
      ) {
        pending.add(taskId);
      }
    }
    return pending;
  }, [allTasks, taskRemoval, workspaceId]);

  const wipQueueByTaskId = useMemo(
    () => buildWipQueueByTaskId(pageEntries, allSteps, stepsByWorkflowId, allTasks),
    [pageEntries, allSteps, stepsByWorkflowId, allTasks],
  );
  const workspaceWorkflows = useMemo<TaskMoveWorkflow[]>(
    () =>
      filteredWorkflows.map((workflow) => ({
        id: workflow.id,
        name: workflow.name,
        hidden: workflow.hidden,
      })),
    [filteredWorkflows],
  );

  const workspaceContextStatus = getWorkspaceContextStatus(
    workspaceContextRead,
    workspaceContextGeneration,
    workspaceId,
  );
  const emptyMetadata: AggregatedSidebarTasks = {
    allTasks,
    allSteps,
    stepsByWorkflowId,
  };

  return {
    ...emptyMetadata,
    allTasks,
    pendingArchiveTaskIds,
    allSteps,
    stepsByWorkflowId,
    wipQueueByTaskId,
    workflows: workspaceWorkflows,
    isLoading: page.isLoading,
    archivedError: page.error,
    retryArchivedTasks: page.refresh,
    page,
    pageEntries,
    workspaceContextError: workspaceContextStatus.error,
    workspaceContextPending: workspaceContextStatus.pending,
    workspaceContextAccessDenied: workspaceContextStatus.accessDenied,
    retryWorkspaceContext: workspaceContextStatus.canRetry ? retryWorkspaceContext : undefined,
  };
}
