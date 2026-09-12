import { describe, it, expect, beforeEach } from "vitest";
import { create } from "zustand";
import { immer } from "zustand/middleware/immer";
import { createSessionRuntimeSlice, purgeSessionRuntimeState } from "./session-runtime-slice";
import type { SessionRuntimeSlice } from "./types";

function makeStore() {
  return create<SessionRuntimeSlice>()(immer<SessionRuntimeSlice>(createSessionRuntimeSlice));
}

const SESSION_ID = "session-1";
const SHARED_ENVIRONMENT_ID = "env-shared";

describe("purgeSessionRuntimeState", () => {
  let store: ReturnType<typeof makeStore>;

  beforeEach(() => {
    store = makeStore();
  });

  it("drops per-session maps, process output, and env-scoped buffers", () => {
    const s = store.getState();
    s.registerSessionEnvironment(SESSION_ID, "env-1");
    s.appendShellOutput(SESSION_ID, "shell noise");
    s.setShellStatus(SESSION_ID, { available: true });
    s.setContextWindow(SESSION_ID, {
      size: 1,
      used: 1,
      remaining: 0,
      efficiency: 1,
      compactionCount: 0,
    });
    s.setSessionTodos(SESSION_ID, [{ description: "do", status: "pending" }]);
    s.beginWorkspaceRestoration("task-1", SESSION_ID, "env-1");
    s.upsertProcessStatus({
      processId: "proc-1",
      sessionId: SESSION_ID,
      kind: "dev",
      status: "running",
    });
    s.appendProcessOutput("proc-1", "process noise");

    store.setState((draft) => {
      purgeSessionRuntimeState(draft, SESSION_ID);
    });

    const after = store.getState();
    expect(after.environmentIdBySessionId[SESSION_ID]).toBeUndefined();
    expect(after.contextWindow.bySessionId[SESSION_ID]).toBeUndefined();
    expect(after.sessionTodos.bySessionId[SESSION_ID]).toBeUndefined();
    expect(after.processes.processIdsBySessionId[SESSION_ID]).toBeUndefined();
    expect(after.processes.devProcessBySessionId[SESSION_ID]).toBeUndefined();
    expect(after.processes.processesById["proc-1"]).toBeUndefined();
    expect(after.processes.outputsByProcessId["proc-1"]).toBeUndefined();
    // env-scoped buffers gone because no other session references env-1.
    expect(after.shell.outputs["env-1"]).toBeUndefined();
    expect(after.shell.statuses["env-1"]).toBeUndefined();
    expect(after.workspaceRestoration.byEnvironmentId["env-1"]).toBeUndefined();
  });

  it("retains env-scoped buffers while another session shares the environment", () => {
    const s = store.getState();
    s.registerSessionEnvironment(SESSION_ID, SHARED_ENVIRONMENT_ID);
    s.registerSessionEnvironment("session-2", SHARED_ENVIRONMENT_ID);
    s.appendShellOutput(SESSION_ID, "shared shell");
    s.beginWorkspaceRestoration("task-1", SESSION_ID, SHARED_ENVIRONMENT_ID);

    store.setState((draft) => {
      purgeSessionRuntimeState(draft, SESSION_ID);
    });

    const after = store.getState();
    expect(after.environmentIdBySessionId[SESSION_ID]).toBeUndefined();
    expect(after.environmentIdBySessionId["session-2"]).toBe(SHARED_ENVIRONMENT_ID);
    // session-2 still uses env-shared, so its shell output must survive.
    expect(after.shell.outputs[SHARED_ENVIRONMENT_ID]).toBe("shared shell");
    expect(after.workspaceRestoration.byEnvironmentId[SHARED_ENVIRONMENT_ID]).toMatchObject({
      status: "pending",
    });
  });
});
