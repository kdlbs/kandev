import { useMemo, useRef } from "react";
import { useAppStore } from "@/components/state-provider";
import { useSidebarTaskPage } from "@/hooks/domains/kanban/use-sidebar-task-page";
import { toKanbanTask } from "@/lib/kanban/map-task";
import type { AggregatedSidebarTasks } from "@/components/task/task-session-sidebar-aggregate";
import type { TaskMoveWorkflow } from "@/components/task/task-move-context-menu";
import type { WorkspaceContextReadError } from "@/lib/state/slices/kanban/types";
import type { AppState } from "@/lib/state/store";
import type { Task } from "@/lib/types/http";
import type { WipQueueStatus } from "@/lib/kanban/wip-queue";
import type { TaskStatusSummary } from "@/lib/types/task-status-summary";
import { pickFreshestStatusSummary } from "@/lib/task-status-summary";
import { useShallow } from "zustand/react/shallow";

export type WorkspaceSidebarTasksResult = AggregatedSidebarTasks & {
  pendingRemovalTaskIds: ReadonlySet<string>;
  workflows: TaskMoveWorkflow[];
  wipQueueByTaskId: Map<string, WipQueueStatus>;
  isLoading: boolean;
  archivedError: string | null;
  retryArchivedTasks: () => void;
  page: ReturnType<typeof useSidebarTaskPage>;
  pageEntries: NonNullable<ReturnType<typeof useSidebarTaskPage>["response"]>["entries"];
  workspaceContextError: WorkspaceContextReadError | null;
  workspaceContextPending: boolean;
  workspaceContextAccessDenied: boolean;
  retryWorkspaceContext: () => void;
};

const NOOP_REFRESH = () => {};

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
  entries: NonNullable<ReturnType<typeof useSidebarTaskPage>["response"]>["entries"],
  allSteps: AggregatedSidebarTasks["allSteps"],
  stepsByWorkflowId: AggregatedSidebarTasks["stepsByWorkflowId"],
): Map<string, WipQueueStatus> {
  const result = new Map<string, WipQueueStatus>();
  for (const entry of entries) {
    if (
      entry.kind !== "task" ||
      !entry.task ||
      !entry.wip_queue_position ||
      !entry.wip_queue_total
    ) {
      continue;
    }
    const task = toKanbanTask(entry.task);
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
  };
}

function workspacePageTasks(
  entries: NonNullable<ReturnType<typeof useSidebarTaskPage>["response"]>["entries"],
  statusSummaryByTaskId: Record<string, TaskStatusSummary>,
) {
  const tasks: SidebarTask[] = [];
  for (const entry of entries) {
    if (entry.kind !== "task" || !entry.task) continue;
    const task = toKanbanTask(entry.task as Task);
    tasks.push({
      ...task,
      statusSummary: pickFreshestStatusSummary(task.statusSummary, statusSummaryByTaskId[task.id]),
      _workflowId: entry.task.workflow_id ?? "",
    });
  }
  return tasks;
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

/**
 * Sidebar rows come from the server's bounded, globally ordered query. Workflow
 * metadata can reuse snapshots already loaded by a board, but this hook never
 * fetches complete task snapshots for the sidebar.
 */
export function useWorkspaceSidebarTasks(workspaceId: string | null): WorkspaceSidebarTasksResult {
  const page = useSidebarTaskPage(workspaceId);
  const taskRemoval = useAppStore((state) => state.taskRemoval);
  const { filteredWorkflows, stepsByWorkflowId, allSteps } =
    useWorkspaceWorkflowMetadata(workspaceId);
  const workspaceContextGeneration = useAppStore((state) => state.workspaceContextGeneration ?? 0);
  const workspaceContextRead = useAppStore((state) => state.workspaceContextRead);
  const retryWorkspaceContext = useAppStore(
    (state) => state.requestWorkspaceContextRefresh ?? NOOP_REFRESH,
  );

  const pageEntries = page.response?.entries ?? [];
  const pageTaskIds = useMemo(
    () =>
      pageEntries.flatMap((entry) => (entry.kind === "task" && entry.task ? [entry.task.id] : [])),
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
    () => workspacePageTasks(pageEntries, statusSummaryByTaskId),
    [pageEntries, statusSummaryByTaskId],
  );
  const previousTasksRef = useRef<SidebarTask[]>([]);
  const allTasks = useMemo(() => {
    const tasks = reuseUnchangedTasks(previousTasksRef.current, nextPageTasks);
    previousTasksRef.current = tasks;
    return tasks;
  }, [nextPageTasks]);

  const pendingRemovalTaskIds = useMemo(() => {
    const pending = new Set<string>();
    for (const task of allTasks) {
      const token = taskRemoval.pendingTokenByTaskId[task.id];
      if (!token) continue;
      const operation = taskRemoval.operationsByToken[token];
      if (
        operation?.workspaceId === workspaceId &&
        (operation?.action === "delete" ||
          (operation?.action === "archive" && task.isArchived !== true))
      ) {
        pending.add(task.id);
      }
    }
    return pending;
  }, [allTasks, taskRemoval, workspaceId]);

  const wipQueueByTaskId = useMemo(
    () => buildWipQueueByTaskId(pageEntries, allSteps, stepsByWorkflowId),
    [pageEntries, allSteps, stepsByWorkflowId],
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
    pendingRemovalTaskIds,
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
    retryWorkspaceContext,
  };
}
