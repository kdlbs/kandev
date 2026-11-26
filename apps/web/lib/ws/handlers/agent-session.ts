/* eslint-disable max-lines -- session WebSocket handlers share one event registry. */
import type { StoreApi } from "zustand";
import { createDebugLogger } from "@/lib/debug/log";
import type { AppState } from "@/lib/state/store";
import {
  mergeQueuePolicyMeta,
  queuePolicyStatusMeta,
} from "@/lib/state/slices/session/queue-policy-meta";
import type { QueueMeta } from "@/lib/state/slices/session/types";
import type { WsHandlers } from "@/lib/ws/handlers/types";
import {
  sessionId as toSessionId,
  taskId as toTaskId,
  type SessionId,
  type TaskId,
  type TaskSession,
  type TaskSessionState,
} from "@/lib/types/http";
import type {
  QueueStatusChangedPayload,
  TaskSessionActivityChangedPayload,
  TaskSessionAgentctlPayload,
} from "@/lib/types/backend";
import { isConfirmedSessionRemoved } from "@/lib/state/session-removal";
import { syncKanbanPrimarySessionState } from "@/lib/ws/handlers/agent-session-kanban-sync";
import {
  mergeCompactionCountValue,
  mergeCompactionReportingValue,
  reconcileContextWindowMetadata,
  sameContextWindowEntry,
} from "@/lib/state/slices/session-runtime/context-window";
import { ROUTE_SESSION_FIELDS } from "@/lib/ws/handlers/agent-session-route-fields";
import { t } from "@/lib/i18n";
import { maybeMarkQuickChatUnseenIdle } from "@/lib/ws/handlers/quick-chat-unseen";
import { readLastAgentError } from "@/lib/session-last-agent-error";
import {
  isStaleSessionStateEvent,
  sessionStateConfirmsAgentctlExecutionReady,
} from "@/lib/session-state";
import { isSessionRestartPending } from "@/lib/session/session-restart-state";
import { getSessionWorkspacePath } from "@/lib/session-workspace-path";
import {
  sanitizeWorkspaceRestorationDetails,
  type WorkspaceRestorationAttempt,
} from "@/lib/state/slices/session-runtime/workspace-restoration";
import { applyForegroundActivity, applyCancellationPending } from "./session-activity";
import {
  clearPinnedSessionIfOverridden,
  isTerminalSessionState,
  shouldPreservePinnedSessionForTask,
} from "./session-selection";
import { followRemovedSession, followSessionTransition } from "./task-primary-focus";

const debug = createDebugLogger("session:state");

function isSiblingWorktree(
  existing: { worktree_id?: string } | undefined,
  payload: { worktree_id?: string } | undefined,
): boolean {
  const existingWorktreeId = existing?.worktree_id;
  const payloadWorktreeId = payload?.worktree_id;
  return !!existingWorktreeId && !!payloadWorktreeId && existingWorktreeId !== payloadWorktreeId;
}

function getAgentctlWorktreeFields(
  payload: { worktree_id?: string; worktree_path?: string; worktree_branch?: string },
  isSibling: boolean,
): Record<string, string | undefined> {
  if (isSibling) return {};
  return {
    worktree_id: payload.worktree_id,
    worktree_path: payload.worktree_path,
    worktree_branch: payload.worktree_branch,
  };
}

/** Promote agentctl status to "ready" when the session enters a live state.
 *  Acts as a fallback for missed/late agentctl_ready WS events — the backend
 *  cannot reach RUNNING/WAITING_FOR_INPUT without a live agentctl. Never
 *  downgrades an existing "ready" entry. */
function maybePromoteAgentctlReady(
  store: StoreApi<AppState>,
  sessionId: string,
  newState: TaskSessionState | undefined,
  timestamp: string | undefined,
): void {
  const state = store.getState();
  const current = state.sessionAgentctl?.itemsBySessionId?.[sessionId];
  const session = state.taskSessions?.items?.[sessionId];
  if (
    !sessionStateConfirmsAgentctlExecutionReady(
      newState,
      session?.agent_execution_id,
      current?.agentExecutionId,
    )
  ) {
    return;
  }
  if (current?.status === "ready" && !current.startingExecutionId) return;
  store.getState().setSessionAgentctlStatus(sessionId, {
    status: "ready",
    agentExecutionId: current?.agentExecutionId,
    updatedAt: timestamp,
  });
}

/**
 * When the backend creates a new session for the active task (e.g., due to a
 * workflow step transition with a different agent profile), the chat UI should
 * follow the switch. Returns true when the caller should adopt the new session
 * as the task's active session.
 *
 * Adopts only when the current active session is missing, cross-task, parked
 * IDLE, completed, or cancelled. Failed sessions retain their recovery view.
 */
