import type { RecentTaskEntry } from "@/lib/recent-tasks";
import type { KanbanState, WorkflowSnapshotData } from "@/lib/state/slices/kanban/types";
import type { Repository, TaskSession, TaskSessionState, TaskState } from "@/lib/types/http";
import { getRepositoryDisplayName } from "@/lib/utils";
import { getSessionInfoForTask } from "@/lib/utils/session-info";

type KanbanTask = KanbanState["tasks"][number];

export type RecentTaskBuildContext = {
  activeTaskId: string | null;
  activeWorkspaceId?: string | null;
  kanbanWorkflowId: string | null;
  kanbanTasks: KanbanTask[];
  kanbanSteps: KanbanState["steps"];
  snapshots: Record<string, WorkflowSnapshotData>;
  workflows: Array<{ id: string; workspaceId: string; name: string }>;
  repositoriesByWorkspace: Record<string, Repository[]>;
  sessionsByTaskId: Record<string, TaskSession[]>;
  gitStatusByEnvId: Record<
    string,
    {
      files?: Record<string, { additions?: number; deletions?: number }>;
      branch_additions?: number;
      branch_deletions?: number;
    }
  >;
  environmentIdBySessionId: Record<string, string>;
};

export type RecentTaskBadge = {
  label: string;
  variant: "default" | "secondary" | "outline" | "destructive";
};

export type RecentTaskDisplayItem = {
  taskId: string;
  title: string;
  visitedAt: string;
  isCurrent: boolean;
  taskState?: TaskState;
  sessionState?: TaskSessionState;
  repositoryPath?: string;
  workflowId?: string;
  workflowName?: string;
  workflowStepTitle?: string;
  statusBadge: RecentTaskBadge;
};

type LiveTask = {
  task: KanbanTask;
  workflowId?: string;
  workflowName?: string;
};

type RecentTaskDisplayMaps = {
  repositoryById: Map<string, Repository>;
  stepTitleById: Map<string, string>;
};

type DisplayResolution = {
  live: LiveTask | null;
  task: KanbanTask | undefined;
  taskState: TaskState | undefined;
  sessionState: TaskSessionState | undefined;
  workflowId: string | undefined;
};

const UNTITLED_TASK = "Untitled task";

function getWorkflowName(workflowId: string | undefined, ctx: RecentTaskBuildContext) {
  if (!workflowId) return undefined;
  return ctx.workflows.find((workflow) => workflow.id === workflowId)?.name;
}

function findLiveTask(taskId: string, ctx: RecentTaskBuildContext): LiveTask | null {
  const kanbanTask = ctx.kanbanTasks.find((task) => task.id === taskId);
  if (kanbanTask) {
    return {
      task: kanbanTask,
      workflowId: ctx.kanbanWorkflowId ?? undefined,
      workflowName: getWorkflowName(ctx.kanbanWorkflowId ?? undefined, ctx),
    };
  }

  for (const snapshot of Object.values(ctx.snapshots)) {
    const task = snapshot.tasks.find((item) => item.id === taskId);
    if (task) {
      return { task, workflowId: snapshot.workflowId, workflowName: snapshot.workflowName };
    }
  }
  return null;
}

function buildStepTitleMap(ctx: RecentTaskBuildContext): Map<string, string> {
  const map = new Map<string, string>();
  for (const step of ctx.kanbanSteps) map.set(step.id, step.title);
  for (const snapshot of Object.values(ctx.snapshots)) {
    for (const step of snapshot.steps) map.set(step.id, step.title);
  }
  return map;
}

function buildRepositoryMap(ctx: RecentTaskBuildContext): Map<string, Repository> {
  const map = new Map<string, Repository>();
  for (const repositories of Object.values(ctx.repositoriesByWorkspace)) {
    for (const repository of repositories) map.set(repository.id, repository);
  }
  return map;
}

function formatRepository(repository: Repository | undefined): string | undefined {
  if (!repository) return undefined;
  if (repository.provider_owner && repository.provider_name) {
    return `${repository.provider_owner}/${repository.provider_name}`;
  }
  return (
    repository.name || getRepositoryDisplayName(repository.local_path) || repository.local_path
  );
}

function getResolvedSessionState(
  taskId: string,
  liveTask: KanbanTask | undefined,
  entry: RecentTaskEntry,
  ctx: RecentTaskBuildContext,
): TaskSessionState | undefined {
  const sessionInfo = getSessionInfoForTask(
    taskId,
    ctx.sessionsByTaskId,
    ctx.gitStatusByEnvId,
    ctx.environmentIdBySessionId,
  );
  return (
    sessionInfo.sessionState ??
    (liveTask?.primarySessionState as TaskSessionState | undefined) ??
    entry.sessionState ??
    undefined
  );
}

