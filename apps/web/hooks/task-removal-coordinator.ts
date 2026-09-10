import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import { fetchTask } from "@/lib/api";
import { linkToTask, linkToTaskOverview } from "@/lib/links";
import { softNavigate } from "@/lib/routing/client-router";
import { ownsTaskRemovalDeparture, type TaskRemovalAction } from "@/lib/state/task-removal";
import type {
  RemoveFromBoardOptions,
  RemoveFromBoardResult,
  TaskRemovalBatchResult,
  TaskRemovalRequest,
  TaskRemovalRunOptions,
  TaskRemovalSuccessNotifier,
} from "./use-task-removal";

type TaskRemovalStore = StoreApi<AppState>;

type RemovalIdsByRequest = Map<string, ReadonlySet<string>>;

type TaskRemovalCoordinatorDeps = {
  store: TaskRemovalStore;
  removeTaskFromBoard: (
    taskId: string,
    options?: RemoveFromBoardOptions,
  ) => Promise<RemoveFromBoardResult>;
  getRemovalIds: (requestIds: string[], cascade: boolean) => RemovalIdsByRequest;
  notifySuccess?: TaskRemovalSuccessNotifier;
};

type SettledRequests = {
  succeededRequests: TaskRemovalRequest[];
  failedRequests: TaskRemovalRequest[];
  errorsByTaskId: Record<string, unknown>;
};

function taskWorkspaceId(store: TaskRemovalStore, taskId: string): string | null {
  const state = store.getState();
  const task =
    state.kanban.tasks.find((item) => item.id === taskId) ??
    Object.values(state.kanbanMulti.snapshots)
      .flatMap((snapshot) => snapshot.tasks)
      .find((item) => item.id === taskId);
  return task?.workspaceId ?? state.workspaces.activeId ?? null;
}

function resolveWorkspaceId(
  store: TaskRemovalStore,
  activeTaskId: string | null,
  requestedWorkspaceId?: string | null,
): string | null {
  if (requestedWorkspaceId !== undefined) return requestedWorkspaceId;
  const state = store.getState();
  return activeTaskId ? taskWorkspaceId(store, activeTaskId) : state.workspaces.activeId;
}

function collectRemovalIds(
  deps: TaskRemovalCoordinatorDeps,
  requests: TaskRemovalRequest[],
  opts: TaskRemovalRunOptions | undefined,
): { requestIds: string[]; removalIdsByRequest: RemovalIdsByRequest; removalTaskIds: Set<string> } {
  const requestIds = requests.map((request) => request.taskId);
  const removalIdsByRequest = deps.getRemovalIds(requestIds, opts?.cascade === true);
  const removalTaskIds = new Set<string>();
  for (const taskIds of removalIdsByRequest.values()) {
    for (const taskId of taskIds) removalTaskIds.add(taskId);
  }
  return { requestIds, removalIdsByRequest, removalTaskIds };
}

function beginRemovalOperation(
  deps: TaskRemovalCoordinatorDeps,
  action: TaskRemovalAction,
  requestIds: string[],
  removalTaskIds: Set<string>,
  opts: TaskRemovalRunOptions | undefined,
): {
  token: string;
  activeTaskId: string;
  activeSessionId: string | null;
  workspaceId: string | null;
  navigationRevision: number;
  departure: { taskId: string; sessionId: string | null; navigationRevision: number };
} | null {
  const { store } = deps;
  const state = store.getState();
  const activeTaskId = state.tasks.activeTaskId;
  if (!activeTaskId || !removalTaskIds.has(activeTaskId)) return null;

  const activeSessionId = state.tasks.activeSessionId;
  const workspaceId = resolveWorkspaceId(store, activeTaskId, opts?.workspaceId);
  const navigationRevision = state.taskRemoval.navigationRevision;
  const departure = { taskId: activeTaskId, sessionId: activeSessionId, navigationRevision };
  const token = state.beginTaskRemoval({
    action,
    workspaceId,
    taskIds: [...removalTaskIds],
    requestIds,
    departure,
  });
  if (!token) return null;
  return {
    token,
    activeTaskId,
    activeSessionId,
    workspaceId,
    navigationRevision,
    departure,
  };
}

