import { describe, it, expect, beforeEach } from "vitest";
import { create } from "zustand";
import { immer } from "zustand/middleware/immer";
import { createSessionRuntimeSlice } from "./session-runtime-slice";
import type { SessionRuntimeSlice } from "./types";

const MAX_BYTES = 2 * 1024 * 1024;

function makeStore() {
  return create<SessionRuntimeSlice>()(immer<SessionRuntimeSlice>(createSessionRuntimeSlice));
}

describe("shell output caps", () => {
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
});
