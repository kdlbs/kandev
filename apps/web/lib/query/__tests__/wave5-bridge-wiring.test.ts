/**
 * Wave-5 bridge wiring smoke test.
 *
 * Asserts that `registerQueryBridge` calls all three wave-5 session
 * registrars end-to-end: a WS event dispatched through the top-level
 * bridge must write the expected TQ cache key.
 *
 * One representative event per bridge:
 *   - registerSessionBridge        → session.message.added  → qk.session.messages(sid)
 *   - registerSessionRuntimeBridge → session.todos_updated  → qk.session.todos(sid)
 *   - registerSessionRuntimeStreamsBridge → session.shell.output → ring (no TQ key)
 *
 * The streams bridge writes into ring buffers (not TQ), so we verify it
 * registers handlers without error and returns a working cleanup.
 */

import { describe, it, expect, vi } from "vitest";
import { QueryClient } from "@tanstack/react-query";
import { registerQueryBridge } from "@/lib/query/bridge/index";
import { qk } from "@/lib/query/keys";
import type { MessagesData } from "@/lib/query/query-options/session";
import type { TodoEntry } from "@/lib/state/slices/session-runtime/types";

// ---------------------------------------------------------------------------
// Minimal fake WebSocketClient
// ---------------------------------------------------------------------------

type Handler<T> = (msg: T) => void;

function makeFakeWs() {
  const listeners = new Map<string, Set<Handler<unknown>>>();

  return {
    on: vi.fn(<T>(type: string, handler: Handler<T>) => {
      const set = listeners.get(type) ?? new Set<Handler<unknown>>();
      set.add(handler as Handler<unknown>);
      listeners.set(type, set);
      return () => {
        listeners.get(type)?.delete(handler as Handler<unknown>);
      };
    }),
    emit(type: string, message: unknown) {
      listeners.get(type)?.forEach((h) => h(message));
    },
    listenerCount(type: string) {
      return listeners.get(type)?.size ?? 0;
    },
  };
}

function makeClient() {
  return new QueryClient({ defaultOptions: { queries: { retry: false, gcTime: 0 } } });
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe("registerQueryBridge wave-5 wiring", () => {
  const SESSION_ID = "sess-abc";

  it("session.message.added (registerSessionBridge) writes into messages cache", () => {
    const ws = makeFakeWs();
    const qc = makeClient();

    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    registerQueryBridge(ws as any, qc, {
      getActiveWorkspaceId: () => undefined,
      getEnvKey: (sid) => sid,
    });

    ws.emit("session.message.added", {
      payload: {
        message_id: "msg-1",
        session_id: SESSION_ID,
        task_id: "task-1",
        author_type: "agent",
        content: "hello",
        type: "message",
        created_at: "2026-01-01T00:00:00Z",
      },
    });

    const data = qc.getQueryData<MessagesData>(qk.session.messages(SESSION_ID));
    expect(data).toBeDefined();
    expect(data?.messages).toHaveLength(1);
    expect(data?.messages[0].id).toBe("msg-1");
  });

  it("session.todos_updated (registerSessionRuntimeBridge) writes into todos cache", () => {
    const ws = makeFakeWs();
    const qc = makeClient();

    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    registerQueryBridge(ws as any, qc, {
      getActiveWorkspaceId: () => undefined,
      getEnvKey: (sid) => sid,
    });

    ws.emit("session.todos_updated", {
      payload: {
        session_id: SESSION_ID,
        entries: [{ description: "Write tests", status: "in_progress", priority: "high" }],
      },
    });

    const todos = qc.getQueryData<TodoEntry[]>(qk.session.todos(SESSION_ID));
    expect(todos).toBeDefined();
    expect(todos).toHaveLength(1);
    expect(todos?.[0].description).toBe("Write tests");
  });

  it("session.shell.output (registerSessionRuntimeStreamsBridge) is registered and cleanup works", () => {
    const ws = makeFakeWs();
    const qc = makeClient();

    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    const cleanup = registerQueryBridge(ws as any, qc, {
      getActiveWorkspaceId: () => undefined,
      getEnvKey: (sid) => sid,
    });

    // Handler for shell output must have been registered
    expect(ws.listenerCount("session.shell.output")).toBeGreaterThan(0);

    // After cleanup there must be no remaining handler
    cleanup();
    expect(ws.listenerCount("session.shell.output")).toBe(0);
  });
});
