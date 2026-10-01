import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import type { GitStatusEntry } from "@/lib/state/slices/session-runtime/types";
import type { GitStatusUpdateEvent } from "@/lib/types/git-events";
import { applyGitStatusUpdateWithOutcome } from "@/lib/ws/handlers/git-status";
import type {
  SessionGitRefreshMode,
  SessionGitRefreshResponse,
  WebSocketClient,
} from "@/lib/ws/client";

type ClientScope = { id: number; generation: number; status: string };
type ActiveAttempt = {
  controller: AbortController;
  requestId: string;
  promise: Promise<void>;
  onCancel?: () => void;
};
type SnapshotOutcome = {
  received: number;
  count: number;
  complete: number;
  incompleteRepositories: Map<string, GitStatusUpdateEvent>;
  pendingDetails: Map<string, GitStatusUpdateEvent>;
};
type GitRefreshContext = {
  client: WebSocketClient;
  store: StoreApi<AppState>;
  sessionId: string;
  environmentId: string;
  scopeKey: string;
  generation: number;
  attemptKey: string;
  requestId: string;
};
type GitRefreshScopeContext = Omit<GitRefreshContext, "attemptKey" | "requestId">;
type RefreshSnapshot = {
  response: SessionGitRefreshResponse | null;
  outcome: SnapshotOutcome | null;
};

const clientScopes = new WeakMap<WebSocketClient, ClientScope>();
const attempts = new Map<string, ActiveAttempt>();
const owners = new Map<string, number>();
const replayTimers = new Map<string, ReturnType<typeof setTimeout>>();
let nextClientScopeId = 0;
let nextRefreshRequestId = 0;

function createRefreshRequestId(): string {
  return `git-refresh-${++nextRefreshRequestId}`;
}

function currentClientScope(client: WebSocketClient): ClientScope {
  const existing = clientScopes.get(client);
  if (existing) return existing;
  const scope = { id: ++nextClientScopeId, generation: 0, status: client.getStatus() };
  clientScopes.set(client, scope);
  client.onConnectionStatus((status) => {
    if (status === scope.status) return;
    scope.generation += 1;
    scope.status = status;
    if (status !== "connected") clearReplayTimersForClient(scope.id);
  });
  return scope;
}

function clientScopeKey(client: WebSocketClient, environmentId: string): string {
  return `${currentClientScope(client).id}\u0000${environmentId}`;
}

function activeAttemptKey(scopeKey: string, generation: number, mode: SessionGitRefreshMode) {
  return `${scopeKey}\u0000${generation}\u0000${mode}`;
}

function completeMembership(status: GitStatusEntry | undefined): boolean {
  return Boolean(status && status.files !== undefined && status.files_complete !== false);
}

function hasPendingDetails(store: StoreApi<AppState>, environmentId: string): boolean {
  const state = store.getState().gitStatus;
  const statuses = [
    ...Object.values(state.byEnvironmentRepo[environmentId] ?? {}),
    state.byEnvironmentId[environmentId],
  ];
  return statuses.some((status) => completeMembership(status) && status.detail_state === "pending");
}

function ownsScope(scopeKey: string): boolean {
  return (owners.get(scopeKey) ?? 0) > 0;
}

function clearReplayTimer(scopeKey: string) {
  const timer = replayTimers.get(scopeKey);
  if (timer) clearTimeout(timer);
  replayTimers.delete(scopeKey);
}

function clearReplayTimersForClient(clientId: number) {
  for (const scopeKey of replayTimers.keys()) {
    if (scopeKey.startsWith(`${clientId}\u0000`)) clearReplayTimer(scopeKey);
  }
}

export function retainGitRefreshScope(client: WebSocketClient, environmentId: string): () => void {
  const scopeKey = clientScopeKey(client, environmentId);
  owners.set(scopeKey, (owners.get(scopeKey) ?? 0) + 1);
  return () => {
    const nextCount = (owners.get(scopeKey) ?? 1) - 1;
    if (nextCount > 0) {
      owners.set(scopeKey, nextCount);
      return;
    }
    owners.delete(scopeKey);
    clearReplayTimer(scopeKey);
    for (const [key, attempt] of attempts) {
      if (!key.startsWith(`${scopeKey}\u0000`)) continue;
      if (attempts.get(key) === attempt) attempts.delete(key);
      attempt.controller.abort();
      attempt.onCancel?.();
    }
  };
}

