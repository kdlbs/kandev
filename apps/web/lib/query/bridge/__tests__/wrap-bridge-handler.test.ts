import { describe, it, expect, beforeEach, afterEach } from "vitest";
import { QueryClient } from "@tanstack/react-query";

function createTestQueryClient(): QueryClient {
  return new QueryClient({
    defaultOptions: {
      queries: { retry: false, gcTime: 0 },
      mutations: { retry: false },
    },
  });
}
import {
  wrapBridgeHandler,
  isBridgeSkippedAction,
  BRIDGE_SKIPPED_ACTIONS,
  BRIDGE_SKIPPED_PREFIXES,
  __setBridgeAuditEnabledForTests,
  __getBridgeAuditBufferForTests,
  __clearBridgeAuditBufferForTests,
} from "../index";

const ACTION = "session.message.added";

describe("isBridgeSkippedAction", () => {
  it("returns true for documented Zustand-only actions", () => {
    expect(isBridgeSkippedAction("message.queue.status_changed")).toBe(true);
    expect(BRIDGE_SKIPPED_ACTIONS.has("message.queue.status_changed")).toBe(true);
  });

  it("returns true for in-container agentctl_* log/status channels", () => {
    expect(isBridgeSkippedAction("agentctl_status")).toBe(true);
    expect(isBridgeSkippedAction("agentctl_anything")).toBe(true);
    expect(BRIDGE_SKIPPED_PREFIXES).toContain("agentctl_");
  });

  it("returns false for migrated bridge actions", () => {
    expect(isBridgeSkippedAction("session.message.added")).toBe(false);
    expect(isBridgeSkippedAction("task.created")).toBe(false);
    expect(isBridgeSkippedAction("workspace.updated")).toBe(false);
    // Now bridged by bridge/session-state.ts (D4+D6 Stage 1):
    expect(isBridgeSkippedAction("session.state_changed")).toBe(false);
    expect(isBridgeSkippedAction("session.agentctl_ready")).toBe(false);
  });
});

describe("wrapBridgeHandler (gate off)", () => {
  beforeEach(() => {
    __setBridgeAuditEnabledForTests(false);
    __clearBridgeAuditBufferForTests();
  });

  it("returns the original handler unchanged", () => {
    const qc = createTestQueryClient();
    const original = () => undefined;
    const wrapped = wrapBridgeHandler(qc, ACTION, original);
    expect(wrapped).toBe(original);
  });

  it("does not install window accessors", () => {
    expect(window.__kandev_bridge_audit__).toBeUndefined();
    expect(window.__kandev_bridge_audit_clear__).toBeUndefined();
  });
});

function enableGate() {
  __setBridgeAuditEnabledForTests(true);
  __clearBridgeAuditBufferForTests();
}

function disableGate() {
  __setBridgeAuditEnabledForTests(false);
  __clearBridgeAuditBufferForTests();
}

