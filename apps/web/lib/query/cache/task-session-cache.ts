/**
 * Imperative TaskSession TQ cache writers (D4 Stage 1).
 *
 * Non-render write paths (server-action responses, agentctl REST acks, session
 * resumption) used to call the Zustand setters `setTaskSession` /
 * `setTaskSessionsForTask` / `upsertTaskSessionFromEvent`. These helpers write
 * the same data into the TQ cache (`qk.taskSession.byId` + `qk.taskSession.byTask`)
 * with identical `mergeTaskSession` semantics, so the by-id observe surface and
 * the per-task list stay in sync without the Zustand mirror.
 *
 * The live WS path goes through `lib/query/bridge/session-state.ts`; this module
 * is the imperative twin for code that already has a full `TaskSession` in hand.
 */

import type { QueryClient } from "@tanstack/react-query";
import { qk } from "@/lib/query/keys";
import { mergeTaskSession } from "@/lib/state/slices/session/session-slice";
import type { TaskSession, TaskSessionsResponse } from "@/lib/types/http";

/**
 * Merge a single TaskSession into the by-id cache and return the merged record.
 * Mirrors `writeById` in the session-state bridge.
 */
function writeById(qc: QueryClient, session: TaskSession): TaskSession {
  let merged = session;
  qc.setQueryData<TaskSession | null>(qk.taskSession.byId(session.id), (prev) => {
    merged = prev ? mergeTaskSession(prev, session) : session;
    return merged;
  });
  return merged;
}

/**
 * Upsert a merged TaskSession into the per-task list cache. Seeds a single-entry
 * list if none is cached yet, matching the bridge's `writeByTask`.
 */
function writeByTask(qc: QueryClient, taskId: string, merged: TaskSession): void {
  qc.setQueryData<TaskSessionsResponse>(qk.taskSession.byTask(taskId), (prev) => {
    if (!prev) return { sessions: [merged], total: 1 };
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
 * Upsert one TaskSession into both the by-id and by-task TQ caches with
 * `mergeTaskSession` semantics. Replaces the Zustand `setTaskSession` /
 * `upsertTaskSessionFromEvent` setters for imperative call sites.
 */
export function mergeTaskSessionIntoCache(qc: QueryClient, session: TaskSession): TaskSession {
  const merged = writeById(qc, session);
  writeByTask(qc, merged.task_id, merged);
  return merged;
}

/**
 * Remove a session from both the by-id and by-task TQ caches. Mirrors the
 * Zustand `removeTaskSession` setter for imperative delete paths.
 */
export function removeTaskSessionFromCache(
  qc: QueryClient,
  taskId: string,
  sessionId: string,
): void {
  qc.removeQueries({ queryKey: qk.taskSession.byId(sessionId), exact: true });
  qc.setQueryData<TaskSessionsResponse>(qk.taskSession.byTask(taskId), (prev) => {
    if (!prev) return prev;
    const sessions = prev.sessions.filter((s) => s.id !== sessionId);
    if (sessions.length === prev.sessions.length) return prev;
    return { sessions, total: sessions.length };
  });
}

/**
 * Replace the per-task session list cache, seeding each session into its by-id
 * slot (merging with any existing record). Replaces the Zustand
 * `setTaskSessionsForTask` setter.
 */
export function setTaskSessionsForTaskInCache(
  qc: QueryClient,
  taskId: string,
  sessions: TaskSession[],
): void {
  const merged = sessions.map((session) => writeById(qc, session));
  qc.setQueryData<TaskSessionsResponse>(qk.taskSession.byTask(taskId), {
    sessions: merged,
    total: merged.length,
  });
}