function isCurrentRequest(
  context: GitRefreshContext,
  response?: SessionGitRefreshResponse,
): boolean {
  const { client, store, scopeKey, generation, sessionId, environmentId, attemptKey, requestId } =
    context;
  const scope = currentClientScope(client);
  const attempt = attempts.get(attemptKey);
  const currentEnvironment = store.getState().environmentIdBySessionId[sessionId] ?? sessionId;
  return (
    attempt?.requestId === requestId &&
    !attempt.controller.signal.aborted &&
    ownsScope(scopeKey) &&
    scope.generation === generation &&
    client.getStatus() === "connected" &&
    currentEnvironment === environmentId &&
    (!response ||
      (response.session_id === sessionId &&
        (!response.task_environment_id || response.task_environment_id === environmentId)))
  );
}

function applyRefreshSnapshots(
  store: StoreApi<AppState>,
  sessionId: string,
  environmentId: string,
  response: SessionGitRefreshResponse,
): SnapshotOutcome {
  const outcome: SnapshotOutcome = {
    received: 0,
    count: 0,
    complete: 0,
    incompleteRepositories: new Map(),
    pendingDetails: new Map(),
  };
  for (const snapshot of response.snapshots ?? []) {
    if (snapshot.action !== "session.git.event" || snapshot.payload.type !== "status_update")
      continue;
    const event = snapshot.payload as GitStatusUpdateEvent;
    if (event.session_id !== sessionId || event.task_environment_id !== environmentId) {
      continue;
    }
    outcome.received += 1;
    const repositoryName = event.status.repository_name ?? "";
    const applied = applyGitStatusUpdateWithOutcome(store, event);
    if (!applied.accepted) continue;
    outcome.count += 1;
    const complete =
      event.status.status_state !== "unavailable" &&
      event.status.status_state !== "loading" &&
      (event.status.files_complete ?? event.status.files !== undefined);
    if (complete) {
      outcome.complete += 1;
      if (event.status.detail_state === "pending")
        outcome.pendingDetails.set(repositoryName, event);
    } else {
      outcome.incompleteRepositories.set(repositoryName, event);
    }
  }
  return outcome;
}

function setIncompleteRepositoriesUnavailable(
  store: StoreApi<AppState>,
  snapshots: Iterable<GitStatusUpdateEvent>,
) {
  for (const snapshot of snapshots) {
    applyGitStatusUpdateWithOutcome(store, {
      ...snapshot,
      status: {
        ...snapshot.status,
        status_state: "unavailable",
        files_complete: false,
        detail_state: "unavailable",
        error_code: snapshot.status.error_code ?? "status_unavailable",
      },
    });
  }
}

async function requestSnapshot(
  client: WebSocketClient,
  sessionId: string,
  mode: SessionGitRefreshMode,
  controller: AbortController,
): Promise<SessionGitRefreshResponse | null> {
  const request = client.refreshSessionData(sessionId, mode, controller.signal);
  if (!request) return null;
  return request;
}

function setPendingDetailsUnavailable(
  store: StoreApi<AppState>,
  environmentId: string,
  sessionId: string,
) {
  const state = store.getState();
  const statuses = Object.entries(state.gitStatus.byEnvironmentRepo[environmentId] ?? {});
  const legacy = state.gitStatus.byEnvironmentId[environmentId];
  if (
    legacy &&
    !statuses.some(([repositoryName]) => repositoryName === (legacy.repository_name ?? ""))
  ) {
    statuses.push([legacy.repository_name ?? "", legacy]);
  }
  for (const [repositoryName, status] of statuses) {
    if (status.detail_state === "pending") {
      applyGitStatusUpdateWithOutcome(store, {
        type: "status_update",
        session_id: sessionId,
        task_environment_id: environmentId,
        timestamp: status.timestamp ?? "",
        status: {
          ...status,
          branch: status.branch ?? "",
          remote_ahead: status.remote_ahead ?? 0,
          remote_behind: status.remote_behind ?? 0,
          repository_name: repositoryName,
          status_state: "unavailable",
          files_complete: false,
          detail_state: "unavailable",
          error_code: "details_unavailable",
        },
      });
    }
  }
}

