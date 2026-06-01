/**
 * Session-state WS → TanStack Query bridge (D4 + D6 Stage 1).
 *
 * Mirrors the live-session-state half of `lib/ws/handlers/agent-session.ts`:
 *   session.state_changed       → session state / agent_profile_id / error /
 *                                 worktree_* / task_environment_id transitions
 *   session.agentctl_starting   → agentctl status ("starting") + env mapping
 *                                 (CREATED sessions have no state_changed
 *                                 carrying task_environment_id)
 *   session.agentctl_ready      → agentctl status ("ready") + worktree_* fields
 *                                 + env mapping
 *   session.agentctl_error      → agentctl status ("error" + errorMessage); no
 *                                 TaskSession field changes
 *
 * The agentctl status badge is fully TQ-backed (qk.session.agentctl), read via
 * sessionAgentctlQueryOptions / useSessionAgentctl.
 *
 * Why a separate bridge from `bridge/session.ts`: that file owns message / turn
 * / task-plan / queue events. This one owns the per-session TaskSession record.
 *
 * Caches written (mergeTaskSession semantics, identical to the Zustand slice):
 *   - qk.taskSession.byId(sessionId)   — the new dedicated by-id observe surface
 *   - qk.taskSession.byTask(taskId)    — the existing per-task session list
 *
 * Cross-domain (D6) side-effects, kept populated in Zustand for now so getEnvKey
 * keeps working:
 *   - setEnvMapping(sessionId, environmentId) → environmentIdBySessionId
 *
 * Prepare-progress is fully TQ-backed: writePrepareProgress(qc, session) seeds
 * qk.session.prepareProgress (seed-if-absent) and the readers use useQuery.
 *
 * NOT migrated here (stays in agent-session.ts as the authority for this stage):
 *   adoption / promotion (maybeAdoptSessionOnTransition, maybePromoteAgentctlReady,
 *   pickReplacementSessionId), failure toasts (maybeNotifySessionFailure),
 *   context-window extraction, and worktree index bookkeeping. Stage 1 is
 *   additive: both stores stay populated in parallel.
 */

import type { QueryClient } from "@tanstack/react-query";
import type { WebSocketClient } from "@/lib/ws/client";
import { qk } from "@/lib/query/keys";
import {
  sessionId as toSessionId,
  taskId as toTaskId,
  type TaskSession,
  type TaskSessionState,
  type TaskSessionsResponse,
} from "@/lib/types/http";
import type { SessionPrepareState } from "@/lib/state/slices/session-runtime/types";
import { prepareResultToSessionState } from "@/lib/state/slices/session-runtime/prepare-result";
import { mergeTaskSession } from "@/lib/state/slices/session/session-slice";
import { writeAgentctlStatus } from "@/lib/query/agentctl-status";
import { isStaleSessionStateEvent } from "@/lib/ws/handlers/agent-session";
import { wrapBridgeHandler } from "./index";

// ---------------------------------------------------------------------------
// Cross-domain (D6) side-effect callbacks
// ---------------------------------------------------------------------------

/**
 * Callbacks the registrar caller wires to the Zustand store so the bridge keeps
 * D6 client-side indices populated while the migration is mid-flight:
 *   - setEnvMapping → environmentIdBySessionId (read by getEnvKey for git keys)
 */
export interface SessionStateBridgeDeps {
  setEnvMapping: (sessionId: string, environmentId: string) => void;
}

// ---------------------------------------------------------------------------
// Payload shaping (loose — the live handler reads fields not all in the type,
// e.g. session_metadata, so we mirror it through a record view).
// ---------------------------------------------------------------------------

type LoosePayload = Record<string, unknown>;

function str(payload: LoosePayload, key: string): string | undefined {
  const v = payload[key];
  return typeof v === "string" ? v : undefined;
}

/**
 * Build the set of TaskSession fields this event mutates. Mirrors
 * `buildSessionUpdate` in agent-session.ts plus the agentctl_ready worktree
 * fields. Only sets keys present in the payload so mergeTaskSession preserves
 * everything else.
 */
