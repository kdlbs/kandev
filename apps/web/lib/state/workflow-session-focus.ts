export type WorkflowSessionFocusStart = {
  taskId: string;
  workflowId: string;
  destinationStepId: string;
  presentationToken: number;
  navigationRevision: number;
};

export type WorkflowSessionFocusIntent = WorkflowSessionFocusStart & {
  requestId: number;
  entryIdentity: string | null;
};

export type WorkflowSessionFocusRequest = {
  requestId: number;
  presentationToken: number;
  taskId: string;
  sessionId: string;
};

export type WorkflowSessionFocusState = {
  nextRequestId: number;
  intent: WorkflowSessionFocusIntent | null;
  request: WorkflowSessionFocusRequest | null;
};

export type WorkflowSessionFocusCancelScope = {
  requestId?: number;
  presentationToken?: number;
  taskId?: string;
};

export type WorkflowSessionFocusRoute = {
  operationId: string;
  destinationStepId: string;
  entryIdentity: string;
  destinationSessionId: string;
  phase: "committed";
};

/**
 * The task projection returned by the move request. It is a candidate only;
 * the live task projection remains authoritative when it is newer.
 */
export type WorkflowSessionFocusTaskProjection = {
  metadata: unknown;
  updatedAt?: string | null;
  entryIdentity?: string | null;
};

export type WorkflowSessionFocusReconcileInput = {
  activeTaskId: string | null;
  navigationRevision: number;
  routeMetadata: unknown;
  routeUpdatedAt?: string | null;
  responseProjection?: WorkflowSessionFocusTaskProjection;
  knownSessionIds: readonly string[];
};

export type WorkflowSessionFocusReconcileResult = {
  state: WorkflowSessionFocusState;
  sessionId: string | null;
};

export function createWorkflowSessionFocusState(): WorkflowSessionFocusState {
  return { nextRequestId: 0, intent: null, request: null };
}

function clearWorkflowSessionFocusState(
  state: WorkflowSessionFocusState,
): WorkflowSessionFocusState {
  return { ...state, intent: null, request: null };
}

function nonEmptyString(value: unknown): string | null {
  if (typeof value !== "string") return null;
  const normalized = value.trim();
  return normalized === "" ? null : normalized;
}

function asRecord(value: unknown): Record<string, unknown> | null {
  return value && typeof value === "object" && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : null;
}

function timestampMillis(value: string | null | undefined): number | null {
  if (!value) return null;
  const timestamp = Date.parse(value);
  return Number.isFinite(timestamp) ? timestamp : null;
}

function acceptedRouteMetadata(
  intent: WorkflowSessionFocusIntent,
  input: WorkflowSessionFocusReconcileInput,
): unknown {
  const liveRoute = readCommittedWorkflowSessionRoute(input.routeMetadata);
  if (liveRoute?.entryIdentity === intent.entryIdentity) return input.routeMetadata;

  const response = input.responseProjection;
  if (!response) return input.routeMetadata;

  const responseRoute = readCommittedWorkflowSessionRoute(response.metadata);
  const responseEntryIdentity = nonEmptyString(response.entryIdentity);
  const liveUpdatedAt = timestampMillis(input.routeUpdatedAt);
  const responseUpdatedAt = timestampMillis(response.updatedAt);
  const responseIsEntryCorrelated =
    responseEntryIdentity === intent.entryIdentity &&
    responseRoute?.entryIdentity === intent.entryIdentity;
  const responseIsFreshEnough =
    responseIsEntryCorrelated &&
    (liveUpdatedAt === null || (responseUpdatedAt !== null && responseUpdatedAt >= liveUpdatedAt));
  if (!responseIsFreshEnough) return input.routeMetadata;

  // When timestamps tie, retain a different live committed entry. Equal
  // timestamps are common for grouped task writes, and the live entry is
  // the projection that won the store's ordering guard.
  const liveHasDifferentCommittedEntry =
    liveRoute !== null && liveRoute.entryIdentity !== intent.entryIdentity;
  return liveHasDifferentCommittedEntry && responseUpdatedAt === liveUpdatedAt
    ? input.routeMetadata
    : response.metadata;
}