function beginUnselectedRemovalOperation(
  deps: TaskRemovalCoordinatorDeps,
  action: TaskRemovalAction,
  requestIds: string[],
  removalTaskIds: Set<string>,
  opts: TaskRemovalRunOptions | undefined,
): {
  token: string;
  activeTaskId: null;
  activeSessionId: null;
  workspaceId: string | null;
  navigationRevision: number;
  departure: null;
} | null {
  const { store } = deps;
  const state = store.getState();
  const activeTaskId = state.tasks.activeTaskId;
  if (activeTaskId && removalTaskIds.has(activeTaskId)) return null;
  const workspaceId = resolveWorkspaceId(store, null, opts?.workspaceId);
  const token = state.beginTaskRemoval({
    action,
    workspaceId,
    taskIds: [...removalTaskIds],
    requestIds,
    departure: null,
  });
  if (!token) return null;
  return {
    token,
    activeTaskId: null,
    activeSessionId: null,
    workspaceId,
    navigationRevision: state.taskRemoval.navigationRevision,
    departure: null,
  };
}

function beginOperation(
  deps: TaskRemovalCoordinatorDeps,
  action: TaskRemovalAction,
  requestIds: string[],
  removalTaskIds: Set<string>,
  opts: TaskRemovalRunOptions | undefined,
) {
  return (
    beginRemovalOperation(deps, action, requestIds, removalTaskIds, opts) ??
    beginUnselectedRemovalOperation(deps, action, requestIds, removalTaskIds, opts)
  );
}

function settleRequests(
  store: TaskRemovalStore,
  token: string,
  requests: TaskRemovalRequest[],
  results: PromiseSettledResult<void>[],
): SettledRequests {
  const succeededRequests = requests.filter((_, index) => results[index]?.status === "fulfilled");
  const failedRequests = requests.filter((_, index) => results[index]?.status === "rejected");
  const errorsByTaskId: Record<string, unknown> = {};
  for (const [index, request] of requests.entries()) {
    const result = results[index];
    if (result?.status === "rejected") errorsByTaskId[request.taskId] = result.reason;
  }
  store.getState().recordTaskRemovalResult(
    token,
    succeededRequests.map((request) => request.taskId),
    "succeeded",
  );
  store.getState().recordTaskRemovalResult(
    token,
    failedRequests.map((request) => request.taskId),
    "failed",
  );
  return { succeededRequests, failedRequests, errorsByTaskId };
}

function collectSucceededRemovalIds(
  succeededRequests: TaskRemovalRequest[],
  removalIdsByRequest: RemovalIdsByRequest,
): Set<string> {
  const succeededRemovalIds = new Set<string>();
  for (const request of succeededRequests) {
    for (const taskId of removalIdsByRequest.get(request.taskId) ?? [request.taskId]) {
      succeededRemovalIds.add(taskId);
    }
  }
  return succeededRemovalIds;
}

function departureRequestFor(
  departureTaskId: string | null,
  requests: TaskRemovalRequest[],
  removalIdsByRequest: RemovalIdsByRequest,
): TaskRemovalRequest | undefined {
  if (!departureTaskId) return undefined;
  return requests.find((request) => removalIdsByRequest.get(request.taskId)?.has(departureTaskId));
}

function ownsDeparture(
  store: TaskRemovalStore,
  token: string,
  navigationRevision: number,
): boolean {
  return ownsTaskRemovalDeparture(store.getState().taskRemoval, token, navigationRevision);
}

function isDefinitiveRemovalFailure(error: unknown): boolean {
  if (!error || typeof error !== "object") return false;
  const status = (error as { status?: unknown }).status;
  return typeof status === "number" && status >= 400 && status < 500 && status !== 408;
}

function sessionBelongsToTask(store: TaskRemovalStore, taskId: string, sessionId: string): boolean {
  const state = store.getState();
  const session = state.taskSessions.items[sessionId];
  if (session?.task_id && session.task_id !== taskId) return false;
  const knownSessions = state.taskSessionsByTask.itemsByTaskId[taskId] ?? [];
  return (
    knownSessions.length === 0 || knownSessions.some((candidate) => candidate.id === sessionId)
  );
}

function setActiveTaskAutomatically(store: TaskRemovalStore, taskId: string): void {
  const state = store.getState() as AppState & { setActiveTaskAuto?: (id: string) => void };
  if (state.setActiveTaskAuto) state.setActiveTaskAuto(taskId);
  else state.setActiveTask(taskId);
}

