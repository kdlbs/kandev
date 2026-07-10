import { describe, it, expect, beforeEach } from "vitest";
import { create } from "zustand";
import { immer } from "zustand/middleware/immer";
import { createSessionSlice } from "./session-slice";
import { createSessionRuntimeSlice } from "../session-runtime/session-runtime-slice";
import type { SessionSlice } from "./types";
import type { SessionRuntimeSlice } from "../session-runtime/types";

type CombinedSlice = SessionSlice & SessionRuntimeSlice;

function makeStore() {
  return create<CombinedSlice>()(
    immer((set) => ({
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      ...(createSessionSlice as any)(set),
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      ...(createSessionRuntimeSlice as any)(set),
    })),
  );
}

const TASK_ID = "task-1";
const SESSION_ID = "session-1";

describe("removeTaskSession cleanup cascade", () => {
  let store: ReturnType<typeof makeStore>;

  beforeEach(() => {
    store = makeStore();
  });

  it("purges messages, turns, and runtime buffers for the removed session", () => {
    const s = store.getState();
    s.registerSessionEnvironment(SESSION_ID, "env-1");
    s.appendShellOutput(SESSION_ID, "shell output");
    s.setContextWindow(SESSION_ID, { size: 1, used: 1, remaining: 0, efficiency: 1 });
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    s.addMessage({ id: "m1", session_id: SESSION_ID, role: "user", content: "hi" } as any);
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    s.addTurn({ id: "t1", session_id: SESSION_ID } as any);

    expect(store.getState().messages.bySession[SESSION_ID]).toHaveLength(1);
    expect(store.getState().turns.bySession[SESSION_ID]).toHaveLength(1);

    store.getState().removeTaskSession(TASK_ID, SESSION_ID);

    const after = store.getState();
    expect(after.messages.bySession[SESSION_ID]).toBeUndefined();
    expect(after.turns.bySession[SESSION_ID]).toBeUndefined();
    expect(after.contextWindow.bySessionId[SESSION_ID]).toBeUndefined();
    expect(after.shell.outputs["env-1"]).toBeUndefined();
    expect(after.environmentIdBySessionId[SESSION_ID]).toBeUndefined();
  });
});