export function shouldAdoptNewSession(
  state: AppState,
  taskId: string,
  newState: TaskSessionState | undefined,
): boolean {
  if (!newState || isTerminalSessionState(newState)) return false;
  if (state.tasks.activeTaskId !== taskId) return false;
  const activeSessionId = state.tasks.activeSessionId;
  if (activeSessionId) {
    const activeSession = state.taskSessions.items[activeSessionId];
    if (
      activeSession?.task_id === taskId &&
      (activeSession.state === "FAILED" ||
        (activeSession.state !== "IDLE" && !isTerminalSessionState(activeSession.state)))
    ) {
      return false;
    }
  }
  return true;
}

// Fields carried onto the update object verbatim whenever the payload defines
// them (undefined = key omitted so it never clobbers live client state).
// `name` is present here so a rename event's cleared label ("") still applies;
// foreground_activity/active_subagent_count carry the ADR-0049 activity
// substate; supports_steering carries the live steer-eligibility flip so the
// composer can switch affordance without a refetch; cancellation_* carry the
// backend-owned cancellation projection.
const CARRIED_WHEN_DEFINED = [
  "session_recovery_blocks",
  "review_status",
  "error_message",
  "is_passthrough",
  "name",
  "foreground_activity",
  "active_subagent_count",
  "supports_steering",
  "cancellation_pending",
  "cancellation_revision",
  "runtime_storage_health",
  "agent_workspace_path",
  "execution_profile_id",
  "route_generation",
  "route_state",
  "route_reason",
  ...ROUTE_SESSION_FIELDS,
  "downstream_acp_session_id",
] as const;

/** Copy each CARRIED_WHEN_DEFINED field onto `update` only when the payload defines it. */
function carryDefinedFields(
  update: Record<string, unknown>,
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  payload: any,
): void {
  for (const key of CARRIED_WHEN_DEFINED) {
    if (payload[key] !== undefined) update[key] = payload[key];
  }
}

/** Build a session update object from the state_changed payload. */
// eslint-disable-next-line @typescript-eslint/no-explicit-any
function buildSessionUpdate(payload: any): Record<string, unknown> {
  const update: Record<string, unknown> = {};
  if (payload.new_state) update.state = payload.new_state;
  if (payload.agent_profile_id) update.agent_profile_id = payload.agent_profile_id;
  if (payload.agent_profile_snapshot)
    update.agent_profile_snapshot = payload.agent_profile_snapshot;
  const metadataSources = [payload.session_metadata, payload.metadata].filter(
    (candidate) => candidate && typeof candidate === "object",
  ) as Array<Record<string, unknown>>;
  if (metadataSources.length > 0) update.metadata = Object.assign({}, ...metadataSources);
  if (payload.task_environment_id) update.task_environment_id = payload.task_environment_id;
  if (payload.updated_at) update.updated_at = payload.updated_at;
  carryDefinedFields(update, payload);
  return update;
}

/** Upsert the session in the per-task sessions list from a WS event.
 *  Uses `upsertTaskSessionFromEvent` so a new partial row invalidates any
 *  previously loaded list, allowing API hydration to fill fields like
 *  agent_profile_id / repository_id / worktree_path. */
function upsertTaskSessionList(
  store: StoreApi<AppState>,
  taskId: TaskId,
  sessionId: SessionId,
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  payload: any,
  sessionUpdate: Record<string, unknown>,
): void {
  const newState = payload.new_state as TaskSessionState | undefined;
  const existing = store.getState().taskSessions.items[sessionId];
  if (!existing && !newState) return;

  store.getState().upsertTaskSessionFromEvent(taskId, {
    id: sessionId,
    task_id: taskId,
    state: (newState ?? existing?.state) as TaskSessionState,
    cancellation_pending: existing?.cancellation_pending ?? false,
    cancellation_revision: existing?.cancellation_revision ?? 0,
    started_at: existing?.started_at ?? "",
    updated_at: (sessionUpdate.updated_at as string | undefined) ?? existing?.updated_at ?? "",
    ...(payload.agent_profile_id ? { agent_profile_id: payload.agent_profile_id } : {}),
    ...sessionUpdate,
  });
}

// Fan out the office refetch trigger only when the session's state
// actually changed. The WS layer fires `session.state_changed` for
// several adjacent reasons (agentctl status, context window, model
// updates) where `new_state` is undefined or unchanged; without this
// gate every one of those storms the dashboard-card re-render path.
function maybeFanOutOfficeRefetch(
  store: StoreApi<AppState>,
  newState: TaskSessionState | undefined,
  prevState: TaskSessionState | undefined,
): void {
  if (!newState || newState === prevState) return;
  const setOfficeTrigger = store.getState().setOfficeRefetchTrigger;
  if (!setOfficeTrigger) return;
  setOfficeTrigger("dashboard");
  setOfficeTrigger("agents");
}