function buildSessionPatch(payload: LoosePayload): Partial<TaskSession> {
  const patch: Partial<TaskSession> = {};
  const newState = str(payload, "new_state");
  if (newState) patch.state = newState as TaskSessionState;
  const profileId = str(payload, "agent_profile_id");
  if (profileId) patch.agent_profile_id = profileId as TaskSession["agent_profile_id"];
  if (payload.review_status !== undefined)
    patch.review_status = payload.review_status as TaskSession["review_status"];
  if (payload.error_message !== undefined)
    patch.error_message = payload.error_message as TaskSession["error_message"];
  if (payload.agent_profile_snapshot)
    patch.agent_profile_snapshot =
      payload.agent_profile_snapshot as TaskSession["agent_profile_snapshot"];
  if (payload.is_passthrough !== undefined)
    patch.is_passthrough = payload.is_passthrough as TaskSession["is_passthrough"];
  if (payload.session_metadata !== undefined)
    patch.metadata = payload.session_metadata as TaskSession["metadata"];
  const envId = str(payload, "task_environment_id");
  if (envId) patch.task_environment_id = envId;
  // agentctl_ready worktree fields
  const worktreeId = str(payload, "worktree_id");
  if (worktreeId) patch.worktree_id = worktreeId;
  const worktreePath = str(payload, "worktree_path");
  if (worktreePath) patch.worktree_path = worktreePath;
  const worktreeBranch = str(payload, "worktree_branch");
  if (worktreeBranch) patch.worktree_branch = worktreeBranch;
  // Authoritative row timestamp — recorded so later out-of-order snapshots can
  // be dropped by isStaleSessionStateEvent.
  const updatedAt = str(payload, "updated_at");
  if (updatedAt) patch.updated_at = updatedAt;
  return patch;
}

// ---------------------------------------------------------------------------
// TQ cache writers
// ---------------------------------------------------------------------------

/** Apply a session patch to the by-id cache, merging with any existing record. */
function writeById(qc: QueryClient, session: TaskSession): TaskSession {
  let merged = session;
  qc.setQueryData<TaskSession | null>(qk.taskSession.byId(session.id), (prev) => {
    merged = prev ? mergeTaskSession(prev, session) : session;
    return merged;
  });
  return merged;
}

/** Upsert the merged session into the per-task list cache (if that list exists). */
function writeByTask(qc: QueryClient, taskId: string, merged: TaskSession): void {
  qc.setQueryData<TaskSessionsResponse>(qk.taskSession.byTask(taskId), (prev) => {
    if (!prev) {
      // No list cached for this task yet — seed a single-entry list so by-task
      // consumers (sidebar, switcher) see the session immediately.
      return { sessions: [merged], total: 1 };
    }
    const idx = prev.sessions.findIndex((s) => s.id === merged.id);
    if (idx >= 0) {
      const sessions = [...prev.sessions];
      sessions[idx] = merged;
      return { ...prev, sessions };
    }
    return { sessions: [...prev.sessions, merged], total: prev.total + 1 };
  });
}

/**
 * Mirror a session's `metadata.prepare_result` into the D6 prepare-progress
 * TQ cache (seed-if-absent so live executor.prepare.* events aren't clobbered),
 * matching the Zustand `syncPrepareProgress` semantics.
 */
function writePrepareProgress(qc: QueryClient, session: TaskSession): void {
  const key = qk.session.prepareProgress(session.id);
  if (qc.getQueryData(key)) return;
  const prepareState: SessionPrepareState | null = prepareResultToSessionState(
    session.id,
    session.metadata,
  );
  if (prepareState) qc.setQueryData(key, prepareState);
}

// ---------------------------------------------------------------------------
// Core apply: merge into a full TaskSession, write all caches + D6 side-effects
// ---------------------------------------------------------------------------

function applySessionEvent(
  qc: QueryClient,
  deps: SessionStateBridgeDeps,
  rawTaskId: string,
  rawSessionId: string,
  patch: Partial<TaskSession>,
): void {
  const taskIdBranded = toTaskId(rawTaskId);
  const sessionIdBranded = toSessionId(rawSessionId);

  // Build a candidate full record; mergeTaskSession (in writeById) fills the
  // rest from any existing cached row. Defaults match upsertTaskSessionList /
  // syncEnvFromAgentctlPayload in agent-session.ts (state CREATED when unknown).
  const existing = qc.getQueryData<TaskSession | null>(qk.taskSession.byId(sessionIdBranded));
  const candidate: TaskSession = {
    id: sessionIdBranded,
    task_id: taskIdBranded,
    state: (patch.state ?? existing?.state ?? "CREATED") as TaskSessionState,
    started_at: existing?.started_at ?? "",
    updated_at: patch.updated_at ?? existing?.updated_at ?? "",
    ...patch,
  };

  const merged = writeById(qc, candidate);
  writeByTask(qc, taskIdBranded, merged);

  // D6 side-effect: keep the Zustand env-mapping index populated for getEnvKey.
  if (merged.task_environment_id) {
    deps.setEnvMapping(sessionIdBranded, merged.task_environment_id);
  }
  // Prepare-progress is fully TQ-backed (seed-if-absent so live executor.prepare.*
  // events aren't clobbered).
  writePrepareProgress(qc, merged);
}

