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
import type { Message, Turn, TaskPlan } from "@/lib/types/http";
import type { MessagesData, TurnsData, TaskPlanData } from "@/lib/query/query-options/session";

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
  const unsubAdded = ws.on("session.message.added", (message) => {
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
  });

  const unsubUpdated = ws.on("session.message.updated", (message) => {
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
  });

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

function registerTurnHandlers(ws: WebSocketClient, qc: QueryClient): Array<() => void> {
  const unsubStarted = ws.on("session.turn.started", (message) => {
    const payload = message.payload;
    if (!payload.session_id) return;
    const sid = payload.session_id as string;
    const turn = buildTurnFromPayload(payload as Record<string, unknown>);

    qc.setQueryData<TurnsData>(qk.session.turns(sid), (prev) => {
      const turns = prev?.turns ?? [];
      if (turns.some((t) => t.id === turn.id)) return prev ?? { turns, activeTurnId: turn.id };
      return { turns: [...turns, turn], activeTurnId: turn.id };
    });
  });

  const unsubCompleted = ws.on("session.turn.completed", (message) => {
    const payload = message.payload;
    if (!payload.session_id || !payload.id) return;
    const sid = payload.session_id as string;
    const turnId = payload.id as string;
    const completedAt = (payload.completed_at as string | undefined) ?? new Date().toISOString();

    qc.setQueryData<TurnsData>(qk.session.turns(sid), (prev) => {
      if (!prev) return prev;
      const turns = prev.turns.map((t) =>
        t.id === turnId ? { ...t, completed_at: completedAt } : t,
      );
      // Mark tool calls without terminal status as complete (safety net)
      return { turns, activeTurnId: null };
    });

    // Also sweep any in-progress tool-call messages to "complete" status
    qc.setQueryData<MessagesData>(qk.session.messages(sid), (prev) => {
      if (!prev) return prev;
      const updated = prev.messages.map((msg) => {
        if (msg.type === "permission_request") return msg;
        const meta = msg.metadata as Record<string, unknown> | undefined;
        if (meta?.tool_call_id && meta.status !== "complete" && meta.status !== "error") {
          return { ...msg, metadata: { ...meta, status: "complete" } };
        }
        return msg;
      });
      return { ...prev, messages: updated };
    });
  });

  return [unsubStarted, unsubCompleted];
}

// ---------------------------------------------------------------------------
// Task plan handlers sub-registrar
// ---------------------------------------------------------------------------

function registerTaskPlanHandlers(ws: WebSocketClient, qc: QueryClient): Array<() => void> {
  const invalidatePlan = (taskId: string) =>
    void qc.invalidateQueries({ queryKey: qk.taskSession.plans(taskId) });

  function upsertPlan(taskId: string, plan: TaskPlan, markedSeen = false) {
    qc.setQueryData<TaskPlanData>(qk.taskSession.plans(taskId), (prev) => ({
      plan,
      lastSeenUpdatedAt: markedSeen ? plan.updated_at : (prev?.lastSeenUpdatedAt ?? null),
    }));
  }

  const unsubCreated = ws.on("task.plan.created", (message) => {
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
  });

  const unsubUpdated = ws.on("task.plan.updated", (message) => {
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
  });

  const unsubDeleted = ws.on("task.plan.deleted", (message) => {
    const taskId = message.payload.task_id as string;
    qc.setQueryData<TaskPlanData>(qk.taskSession.plans(taskId), (prev) => ({
      plan: null,
      lastSeenUpdatedAt: prev?.lastSeenUpdatedAt ?? null,
    }));
  });

  const unsubRevisionCreated = ws.on("task.plan.revision.created", (message) => {
    const p = message.payload;
    // Revisions are managed in a separate query key; invalidate to trigger refetch
    void qc.invalidateQueries({
      queryKey: qk.taskSession.plansRevisions(p.task_id as string),
    });
    invalidatePlan(p.task_id as string);
  });

  // task.plan.reverted: notification only — revision list re-fetches via the above
  const unsubReverted = ws.on("task.plan.reverted", () => {});

  return [unsubCreated, unsubUpdated, unsubDeleted, unsubRevisionCreated, unsubReverted];
}

// ---------------------------------------------------------------------------
// Queue invalidation handler
// ---------------------------------------------------------------------------

function registerQueueHandler(ws: WebSocketClient, qc: QueryClient): () => void {
  return ws.on("message.queue.status_changed", (message) => {
    const sid = message.payload.session_id as string | undefined;
    if (!sid) return;
    // Invalidate the TQ queue key so the next access re-fetches
    void qc.invalidateQueries({ queryKey: qk.session.queue(sid) });
  });
}

// ---------------------------------------------------------------------------
// Top-level bridge registrar
// ---------------------------------------------------------------------------

/**
 * Registers WS handlers for session message, turn, task-plan, and queue events.
 * Returns a cleanup function.
 */
export function registerSessionBridge(ws: WebSocketClient, qc: QueryClient): () => void {
  const all = [
    ...registerMessageHandlers(ws, qc),
    ...registerTurnHandlers(ws, qc),
    ...registerTaskPlanHandlers(ws, qc),
    registerQueueHandler(ws, qc),
  ];
  return () => {
    for (const fn of all) fn();
  };
}
