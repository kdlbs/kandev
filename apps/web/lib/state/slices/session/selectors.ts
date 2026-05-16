import type { AppState } from "@/lib/state/store";
import type { TaskSession } from "@/lib/types/http";

/**
 * Whether a session is "live" (an agent is actively working). Office and
 * kanban sessions diverge here:
 *
 * - **Office sessions** (`agent_profile_id` is set) cycle RUNNING ↔ IDLE.
 *   IDLE means the agent process + executor backend are torn down and the
 *   conversation is paused; only RUNNING counts as live. Office sessions
 *   never enter `WAITING_FOR_INPUT`.
 * - **Kanban / quick-chat sessions** (`agent_profile_id` is unset) keep
 *   the per-launch warm model: between turns they sit in
 *   `WAITING_FOR_INPUT` with the executor still warm, which still counts
 *   as live so the topbar/sidebar indicators stay up.
 */
function isLiveSession(session: TaskSession): boolean {
  if (session.state === "RUNNING") return true;
  // Kanban sessions only — office sessions skip WAITING_FOR_INPUT entirely.
  if (!session.agent_profile_id && session.state === "WAITING_FOR_INPUT") {
    return true;
  }
  return false;
}

/**
 * Returns the count of sessions for the given agent that are in an
 * "actively working" state. See `isLiveSession` for the office vs kanban
 * gating.
 *
 * Driven exclusively by the existing `taskSessions` store, which is kept
 * fresh by `session.state_changed` WS events. No polling, no extra fetches.
 */
export function selectActiveSessionsForAgent(state: AppState, agentProfileId: string): number {
  if (!agentProfileId) return 0;
  let count = 0;
  for (const session of Object.values(state.taskSessions.items)) {
    if (session.agent_profile_id !== agentProfileId) continue;
    if (isLiveSession(session)) count++;
  }
  return count;
}

/**
 * Returns the most recent session for the given task that is in a
 * "live" state. "Most recent" is determined by `started_at` (lexicographic
 * ISO compare). Returns null if none.
 *
 * Reads from `taskSessions.items` (kept fresh by `session.state_changed`
 * WS events). No fetches.
 */
export function selectLiveSessionForTask(state: AppState, taskId: string): TaskSession | null {
  if (!taskId) return null;
  let best: TaskSession | null = null;
  for (const session of Object.values(state.taskSessions.items)) {
    if (session.task_id !== taskId) continue;
    if (!isLiveSession(session)) continue;
    if (!best || (session.started_at ?? "") > (best.started_at ?? "")) {
      best = session;
    }
  }
  return best;
}

/**
 * Returns all sessions for the given task, sorted by `started_at` ascending.
 * Empty array if none.
 */
export function selectAllSessionsForTask(state: AppState, taskId: string): TaskSession[] {
  if (!taskId) return [];
  const list: TaskSession[] = [];
  for (const session of Object.values(state.taskSessions.items)) {
    if (session.task_id === taskId) list.push(session);
  }
  list.sort((a, b) => (a.started_at ?? "").localeCompare(b.started_at ?? ""));
  return list;
}

/**
 * Returns sessions for the given task grouped by agent instance, suitable
 * for rendering one timeline entry per (task, agent). Sessions without
 * an `agent_profile_id` (kanban / quick-chat) are grouped under the empty
 * string key — callers can decide whether to show them.
 *
 * Within each bucket, sessions are sorted by `started_at` ascending. The
 * Map insertion order is by most-recent activity (latest `updated_at`)
 * descending so callers can iterate without re-sorting.
 */
export function selectSessionsByAgentForTask(
  state: AppState,
  taskId: string,
): Map<string, TaskSession[]> {
  if (!taskId) return new Map();
  const buckets = new Map<string, TaskSession[]>();
  for (const session of Object.values(state.taskSessions.items)) {
    if (session.task_id !== taskId) continue;
    const key = session.agent_profile_id ?? "";
    const list = buckets.get(key);
    if (list) list.push(session);
    else buckets.set(key, [session]);
  }
  for (const list of buckets.values()) {
    list.sort((a, b) => (a.started_at ?? "").localeCompare(b.started_at ?? ""));
  }
  // Reorder buckets by most-recent activity (latest updated_at desc).
  const ordered = new Map<string, TaskSession[]>();
  const entries = Array.from(buckets.entries());
  entries.sort(([, aList], [, bList]) => {
    const aLatest = aList[aList.length - 1]?.updated_at ?? "";
    const bLatest = bList[bList.length - 1]?.updated_at ?? "";
    return bLatest.localeCompare(aLatest);
  });
  for (const [k, v] of entries) ordered.set(k, v);
  return ordered;
}

/**
 * Workspace-wide total of sessions in a "live" state. Drives the sidebar
 * Dashboard live badge. Office vs kanban gating lives in `isLiveSession`.
 */
export function selectTotalLiveSessions(state: AppState): number {
  let count = 0;
  for (const session of Object.values(state.taskSessions.items)) {
    if (isLiveSession(session)) count++;
  }
  return count;
}

/**
 * Returns the count of `tool_call` messages for the given session.
 * Drives the "ran N commands" segment in inline session timeline entries.
 */
export function selectCommandCount(state: AppState, sessionId: string): number {
  if (!sessionId) return 0;
  const messages = state.messages?.bySession?.[sessionId];
  if (!messages || messages.length === 0) return 0;
  let count = 0;
  for (const msg of messages) {
    if (msg.type === "tool_call") count++;
  }
  return count;
}