// ---------------------------------------------------------------------------
// Handlers
// ---------------------------------------------------------------------------

function registerStateChangedHandler(
  ws: WebSocketClient,
  qc: QueryClient,
  deps: SessionStateBridgeDeps,
): () => void {
  return ws.on(
    "session.state_changed",
    wrapBridgeHandler(qc, "session.state_changed", (message) => {
      const payload = message.payload as LoosePayload;
      const rawTaskId = str(payload, "task_id");
      const rawSessionId = str(payload, "session_id");
      if (!rawTaskId || !rawSessionId) return;
      // Match the Zustand guard: ignore an unknown session learned about only
      // via a payload that carries no new_state (nothing to record).
      const existing = qc.getQueryData<TaskSession | null>(qk.taskSession.byId(rawSessionId));
      if (!existing && !str(payload, "new_state")) return;
      // Drop out-of-order subscribe snapshots whose updated_at predates the
      // record we already hold — writing them would stomp a fresher state
      // (e.g. revert a live WAITING_FOR_INPUT back to a stale STARTING and
      // block idle input). Mirrors the handler-side guard in agent-session.ts.
      if (isStaleSessionStateEvent(existing, str(payload, "updated_at"))) return;
      applySessionEvent(qc, deps, rawTaskId, rawSessionId, buildSessionPatch(payload));
    }),
  );
}

/** Map an agentctl WS action to the agentctl status it records. */
type AgentctlStatusValue = "starting" | "ready" | "error";

/**
 * Write the agentctl status badge into the TQ cache (qk.session.agentctl).
 * Mirrors the per-event setSessionAgentctlStatus calls the Zustand
 * agent-session handler used to make.
 */
function writeAgentctlStatusFromEvent(
  qc: QueryClient,
  sessionId: string,
  status: AgentctlStatusValue,
  payload: LoosePayload,
  timestamp: string | undefined,
): void {
  writeAgentctlStatus(qc, sessionId, {
    status,
    agentExecutionId: str(payload, "agent_execution_id"),
    errorMessage: status === "error" ? str(payload, "error_message") : undefined,
    updatedAt: timestamp,
  });
}

function registerAgentctlHandlers(
  ws: WebSocketClient,
  qc: QueryClient,
  deps: SessionStateBridgeDeps,
): Array<() => void> {
  const handle = (action: string, status: AgentctlStatusValue) =>
    wrapBridgeHandler(qc, action, (message) => {
      const payload = message.payload as LoosePayload;
      const rawSessionId = str(payload, "session_id");
      if (!rawSessionId) return;
      // agentctl status badge: now TQ-backed (qk.session.agentctl).
      const timestamp = (message as { timestamp?: string }).timestamp;
      writeAgentctlStatusFromEvent(qc, rawSessionId, status, payload, timestamp);
      // The live bus-published agentctl events carry task_id, but the
      // snapshot/replay path (`appendAgentctlStatusMessage` in the backend,
      // sent on subscribe/focus) omits it — it only carries session_id +
      // env/worktree fields. Fall back to the cached session's task_id so the
      // env-mapping / worktree merge still lands in the by-id + by-task caches
      // (matching the Zustand handler, which keys purely on session_id).
      const existing = qc.getQueryData<TaskSession | null>(qk.taskSession.byId(rawSessionId));
      const rawTaskId = str(payload, "task_id") ?? existing?.task_id;
      if (!rawTaskId) return;
      applySessionEvent(qc, deps, rawTaskId, rawSessionId, buildSessionPatch(payload));
    });

  return [
    ws.on("session.agentctl_starting", handle("session.agentctl_starting", "starting")),
    ws.on("session.agentctl_ready", handle("session.agentctl_ready", "ready")),
    ws.on("session.agentctl_error", handle("session.agentctl_error", "error")),
  ];
}

// ---------------------------------------------------------------------------
// Top-level registrar
// ---------------------------------------------------------------------------

/**
 * Registers the session-state WS → TQ bridge. Returns a cleanup function.
 *
 * `deps` carries the Zustand-backed D6 env-mapping side-effect callback so the
 * `environmentIdBySessionId` client index stays populated during the staged
 * migration.
 */
export function registerSessionStateBridge(
  ws: WebSocketClient,
  qc: QueryClient,
  deps: SessionStateBridgeDeps,
): () => void {
  const all = [
    registerStateChangedHandler(ws, qc, deps),
    ...registerAgentctlHandlers(ws, qc, deps),
  ];
  return () => {
    for (const fn of all) fn();
  };
}
