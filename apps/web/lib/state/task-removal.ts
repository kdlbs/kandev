export type TaskRemovalAction = "archive" | "delete";

export type TaskRemovalOutcome = "pending" | "succeeded" | "failed" | "unknown";

export type TaskRemovalDepartureOrigin = "detail" | "preview";

export type TaskRemovalDeparture = {
  taskId: string;
  sessionId: string | null;
  navigationRevision: number;
  origin: TaskRemovalDepartureOrigin;
};

export type TaskRemovalOperation = {
  token: string;
  action: TaskRemovalAction;
  workspaceId: string | null;
  taskIds: string[];
  requestIds: string[];
  outcomesByTaskId: Record<string, TaskRemovalOutcome>;
  departure: TaskRemovalDeparture | null;
  /** True when the accepted request also covers the task's cached descendants. */
  cascade?: boolean;
};

export type TaskRemovalState = {
  navigationRevision: number;
  operationsByToken: Record<string, TaskRemovalOperation>;
  pendingTokenByTaskId: Record<string, string>;
};

export type BeginTaskRemovalInput = {
  token: string;
  action: TaskRemovalAction;
  workspaceId: string | null;
  taskIds: string[];
  requestIds: string[];
  departure: TaskRemovalDeparture | null;
  cascade?: boolean;
};

export function createTaskRemovalState(): TaskRemovalState {
  return {
    navigationRevision: 0,
    operationsByToken: {},
    pendingTokenByTaskId: {},
  };
}

function uniqueIds(ids: string[]): string[] {
  return [...new Set(ids)];
}

export function beginTaskRemoval(
  state: TaskRemovalState,
  input: BeginTaskRemovalInput,
): TaskRemovalState | null {
  const taskIds = uniqueIds(input.taskIds);
  const requestIds = uniqueIds(input.requestIds);
  if (taskIds.length === 0 || requestIds.length === 0) return null;
  if (taskIds.some((taskId) => state.pendingTokenByTaskId[taskId])) return null;

  const outcomesByTaskId = Object.fromEntries(
    requestIds.map((taskId) => [taskId, "pending" as const]),
  );
  const operation: TaskRemovalOperation = {
    token: input.token,
    action: input.action,
    workspaceId: input.workspaceId,
    taskIds,
    requestIds,
    outcomesByTaskId,
    departure: input.departure,
    cascade: input.cascade === true,
  };

  const pendingTokenByTaskId = { ...state.pendingTokenByTaskId };
  for (const taskId of taskIds) pendingTokenByTaskId[taskId] = input.token;

  return {
    ...state,
    operationsByToken: {
      ...state.operationsByToken,
      [input.token]: operation,
    },
    pendingTokenByTaskId,
  };
}

export function recordTaskRemovalResult(
  state: TaskRemovalState,
  token: string,
  taskIds: string[],
  outcome: Exclude<TaskRemovalOutcome, "pending">,
): TaskRemovalState {
  const operation = state.operationsByToken[token];
  if (!operation) return state;
  const outcomesByTaskId = { ...operation.outcomesByTaskId };
  for (const taskId of taskIds) {
    if (operation.taskIds.includes(taskId)) outcomesByTaskId[taskId] = outcome;
  }
  return {
    ...state,
    operationsByToken: {
      ...state.operationsByToken,
      [token]: { ...operation, outcomesByTaskId },
    },
  };
}

export function advanceTaskNavigationRevision(state: TaskRemovalState): TaskRemovalState {
  return { ...state, navigationRevision: state.navigationRevision + 1 };
}

export function ownsTaskRemovalDeparture(
  state: TaskRemovalState,
  token: string,
  navigationRevision: number,
): boolean {
  const operation = state.operationsByToken[token];
  return (
    state.navigationRevision === navigationRevision &&
    operation?.departure?.navigationRevision === navigationRevision
  );
}

/**
 * Resolves the accepted action for one task from the token map and its matching
 * operation. A stale token or a captured workspace mismatch is not membership.
 */
export function pendingTaskRemovalAction(
  state: TaskRemovalState | undefined,
  taskId: string,
  workspaceId: string | null,
): TaskRemovalAction | null {
  const token = state?.pendingTokenByTaskId[taskId];
  if (!state || !token) return null;
  const operation = state.operationsByToken[token];
  if (!operation) return null;
  if (operation.workspaceId !== workspaceId) return null;
  return operation.action;
}

export function isTaskRemovalPending(
  state: TaskRemovalState | undefined,
  taskId: string,
  workspaceId: string | null,
  actions?: readonly TaskRemovalAction[],
): boolean {
  const action = pendingTaskRemovalAction(state, taskId, workspaceId);
  if (action === null) return false;
  return actions === undefined || actions.includes(action);
}

/** Every task under an accepted removal in one captured workspace. */
export function pendingTaskRemovalTaskIds(
  state: TaskRemovalState | undefined,
  workspaceId: string | null,
): Set<string> {
  const pending = new Set<string>();
  if (!state) return pending;
  for (const taskId of Object.keys(state.pendingTokenByTaskId)) {
    if (pendingTaskRemovalAction(state, taskId, workspaceId) !== null) pending.add(taskId);
  }
  return pending;
}

/**
 * Task ids captured by accepted cascade removals. Their descendants may not
 * have their own token when they were absent from cached boards, so ancestry
 * checks use this coverage to keep a server-page child out of the destination.
 */
export function cascadeTaskRemovalTaskIds(
  state: TaskRemovalState | undefined,
  workspaceId: string | null,
): Set<string> {
  const covered = new Set<string>();
  if (!state) return covered;
  for (const operation of Object.values(state.operationsByToken)) {
    if (operation.workspaceId !== workspaceId || operation.cascade !== true) continue;
    for (const taskId of operation.taskIds) covered.add(taskId);
  }
  return covered;
}

export function taskRemovalCoversTask(state: TaskRemovalState, taskId: string): boolean {
  return Object.values(state.operationsByToken).some((operation) =>
    operation.taskIds.includes(taskId),
  );
}

export function taskRemovalOwnsDepartureForTask(
  state: TaskRemovalState | undefined,
  taskId: string,
): boolean {
  if (!state) return false;
  return Object.values(state.operationsByToken).some(
    (operation) =>
      operation.taskIds.includes(taskId) &&
      operation.departure?.navigationRevision === state.navigationRevision,
  );
}

export function releaseTaskRemoval(state: TaskRemovalState, token: string): TaskRemovalState {
  const operation = state.operationsByToken[token];
  if (!operation) return state;

  const operationsByToken = { ...state.operationsByToken };
  delete operationsByToken[token];
  const pendingTokenByTaskId = { ...state.pendingTokenByTaskId };
  for (const taskId of operation.taskIds) {
    if (pendingTokenByTaskId[taskId] === token) delete pendingTokenByTaskId[taskId];
  }

  return { ...state, operationsByToken, pendingTokenByTaskId };
}