/** Extract context-window data or an explicit cache invalidation from a session event. */
// eslint-disable-next-line @typescript-eslint/no-explicit-any
function extractContextWindow(store: StoreApi<AppState>, sessionId: string, payload: any): void {
  const metadataSources = [payload.metadata, payload.session_metadata];
  const metadata = metadataSources.find(
    (candidate) =>
      candidate &&
      typeof candidate === "object" &&
      (Object.prototype.hasOwnProperty.call(candidate, "context_window") ||
        Object.prototype.hasOwnProperty.call(candidate, "context_compaction_count") ||
        Object.prototype.hasOwnProperty.call(candidate, "context_compaction_reporting")),
  ) as Record<string, unknown> | undefined;
  if (!metadata) return;
  const existing = store.getState().contextWindow?.bySessionId?.[sessionId];
  const sessionMetadata = store.getState().taskSessions?.items?.[sessionId]?.metadata as
    | Record<string, unknown>
    | undefined;
  const count = mergeCompactionCountValue(
    metadata.context_compaction_count,
    sessionMetadata?.context_compaction_count,
  );
  const reporting = mergeCompactionReportingValue(
    metadata.context_compaction_reporting,
    sessionMetadata?.context_compaction_reporting,
  );
  const hasGauge = Object.prototype.hasOwnProperty.call(metadata, "context_window");
  const next = reconcileContextWindowMetadata({
    current: existing,
    gauge: metadata.context_window,
    hasGauge,
    count,
    reporting,
    timestamp: new Date().toISOString(),
    allowExplicitClear: true,
  });
  if (next === undefined) return;
  if (next === null) {
    store.getState().clearContextWindow(sessionId);
    return;
  }
  if (!sameContextWindowEntry(next, existing)) {
    store.getState().setContextWindow(sessionId, next);
  }
}

/** Retains the active restart target or a protected user pin during automatic adoption. */
function shouldPreserveSessionSelection(
  store: StoreApi<AppState>,
  taskId: string,
  incoming: { sessionId: string; newState: TaskSessionState | undefined },
) {
  const state = store.getState();
  return (
    (state.tasks.activeTaskId === taskId &&
      isSessionRestartPending(store, state.tasks.activeSessionId)) ||
    (state.tasks.pinnedSessionId !== incoming.sessionId &&
      shouldPreservePinnedSessionForTask(state, taskId, incoming))
  );
}

/**
 * After a `session.state_changed` event, decide whether the chat UI should
 * follow a workflow-driven session switch. Same-task lifecycle handoffs verify
 * predecessor state before changing focus; a failed session keeps its recovery view.
 */
// eslint-disable-next-line max-params -- newState/previousState/wasKnownToStore are all needed by downstream branches
function maybeAdoptSessionOnTransition(
  store: StoreApi<AppState>,
  taskId: string,
  sessionId: string,
  newState: TaskSessionState | undefined,
  wasKnownToStore: boolean,
  previousState: TaskSessionState | undefined,
): void {
  const state = store.getState();
  if (shouldPreserveSessionSelection(store, taskId, { sessionId, newState })) return;

  if (!wasKnownToStore && shouldAdoptNewSession(state, taskId, newState)) {
    if (followSessionTransition(store, taskId, sessionId, "new-session")) return;
    clearPinnedSessionIfOverridden(store, sessionId);
    state.setActiveSessionAuto(taskId, sessionId);
    return;
  }

  if (state.tasks.activeSessionId !== sessionId) return;
  handOffFromEndedSession(store, { taskId, sessionId, newState, previousState });
}

/** Follows a workflow switch away from the active session once it ends. */
function handOffFromEndedSession(
  store: StoreApi<AppState>,
  transition: {
    taskId: string;
    sessionId: string;
    newState: TaskSessionState | undefined;
    previousState: TaskSessionState | undefined;
  },
): void {
  const { taskId, sessionId, newState, previousState } = transition;
  if (!newState || !isTerminalSessionState(newState)) return;
  // A failed session has no successor to follow. Its failure output and
  // recovery actions belong on screen, where they are read.
  if (newState === "FAILED") return;
  const state = store.getState();
  // When the user clicked open a terminal session (e.g. to review a
  // completed run), setActiveSession pins it. If the backend then
  // re-emits the same terminal state_changed (a replay — the previous
  // stored state was already terminal), honor the pin and do NOT hand
  // off to a running session. A genuine RUNNING→COMPLETED transition
  // (previousState non-terminal) still hands off normally.
  if (
    state.tasks.pinnedSessionId === sessionId &&
    previousState &&
    isTerminalSessionState(previousState)
  )
    return;
  followSessionTransition(store, taskId, undefined, "terminal");
}