function setActiveSessionAutomatically(
  store: TaskRemovalStore,
  taskId: string,
  sessionId: string,
): void {
  const state = store.getState() as AppState & {
    setActiveSessionAuto?: (task: string, session: string) => void;
  };
  if (state.setActiveSessionAuto) state.setActiveSessionAuto(taskId, sessionId);
  else state.setActiveSession(taskId, sessionId);
}

async function canRestoreDeparture(
  action: TaskRemovalAction,
  departureTaskId: string,
  error: unknown,
): Promise<boolean> {
  if (isDefinitiveRemovalFailure(error)) return true;
  try {
    const currentTask = await fetchTask(departureTaskId, { cache: "no-store" });
    return action === "delete" || !currentTask.archived_at;
  } catch {
    return false;
  }
}

async function recoverFailedDeparture(params: {
  action: TaskRemovalAction;
  store: TaskRemovalStore;
  departure: { taskId: string; sessionId: string | null; navigationRevision: number } | null;
  departureRequest: TaskRemovalRequest | undefined;
  errorsByTaskId: Record<string, unknown>;
  token: string;
  workspaceId: string | null;
}): Promise<void> {
  const { action, store, departure, departureRequest, errorsByTaskId, token, workspaceId } = params;
  if (
    !departure ||
    !departureRequest ||
    !ownsDeparture(store, token, departure.navigationRevision)
  ) {
    return;
  }
  const canRestore = await canRestoreDeparture(
    action,
    departure.taskId,
    errorsByTaskId[departureRequest.taskId],
  );
  if (!ownsDeparture(store, token, departure.navigationRevision)) return;
  if (!canRestore) {
    softNavigate(linkToTaskOverview({ workspaceId: workspaceId ?? undefined }), "replace");
    return;
  }
  if (departure.sessionId && sessionBelongsToTask(store, departure.taskId, departure.sessionId)) {
    setActiveSessionAutomatically(store, departure.taskId, departure.sessionId);
  } else {
    setActiveTaskAutomatically(store, departure.taskId);
  }
  softNavigate(linkToTask(departure.taskId), "replace");
}

async function reconcileSuccessfulRemoval(params: {
  deps: TaskRemovalCoordinatorDeps;
  departure: { taskId: string; sessionId: string | null; navigationRevision: number } | null;
  activeSucceeded: boolean;
  succeededRemovalIds: Set<string>;
  token: string;
  navigationRevision: number;
  removalTaskIds: ReadonlySet<string>;
  activeTaskId: string | null;
  activeSessionId: string | null;
  workspaceId: string | null;
  validateTaskAncestry: boolean;
  switchedTaskId: string | null;
  errorsByTaskId: Record<string, unknown>;
}): Promise<string | null> {
  const {
    deps,
    departure,
    activeSucceeded,
    succeededRemovalIds,
    token,
    navigationRevision,
    removalTaskIds,
    activeTaskId,
    activeSessionId,
    workspaceId,
    validateTaskAncestry,
    switchedTaskId,
    errorsByTaskId,
  } = params;
  if (succeededRemovalIds.size === 0) return switchedTaskId;
  if (!departure || !activeSucceeded) {
    removeTasksFromSnapshots(deps.store, succeededRemovalIds);
    return switchedTaskId;
  }
  try {
    const finalDestination = await deps.removeTaskFromBoard(departure.taskId, {
      wasActiveTaskId: activeTaskId,
      wasActiveSessionId: activeSessionId,
      excludedTaskIds: removalTaskIds,
      removedTaskIds: succeededRemovalIds,
      removalToken: token,
      removalNavigationRevision: navigationRevision,
      workspaceId,
      validateTaskAncestry,
    });
    return finalDestination.switchedTaskId ?? switchedTaskId;
  } catch (error) {
    errorsByTaskId[departure.taskId] = error;
    if (ownsDeparture(deps.store, token, navigationRevision)) {
      softNavigate(linkToTaskOverview({ workspaceId: workspaceId ?? undefined }), "replace");
    }
    return switchedTaskId;
  }
}

function skippedResult(): TaskRemovalBatchResult {
  return {
    skipped: true,
    operationToken: null,
    switchedTaskId: null,
    succeededTaskIds: [],
    failedTaskIds: [],
    errorsByTaskId: {},
  };
}

function notifySuccessfulRemoval(
  deps: TaskRemovalCoordinatorDeps,
  action: TaskRemovalAction,
  settled: SettledRequests,
): void {
  if (settled.failedRequests.length > 0 || settled.succeededRequests.length === 0) return;
  deps.notifySuccess?.(action, settled.succeededRequests.length);
}

