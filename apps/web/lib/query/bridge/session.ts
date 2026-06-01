/**
 * Session domain WS → TanStack Query bridge (Wave 5c).
 *
 * Mirrors the following WS handlers 1:1 (event coverage):
 *   lib/ws/handlers/messages.ts        → session.message.added / updated
 *   lib/ws/handlers/turns.ts           → session.turn.started / completed
 *   lib/ws/handlers/task-plans.ts      → task.plan.* events
 *   lib/ws/handlers/agent-session.ts   → session.state_changed / agentctl_* events
 *                                        (partial — UI-critical path stays in Zustand)
 *
 * Design decisions:
 *   - Messages: bridge writes into the `MessagesData` cache at
 *     `qk.session.messages(sessionId)`. The add handler deduplicates by id;
 *     the update handler merges non-undefined fields. This matches the
 *     Zustand `addMessage` / `updateMessage` semantics.
 *   - Turns: bridge patches `TurnsData` at `["session", id, "turns"]`.
 *   - Task plans: bridge patches `TaskPlanData` at `["session", "plans", taskId]`.
 *     Plan revisions are upserted into `TaskPlanRevisionsData` at
 *     `["session", "plans", taskId, "revisions"]` on `task.plan.revision.created`.
 *   - session.state_changed + agentctl_* events: these drive the session-adoption
 *     logic that lives deep in Zustand + the office refetch trigger. They are NOT
 *     migrated here — the Zustand handler in agent-session.ts remains the
 *     authority. The bridge therefore only handles the events listed above.
 *
 * Queue events (message.queue.status_changed) continue to be handled by the
 * Zustand agent-session handler because the queue UI is tightly coupled to
 * Zustand mutation paths. The bridge invalidates the TQ queue key on each
 * event as a safe fallback.
 *
 * Sub-registrar breakdown:
 *   registerMessageHandlers   — message.added / updated (~40 LOC)
 *   registerTurnHandlers      — turn.started / completed (~40 LOC)
 *   registerTaskPlanHandlers  — task.plan.* events (~50 LOC)
 *   registerSessionBridge     — top-level aggregator
 */

import type { QueryClient } from "@tanstack/react-query";
import type { WebSocketClient } from "@/lib/ws/client";
import { qk } from "@/lib/query/keys";
import {
  sessionId as toSessionId,
  taskId as toTaskId,
  type MessageType,
  type MessageAuthorType,
} from "@/lib/types/http";
import type { Message, Turn, TaskPlan, TaskPlanRevision } from "@/lib/types/http";
import type {
  MessagesData,
  TurnsData,
  TaskPlanData,
  TaskPlanRevisionsData,
} from "@/lib/query/query-options/session";
import { sortRevisionsDesc } from "@/lib/query/query-options/session";
import { emitEmptyTurnNoticeIntoCache } from "@/lib/ws/handlers/empty-turn-notice";
import type { TurnEventPayload } from "@/lib/types/backend";
import { wrapBridgeHandler } from "./index";

/** Deps the session bridge reads from client-only Zustand UI state. */
export interface SessionBridgeDeps {
  /** True when the session is a quick-chat / config-chat surface. */
  isEphemeralSurface: (sessionId: string) => boolean;
}

// ---------------------------------------------------------------------------
// Message helpers
// ---------------------------------------------------------------------------

/** Merge non-undefined fields from source into a message (mirrors Zustand mergeMessageFields). */
function mergeMessage(existing: Message, incoming: Partial<Message>): Message {
  const merged = { ...existing };
  for (const key of Object.keys(incoming) as (keyof Message)[]) {
    if (incoming[key] !== undefined) {
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      (merged as any)[key] = incoming[key];
    }
  }
  return merged;
}

function buildMessageFromPayload(payload: Record<string, unknown>): Message {
  return {
    id: payload.message_id as string,
    session_id: toSessionId(payload.session_id as string),
    task_id: toTaskId(payload.task_id as string),
    turn_id: payload.turn_id as string | undefined,
    author_type: (payload.author_type as MessageAuthorType) ?? "agent",
    author_id: payload.author_id as string | undefined,
    content: payload.content as string,
    raw_content: payload.raw_content as string | undefined,
    type: (payload.type as MessageType) || "message",
    metadata: payload.metadata as Record<string, unknown> | undefined,
    requests_input: payload.requests_input as boolean | undefined,
    created_at: payload.created_at as string,
  };
}

// ---------------------------------------------------------------------------
// Message handlers sub-registrar
// ---------------------------------------------------------------------------

