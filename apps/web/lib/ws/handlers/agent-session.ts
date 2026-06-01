import type { StoreApi } from "zustand";
import type { QueryClient } from "@tanstack/react-query";
import type { AppState } from "@/lib/state/store";
import type { WsHandlers } from "@/lib/ws/handlers/types";
import { qk } from "@/lib/query/keys";
import {
  type SessionId,
  type TaskId,
  type TaskSession,
  type TaskSessionState,
  type TaskSessionsResponse,
} from "@/lib/types/http";
import type { QueuedMessage } from "@/lib/state/slices/session/types";
import { readAgentctlStatus, writeAgentctlStatus } from "@/lib/query/agentctl-status";
import { createDebugLogger } from "@/lib/debug/log";

const debug = createDebugLogger("session:state");

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

/** Ignore subscribe snapshots that were read before a newer state landed. */
export function isStaleSessionStateEvent(
  existing: { updated_at?: string } | null | undefined,
  payloadUpdatedAt: string | undefined,
): boolean {
  if (!payloadUpdatedAt || !existing?.updated_at) return false;
  const payloadTime = Date.parse(payloadUpdatedAt);
  const existingTime = Date.parse(existing.updated_at);
  if (Number.isNaN(payloadTime) || Number.isNaN(existingTime)) return false;
  // Strict less-than: equal timestamps are treated as not-stale so identical
  // events upsert idempotently rather than being silently dropped.
  return payloadTime < existingTime;
}

// ---------------------------------------------------------------------------
// TQ cache reads (the TaskSession record now lives in TanStack Query; the
// session-state bridge in lib/query/bridge/session-state.ts owns the writes).
// ---------------------------------------------------------------------------

/** Read a single TaskSession from the TQ by-id cache. */
function readSessionById(qc: QueryClient, sessionId: string): TaskSession | null {
  return qc.getQueryData<TaskSession | null>(qk.taskSession.byId(sessionId)) ?? null;
}

/** Read the per-task session list from the TQ by-task cache. */
function readSessionsByTask(qc: QueryClient, taskId: string): TaskSession[] {
  return qc.getQueryData<TaskSessionsResponse>(qk.taskSession.byTask(taskId))?.sessions ?? [];
}

/** Promote agentctl status to "ready" when the session enters a live state.
 *  Acts as a fallback for missed/late agentctl_ready WS events — the backend
 *  cannot reach RUNNING/WAITING_FOR_INPUT without a live agentctl. Never
 *  downgrades an existing "ready" entry. */
