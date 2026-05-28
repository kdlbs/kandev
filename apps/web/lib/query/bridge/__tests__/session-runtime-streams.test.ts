import { describe, it, expect, beforeEach } from "vitest";
import { createTestQueryClient } from "@/test-utils/render-with-query";
import {
  registerSessionRuntimeStreamsBridge,
  shellRingKey,
  processRingKey,
  terminalRingKey,
  passthroughRingKey,
} from "../session-runtime-streams";
import { getRingBuffer, clearRing, appendToRing } from "@/lib/query/streams/ring";

// ---------------------------------------------------------------------------
// Fake WebSocket client
// ---------------------------------------------------------------------------

type Handler = (message: { payload: Record<string, unknown> }) => void;

function makeFakeWs() {
  const handlers = new Map<string, Set<Handler>>();

  return {
    on(event: string, handler: Handler) {
      let set = handlers.get(event);
      if (!set) {
        set = new Set();
        handlers.set(event, set);
      }
      set.add(handler);
      return () => {
        set?.delete(handler);
      };
    },
    emit(event: string, payload: Record<string, unknown>) {
      const set = handlers.get(event);
      if (!set) return;
      for (const h of set) {
        h({ payload });
      }
    },
  };
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function getSnapshot(key: string): string[] {
  const buf = getRingBuffer(key);
  const result: string[] = [];
  const start = buf.size < buf.capacity ? 0 : buf.head;
  for (let i = 0; i < buf.size; i++) {
    result.push(buf.lines[(start + i) % buf.capacity]);
  }
  return result;
}

// ---------------------------------------------------------------------------
// WS event name constants
// ---------------------------------------------------------------------------

const SHELL_OUTPUT_EVENT = "session.shell.output";

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe("session-runtime-streams bridge", () => {
  let ws: ReturnType<typeof makeFakeWs>;
  let cleanup: () => void;

  beforeEach(() => {
    ws = makeFakeWs();
    const qc = createTestQueryClient();
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    cleanup = registerSessionRuntimeStreamsBridge(ws as any, qc);

    // Clear ring buffers before each test
    clearRing(shellRingKey("sess-1"));
    clearRing(processRingKey("proc-1"));
    clearRing(terminalRingKey("term-1"));
    clearRing(passthroughRingKey("sess-pt"));
  });

  it("appends shell output to the shell ring buffer", () => {
    ws.emit(SHELL_OUTPUT_EVENT, {
      session_id: "sess-1",
      type: "output",
      data: "hello\n",
    });
    expect(getSnapshot(shellRingKey("sess-1"))).toEqual(["hello\n"]);
  });

  it("clears shell ring buffer on exit event", () => {
    appendToRing(shellRingKey("sess-1"), "old data");
    ws.emit(SHELL_OUTPUT_EVENT, {
      session_id: "sess-1",
      type: "exit",
    });
    expect(getSnapshot(shellRingKey("sess-1"))).toEqual([]);
  });

  it("ignores shell output events without session_id", () => {
    ws.emit(SHELL_OUTPUT_EVENT, { type: "output", data: "oops" });
    // No-op; shouldn't throw
  });

  it("appends process output to process ring buffer", () => {
    ws.emit("session.process.output", {
      process_id: "proc-1",
      session_id: "sess-1",
      kind: "script",
      data: "step 1\n",
    });
    expect(getSnapshot(processRingKey("proc-1"))).toEqual(["step 1\n"]);
  });

  it("also appends agent_passthrough output to passthrough ring", () => {
    ws.emit("session.process.output", {
      process_id: "proc-1",
      session_id: "sess-pt",
      kind: "agent_passthrough",
      data: "agent: hello\n",
    });
    expect(getSnapshot(processRingKey("proc-1"))).toEqual(["agent: hello\n"]);
    expect(getSnapshot(passthroughRingKey("sess-pt"))).toEqual(["agent: hello\n"]);
  });

  it("ignores process output without process_id or data", () => {
    ws.emit("session.process.output", { session_id: "s", kind: "script" });
    // No-op; shouldn't throw
  });

  it("appends terminal output to terminal ring buffer", () => {
    ws.emit("terminal.output", {
      terminalId: "term-1",
      data: "terminal line\n",
    });
    expect(getSnapshot(terminalRingKey("term-1"))).toEqual(["terminal line\n"]);
  });

  it("ignores terminal output without terminalId", () => {
    ws.emit("terminal.output", { data: "no-id" });
    // No-op; shouldn't throw
  });

  it("accumulates 1000 shell lines within ring capacity", () => {
    const sessionId = "perf-sess";
    clearRing(shellRingKey(sessionId));

    const N = 1000;
    for (let i = 0; i < N; i++) {
      ws.emit(SHELL_OUTPUT_EVENT, {
        session_id: sessionId,
        type: "output",
        data: `line ${i}\n`,
      });
    }

    const snap = getSnapshot(shellRingKey(sessionId));
    // All 1000 lines should be present (capacity is 10_000)
    expect(snap).toHaveLength(N);
    expect(snap[0]).toBe("line 0\n");
    expect(snap[N - 1]).toBe(`line ${N - 1}\n`);
  });

  it("cleanup removes handlers so subsequent events are ignored", () => {
    cleanup();
    ws.emit(SHELL_OUTPUT_EVENT, {
      session_id: "after-cleanup",
      type: "output",
      data: "should not appear",
    });
    expect(getSnapshot(shellRingKey("after-cleanup"))).toEqual([]);
  });
});