export async function coordinateTaskRemovalBatch(
  deps: TaskRemovalCoordinatorDeps,
  action: TaskRemovalAction,
  requests: TaskRemovalRequest[],
  opts?: TaskRemovalRunOptions,
): Promise<TaskRemovalBatchResult> {
  if (requests.length === 0) {
    return { ...skippedResult(), skipped: false };
  }
  const { requestIds, removalIdsByRequest, removalTaskIds } = collectRemovalIds(
    deps,
    requests,
    opts,
  );
  const operation = beginOperation(deps, action, requestIds, removalTaskIds, opts);
  if (!operation) return skippedResult();

  const destinationPromise = operation.departure
    ? deps.removeTaskFromBoard(operation.departure.taskId, {
        wasActiveTaskId: operation.activeTaskId,
        wasActiveSessionId: operation.activeSessionId,
        switchOnly: true,
        excludedTaskIds: removalTaskIds,
        removalToken: operation.token,
        removalNavigationRevision: operation.navigationRevision,
        workspaceId: operation.workspaceId,
        validateTaskAncestry: opts?.cascade === true,
      })
    : Promise.resolve({ switchedTaskId: null, excludedTaskIds: undefined });
  const mutationPromise = Promise.allSettled(requests.map((request) => request.mutate()));

  try {
    const [destinationOutcome, mutationResults] = await Promise.all([
      destinationPromise.then(
        (value) => ({ ok: true as const, value }),
        (error: unknown) => ({ ok: false as const, error }),
      ),
      mutationPromise,
    ]);
    const settled = settleRequests(deps.store, operation.token, requests, mutationResults);
    const succeededRemovalIds = collectSucceededRemovalIds(
      settled.succeededRequests,
      removalIdsByRequest,
    );
    const activeSucceeded = operation.departure
      ? succeededRemovalIds.has(operation.departure.taskId)
      : false;
    let switchedTaskId = destinationOutcome.ok
      ? (destinationOutcome.value.switchedTaskId ?? null)
      : null;
    switchedTaskId = await reconcileSuccessfulRemoval({
      deps,
      departure: operation.departure,
      activeSucceeded,
      succeededRemovalIds,
      token: operation.token,
      navigationRevision: operation.navigationRevision,
      removalTaskIds,
      activeTaskId: operation.activeTaskId,
      activeSessionId: operation.activeSessionId,
      workspaceId: operation.workspaceId,
      validateTaskAncestry: opts?.cascade === true,
      switchedTaskId,
      errorsByTaskId: settled.errorsByTaskId,
    });
    const departureRequest = departureRequestFor(
      operation.departure?.taskId ?? null,
      requests,
      removalIdsByRequest,
    );
    if (operation.departure && !activeSucceeded && departureRequest) {
      await recoverFailedDeparture({
        action,
        store: deps.store,
        departure: operation.departure,
        departureRequest,
        errorsByTaskId: settled.errorsByTaskId,
        token: operation.token,
        workspaceId: operation.workspaceId,
      });
    }
    notifySuccessfulRemoval(deps, action, settled);
    return {
      skipped: false,
      operationToken: operation.token,
      switchedTaskId,
      succeededTaskIds: settled.succeededRequests.map((request) => request.taskId),
      failedTaskIds: settled.failedRequests.map((request) => request.taskId),
      errorsByTaskId: settled.errorsByTaskId,
    };
  } finally {
    deps.store.getState().releaseTaskRemoval(operation.token);
  }
}

function removeTasksFromSnapshots(store: TaskRemovalStore, taskIds: ReadonlySet<string>): void {
  const state = store.getState();
  for (const [workflowId, snapshot] of Object.entries(state.kanbanMulti.snapshots)) {
    if (!snapshot.tasks.some((task) => taskIds.has(task.id))) continue;
    state.setWorkflowSnapshot(workflowId, {
      ...snapshot,
      tasks: snapshot.tasks.filter((task) => !taskIds.has(task.id)),
    });
  }
  if (!state.kanban.tasks.some((task) => taskIds.has(task.id))) return;
  store.setState((current) => ({
    ...current,
    kanban: {
      ...current.kanban,
      tasks: current.kanban.tasks.filter((task) => !taskIds.has(task.id)),
    },
  }));
}