/** Seed the session→environment mapping from an agentctl event payload.
 *  Brand-new CREATED sessions never receive a `state_changed` carrying
 *  `task_environment_id` (no transition fires), so the env mapping would
 *  otherwise stay empty and env-routed shell terminals stall on
 *  "Connecting terminal...". Routes through `upsertTaskSessionFromEvent`
 *  which calls `syncEnvironmentMapping` and merges with any existing row. */
function sessionSeedFields(existing: TaskSession | undefined): {
  state: TaskSessionState;
  cancellation_pending: boolean;
  cancellation_revision: number;
  started_at: string;
  updated_at: string;
} {
  return {
    state: existing?.state ?? "CREATED",
    cancellation_pending: existing?.cancellation_pending ?? false,
    cancellation_revision: existing?.cancellation_revision ?? 0,
    started_at: existing?.started_at ?? "",
    updated_at: existing?.updated_at ?? "",
  };
}

function syncEnvFromAgentctlPayload(
  store: StoreApi<AppState>,
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  payload: any,
): void {
  const rawTaskId = payload?.task_id;
  const rawSessionId = payload?.session_id;
  const envId = payload?.task_environment_id;
  if (!rawTaskId || !rawSessionId || !envId) return;
  const taskId = toTaskId(rawTaskId);
  const sessionId = toSessionId(rawSessionId);
  const existing = store.getState().taskSessions.items[sessionId];
  const isSibling = isSiblingWorktree(existing, payload);
  store.getState().upsertTaskSessionFromEvent(taskId, {
    id: sessionId,
    task_id: taskId,
    ...sessionSeedFields(existing),
    task_environment_id: envId,
    ...getAgentctlWorktreeFields(payload, isSibling),
    workspace_path: payload.workspace_path ?? payload.task_workspace_path ?? payload.worktree_path,
    ...(payload.agent_workspace_path !== undefined
      ? { agent_workspace_path: payload.agent_workspace_path }
      : {}),
  });
  store.getState().reconcileWorkflowSessionFocus?.(taskId);
}

/** Builds the partial-session patch applied for an agentctl_ready event.
 *  On sibling materialize we update workspace_path to the task root and keep
 *  the primary's id/path/branch; the initial ready event sets id/path/branch
 *  straight from the payload. */
function buildAgentctlReadySessionUpdate(
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  payload: any,
  isSibling: boolean,
): Record<string, unknown> {
  const update: Record<string, unknown> = {};
  if (payload.agent_workspace_path !== undefined) {
    // The launched execution keeps running in its original checkout even when
    // the ready event promotes the Files root to the task root.
    update.agent_workspace_path = payload.agent_workspace_path;
  }
  if (isSibling) {
    const workspacePath = payload.workspace_path ?? payload.task_workspace_path;
    if (workspacePath) update.workspace_path = workspacePath;
    return update;
  }
  if (payload.worktree_id) update.worktree_id = payload.worktree_id;
  if (payload.worktree_path) update.worktree_path = payload.worktree_path;
  if (payload.worktree_branch) update.worktree_branch = payload.worktree_branch;
  const workspacePath =
    payload.workspace_path ?? payload.task_workspace_path ?? payload.worktree_path;
  if (workspacePath) update.workspace_path = workspacePath;
  return update;
}

/** Adds the materialized worktree to the worktrees map + the per-session list. */
function recordAgentctlReadyWorktree(
  store: StoreApi<AppState>,
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  payload: any,
  existingSession: { repository_id?: string; worktree_path?: string; worktree_branch?: string },
): void {
  if (!payload.worktree_id) return;
  store.getState().setWorktree({
    id: payload.worktree_id,
    sessionId: payload.session_id,
    // The sibling's repository identity comes from the materializer. Falling
    // back to the session's primary repository would attach both worktrees to
    // one repository and erase the source-qualified link prefixes.
    repositoryId: payload.worktree_repository_id ?? existingSession.repository_id ?? undefined,
    path: payload.worktree_path ?? existingSession.worktree_path ?? undefined,
    branch: payload.worktree_branch ?? existingSession.worktree_branch ?? undefined,
  });
  const existing =
    store.getState().sessionWorktreesBySessionId.itemsBySessionId[payload.session_id] ?? [];
  if (!existing.includes(payload.worktree_id)) {
    store.getState().setSessionWorktrees(payload.session_id, [...existing, payload.worktree_id]);
  }
}