function registerMessageHandlers(ws: WebSocketClient, qc: QueryClient): Array<() => void> {
  const unsubAdded = ws.on(
    "session.message.added",
    wrapBridgeHandler(qc, "session.message.added", (message) => {
      const payload = message.payload;
      if (!payload.session_id) return;
      const sid = payload.session_id as string;
      const incoming = buildMessageFromPayload(payload as Record<string, unknown>);

      qc.setQueryData<MessagesData>(qk.session.messages(sid), (prev) => {
        if (!prev) {
          // Cache miss — seed with the new message; hook will backfill on mount
          return { messages: [incoming], hasMore: false, oldestCursor: incoming.id };
        }
        const idx = prev.messages.findIndex((m) => m.id === incoming.id);
        if (idx >= 0) {
          // Duplicate — merge fields
          const updated = [...prev.messages];
          updated[idx] = mergeMessage(updated[idx], incoming);
          return { ...prev, messages: updated };
        }
        return { ...prev, messages: [...prev.messages, incoming] };
      });
    }),
  );

  const unsubUpdated = ws.on(
    "session.message.updated",
    wrapBridgeHandler(qc, "session.message.updated", (message) => {
      const payload = message.payload;
      if (!payload.session_id) return;
      const sid = payload.session_id as string;
      const incoming = buildMessageFromPayload(payload as Record<string, unknown>);

      qc.setQueryData<MessagesData>(qk.session.messages(sid), (prev) => {
        if (!prev) return prev;
        const idx = prev.messages.findIndex((m) => m.id === incoming.id);
        if (idx < 0) return prev;
        const updated = [...prev.messages];
        updated[idx] = mergeMessage(updated[idx], incoming);
        return { ...prev, messages: updated };
      });
    }),
  );

  return [unsubAdded, unsubUpdated];
}

// ---------------------------------------------------------------------------
// Turn helpers + handlers sub-registrar
// ---------------------------------------------------------------------------

function buildTurnFromPayload(payload: Record<string, unknown>): Turn {
  return {
    id: payload.id as string,
    session_id: toSessionId(payload.session_id as string),
    task_id: toTaskId(payload.task_id as string),
    started_at: payload.started_at as string,
    completed_at: payload.completed_at as string | undefined,
    metadata: payload.metadata as Record<string, unknown> | undefined,
    created_at: payload.created_at as string,
    updated_at: payload.updated_at as string,
  };
}

function registerTurnHandlers(
  ws: WebSocketClient,
  qc: QueryClient,
  deps: SessionBridgeDeps,
): Array<() => void> {
  const unsubStarted = ws.on(
    "session.turn.started",
    wrapBridgeHandler(qc, "session.turn.started", (message) => {
      const payload = message.payload;
      if (!payload.session_id) return;
      const sid = payload.session_id as string;
      const turn = buildTurnFromPayload(payload as Record<string, unknown>);

      qc.setQueryData<TurnsData>(qk.session.turns(sid), (prev) => {
        const turns = prev?.turns ?? [];
        if (turns.some((t) => t.id === turn.id)) return prev ?? { turns, activeTurnId: turn.id };
        return { turns: [...turns, turn], activeTurnId: turn.id };
      });
    }),
  );

  const unsubCompleted = ws.on(
    "session.turn.completed",
    wrapBridgeHandler(qc, "session.turn.completed", (message) => {
      const payload = message.payload;
      if (!payload.session_id || !payload.id) return;
      const sid = payload.session_id as string;
      const turnId = payload.id as string;
      const completedAt = (payload.completed_at as string | undefined) ?? new Date().toISOString();

      const turnsPrev = qc.getQueryData<TurnsData>(qk.session.turns(sid));
      if (turnsPrev) {
        qc.setQueryData<TurnsData>(qk.session.turns(sid), {
          turns: turnsPrev.turns.map((t) =>
            t.id === turnId ? { ...t, completed_at: completedAt } : t,
          ),
          activeTurnId: null,
        });
      } else {
        // Cache not yet hydrated for this session — invalidate so any future
        // consumer fetches fresh state including the completed turn.
        qc.invalidateQueries({ queryKey: qk.session.turns(sid) });
      }

      // Also sweep any in-progress tool-call messages to "complete" status
      const messagesPrev = qc.getQueryData<MessagesData>(qk.session.messages(sid));
      if (messagesPrev) {
        qc.setQueryData<MessagesData>(qk.session.messages(sid), {
          ...messagesPrev,
          messages: messagesPrev.messages.map((msg) => {
            if (msg.type === "permission_request") return msg;
            const meta = msg.metadata as Record<string, unknown> | undefined;
            if (meta?.tool_call_id && meta.status !== "complete" && meta.status !== "error") {
              return { ...msg, metadata: { ...meta, status: "complete" } };
            }
            return msg;
          }),
        });
      }

      // Surface a notice when the turn finished with no agent output. The chat
      // UI reads from this messages cache, so the synthetic notice must land
      // here (the Zustand mirror is no longer the rendered source).
      emitEmptyTurnNoticeIntoCache(
        qc,
        payload as unknown as TurnEventPayload,
        deps.isEphemeralSurface(sid),
      );
    }),
  );

  return [unsubStarted, unsubCompleted];
}

// ---------------------------------------------------------------------------
// Task plan handlers sub-registrar
// ---------------------------------------------------------------------------

function buildRevisionFromPayload(p: Record<string, unknown>): TaskPlanRevision {
  return {
    id: p.id as string,
    task_id: p.task_id as string,
    revision_number: p.revision_number as number,
    title: p.title as string,
    author_kind: p.author_kind as TaskPlanRevision["author_kind"],
    author_name: (p.author_name as string | undefined) ?? "",
    revert_of_revision_id: (p.revert_of_revision_id as string | undefined) ?? null,
    created_at: p.created_at as string,
    updated_at: p.updated_at as string,
  };
}