export function readCommittedWorkflowSessionRoute(
  routeMetadata: unknown,
): WorkflowSessionFocusRoute | null {
  const metadata = asRecord(routeMetadata);
  const route = asRecord(metadata?.workflow_session_route ?? routeMetadata);
  if (!route) return null;

  const operationId = nonEmptyString(route.operation_id);
  const destinationStepId = nonEmptyString(route.destination_step_id);
  const entryIdentity = nonEmptyString(route.entry_identity);
  const destinationSessionId = nonEmptyString(route.destination_session_id);
  if (
    !operationId ||
    !destinationStepId ||
    !entryIdentity ||
    !destinationSessionId ||
    route.phase !== "committed"
  ) {
    return null;
  }

  return {
    operationId,
    destinationStepId,
    entryIdentity,
    destinationSessionId,
    phase: "committed",
  };
}

export function beginWorkflowSessionFocus(
  state: WorkflowSessionFocusState,
  input: WorkflowSessionFocusStart,
): { state: WorkflowSessionFocusState; requestId: number | null } {
  if (!input.taskId || !input.workflowId || !input.destinationStepId) {
    return { state, requestId: null };
  }

  const requestId = state.nextRequestId + 1;
  return {
    requestId,
    state: {
      nextRequestId: requestId,
      intent: { ...input, requestId, entryIdentity: null },
      request: null,
    },
  };
}

export function bindWorkflowSessionFocus(
  state: WorkflowSessionFocusState,
  input: {
    requestId: number;
    presentationToken: number;
    entryIdentity: string;
  },
): WorkflowSessionFocusState {
  const intent = state.intent;
  if (
    !intent ||
    intent.requestId !== input.requestId ||
    intent.presentationToken !== input.presentationToken
  ) {
    return state;
  }

  const entryIdentity = nonEmptyString(input.entryIdentity);
  if (!entryIdentity) return clearWorkflowSessionFocusState(state);

  return { ...state, intent: { ...intent, entryIdentity } };
}

function scopeMatches(
  value: WorkflowSessionFocusIntent | WorkflowSessionFocusRequest,
  scope: WorkflowSessionFocusCancelScope,
): boolean {
  if (scope.requestId !== undefined && value.requestId !== scope.requestId) return false;
  if (
    scope.presentationToken !== undefined &&
    value.presentationToken !== scope.presentationToken
  ) {
    return false;
  }
  if (scope.taskId !== undefined && value.taskId !== scope.taskId) return false;
  return true;
}

export function cancelWorkflowSessionFocus(
  state: WorkflowSessionFocusState,
  scope: WorkflowSessionFocusCancelScope = {},
): WorkflowSessionFocusState {
  const activeValues = [state.intent, state.request].filter(
    (value): value is WorkflowSessionFocusIntent | WorkflowSessionFocusRequest => value !== null,
  );
  if (activeValues.length > 0 && !activeValues.some((value) => scopeMatches(value, scope))) {
    return state;
  }
  return clearWorkflowSessionFocusState(state);
}

export function reconcileWorkflowSessionFocus(
  state: WorkflowSessionFocusState,
  input: WorkflowSessionFocusReconcileInput,
): WorkflowSessionFocusReconcileResult {
  const intent = state.intent;
  if (!intent || !intent.entryIdentity) return { state, sessionId: null };

  if (
    input.activeTaskId !== intent.taskId ||
    input.navigationRevision !== intent.navigationRevision
  ) {
    return { state: clearWorkflowSessionFocusState(state), sessionId: null };
  }

  const routeMetadata = acceptedRouteMetadata(intent, input);
  const route = readCommittedWorkflowSessionRoute(routeMetadata);
  if (
    !route ||
    route.destinationStepId !== intent.destinationStepId ||
    route.entryIdentity !== intent.entryIdentity ||
    !input.knownSessionIds.includes(route.destinationSessionId)
  ) {
    return { state, sessionId: null };
  }

  return {
    state: {
      ...state,
      intent: null,
      request: {
        requestId: intent.requestId,
        presentationToken: intent.presentationToken,
        taskId: intent.taskId,
        sessionId: route.destinationSessionId,
      },
    },
    sessionId: route.destinationSessionId,
  };
}

export function acknowledgeWorkflowSessionFocus(
  state: WorkflowSessionFocusState,
  requestId: number,
): WorkflowSessionFocusState {
  if (state.request?.requestId !== requestId) return state;
  return { ...state, request: null };
}