describe("wrapBridgeHandler (gate on) — handler dispatch", () => {
  beforeEach(enableGate);
  afterEach(disableGate);

  it("invokes the underlying handler with the message", () => {
    const qc = createTestQueryClient();
    const received: unknown[] = [];
    const handler = (msg: { payload?: unknown }) => {
      received.push(msg);
    };
    const wrapped = wrapBridgeHandler(qc, ACTION, handler);
    const message = { payload: { session_id: "s1" } };
    wrapped(message);
    expect(received).toEqual([message]);
  });

  it("records cacheChanged=true when the handler mutates the cache", () => {
    const qc = createTestQueryClient();
    const wrapped = wrapBridgeHandler(qc, ACTION, () => {
      qc.setQueryData(["test", "key"], { value: 1 });
    });

    wrapped({ payload: { session_id: "s1", task_id: "t1" } });

    const buffer = __getBridgeAuditBufferForTests();
    expect(buffer).toHaveLength(1);
    expect(buffer[0]).toMatchObject({
      action: ACTION,
      sessionId: "s1",
      taskId: "t1",
      cacheChanged: true,
    });
    expect(buffer[0].mutationCount).toBeGreaterThan(0);
    expect(typeof buffer[0].timestamp).toBe("number");
  });

  it("records cacheChanged=true for an invalidation-only handler whose key was never fetched", () => {
    // Regression: invalidation-based handlers (office/dashboard events)
    // invalidate a key that has no cached entry yet. A getQueryCache()
    // subscribe-counter sees no notification and would report a false
    // cache-unchanged drop; spying the method call captures the intent.
    const qc = createTestQueryClient();
    const wrapped = wrapBridgeHandler(qc, ACTION, () => {
      qc.invalidateQueries({ queryKey: ["never", "fetched"] });
    });

    wrapped({ payload: { session_id: "s1" } });

    const buffer = __getBridgeAuditBufferForTests();
    expect(buffer).toHaveLength(1);
    expect(buffer[0]).toMatchObject({ action: ACTION, cacheChanged: true });
    expect(buffer[0].mutationCount).toBeGreaterThan(0);
  });

  it("records cacheChanged=false when the handler is a no-op", () => {
    const qc = createTestQueryClient();
    const wrapped = wrapBridgeHandler(qc, ACTION, () => {
      // no cache writes
    });

    wrapped({ payload: { session_id: "s1" } });

    const buffer = __getBridgeAuditBufferForTests();
    expect(buffer).toHaveLength(1);
    expect(buffer[0]).toMatchObject({
      action: ACTION,
      sessionId: "s1",
      taskId: null,
      cacheChanged: false,
      mutationCount: 0,
    });
  });

  it("extracts session_id and task_id only when they are strings", () => {
    const qc = createTestQueryClient();
    const wrapped = wrapBridgeHandler(qc, ACTION, () => undefined);

    wrapped({ payload: { session_id: 42, task_id: undefined } });
    wrapped({ payload: undefined });
    wrapped({ payload: { task_id: "only-task" } });

    const buffer = __getBridgeAuditBufferForTests();
    expect(buffer[0]).toMatchObject({ sessionId: null, taskId: null });
    expect(buffer[1]).toMatchObject({ sessionId: null, taskId: null });
    expect(buffer[2]).toMatchObject({ sessionId: null, taskId: "only-task" });
  });

  it("still records when the handler throws", () => {
    const qc = createTestQueryClient();
    const wrapped = wrapBridgeHandler(qc, ACTION, () => {
      throw new Error("boom");
    });

    expect(() => wrapped({ payload: { session_id: "s1" } })).toThrow("boom");
    const buffer = __getBridgeAuditBufferForTests();
    expect(buffer).toHaveLength(1);
    expect(buffer[0].action).toBe(ACTION);
  });
});

describe("wrapBridgeHandler (gate on) — buffer and window accessors", () => {
  beforeEach(enableGate);
  afterEach(disableGate);

  it("caps the ring buffer at 5000 entries (FIFO eviction)", () => {
    const qc = createTestQueryClient();
    const wrapped = wrapBridgeHandler(qc, ACTION, () => undefined);

    for (let i = 0; i < 5050; i++) {
      wrapped({ payload: { session_id: `s${i}` } });
    }

    const buffer = __getBridgeAuditBufferForTests();
    expect(buffer).toHaveLength(5000);
    // Oldest entries evicted — the first surviving session id should be 50.
    expect(buffer[0].sessionId).toBe("s50");
    expect(buffer[buffer.length - 1].sessionId).toBe("s5049");
  });

  it("exposes window.__kandev_bridge_audit__ returning a copy of the buffer", () => {
    const qc = createTestQueryClient();
    const wrapped = wrapBridgeHandler(qc, ACTION, () => undefined);
    wrapped({ payload: { session_id: "s1" } });

    expect(typeof window.__kandev_bridge_audit__).toBe("function");
    const snapshot = window.__kandev_bridge_audit__!();
    expect(snapshot).toHaveLength(1);
    expect(snapshot[0].sessionId).toBe("s1");

    // Returned array is a copy — mutating it must not affect the internal buffer.
    snapshot.pop();
    expect(__getBridgeAuditBufferForTests()).toHaveLength(1);
  });

  it("window.__kandev_bridge_audit_clear__ empties the buffer", () => {
    const qc = createTestQueryClient();
    const wrapped = wrapBridgeHandler(qc, ACTION, () => undefined);
    wrapped({ payload: { session_id: "s1" } });
    wrapped({ payload: { session_id: "s2" } });

    expect(__getBridgeAuditBufferForTests()).toHaveLength(2);
    expect(typeof window.__kandev_bridge_audit_clear__).toBe("function");
    window.__kandev_bridge_audit_clear__!();
    expect(__getBridgeAuditBufferForTests()).toHaveLength(0);
    expect(window.__kandev_bridge_audit__!()).toEqual([]);
  });
});