/** Handle the agentctl_ready event: update session worktree info.
 *
 *  Two shapes share this event:
 *    1. Initial session ready — payload describes the session's primary
 *       worktree; we set worktree_id/path/branch on the session row.
 *    2. Sibling materialized (multi-branch add_branch flow) — payload
 *       describes a NEW worktree being added alongside the primary. The
 *       primary's worktree_id/branch must NOT be clobbered (they still own
 *       the chat/agent process); only workspace_path moves to the task root
 *       so the file browser repoints from "primary worktree" to "task root
 *       containing both worktree siblings". A commits refetch is bumped so
 *       the Commits panel re-queries with the new multi-repo subpaths
 *       (each commit then carries its repo/branch slug for grouping).
 */
// eslint-disable-next-line @typescript-eslint/no-explicit-any
function handleAgentctlReady(
  store: StoreApi<AppState>,
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  payload: any,
  sessionBeforeSync?: TaskSession,
): void {
  const existingSession = store.getState().taskSessions.items[payload.session_id];
  if (!existingSession) return;

  const isSibling = isSiblingWorktree(existingSession, payload);
  const previousWorkspacePath = getSessionWorkspacePath(sessionBeforeSync);
  const rootPromoted = !!(
    payload.task_workspace_path &&
    (!previousWorkspacePath || previousWorkspacePath !== payload.task_workspace_path)
  );

  const sessionUpdate = buildAgentctlReadySessionUpdate(payload, isSibling);
  if (Object.keys(sessionUpdate).length > 0) {
    store.getState().setTaskSession({ ...existingSession, ...sessionUpdate });
  }

  recordAgentctlReadyWorktree(store, payload, existingSession);

  if (isSibling) {
    // A promoted root retires the old empty-name Git scope, so drop its frozen
    // snapshot instead of masking the per-repo updates. A root-preserving
    // attachment keeps a valid unnamed root and only rebuilds the inventory.
    if (rootPromoted) {
      store.getState().clearLegacyGitStatusEntry(payload.session_id);
    }
    store.getState().invalidateGitStatusInventory(payload.session_id);
    store.getState().bumpSessionGitCheckoutGeneration(payload.session_id);
    store.getState().bumpSessionCommitsRefetch(payload.session_id);
  }
}

function resolveWorkspaceEventEnvironmentId(
  state: AppState,
  sessionId: string,
  payloadEnvironmentId: string,
): string | null {
  const mappedEnvironmentId = state.environmentIdBySessionId?.[sessionId];
  if (mappedEnvironmentId && payloadEnvironmentId && mappedEnvironmentId !== payloadEnvironmentId) {
    return null;
  }
  return payloadEnvironmentId || mappedEnvironmentId || null;
}

function workspaceRestorationTarget(
  store: StoreApi<AppState>,
  payload: TaskSessionAgentctlPayload,
  includeFailed = false,
): { sessionId: string; attempt: WorkspaceRestorationAttempt } | null {
  const sessionId = payload.session_id?.trim() ?? "";
  if (!sessionId) return null;
  const state = store.getState();
  const environmentId = resolveWorkspaceEventEnvironmentId(
    state,
    sessionId,
    payload.task_environment_id?.trim() ?? "",
  );
  if (!environmentId) return null;
  const attempt = state.workspaceRestoration?.byEnvironmentId?.[environmentId];
  if (
    !attempt ||
    (attempt.status !== "pending" && !(includeFailed && attempt.status === "error")) ||
    attempt.sessionId !== sessionId
  ) {
    return null;
  }
  return { sessionId, attempt };
}

/** Settle a workspace-only restore after the backend proves agentctl health. */
function settleWorkspaceRestorationFromAgentctl(
  store: StoreApi<AppState>,
  payload: TaskSessionAgentctlPayload,
  status: "ready" | "error",
): void {
  // Readiness from the same environment also supersedes a local restore error
  // recorded while an accepted server-side recovery was still running.
  const target = workspaceRestorationTarget(store, payload, status === "ready");
  if (!target) return;
  const state = store.getState();

  if (status === "ready") {
    if (state.completeWorkspaceRestoration?.(target.attempt)) {
      state.bumpWorkspaceFilesRefresh?.(target.sessionId);
    }
    return;
  }

  const detail = sanitizeWorkspaceRestorationDetails(
    payload.error_message || t("task:failedToRestoreWorkspace"),
  );
  state.failWorkspaceRestoration?.(target.attempt, detail);
}

interface SessionFailureContext {
  taskId: TaskId;
  sessionId: SessionId;
  newState: TaskSessionState | undefined;
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  payload: any;
  previousState: TaskSessionState | undefined;
}