function maybePromoteAgentctlReady(
  qc: QueryClient,
  sessionId: string,
  newState: TaskSessionState | undefined,
  timestamp: string | undefined,
): void {
  if (!newState || !AGENT_LIVE_STATES.has(newState)) return;
  const current = readAgentctlStatus(qc, sessionId);
  if (current?.status === "ready") return;
  writeAgentctlStatus(qc, sessionId, {
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
  store: StoreApi<AppState>,
  qc: QueryClient,
  taskId: string,
  newState: TaskSessionState | undefined,
): boolean {
  if (!newState || isTerminalSessionState(newState)) return false;
  const state = store.getState();
  if (state.tasks.activeTaskId !== taskId) return false;
  const activeSessionId = state.tasks.activeSessionId;
  if (activeSessionId) {
    const activeSession = readSessionById(qc, activeSessionId);
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
export function pickReplacementSessionId(qc: QueryClient, taskId: string): string | null {
  const sessions = readSessionsByTask(qc, taskId);
  if (sessions.length === 0) return null;
  for (let i = sessions.length - 1; i >= 0; i -= 1) {
    const candidate = sessions[i];
    if (!isTerminalSessionState(candidate.state)) return candidate.id;
  }
  return null;
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
function inheritAgentctlStatus(qc: QueryClient, fromSessionId: string, toSessionId: string): void {
  const oldAgentctl = readAgentctlStatus(qc, fromSessionId);
  if (oldAgentctl?.status === "ready") {
    writeAgentctlStatus(qc, toSessionId, oldAgentctl);
  }
}

interface AdoptionContext {
  store: StoreApi<AppState>;
  qc: QueryClient;
  taskId: string;
  sessionId: string;
  newState: TaskSessionState | undefined;
  wasKnownToStore: boolean;
}

/**
 * After a `session.state_changed` event, decide whether the chat UI should
 * follow a workflow-driven session switch. Covers both event orderings:
 *   1. New non-terminal session appears for the active task before the old
 *      one is torn down — adopt immediately.
 *   2. The current active session transitions to a terminal state — hand off
 *      to the newest non-terminal session for the same task, if any.
 */
function maybeAdoptSessionOnTransition(ctx: AdoptionContext): void {
  const { store, qc, taskId, sessionId, newState, wasKnownToStore } = ctx;
  const state = store.getState();

  if (!wasKnownToStore && shouldAdoptNewSession(store, qc, taskId, newState)) {
    const oldSessionId = state.tasks.activeSessionId;
    // Reverse-ordering guard: if the events arrive as old=COMPLETED then
    // new=STARTING (instead of the typical new=STARTING then old=COMPLETED),
    // shouldAdoptNewSession returns true on the second event because the old
    // session is now terminal. But the user may have pinned the old session —
    // in that case the symmetric guard below was skipped (no terminal event
    // for the new session), and we'd auto-yank them off their pinned session
    // here. Match the terminal-handoff path's pinning check.
    if (oldSessionId && state.tasks.pinnedSessionId === oldSessionId) return;
    if (oldSessionId) inheritAgentctlStatus(qc, oldSessionId, sessionId);
    state.setActiveSessionAuto(taskId, sessionId);
    return;
  }

  const isActive = state.tasks.activeSessionId === sessionId;
  if (isActive && newState && isTerminalSessionState(newState)) {
    // If the user explicitly pinned this session (manual click), don't yank
    // them away just because the workflow moved it to a terminal state.
    if (state.tasks.pinnedSessionId === sessionId) return;
    const replacement = pickReplacementSessionId(qc, taskId);
    if (replacement && replacement !== sessionId) {
      inheritAgentctlStatus(qc, sessionId, replacement);
      state.setActiveSessionAuto(taskId, replacement);
    }
  }
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

  store.getState().setSessionFailureNotification({
    sessionId,
    taskId,
    message: payload.error_message ? String(payload.error_message) : "Session failed unexpectedly",
  });
}

export function registerTaskSessionHandlers(
  store: StoreApi<AppState>,
  qc: QueryClient,
): WsHandlers {
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
      const { task_id: rawTaskId, session_id: rawSessionId } = payload;
      const newState = payload.new_state as TaskSessionState | undefined;

      if (!rawSessionId) return;
      const taskId = rawTaskId as string;
      const sessionId = rawSessionId as string;

      // Snapshot the prior record BEFORE the session-state bridge mutates the
      // TQ cache — adoption + failure-toast logic both branch on whether this
      // session was already known and on its previous state.
      const existingSession = readSessionById(qc, sessionId);

      // Drop out-of-order subscribe snapshots: a state_changed carrying an
      // older updated_at than the record we already have would otherwise stomp
      // a fresher state (e.g. a stale STARTING snapshot reverting a live
      // WAITING_FOR_INPUT and blocking idle input). The session-state bridge
      // applies the same guard on the cache-write side; this short-circuits the
      // adoption / failure-toast logic for the same reason.
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
        // Logged before the session-state bridge upserts the TQ row, so on the
        // first event for a session the cache has no record yet and the
        // auto-resolver can't map it — the oldState="-" anchor line. taskId is
        // already in scope, so pass it directly (rendered as task_id=).
        task_id: taskId,
        oldState: existingSession?.state ?? "-",
        newState: newState ?? "-",
      });

      extractContextWindow(store, sessionId, payload);
      maybePromoteAgentctlReady(qc, sessionId, newState, message.timestamp);

      maybeAdoptSessionOnTransition({
        store,
        qc,
        taskId,
        sessionId,
        newState,
        wasKnownToStore: !!existingSession,
      });

      maybeNotifySessionFailure(store, {
        taskId: taskId as TaskId,
        sessionId: sessionId as SessionId,
        newState,
        payload,
        previousState: existingSession?.state,
      });
    },
  };
}
