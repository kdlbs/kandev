import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import type { WsHandlers } from "@/lib/ws/handlers/types";
import type { TaskSessionState } from "@/lib/types/http";
import type { QueuedMessage } from "@/lib/state/slices/session/types";

const TERMINAL_SESSION_STATES: ReadonlySet<TaskSessionState> = new Set([
  "COMPLETED",
  "CANCELLED",
  "FAILED",
]);

// States that imply agentctl is up and processing (or has fully processed) input.
// If we observe a session in one of these, agentctl must be ready even if we
// missed the agentctl_ready WS event (e.g. it fired before our subscription).
const AGENT_LIVE_STATES: ReadonlySet<TaskSessionState> = new Set(["RUNNING", "WAITING_FOR_INPUT"]);

export function isTerminalSessionState(state: TaskSessionState | undefined): boolean {
  return !!state && TERMINAL_SESSION_STATES.has(state);
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
  if (!newState || !AGENT_LIVE_STATES.has(newState)) return;
  const current = store.getState().sessionAgentctl?.itemsBySessionId?.[sessionId];
  if (current?.status === "ready") return;
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
 * Adopts only when the current active session is missing, cross-task, or
 * already terminal — not while a live session for the same task is still
 * running (the backend only creates sessions during workflow step transitions
 * after stopping the previous one, but WS events may arrive out of order).
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
    if (activeSession?.task_id === taskId && !isTerminalSessionState(activeSession.state)) {
      return false;
    }
  }
  return true;
}

/**
 * Pick the newest non-terminal session for a task. Used when the currently
 * active session just reached a terminal state — we want to hand focus to the
 * session that replaced it (typically created by a workflow step transition).
 */
export function pickReplacementSessionId(state: AppState, taskId: string): string | null {
  const sessions = state.taskSessionsByTask.itemsByTaskId[taskId];
  if (!sessions) return null;
  for (let i = sessions.length - 1; i >= 0; i -= 1) {
    const candidate = sessions[i];
    if (!isTerminalSessionState(candidate.state)) return candidate.id;
  }
  return null;
}

/** Build a session update object from the state_changed payload. */
// eslint-disable-next-line @typescript-eslint/no-explicit-any
function buildSessionUpdate(payload: any): Record<string, unknown> {
  const update: Record<string, unknown> = {};
  if (payload.new_state) update.state = payload.new_state;
  if (payload.agent_profile_id) update.agent_profile_id = payload.agent_profile_id;
  if (payload.agent_profile_id) update.agent_profile_id = payload.agent_profile_id;
  if (payload.review_status !== undefined) update.review_status = payload.review_status;
  if (payload.error_message !== undefined) update.error_message = payload.error_message;
  if (payload.agent_profile_snapshot)
    update.agent_profile_snapshot = payload.agent_profile_snapshot;
  if (payload.is_passthrough !== undefined) update.is_passthrough = payload.is_passthrough;
  if (payload.session_metadata !== undefined) update.metadata = payload.session_metadata;
  if (payload.task_environment_id) update.task_environment_id = payload.task_environment_id;
  return update;
}

/** Upsert the session in the per-task sessions list from a WS event.
 *  Uses `upsertTaskSessionFromEvent` so the per-task list is not marked as
 *  fully loaded — partial event payloads must not gate the API hydration that
 *  fills in fields like agent_profile_id / repository_id / worktree_path. */