async function runReplay(context: GitRefreshScopeContext) {
  const { client, store, sessionId, environmentId, scopeKey, generation } = context;
  if (!ownsScope(scopeKey) || hasPendingDetails(store, environmentId) === false) return;
  const attemptKey = activeAttemptKey(scopeKey, generation, "replay");
  const existing = attempts.get(attemptKey);
  if (existing && !existing.controller.signal.aborted) return existing.promise;
  const controller = new AbortController();
  const requestId = createRefreshRequestId();
  const attemptContext = { ...context, attemptKey, requestId };
  const attempt: ActiveAttempt = {
    controller,
    requestId,
    promise: Promise.resolve(),
    onCancel: () => setPendingDetailsUnavailable(store, environmentId, sessionId),
  };
  attempt.promise = (async () => {
    try {
      const response = await requestSnapshot(client, sessionId, "replay", controller);
      if (!response || !isCurrentRequest(attemptContext, response)) {
        return;
      }
      const outcome = applyRefreshSnapshots(store, sessionId, environmentId, response);
      if (hasPendingDetails(store, environmentId)) {
        if (outcome.pendingDetails.size > 0) {
          setIncompleteRepositoriesUnavailable(store, outcome.pendingDetails.values());
        } else {
          setPendingDetailsUnavailable(store, environmentId, sessionId);
        }
      }
    } catch {
      if (!controller.signal.aborted && isCurrentRequest(attemptContext)) {
        setPendingDetailsUnavailable(store, environmentId, sessionId);
      }
    }
  })().finally(() => {
    if (attempts.get(attemptKey) === attempt) attempts.delete(attemptKey);
  });
  attempts.set(attemptKey, attempt);
  return attempt.promise;
}

function scheduleDetailsReplay(context: GitRefreshScopeContext) {
  const { store, environmentId, scopeKey } = context;
  if (!ownsScope(scopeKey) || !hasPendingDetails(store, environmentId)) return;
  clearReplayTimer(scopeKey);
  replayTimers.set(
    scopeKey,
    setTimeout(() => {
      replayTimers.delete(scopeKey);
      void runReplay(context);
    }, 60_000),
  );
}

async function readRefreshSnapshot(
  context: GitRefreshContext,
  mode: SessionGitRefreshMode,
  controller: AbortController,
): Promise<RefreshSnapshot | null> {
  const { client, store, sessionId, environmentId } = context;
  let response: SessionGitRefreshResponse | null = null;
  try {
    response = await requestSnapshot(client, sessionId, mode, controller);
  } catch {
    if (controller.signal.aborted) return null;
  }
  if (!isCurrentRequest(context, response ?? undefined)) return null;
  return {
    response,
    outcome: response ? applyRefreshSnapshots(store, sessionId, environmentId, response) : null,
  };
}

function needsFreshRecovery(snapshot: RefreshSnapshot): boolean {
  const { response, outcome } = snapshot;
  return (
    !response ||
    !outcome ||
    outcome.complete === 0 ||
    outcome.incompleteRepositories.size > 0 ||
    response.success === false
  );
}

function setRefreshForAttempt(
  store: StoreApi<AppState>,
  environmentId: string,
  requestId: string,
  refresh: { state: "pending" | "unavailable"; error_code?: string } | null,
) {
  const current = store.getState().gitStatus.refreshByEnvironmentId?.[environmentId];
  if (current?.request_id !== requestId) {
    return;
  }
  store
    .getState()
    .setGitStatusRefresh(
      environmentId,
      undefined,
      refresh && { ...current, ...refresh, request_id: requestId },
    );
}

function markRefreshPendingForAttempt(
  store: StoreApi<AppState>,
  environmentId: string,
  requestId: string,
) {
  const current = store.getState().gitStatus.refreshByEnvironmentId?.[environmentId];
  store.getState().setGitStatusRefresh(environmentId, undefined, {
    ...current,
    state: "pending",
    error_code: undefined,
    request_id: requestId,
  });
}

function restorePreviousRefreshForAttempt(
  store: StoreApi<AppState>,
  environmentId: string,
  requestId: string,
  previousRefresh: NonNullable<AppState["gitStatus"]["refreshByEnvironmentId"]>[string] | undefined,
) {
  if (
    store.getState().gitStatus.refreshByEnvironmentId?.[environmentId]?.request_id !== requestId
  ) {
    return;
  }
  store.getState().setGitStatusRefresh(environmentId, undefined, previousRefresh ?? null);
}

