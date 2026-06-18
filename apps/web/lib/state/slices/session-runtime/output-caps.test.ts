import { describe, it, expect, beforeEach } from "vitest";
import { create } from "zustand";
import { immer } from "zustand/middleware/immer";
import { createSessionRuntimeSlice } from "./session-runtime-slice";
import type { SessionRuntimeSlice } from "./types";

const MAX_BYTES = 2 * 1024 * 1024;

function makeStore() {
  return create<SessionRuntimeSlice>()(immer<SessionRuntimeSlice>(createSessionRuntimeSlice));
}

describe("shell + terminal output caps", () => {
  let store: ReturnType<typeof makeStore>;

  beforeEach(() => {
    store = makeStore();
  });

  it("appendShellOutput keeps the tail bounded at the cap", () => {
    const chunk = "x".repeat(512 * 1024);
    for (let i = 0; i < 10; i += 1) {
      store.getState().appendShellOutput("session-1", chunk);
    }
    const output = store.getState().shell.outputs["session-1"];
    expect(output.length).toBe(MAX_BYTES);
  });

  it("appendShellOutput preserves the most recent bytes", () => {
    store.getState().appendShellOutput("session-1", "a".repeat(MAX_BYTES));
    store.getState().appendShellOutput("session-1", "TAIL");
    const output = store.getState().shell.outputs["session-1"];
    expect(output.endsWith("TAIL")).toBe(true);
    expect(output.length).toBe(MAX_BYTES);
  });

  it("appendShellOutput leaves small buffers untouched", () => {
    store.getState().appendShellOutput("session-1", "hello ");
    store.getState().appendShellOutput("session-1", "world");
    expect(store.getState().shell.outputs["session-1"]).toBe("hello world");
  });

  it("setTerminalOutput drops oldest chunks once over the cap", () => {
    const chunk = "y".repeat(512 * 1024);
    for (let i = 0; i < 10; i += 1) {
      store.getState().setTerminalOutput("term-1", chunk);
    }
    const terminal = store.getState().terminal.terminals.find((t) => t.id === "term-1");
    const total = terminal?.output.reduce((sum, c) => sum + c.length, 0) ?? 0;
    expect(total).toBeLessThanOrEqual(MAX_BYTES);
    // Never empties the buffer entirely while data is flowing.
    expect(terminal?.output.length ?? 0).toBeGreaterThan(0);
  });

  it("setTerminalOutput keeps small buffers as separate chunks", () => {
    store.getState().setTerminalOutput("term-1", "a");
    store.getState().setTerminalOutput("term-1", "b");
    const terminal = store.getState().terminal.terminals.find((t) => t.id === "term-1");
    expect(terminal?.output).toEqual(["a", "b"]);
  });
});