function upsertTaskSessionList(
  store: StoreApi<AppState>,
  taskId: string,
  sessionId: string,
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
    started_at: existing?.started_at ?? "",
    updated_at: existing?.updated_at ?? "",
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

/** Extract context window data from payload metadata and store it. */
// eslint-disable-next-line @typescript-eslint/no-explicit-any
function extractContextWindow(store: StoreApi<AppState>, sessionId: string, payload: any): void {
  const metadata = payload.metadata;
  if (!metadata || typeof metadata !== "object") return;
  const contextWindow = (metadata as Record<string, unknown>).context_window;
  if (!contextWindow || typeof contextWindow !== "object") return;
  const cw = contextWindow as Record<string, unknown>;
  store.getState().setContextWindow(sessionId, {
    size: (cw.size as number) ?? 0,
    used: (cw.used as number) ?? 0,
    remaining: (cw.remaining as number) ?? 0,
    efficiency: (cw.efficiency as number) ?? 0,
    timestamp: new Date().toISOString(),
  });
}

/** Copy agentctl "ready" status from one session to another (same-task switch). */
function inheritAgentctlStatus(state: AppState, fromSessionId: string, toSessionId: string): void {
  const oldAgentctl = state.sessionAgentctl?.itemsBySessionId?.[fromSessionId];
  if (oldAgentctl?.status === "ready") {
    state.setSessionAgentctlStatus(toSessionId, oldAgentctl);
  }
}

/**
 * After a `session.state_changed` event, decide whether the chat UI should
 * follow a workflow-driven session switch. Covers both event orderings:
 *   1. New non-terminal session appears for the active task before the old
 *      one is torn down — adopt immediately.
 *   2. The current active session transitions to a terminal state — hand off
 *      to the newest non-terminal session for the same task, if any.
 */
function maybeAdoptSessionOnTransition(
  store: StoreApi<AppState>,
  taskId: string,
  sessionId: string,
  newState: TaskSessionState | undefined,
  wasKnownToStore: boolean,
): void {
  const state = store.getState();

  if (!wasKnownToStore && shouldAdoptNewSession(state, taskId, newState)) {
    const oldSessionId = state.tasks.activeSessionId;
    // Reverse-ordering guard: if the events arrive as old=COMPLETED then
    // new=STARTING (instead of the typical new=STARTING then old=COMPLETED),
    // shouldAdoptNewSession returns true on the second event because the old
    // session is now terminal. But the user may have pinned the old session —
    // in that case the symmetric guard below was skipped (no terminal event
    // for the new session), and we'd auto-yank them off their pinned session
    // here. Match the terminal-handoff path's pinning check.
    if (oldSessionId && state.tasks.pinnedSessionId === oldSessionId) return;
    if (oldSessionId) inheritAgentctlStatus(state, oldSessionId, sessionId);
    state.setActiveSessionAuto(taskId, sessionId);
    return;
  }

  const isActive = state.tasks.activeSessionId === sessionId;
  if (isActive && newState && isTerminalSessionState(newState)) {
    // If the user explicitly pinned this session (manual click), don't yank
    // them away just because the workflow moved it to a terminal state.
    if (state.tasks.pinnedSessionId === sessionId) return;
    const replacement = pickReplacementSessionId(state, taskId);
    if (replacement && replacement !== sessionId) {
      inheritAgentctlStatus(state, sessionId, replacement);
      state.setActiveSessionAuto(taskId, replacement);
    }
  }
}

/** Seed the session→environment mapping from an agentctl event payload.
 *  Brand-new CREATED sessions never receive a `state_changed` carrying
 *  `task_environment_id` (no transition fires), so the env mapping would
 *  otherwise stay empty and env-routed shell terminals stall on
 *  "Connecting terminal...". Routes through `upsertTaskSessionFromEvent`
 *  which calls `syncEnvironmentMapping` and merges with any existing row. */
function syncEnvFromAgentctlPayload(
  store: StoreApi<AppState>,
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  payload: any,
): void {
  const taskId = payload?.task_id;
  const sessionId = payload?.session_id;
  const envId = payload?.task_environment_id;
  if (!taskId || !sessionId || !envId) return;
  const existing = store.getState().taskSessions.items[sessionId];
  store.getState().upsertTaskSessionFromEvent(taskId, {
    id: sessionId,
    task_id: taskId,
    state: existing?.state ?? "CREATED",
    started_at: existing?.started_at ?? "",
    updated_at: existing?.updated_at ?? "",
    task_environment_id: envId,
  });
}

/** Handle the agentctl_ready event: update session worktree info. */
// eslint-disable-next-line @typescript-eslint/no-explicit-any
function handleAgentctlReady(store: StoreApi<AppState>, payload: any): void {
  const existingSession = store.getState().taskSessions.items[payload.session_id];
  if (!existingSession) return;

  const sessionUpdate: Record<string, unknown> = {};
  if (payload.worktree_id) sessionUpdate.worktree_id = payload.worktree_id;
  if (payload.worktree_path) sessionUpdate.worktree_path = payload.worktree_path;
  if (payload.worktree_branch) sessionUpdate.worktree_branch = payload.worktree_branch;

  if (Object.keys(sessionUpdate).length > 0) {
    store.getState().setTaskSession({ ...existingSession, ...sessionUpdate });
  }

  if (payload.worktree_id) {
    store.getState().setWorktree({
      id: payload.worktree_id,
      sessionId: payload.session_id,
      repositoryId: existingSession.repository_id ?? undefined,
      path: payload.worktree_path ?? existingSession.worktree_path ?? undefined,
      branch: payload.worktree_branch ?? existingSession.worktree_branch ?? undefined,
    });
    const existing =
      store.getState().sessionWorktreesBySessionId.itemsBySessionId[payload.session_id] ?? [];
    if (!existing.includes(payload.worktree_id)) {
      store.getState().setSessionWorktrees(payload.session_id, [...existing, payload.worktree_id]);
    }
  }
}

interface SessionFailureContext {
  taskId: string;
  sessionId: string;
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

  store.getState().setSessionFailureNotification({
    sessionId,
    taskId,
    message: payload.error_message ? String(payload.error_message) : "Session failed unexpectedly",
  });
}

export function registerTaskSessionHandlers(store: StoreApi<AppState>): WsHandlers {
  return {
    "message.queue.status_changed": (message) => {
      const payload = message.payload;
      if (!payload?.session_id) {
        console.warn("[Queue] Missing session_id in queue status change event");
        return;
      }
      const sessionId = payload.session_id;
      const entries = (payload.entries as QueuedMessage[] | null | undefined) ?? [];
      const count = typeof payload.count === "number" ? payload.count : entries.length;
      const max = typeof payload.max === "number" ? payload.max : 0;
      store.getState().setQueueEntries(sessionId, entries, { count, max });
    },
    "session.state_changed": (message) => {
      const payload = message.payload;
      if (!payload?.task_id) return;
      const { task_id: taskId, session_id: sessionId } = payload;
      const newState = payload.new_state as TaskSessionState | undefined;

      if (!sessionId) return;

      const sessionUpdate = buildSessionUpdate(payload);
      const existingSession = store.getState().taskSessions.items[sessionId];

      upsertTaskSessionList(store, taskId, sessionId, payload, sessionUpdate);
      extractContextWindow(store, sessionId, payload);
      maybePromoteAgentctlReady(store, sessionId, newState, message.timestamp);

      maybeAdoptSessionOnTransition(store, taskId, sessionId, newState, !!existingSession);

      maybeNotifySessionFailure(store, {
        taskId,
        sessionId,
        newState,
        payload,
        previousState: existingSession?.state,
      });

      maybeFanOutOfficeRefetch(store, newState, existingSession?.state);
    },
    "session.agentctl_starting": (message) => {
      const payload = message.payload;
      if (!payload?.session_id) return;
      store.getState().setSessionAgentctlStatus(payload.session_id, {
        status: "starting",
        agentExecutionId: payload.agent_execution_id,
        updatedAt: message.timestamp,
      });
      syncEnvFromAgentctlPayload(store, payload);
    },
    "session.agentctl_ready": (message) => {
      const payload = message.payload;
      if (!payload?.session_id) return;
      store.getState().setSessionAgentctlStatus(payload.session_id, {
        status: "ready",
        agentExecutionId: payload.agent_execution_id,
        updatedAt: message.timestamp,
      });
      syncEnvFromAgentctlPayload(store, payload);
      handleAgentctlReady(store, payload);
    },
    "session.agentctl_error": (message) => {
      const payload = message.payload;
      if (!payload?.session_id) return;
      store.getState().setSessionAgentctlStatus(payload.session_id, {
        status: "error",
        agentExecutionId: payload.agent_execution_id,
        errorMessage: payload.error_message,
        updatedAt: message.timestamp,
      });
    },
  };
}
