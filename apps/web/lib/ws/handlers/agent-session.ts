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

export function isTerminalSessionState(state: TaskSessionState | undefined): boolean {
  return !!state && TERMINAL_SESSION_STATES.has(state);
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
  if (payload.review_status !== undefined) update.review_status = payload.review_status;
  if (payload.error_message !== undefined) update.error_message = payload.error_message;
  if (payload.agent_profile_snapshot)
    update.agent_profile_snapshot = payload.agent_profile_snapshot;
  if (payload.is_passthrough !== undefined) update.is_passthrough = payload.is_passthrough;
  if (payload.session_metadata !== undefined) update.metadata = payload.session_metadata;
  if (payload.task_environment_id) update.task_environment_id = payload.task_environment_id;
  return update;
}

/** Upsert the session in the per-task sessions list. */
function upsertTaskSessionList(
  store: StoreApi<AppState>,
  taskId: string,
  sessionId: string,
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  payload: any,
  sessionUpdate: Record<string, unknown>,
): void {
  const sessionsByTask = store.getState().taskSessionsByTask.itemsByTaskId[taskId];
  const newState = payload.new_state as TaskSessionState | undefined;

  if (!sessionsByTask) {
    // Initialize per-task sessions from WS event so the sidebar can
    // display up-to-date status even for tasks not yet clicked.
    if (newState) {
      store.getState().setTaskSessionsForTask(taskId, [
        {
          id: sessionId,
          task_id: taskId,
          state: newState,
          started_at: "",
          updated_at: "",
          ...(payload.agent_profile_id ? { agent_profile_id: payload.agent_profile_id } : {}),
          ...sessionUpdate,
        },
      ]);
    }
    return;
  }

  const hasSession = sessionsByTask.some((s) => s.id === sessionId);

  if (!hasSession && newState) {
    store.getState().setTaskSessionsForTask(taskId, [
      ...sessionsByTask,
      {
        id: sessionId,
        task_id: taskId,
        state: newState,
        started_at: "",
        updated_at: "",
        ...(payload.agent_profile_id ? { agent_profile_id: payload.agent_profile_id } : {}),
        ...sessionUpdate,
      },
    ]);
  } else if (hasSession) {
    store.getState().setTaskSessionsForTask(
      taskId,
      sessionsByTask.map((s) => (s.id === sessionId ? { ...s, ...sessionUpdate } : s)),
    );
  }
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
    if (oldSessionId) inheritAgentctlStatus(state, oldSessionId, sessionId);
    state.setActiveSession(taskId, sessionId);
    return;
  }

  const isActive = state.tasks.activeSessionId === sessionId;
  if (isActive && newState && isTerminalSessionState(newState)) {
    const replacement = pickReplacementSessionId(state, taskId);
    if (replacement && replacement !== sessionId) {
      inheritAgentctlStatus(state, sessionId, replacement);
      state.setActiveSession(taskId, replacement);
    }
  }
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

export function registerTaskSessionHandlers(store: StoreApi<AppState>): WsHandlers {
  return {
    "message.queue.status_changed": (message) => {
      const payload = message.payload;
      if (!payload?.session_id) {
        console.warn("[Queue] Missing session_id in queue status change event");
        return;
      }
      const sessionId = payload.session_id;
      const isQueued = payload.is_queued as boolean;
      const queuedMessage = payload.message as QueuedMessage | null | undefined;
      store.getState().setQueueStatus(sessionId, { is_queued: isQueued, message: queuedMessage });
    },
    "session.state_changed": (message) => {
      const payload = message.payload;
      if (!payload?.task_id) return;
      const { task_id: taskId, session_id: sessionId } = payload;
      const newState = payload.new_state as TaskSessionState | undefined;

      if (!sessionId) return;

      const sessionUpdate = buildSessionUpdate(payload);
      const existingSession = store.getState().taskSessions.items[sessionId];

      if (existingSession) {
        store.getState().setTaskSession({ ...existingSession, ...sessionUpdate });
      } else if (newState) {
        store.getState().setTaskSession({
          id: sessionId,
          task_id: taskId,
          state: newState as TaskSessionState,
          started_at: "",
          updated_at: "",
          ...(payload.agent_profile_id ? { agent_profile_id: payload.agent_profile_id } : {}),
          ...sessionUpdate,
        });
      }

      upsertTaskSessionList(store, taskId, sessionId, payload, sessionUpdate);
      extractContextWindow(store, sessionId, payload);

      maybeAdoptSessionOnTransition(store, taskId, sessionId, newState, !!existingSession);

      if (
        newState === "FAILED" &&
        payload.suppress_toast !== true &&
        existingSession?.state !== "FAILED" // replay guard; hook dedup is the second layer
      ) {
        store.getState().setSessionFailureNotification({
          sessionId,
          taskId,
          message: payload.error_message
            ? String(payload.error_message)
            : "Session failed unexpectedly",
        });
      }
    },
    "session.agentctl_starting": (message) => {
      const payload = message.payload;
      if (!payload?.session_id) return;
      store.getState().setSessionAgentctlStatus(payload.session_id, {
        status: "starting",
        agentExecutionId: payload.agent_execution_id,
        updatedAt: message.timestamp,
      });
    },
    "session.agentctl_ready": (message) => {
      const payload = message.payload;
      if (!payload?.session_id) return;
      store.getState().setSessionAgentctlStatus(payload.session_id, {
        status: "ready",
        agentExecutionId: payload.agent_execution_id,
        updatedAt: message.timestamp,
      });
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