/** Emits a session-failure notification for FAILED transitions, honoring suppress_toast and replay guards. */
function maybeNotifySessionFailure(store: StoreApi<AppState>, ctx: SessionFailureContext): void {
  const { taskId, sessionId, newState, payload, previousState } = ctx;
  if (newState !== "FAILED") return;

  // Only toast on observed transitions (previousState present and not FAILED).
  // If previousState is undefined we're learning about this session for the
  // first time — that's a snapshot of an already-failed session being replayed
  // by the backend (e.g. on page load / WS reconnect), not a fresh failure.
  if (
    payload.suppress_toast === true ||
    previousState === undefined ||
    previousState === "FAILED"
  ) {
    return;
  }

  const metadata =
    payload.session_metadata && typeof payload.session_metadata === "object"
      ? (payload.session_metadata as Record<string, unknown>)
      : null;
  const launchError = readLastAgentError(metadata);
  const isLaunchFailure = Boolean(launchError?.stamp && launchError.code);
  let message = t("task:sessionFailedUnexpectedly");
  if (isLaunchFailure) {
    message = t("task:launchFailedSeeDetails");
  } else if (payload.error_message) {
    message = String(payload.error_message);
  }

  store.getState().setSessionFailureNotification({
    sessionId,
    taskId,
    message,
    ...(isLaunchFailure ? { isLaunchFailure: true } : {}),
  });
}

/**
 * Applies a workspace-sources adoption event: updates the session's
 * workspace path and records the server-issued adoption boundary (WS envelope
 * timestamp) so pre-adoption turns can never become active again.
 */
function handleWorkspaceSourcesUpdated(
  store: StoreApi<AppState>,
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  payload: any,
  boundaryTimestamp?: string,
): void {
  const {
    session_id: sessionId,
    workspace_path: workspacePath,
    adopted_session_ids: adoptedSessionIds,
  } = payload;
  const existing = store.getState().taskSessions.items[sessionId];
  const previousWorkspacePath = getSessionWorkspacePath(existing);
  const rootPromoted = !!(
    workspacePath &&
    (!previousWorkspacePath || previousWorkspacePath !== workspacePath)
  );
  if (existing && workspacePath) {
    store.getState().setTaskSession({ ...existing, workspace_path: workspacePath });
  }
  // The adoption boundary must be SERVER time, so forward the WS envelope's
  // server-issued timestamp — never a client-clock value (a browser clock
  // ahead of the backend would reject legitimate new turn.started events
  // until server time caught up).
  store
    .getState()
    .reconcileWorkspaceSourcesAdopted(adoptedSessionIds ?? [sessionId], boundaryTimestamp);
  store.getState().bumpWorkspaceFilesRefresh(sessionId);
  if (rootPromoted) {
    store.getState().clearLegacyGitStatusEntry(sessionId);
  }
  store.getState().invalidateGitStatusInventory(sessionId);
  store.getState().bumpSessionGitCheckoutGeneration(sessionId);
  store.getState().bumpSessionCommitsRefetch(sessionId);
}

function handleForegroundActivityMessage(
  store: StoreApi<AppState>,
  payload: TaskSessionActivityChangedPayload,
): void {
  applyForegroundActivity(store, payload);
}

function handleCancellationPendingMessage(
  store: StoreApi<AppState>,
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  payload: any,
): void {
  applyCancellationPending(store, payload);
}

function isCompleteQueueStatus(payload: QueueStatusChangedPayload): boolean {
  return (
    payload.task_id !== undefined &&
    payload.session_incarnation_id !== undefined &&
    payload.status_epoch !== undefined &&
    payload.status_generation !== undefined &&
    payload.entries !== undefined &&
    typeof payload.count === "number" &&
    typeof payload.max === "number" &&
    typeof payload.merge_enabled === "boolean" &&
    typeof payload.auto_run === "boolean"
  );
}

// eslint-disable-next-line complexity -- incarnation, epoch, and generation form one ordering boundary.
function rejectsQueueStatus(
  state: AppState,
  payload: QueueStatusChangedPayload,
  previousMeta: QueueMeta | undefined,
): boolean {
  const currentSession = state.taskSessions.items[payload.session_id];
  if (currentSession?.queue_incarnation_id && payload.session_incarnation_id === undefined) {
    return true;
  }
  if (
    payload.session_incarnation_id !== undefined &&
    (!currentSession ||
      currentSession.task_id !== payload.task_id ||
      currentSession.queue_incarnation_id !== payload.session_incarnation_id)
  ) {
    return true;
  }
  const statusEpochChanged =
    payload.status_epoch !== undefined &&
    previousMeta?.statusEpoch !== undefined &&
    payload.status_epoch !== previousMeta.statusEpoch;
  if (statusEpochChanged && payload.status_epoch !== undefined) {
    if (previousMeta.retiredStatusEpochs?.includes(payload.status_epoch)) {
      return true;
    }
    return !isCompleteQueueStatus(payload);
  }
  return (
    payload.status_generation !== undefined &&
    previousMeta?.statusGeneration !== undefined &&
    payload.status_generation <= previousMeta.statusGeneration
  );
}