function onlyRejectedSnapshots(outcome: SnapshotOutcome | null): boolean {
  return outcome !== null && outcome.received > 0 && outcome.count === 0;
}

function acceptedSnapshotOutcome(outcome: SnapshotOutcome | null): SnapshotOutcome | null {
  return outcome && outcome.count > 0 ? outcome : null;
}

function refreshScopeIsActive(controller: AbortController, scopeKey: string): boolean {
  return !controller.signal.aborted && ownsScope(scopeKey);
}

async function performForegroundRefresh(context: GitRefreshContext, controller: AbortController) {
  const { store, environmentId, scopeKey, requestId } = context;
  const previousRefresh = store.getState().gitStatus.refreshByEnvironmentId?.[environmentId];
  markRefreshPendingForAttempt(store, environmentId, requestId);
  const freshSnapshot = await readRefreshSnapshot(context, "fresh", controller);
  if (!freshSnapshot) return;
  const finalSnapshot = needsFreshRecovery(freshSnapshot)
    ? await readRefreshSnapshot(context, "recover", controller)
    : freshSnapshot;
  if (!finalSnapshot) return;

  if (!refreshScopeIsActive(controller, scopeKey)) return;
  const receivedOutcome = finalSnapshot.outcome;
  if (onlyRejectedSnapshots(receivedOutcome)) {
    restorePreviousRefreshForAttempt(store, environmentId, requestId, previousRefresh);
    return;
  }
  const outcome = acceptedSnapshotOutcome(receivedOutcome);
  if (!outcome) {
    setRefreshForAttempt(store, environmentId, requestId, {
      state: "unavailable",
      error_code: "status_unavailable",
    });
    return;
  }

  setRefreshForAttempt(store, environmentId, requestId, null);
  setIncompleteRepositoriesUnavailable(store, outcome.incompleteRepositories.values());
  if (hasPendingDetails(store, environmentId)) {
    scheduleDetailsReplay(context);
  }
}

export function requestGitStatusRefresh(
  client: WebSocketClient,
  store: StoreApi<AppState>,
  sessionId: string,
  environmentId: string,
): Promise<void> {
  const scope = currentClientScope(client);
  const scopeKey = clientScopeKey(client, environmentId);
  const attemptKey = activeAttemptKey(scopeKey, scope.generation, "fresh");
  const existing = attempts.get(attemptKey);
  if (existing && !existing.controller.signal.aborted) return existing.promise;
  if (existing) attempts.delete(attemptKey);

  const controller = new AbortController();
  const requestId = createRefreshRequestId();
  const attempt: ActiveAttempt = {
    controller,
    requestId,
    promise: Promise.resolve(),
    onCancel: () =>
      setRefreshForAttempt(store, environmentId, requestId, {
        state: "unavailable",
        error_code: "refresh_cancelled",
      }),
  };
  const context = {
    client,
    store,
    sessionId,
    environmentId,
    scopeKey,
    generation: scope.generation,
    attemptKey,
    requestId,
  };
  attempt.promise = performForegroundRefresh(context, controller).finally(() => {
    if (attempts.get(attemptKey) === attempt) attempts.delete(attemptKey);
  });
  attempts.set(attemptKey, attempt);
  return attempt.promise;
}

export function monitorGitStatusDetails(
  client: WebSocketClient,
  store: StoreApi<AppState>,
  environmentId: string,
): () => void {
  const scopeKey = clientScopeKey(client, environmentId);
  return store.subscribe(() => {
    const state = store.getState().gitStatus;
    const statuses = [
      ...Object.values(state.byEnvironmentRepo[environmentId] ?? {}),
      state.byEnvironmentId[environmentId],
    ].filter((status): status is GitStatusEntry => Boolean(status));
    if (statuses.length > 0 && statuses.every((status) => status.detail_state !== "pending")) {
      clearReplayTimer(scopeKey);
    }
  });
}

export function scheduleReplayIfDetailsPending(
  client: WebSocketClient,
  store: StoreApi<AppState>,
  sessionId: string,
  environmentId: string,
) {
  const scope = currentClientScope(client);
  const scopeKey = clientScopeKey(client, environmentId);
  scheduleDetailsReplay({
    client,
    store,
    sessionId,
    environmentId,
    scopeKey,
    generation: scope.generation,
  });
}