const TASK_STATUS_BADGES: Partial<Record<TaskState, RecentTaskBadge>> = {
  REVIEW: { label: "Review", variant: "secondary" },
  COMPLETED: { label: "Done", variant: "secondary" },
  IN_PROGRESS: { label: "In Progress", variant: "default" },
  SCHEDULING: { label: "In Progress", variant: "default" },
  BLOCKED: { label: "Blocked", variant: "destructive" },
  TODO: { label: "Todo", variant: "outline" },
  CREATED: { label: "New", variant: "outline" },
};

const SESSION_STATUS_BADGES: Partial<Record<TaskSessionState, RecentTaskBadge>> = {
  RUNNING: { label: "Running", variant: "default" },
  STARTING: { label: "Starting", variant: "default" },
  WAITING_FOR_INPUT: { label: "Turn Finished", variant: "secondary" },
  COMPLETED: { label: "Done", variant: "secondary" },
};

export function getTaskStatusBadge(
  taskState?: TaskState | null,
  sessionState?: TaskSessionState | null,
): RecentTaskBadge {
  if (sessionState === "FAILED" || taskState === "FAILED") {
    return { label: "Failed", variant: "destructive" };
  }
  if (sessionState === "CANCELLED" || taskState === "CANCELLED") {
    return { label: "Cancelled", variant: "outline" };
  }
  const sessionBadge = sessionState ? SESSION_STATUS_BADGES[sessionState] : undefined;
  if (sessionBadge) return sessionBadge;
  const taskBadge = taskState ? TASK_STATUS_BADGES[taskState] : undefined;
  if (taskBadge) return taskBadge;
  return { label: "Backlog", variant: "outline" };
}

function resolveDisplay(entry: RecentTaskEntry, ctx: RecentTaskBuildContext): DisplayResolution {
  const live = findLiveTask(entry.taskId, ctx);
  const task = live?.task;
  const taskState = task?.state ?? entry.taskState ?? undefined;
  return {
    live,
    task,
    taskState,
    sessionState: getResolvedSessionState(entry.taskId, task, entry, ctx),
    workflowId: live?.workflowId ?? entry.workflowId ?? undefined,
  };
}

function resolveRepositoryPath(
  entry: RecentTaskEntry,
  task: KanbanTask | undefined,
  maps: RecentTaskDisplayMaps,
): string | undefined {
  return (
    formatRepository(maps.repositoryById.get(task?.repositoryId ?? "")) ??
    entry.repositoryPath ??
    undefined
  );
}

function resolveWorkflowName(
  resolution: DisplayResolution,
  entry: RecentTaskEntry,
  ctx: RecentTaskBuildContext,
): string | undefined {
  return (
    resolution.live?.workflowName ??
    getWorkflowName(resolution.workflowId, ctx) ??
    entry.workflowName ??
    undefined
  );
}

function resolveWorkflowStepTitle(
  entry: RecentTaskEntry,
  task: KanbanTask | undefined,
  maps: RecentTaskDisplayMaps,
): string | undefined {
  return maps.stepTitleById.get(task?.workflowStepId ?? "") ?? entry.workflowStepTitle ?? undefined;
}

function buildDisplayItem(
  entry: RecentTaskEntry,
  ctx: RecentTaskBuildContext,
  maps: RecentTaskDisplayMaps,
): RecentTaskDisplayItem {
  const resolution = resolveDisplay(entry, ctx);
  return {
    taskId: entry.taskId,
    title: resolution.task?.title ?? entry.title,
    visitedAt: entry.visitedAt,
    isCurrent: entry.taskId === ctx.activeTaskId,
    taskState: resolution.taskState,
    sessionState: resolution.sessionState,
    repositoryPath: resolveRepositoryPath(entry, resolution.task, maps),
    workflowId: resolution.workflowId,
    workflowName: resolveWorkflowName(resolution, entry, ctx),
    workflowStepTitle: resolveWorkflowStepTitle(entry, resolution.task, maps),
    statusBadge: getTaskStatusBadge(resolution.taskState, resolution.sessionState),
  };
}

export function buildRecentTaskDisplayItems(
  entries: RecentTaskEntry[],
  ctx: RecentTaskBuildContext,
): RecentTaskDisplayItem[] {
  const maps = {
    stepTitleById: buildStepTitleMap(ctx),
    repositoryById: buildRepositoryMap(ctx),
  };
  return entries.map((entry) => buildDisplayItem(entry, ctx, maps));
}

