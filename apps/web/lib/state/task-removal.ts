export type TaskRemovalAction = "archive" | "delete";

export type TaskRemovalOutcome = "pending" | "succeeded" | "failed" | "unknown";

export type TaskRemovalDeparture = {
  taskId: string;
  sessionId: string | null;
  navigationRevision: number;
};

export type TaskRemovalOperation = {
  token: string;
  action: TaskRemovalAction;
  workspaceId: string | null;
  taskIds: string[];
  requestIds: string[];
  outcomesByTaskId: Record<string, TaskRemovalOutcome>;
  departure: TaskRemovalDeparture | null;
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