// eslint-disable-next-line complexity -- status-epoch replacement retains every prior epoch.
function queueIdentityMeta(
  payload: QueueStatusChangedPayload,
  previousMeta: QueueMeta | undefined,
) {
  if (
    payload.session_incarnation_id === undefined &&
    previousMeta?.sessionIncarnationId === undefined
  ) {
    return {};
  }
  const statusEpoch = payload.status_epoch ?? previousMeta?.statusEpoch;
  const statusEpochChanged =
    payload.status_epoch !== undefined &&
    previousMeta?.statusEpoch !== undefined &&
    payload.status_epoch !== previousMeta.statusEpoch;
  const retiredStatusEpochs = statusEpochChanged
    ? [...(previousMeta?.retiredStatusEpochs ?? []), previousMeta?.statusEpoch]
        .filter((epoch): epoch is string => epoch !== undefined && epoch !== statusEpoch)
        .filter((epoch, index, epochs) => epochs.indexOf(epoch) === index)
    : previousMeta?.retiredStatusEpochs;
  return {
    taskId: payload.task_id ?? previousMeta?.taskId,
    sessionIncarnationId: payload.session_incarnation_id ?? previousMeta?.sessionIncarnationId,
    statusEpoch,
    statusGeneration: payload.status_generation ?? previousMeta?.statusGeneration,
    retiredStatusEpochs,
  };
}

function queuePolicyMeta(payload: QueueStatusChangedPayload, previousMeta: QueueMeta | undefined) {
  return mergeQueuePolicyMeta(previousMeta, {
    ...queuePolicyStatusMeta(payload),
    sessionIncarnationId: payload.session_incarnation_id ?? previousMeta?.sessionIncarnationId,
  });
}

/** Writes a message.queue.status_changed broadcast into the queue slice,
 * preserving known policy and capacity values when an older publisher omits them. */
// eslint-disable-next-line complexity -- one handler atomically establishes a queue snapshot.
function handleQueueStatusChangedMessage(
  store: StoreApi<AppState>,
  payload: QueueStatusChangedPayload,
): void {
  if (!payload.session_id) {
    console.warn("[Queue] Missing session_id in queue status change event");
    return;
  }
  const state = store.getState();
  const previousMeta = state.queue.metaBySessionId[payload.session_id];
  if (rejectsQueueStatus(state, payload, previousMeta)) return;

  const entries = payload.entries ?? [];
  const count = typeof payload.count === "number" ? payload.count : entries.length;
  const max = typeof payload.max === "number" ? payload.max : (previousMeta?.max ?? 0);
  const meta = {
    count,
    max,
    mergeEnabled: payload.merge_enabled ?? previousMeta?.mergeEnabled ?? true,
    autoRun: payload.auto_run ?? previousMeta?.autoRun ?? true,
    ...queueIdentityMeta(payload, previousMeta),
    ...queuePolicyMeta(payload, previousMeta),
  };
  const establishesStatusEpoch =
    payload.status_epoch !== undefined &&
    previousMeta?.statusEpoch !== undefined &&
    payload.status_epoch !== previousMeta.statusEpoch;
  if (establishesStatusEpoch) {
    state.setQueueEntries(payload.session_id, entries, meta, { establishStatusEpoch: true });
    return;
  }
  state.setQueueEntries(payload.session_id, entries, meta);
}

function clearResumeAndLaunchWarnings(store: StoreApi<AppState>, sessionId: string) {
  const state = store.getState();
  state.setResumeSkipped(sessionId, false);
  state.clearLaunchWarning?.(sessionId);
}