/**
 * Upsert a revision into the revisions cache, mirroring the Zustand
 * `upsertPlanRevision` semantics: prepend when new, merge fields when the id
 * already exists, then re-sort newest-first.
 */
function upsertRevision(qc: QueryClient, taskId: string, rev: TaskPlanRevision): void {
  qc.setQueryData<TaskPlanRevisionsData>(qk.taskSession.plansRevisions(taskId), (prev) => {
    const list = prev?.revisions ?? [];
    const idx = list.findIndex((r) => r.id === rev.id);
    const next =
      idx === -1 ? [rev, ...list] : list.map((r) => (r.id === rev.id ? { ...r, ...rev } : r));
    return { revisions: sortRevisionsDesc(next) };
  });
}

function registerTaskPlanHandlers(ws: WebSocketClient, qc: QueryClient): Array<() => void> {
  function upsertPlan(taskId: string, plan: TaskPlan, markedSeen = false) {
    qc.setQueryData<TaskPlanData>(qk.taskSession.plans(taskId), (prev) => ({
      plan,
      lastSeenUpdatedAt: markedSeen ? plan.updated_at : (prev?.lastSeenUpdatedAt ?? null),
    }));
  }

  const unsubCreated = ws.on(
    "task.plan.created",
    wrapBridgeHandler(qc, "task.plan.created", (message) => {
      const p = message.payload;
      upsertPlan(p.task_id as string, {
        id: p.id as string,
        task_id: p.task_id as string,
        title: p.title as string,
        content: p.content as string,
        created_by: (p.created_by as "agent" | "user") ?? "agent",
        created_at: p.created_at as string,
        updated_at: p.updated_at as string,
      });
    }),
  );

  const unsubUpdated = ws.on(
    "task.plan.updated",
    wrapBridgeHandler(qc, "task.plan.updated", (message) => {
      const p = message.payload;
      const plan: TaskPlan = {
        id: p.id as string,
        task_id: p.task_id as string,
        title: p.title as string,
        content: p.content as string,
        created_by: (p.created_by as "agent" | "user") ?? "agent",
        created_at: p.created_at as string,
        updated_at: p.updated_at as string,
      };
      // User-authored writes mark as seen only when content changed
      const prev = qc.getQueryData<TaskPlanData>(qk.taskSession.plans(p.task_id as string));
      const contentChanged = prev?.plan?.content !== plan.content;
      const markedSeen = plan.created_by === "user" && contentChanged;
      upsertPlan(p.task_id as string, plan, markedSeen);
    }),
  );

  const unsubDeleted = ws.on(
    "task.plan.deleted",
    wrapBridgeHandler(qc, "task.plan.deleted", (message) => {
      const taskId = message.payload.task_id as string;
      qc.setQueryData<TaskPlanData>(qk.taskSession.plans(taskId), (prev) => ({
        plan: null,
        lastSeenUpdatedAt: prev?.lastSeenUpdatedAt ?? null,
      }));
    }),
  );

  const unsubRevisionCreated = ws.on(
    "task.plan.revision.created",
    wrapBridgeHandler(qc, "task.plan.revision.created", (message) => {
      const p = message.payload as Record<string, unknown>;
      // Mirror the Zustand upsertPlanRevision: write the new revision directly
      // into the revisions cache so a consumer reading from TQ stays live
      // without a refetch round-trip.
      upsertRevision(qc, p.task_id as string, buildRevisionFromPayload(p));
    }),
  );

  // task.plan.reverted: notification only — revision list re-fetches via the above
  const unsubReverted = ws.on(
    "task.plan.reverted",
    wrapBridgeHandler(qc, "task.plan.reverted", () => {}),
  );

  return [unsubCreated, unsubUpdated, unsubDeleted, unsubRevisionCreated, unsubReverted];
}

// ---------------------------------------------------------------------------
// Queue invalidation handler
// ---------------------------------------------------------------------------

function registerQueueHandler(ws: WebSocketClient, qc: QueryClient): () => void {
  return ws.on(
    "message.queue.status_changed",
    wrapBridgeHandler(qc, "message.queue.status_changed", (message) => {
      const sid = message.payload.session_id as string | undefined;
      if (!sid) return;
      // Invalidate the TQ queue key so the next access re-fetches
      void qc.invalidateQueries({ queryKey: qk.session.queue(sid) });
    }),
  );
}

// ---------------------------------------------------------------------------
// Top-level bridge registrar
// ---------------------------------------------------------------------------

/**
 * Registers WS handlers for session message, turn, task-plan, and queue events.
 * Returns a cleanup function.
 */
export function registerSessionBridge(
  ws: WebSocketClient,
  qc: QueryClient,
  deps: SessionBridgeDeps,
): () => void {
  const all = [
    ...registerMessageHandlers(ws, qc),
    ...registerTurnHandlers(ws, qc, deps),
    ...registerTaskPlanHandlers(ws, qc),
    registerQueueHandler(ws, qc),
  ];
  return () => {
    for (const fn of all) fn();
  };
}