function fallbackRecentEntry(
  taskId: string,
  previous: RecentTaskEntry | undefined,
  visitedAt: string | undefined,
): RecentTaskEntry {
  return previous ?? { taskId, title: UNTITLED_TASK, visitedAt: visitedAt ?? nowIso() };
}

function nowIso(): string {
  return new Date().toISOString();
}

function pickNullable<T>(
  displayValue: T | undefined,
  previousValue: T | null | undefined,
): T | null {
  return displayValue ?? previousValue ?? null;
}

function resolveWorkspaceId(
  taskId: string,
  ctx: RecentTaskBuildContext,
  previous: RecentTaskEntry | undefined,
  repositoryById: Map<string, Repository>,
): string | null {
  return (
    previous?.workspaceId ??
    ctx.activeWorkspaceId ??
    findWorkspaceIdForTask(taskId, ctx, repositoryById) ??
    null
  );
}

function getDisplayItemForEntry(
  taskId: string,
  ctx: RecentTaskBuildContext,
  previous: RecentTaskEntry | undefined,
  visitedAt: string | undefined,
  maps?: RecentTaskDisplayMaps,
): RecentTaskDisplayItem | undefined {
  const entry = fallbackRecentEntry(taskId, previous, visitedAt);
  if (maps) return buildDisplayItem(entry, ctx, maps);
  return buildRecentTaskDisplayItems([entry], ctx)[0];
}

function buildEntryCore(
  taskId: string,
  displayItem: RecentTaskDisplayItem | undefined,
  previous: RecentTaskEntry | undefined,
  visitedAt: string | undefined,
) {
  return {
    taskId,
    title: displayItem?.title ?? previous?.title ?? UNTITLED_TASK,
    visitedAt: visitedAt ?? previous?.visitedAt ?? nowIso(),
  };
}

function buildEntryMetadata(
  displayItem: RecentTaskDisplayItem | undefined,
  previous: RecentTaskEntry | undefined,
) {
  return {
    taskState: pickNullable(displayItem?.taskState, previous?.taskState),
    sessionState: pickNullable(displayItem?.sessionState, previous?.sessionState),
    repositoryPath: pickNullable(displayItem?.repositoryPath, previous?.repositoryPath),
    workflowId: pickNullable(displayItem?.workflowId, previous?.workflowId),
    workflowName: pickNullable(displayItem?.workflowName, previous?.workflowName),
    workflowStepTitle: pickNullable(displayItem?.workflowStepTitle, previous?.workflowStepTitle),
  };
}

export function buildRecentTaskEntry(
  taskId: string,
  ctx: RecentTaskBuildContext,
  previous?: RecentTaskEntry,
  visitedAt?: string,
): RecentTaskEntry {
  const maps = {
    stepTitleById: buildStepTitleMap(ctx),
    repositoryById: buildRepositoryMap(ctx),
  };
  const displayItem = getDisplayItemForEntry(taskId, ctx, previous, visitedAt, maps);

  return {
    ...buildEntryCore(taskId, displayItem, previous, visitedAt),
    ...buildEntryMetadata(displayItem, previous),
    workspaceId: resolveWorkspaceId(taskId, ctx, previous, maps.repositoryById),
  };
}

function findWorkspaceIdForTask(
  taskId: string,
  ctx: RecentTaskBuildContext,
  repositoryById: Map<string, Repository>,
): string | undefined {
  const live = findLiveTask(taskId, ctx);
  const repositoryId = live?.task.repositoryId;
  if (!repositoryId) return undefined;
  const repository = repositoryById.get(repositoryId);
  return repository?.workspace_id;
}

export function getInitialSelectionIndex(
  items: Array<{ taskId: string }>,
  currentTaskId: string | null,
): number {
  if (items.length === 0) return -1;
  const firstNonCurrent = items.findIndex((item) => item.taskId !== currentTaskId);
  return firstNonCurrent === -1 ? 0 : firstNonCurrent;
}

export function getNextSelectionIndex(currentIndex: number, itemCount: number): number {
  if (itemCount === 0) return -1;
  if (currentIndex < 0) return 0;
  return (currentIndex + 1) % itemCount;
}

export function getPreviousSelectionIndex(currentIndex: number, itemCount: number): number {
  if (itemCount === 0) return -1;
  if (currentIndex <= 0) return itemCount - 1;
  return currentIndex - 1;
}