/** Registers the task-session WebSocket handlers (state, messages, workspace sources, queue). */
// eslint-disable-next-line max-lines-per-function -- session events remain one ordered registry.
export function registerTaskSessionHandlers(store: StoreApi<AppState>): WsHandlers {
  return {
    "session.removed": (message) =>
      followRemovedSession(store, message.payload?.session_id, message.payload?.task_id),
    "message.queue.status_changed": (message) =>
      handleQueueStatusChangedMessage(store, message.payload),
    // eslint-disable-next-line complexity -- ordered session reconciliation keeps stale-event guards together
    "session.state_changed": (message) => {
      const payload = message.payload;
      if (!payload?.task_id) return;
      const { task_id: rawTaskId, session_id: rawSessionId } = payload;
      const newState = payload.new_state as TaskSessionState | undefined;

      if (!rawSessionId || isConfirmedSessionRemoved(store.getState().taskSessions, rawSessionId))
        return;
      const taskId = toTaskId(rawTaskId);
      const sessionId = toSessionId(rawSessionId);

      const sessionUpdate = buildSessionUpdate(payload);
      const existingSession = store.getState().taskSessions.items[sessionId];

      if (isStaleSessionStateEvent(existingSession, payload.updated_at)) {
        debug("state_changed ignored stale snapshot", {
          sessionId,
          task_id: taskId,
          existingUpdatedAt: existingSession?.updated_at,
          payloadUpdatedAt: payload.updated_at,
          newState: newState ?? "-",
        });
        return;
      }

      debug("state_changed", {
        sessionId,
        // Logged before upsertTaskSessionList below, so on the first event for a
        // session the store has no row yet and the auto-resolver can't map it —
        // exactly the oldState="-" anchor line. taskId is already in scope, so
        // pass it directly (rendered as task_id=, matching the auto-annotation).
        task_id: taskId,
        oldState: existingSession?.state ?? "-",
        newState: newState ?? "-",
      });

      maybeMarkQuickChatUnseenIdle(store, sessionId, {
        previousState: existingSession?.state,
        fallbackPreviousState: payload.old_state as TaskSessionState | undefined,
        newState,
        updatedAt: payload.updated_at,
      });
      upsertTaskSessionList(store, taskId, sessionId, payload, sessionUpdate);
      store.getState().reconcileWorkflowSessionFocus?.(taskId);
      syncKanbanPrimarySessionState(store, taskId, sessionId, newState);
      extractContextWindow(store, sessionId, payload);
      maybePromoteAgentctlReady(store, sessionId, newState, message.timestamp);
      if (newState === "STARTING") {
        const agentctl = store.getState().sessionAgentctl?.itemsBySessionId?.[sessionId];
        store
          .getState()
          .invalidateConfirmedConfigOptions(
            sessionId,
            agentctl?.startingExecutionId ??
              (agentctl?.status === "starting" ? agentctl.agentExecutionId : undefined),
          );
        if (agentctl?.startingExecutionId) {
          const { startingExecutionId: _startingExecutionId, ...status } = agentctl;
          store.getState().setSessionAgentctlStatus(sessionId, status);
        }
      }

      // A confirmed RUNNING transition clears the resume-skipped marker
      // (prevent-auto-start-on-open). STARTING deliberately does NOT clear
      // it: a failed manual resume emits STARTING before the launch fails,
      // and clearing there would drop the Start agent retry affordance.
      if (newState === "RUNNING") {
        clearResumeAndLaunchWarnings(store, sessionId);
      }

      maybeAdoptSessionOnTransition(
        store,
        taskId,
        sessionId,
        newState,
        !!existingSession,
        existingSession?.state,
      );

      maybeNotifySessionFailure(store, {
        taskId,
        sessionId,
        newState,
        payload,
        previousState: existingSession?.state,
      });

      maybeFanOutOfficeRefetch(store, newState, existingSession?.state);
    },
    "session.activity_changed": (message) =>
      handleForegroundActivityMessage(store, message.payload),
    "session.cancellation_changed": (message) =>
      handleCancellationPendingMessage(store, message.payload),
    "session.agentctl_starting": (message) => {
      const payload = message.payload;
      if (
        !payload?.session_id ||
        isConfirmedSessionRemoved(store.getState().taskSessions, payload.session_id)
      )
        return;
      store
        .getState()
        .invalidateConfirmedConfigOptions(payload.session_id, payload.agent_execution_id);
      store.getState().setSessionAgentctlStatus(payload.session_id, {
        status: "starting",
        agentExecutionId: payload.agent_execution_id,
        updatedAt: message.timestamp,
      });
      syncEnvFromAgentctlPayload(store, payload);
    },
    "session.agentctl_ready": (message) => {
      const payload = message.payload;
      if (
        !payload?.session_id ||
        isConfirmedSessionRemoved(store.getState().taskSessions, payload.session_id)
      )
        return;
      const sessionBeforeSync = store.getState().taskSessions.items[payload.session_id];
      store.getState().setSessionAgentctlStatus(payload.session_id, {
        status: "ready",
        agentExecutionId: payload.agent_execution_id,
        updatedAt: message.timestamp,
      });
      syncEnvFromAgentctlPayload(store, payload);
      settleWorkspaceRestorationFromAgentctl(store, payload, "ready");
      handleAgentctlReady(store, payload, sessionBeforeSync);
    },
    "session.agentctl_error": (message) => {
      const payload = message.payload;
      if (
        !payload?.session_id ||
        isConfirmedSessionRemoved(store.getState().taskSessions, payload.session_id)
      )
        return;
      store.getState().setSessionAgentctlStatus(payload.session_id, {
        status: "error",
        agentExecutionId: payload.agent_execution_id,
        errorMessage: payload.error_message,
        updatedAt: message.timestamp,
      });
      settleWorkspaceRestorationFromAgentctl(store, payload, "error");
    },
    "session.workspace_sources.updated": (message) =>
      handleWorkspaceSourcesUpdated(store, message.payload, message.timestamp),
  };
}
