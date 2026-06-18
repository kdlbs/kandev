import { describe, it, expect, beforeEach } from "vitest";
import { create } from "zustand";
import { immer } from "zustand/middleware/immer";
import { createSessionRuntimeSlice, purgeSessionRuntimeState } from "./session-runtime-slice";
import type { SessionRuntimeSlice } from "./types";

function makeStore() {
  return create<SessionRuntimeSlice>()(immer<SessionRuntimeSlice>(createSessionRuntimeSlice));
}

describe("purgeSessionRuntimeState", () => {
  let store: ReturnType<typeof makeStore>;

  beforeEach(() => {
    store = makeStore();
  });

  it("drops per-session maps, process output, and env-scoped buffers", () => {
    const s = store.getState();
    s.registerSessionEnvironment("session-1", "env-1");
    s.appendShellOutput("session-1", "shell noise");
    s.setShellStatus("session-1", { available: true });
    s.setContextWindow("session-1", { size: 1, used: 1, remaining: 0, efficiency: 1 });
    s.setSessionTodos("session-1", [{ description: "do", status: "pending" }]);
    s.upsertProcessStatus({
      processId: "proc-1",
      sessionId: "session-1",
      kind: "dev",
      status: "running",
    });
    s.appendProcessOutput("proc-1", "process noise");

    store.setState((draft) => {
      purgeSessionRuntimeState(draft, "session-1");
    });

    const after = store.getState();
    expect(after.environmentIdBySessionId["session-1"]).toBeUndefined();
    expect(after.contextWindow.bySessionId["session-1"]).toBeUndefined();
    expect(after.sessionTodos.bySessionId["session-1"]).toBeUndefined();
    expect(after.processes.processIdsBySessionId["session-1"]).toBeUndefined();
    expect(after.processes.devProcessBySessionId["session-1"]).toBeUndefined();
    expect(after.processes.processesById["proc-1"]).toBeUndefined();
    expect(after.processes.outputsByProcessId["proc-1"]).toBeUndefined();
    // env-scoped buffers gone because no other session references env-1.
    expect(after.shell.outputs["env-1"]).toBeUndefined();
    expect(after.shell.statuses["env-1"]).toBeUndefined();
  });

  it("retains env-scoped buffers while another session shares the environment", () => {
    const s = store.getState();
    s.registerSessionEnvironment("session-1", "env-shared");
    s.registerSessionEnvironment("session-2", "env-shared");
    s.appendShellOutput("session-1", "shared shell");

    store.setState((draft) => {
      purgeSessionRuntimeState(draft, "session-1");
    });

    const after = store.getState();
    expect(after.environmentIdBySessionId["session-1"]).toBeUndefined();
    expect(after.environmentIdBySessionId["session-2"]).toBe("env-shared");
    // session-2 still uses env-shared, so its shell output must survive.
    expect(after.shell.outputs["env-shared"]).toBe("shared shell");
  });
});
