import { describe, it, expect, vi, beforeEach } from "vitest";
import { QueryClient } from "@tanstack/react-query";
import { registerTaskSessionHandlers } from "./agent-session";
import { qk } from "@/lib/query/keys";
import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import type { TaskSession } from "@/lib/types/http";

function makeStore(overrides: Record<string, unknown> = {}) {
  const state: Record<string, unknown> = {
    tasks: {
      activeTaskId: null,
      activeSessionId: null,
      pinnedSessionId: null,
      lastSessionByTaskId: {},
    },
    setActiveSession: vi.fn(),
    setActiveSessionAuto: vi.fn(),
    setSessionFailureNotification: vi.fn(),
    setContextWindow: vi.fn(),
    ...overrides,
  };
  return {
    getState: () => state as unknown as AppState,
    setState: vi.fn(),
    subscribe: vi.fn(),
    destroy: vi.fn(),
    getInitialState: vi.fn(),
  } as unknown as StoreApi<AppState>;
}

/** Seed the TQ by-id cache (the TaskSession record the bridge owns). */
function seedById(qc: QueryClient, session: Record<string, unknown> & { id: string }): void {
  qc.setQueryData(qk.taskSession.byId(session.id), session as unknown as TaskSession);
}

/** Read the agentctl status from the TQ cache (where it now lives). */
function readAgentctl(qc: QueryClient, sessionId: string) {
  return qc.getQueryData(qk.session.agentctl(sessionId)) as
    | { status: string; agentExecutionId?: string }
    | undefined;
}

/** Seed the TQ per-task list cache. */
function seedByTask(
  qc: QueryClient,
  taskId: string,
  sessions: Array<Record<string, unknown>>,
): void {
  qc.setQueryData(qk.taskSession.byTask(taskId), {
    sessions: sessions as unknown as TaskSession[],
    total: sessions.length,
  });
}

const STATE_CHANGED_EVENT = "session.state_changed";

function makeMessage(payload: Record<string, unknown>) {
  return { id: "msg-1", type: "notification", action: STATE_CHANGED_EVENT, payload };
}

describe("session.state_changed handler", () => {
  let store: ReturnType<typeof makeStore>;
  let qc: QueryClient;
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  let handler: (msg: any) => void;

  beforeEach(() => {
    vi.clearAllMocks();
    qc = new QueryClient();
  });

  it("sets failure notification on first FAILED event", () => {
    store = makeStore();
    seedById(qc, { id: "s-1", task_id: "t-1", state: "STARTING" });
    handler = registerTaskSessionHandlers(store, qc)[STATE_CHANGED_EVENT]!;

    handler(
      makeMessage({
        task_id: "t-1",
        session_id: "s-1",
        new_state: "FAILED",
        error_message: "container crashed",
      }),
    );

    expect(store.getState().setSessionFailureNotification).toHaveBeenCalledWith({
      sessionId: "s-1",
      taskId: "t-1",
      message: "container crashed",
    });
  });

  it("does not set failure notification when session is already FAILED", () => {
    store = makeStore();
    seedById(qc, { id: "s-1", task_id: "t-1", state: "FAILED" });
    handler = registerTaskSessionHandlers(store, qc)[STATE_CHANGED_EVENT]!;

    handler(
      makeMessage({
        task_id: "t-1",
        session_id: "s-1",
        new_state: "FAILED",
        error_message: "container crashed",
      }),
    );

    expect(store.getState().setSessionFailureNotification).not.toHaveBeenCalled();
  });

  it("does not set failure notification for unknown session (snapshot replay)", () => {
    // When a session is replayed on reconnect/page-load, it lands in the FE
    // store for the first time already in FAILED state. This is not a real
    // transition we just observed, so no toast should fire.
    store = makeStore();
    handler = registerTaskSessionHandlers(store, qc)[STATE_CHANGED_EVENT]!;

    handler(
      makeMessage({
        task_id: "t-1",
        session_id: "s-new",
        new_state: "FAILED",
        error_message: "timeout",
      }),
    );

    expect(store.getState().setSessionFailureNotification).not.toHaveBeenCalled();
  });

  it("respects suppress_toast flag", () => {
    store = makeStore();
    seedById(qc, { id: "s-1", task_id: "t-1", state: "STARTING" });
    handler = registerTaskSessionHandlers(store, qc)[STATE_CHANGED_EVENT]!;

    handler(
      makeMessage({
        task_id: "t-1",
        session_id: "s-1",
        new_state: "FAILED",
        error_message: "missing branch",
        suppress_toast: true,
      }),
    );

    expect(store.getState().setSessionFailureNotification).not.toHaveBeenCalled();
  });
});

describe("session.state_changed stale guard", () => {
  let qc: QueryClient;
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  let handler: (msg: any) => void;
  beforeEach(() => {
    vi.clearAllMocks();
    qc = new QueryClient();
  });

  // A subscribe snapshot read before a newer state landed (older updated_at)
  // must not drive the adoption / agentctl-promotion logic — that would let a
  // stale STARTING snapshot stomp a live WAITING_FOR_INPUT and block idle input.
  it("ignores older state events: no adoption or agentctl promotion", () => {
    const store = makeStore({
      tasks: {
        activeTaskId: "t-1",
        activeSessionId: "s-1",
        pinnedSessionId: null,
        lastSessionByTaskId: {},
      },
    });
    seedById(qc, {
      id: "s-1",
      task_id: "t-1",
      state: "WAITING_FOR_INPUT",
      updated_at: "2026-01-02T00:00:00.000Z",
    });
    handler = registerTaskSessionHandlers(store, qc)[STATE_CHANGED_EVENT]!;

    handler(
      makeMessage({
        task_id: "t-1",
        session_id: "s-1",
        new_state: "RUNNING",
        updated_at: "2026-01-01T00:00:00.000Z",
      }),
    );

    expect(store.getState().setActiveSessionAuto).not.toHaveBeenCalled();
    expect(readAgentctl(qc, "s-1")).toBeUndefined();
  });

  it("applies newer state events (not stale): agentctl promoted", () => {
    const store = makeStore();
    seedById(qc, {
      id: "s-1",
      task_id: "t-1",
      state: "STARTING",
      updated_at: "2026-01-01T00:00:00.000Z",
    });
    handler = registerTaskSessionHandlers(store, qc)[STATE_CHANGED_EVENT]!;

    handler(
      makeMessage({
        task_id: "t-1",
        session_id: "s-1",
        new_state: "RUNNING",
        updated_at: "2026-01-02T00:00:00.000Z",
      }),
    );

    expect(readAgentctl(qc, "s-1")).toMatchObject({ status: "ready" });
  });
});

describe("session.state_changed → active session switching", () => {
  let qc: QueryClient;
  beforeEach(() => {
    vi.clearAllMocks();
    qc = new QueryClient();
  });

  it("adopts a newly-created session for the active task", () => {
    const store = makeStore({
      tasks: {
        activeTaskId: "t-1",
        activeSessionId: null,
        pinnedSessionId: null,
        lastSessionByTaskId: {},
      },
    });
    const handler = registerTaskSessionHandlers(store, qc)[STATE_CHANGED_EVENT]!;

    handler({
      id: "m",
      type: "notification",
      action: STATE_CHANGED_EVENT,
      payload: { task_id: "t-1", session_id: "s-new", new_state: "STARTING" },
    });

    expect(store.getState().setActiveSessionAuto).toHaveBeenCalledWith("t-1", "s-new");
    expect(store.getState().setActiveSession).not.toHaveBeenCalled();
  });

  it("does not adopt a new session for a task that is not active", () => {
    const store = makeStore({
      tasks: {
        activeTaskId: "other-task",
        activeSessionId: null,
        pinnedSessionId: null,
        lastSessionByTaskId: {},
      },
    });
    const handler = registerTaskSessionHandlers(store, qc)[STATE_CHANGED_EVENT]!;

    handler({
      id: "m",
      type: "notification",
      action: STATE_CHANGED_EVENT,
      payload: { task_id: "t-1", session_id: "s-new", new_state: "STARTING" },
    });

    expect(store.getState().setActiveSessionAuto).not.toHaveBeenCalled();
  });

  it("does not adopt while the current active session is still running", () => {
    const store = makeStore({
      tasks: {
        activeTaskId: "t-1",
        activeSessionId: "s-old",
        pinnedSessionId: null,
        lastSessionByTaskId: {},
      },
    });
    seedById(qc, { id: "s-old", task_id: "t-1", state: "RUNNING" });
    const handler = registerTaskSessionHandlers(store, qc)[STATE_CHANGED_EVENT]!;

    handler({
      id: "m",
      type: "notification",
      action: STATE_CHANGED_EVENT,
      payload: { task_id: "t-1", session_id: "s-new", new_state: "STARTING" },
    });

    expect(store.getState().setActiveSessionAuto).not.toHaveBeenCalled();
  });

  // Regression for the reverse-event-ordering race: if the OLD pinned session's
  // COMPLETED event arrives before the NEW session's STARTING event, the
  // terminal-handoff guard (which protects pinning) doesn't run on the COMPLETED
  // event because s-new isn't yet in the store. When the STARTING event
  // arrives, shouldAdoptNewSession returns true (old is now terminal) and would
  // auto-yank the user off their pinned session — unless we re-check pinning on
  // this path too.
  it("does not yank a pinned session on reverse event ordering (old COMPLETED, then new STARTING)", () => {
    const store = makeStore({
      tasks: {
        activeTaskId: "t-1",
        activeSessionId: "s-old",
        pinnedSessionId: "s-old",
        lastSessionByTaskId: {},
      },
    });
    seedById(qc, { id: "s-old", task_id: "t-1", state: "COMPLETED" });
    const handler = registerTaskSessionHandlers(store, qc)[STATE_CHANGED_EVENT]!;

    handler({
      id: "m",
      type: "notification",
      action: STATE_CHANGED_EVENT,
      payload: { task_id: "t-1", session_id: "s-new", new_state: "STARTING" },
    });

    expect(store.getState().setActiveSessionAuto).not.toHaveBeenCalled();
  });
});

describe("session.state_changed → active session handoff on terminal", () => {
  let qc: QueryClient;
  beforeEach(() => {
    vi.clearAllMocks();
    qc = new QueryClient();
  });

  it("hands off when the current active session transitions to terminal", () => {
    const store = makeStore({
      tasks: {
        activeTaskId: "t-1",
        activeSessionId: "s-old",
        pinnedSessionId: null,
        lastSessionByTaskId: {},
      },
    });
    seedById(qc, { id: "s-old", task_id: "t-1", state: "RUNNING" });
    seedByTask(qc, "t-1", [
      { id: "s-old", task_id: "t-1", state: "RUNNING", started_at: "", updated_at: "" },
      { id: "s-new", task_id: "t-1", state: "STARTING", started_at: "", updated_at: "" },
    ]);
    const handler = registerTaskSessionHandlers(store, qc)[STATE_CHANGED_EVENT]!;

    handler({
      id: "m",
      type: "notification",
      action: STATE_CHANGED_EVENT,
      payload: { task_id: "t-1", session_id: "s-old", new_state: "COMPLETED" },
    });

    expect(store.getState().setActiveSessionAuto).toHaveBeenCalledWith("t-1", "s-new");
    expect(store.getState().setActiveSession).not.toHaveBeenCalled();
  });

  // The per-task list here still shows s-old as RUNNING (pre-event state), so
  // pickReplacementSessionId returns s-old itself. This exercises the
  // `replacement !== sessionId` guard — without it, we'd set activeSessionId
  // to the same session that just became terminal.
  it("does not hand off when the only candidate is the terminating session itself", () => {
    const store = makeStore({
      tasks: {
        activeTaskId: "t-1",
        activeSessionId: "s-old",
        pinnedSessionId: null,
        lastSessionByTaskId: {},
      },
    });
    seedById(qc, { id: "s-old", task_id: "t-1", state: "RUNNING" });
    seedByTask(qc, "t-1", [
      { id: "s-old", task_id: "t-1", state: "RUNNING", started_at: "", updated_at: "" },
    ]);
    const handler = registerTaskSessionHandlers(store, qc)[STATE_CHANGED_EVENT]!;

    handler({
      id: "m",
      type: "notification",
      action: STATE_CHANGED_EVENT,
      payload: { task_id: "t-1", session_id: "s-old", new_state: "COMPLETED" },
    });

    expect(store.getState().setActiveSessionAuto).not.toHaveBeenCalled();
  });

  it("does not hand off when all other sessions for the task are terminal", () => {
    const store = makeStore({
      tasks: {
        activeTaskId: "t-1",
        activeSessionId: "s-old",
        pinnedSessionId: null,
        lastSessionByTaskId: {},
      },
    });
    seedById(qc, { id: "s-old", task_id: "t-1", state: "RUNNING" });
    seedByTask(qc, "t-1", [
      { id: "s-done", task_id: "t-1", state: "COMPLETED", started_at: "", updated_at: "" },
      { id: "s-old", task_id: "t-1", state: "RUNNING", started_at: "", updated_at: "" },
    ]);
    const handler = registerTaskSessionHandlers(store, qc)[STATE_CHANGED_EVENT]!;

    handler({
      id: "m",
      type: "notification",
      action: STATE_CHANGED_EVENT,
      payload: { task_id: "t-1", session_id: "s-old", new_state: "COMPLETED" },
    });

    expect(store.getState().setActiveSessionAuto).not.toHaveBeenCalled();
  });
});

describe("session.state_changed → respects user-pinned session", () => {
  let qc: QueryClient;
  beforeEach(() => {
    vi.clearAllMocks();
    qc = new QueryClient();
  });

  it("does NOT hand off when the user pinned the session that just terminated", () => {
    // User manually clicked s-old, so pinnedSessionId === "s-old".
    // When s-old terminates we should respect the pin and stay on it.
    const store = makeStore({
      tasks: {
        activeTaskId: "t-1",
        activeSessionId: "s-old",
        pinnedSessionId: "s-old",
        lastSessionByTaskId: {},
      },
    });
    seedById(qc, { id: "s-old", task_id: "t-1", state: "RUNNING" });
    seedByTask(qc, "t-1", [
      { id: "s-old", task_id: "t-1", state: "RUNNING", started_at: "", updated_at: "" },
      { id: "s-new", task_id: "t-1", state: "STARTING", started_at: "", updated_at: "" },
    ]);
    const handler = registerTaskSessionHandlers(store, qc)[STATE_CHANGED_EVENT]!;

    handler({
      id: "m",
      type: "notification",
      action: STATE_CHANGED_EVENT,
      payload: { task_id: "t-1", session_id: "s-old", new_state: "COMPLETED" },
    });

    expect(store.getState().setActiveSessionAuto).not.toHaveBeenCalled();
    expect(store.getState().setActiveSession).not.toHaveBeenCalled();
  });
});

describe("session.state_changed → agentctl ready fallback", () => {
  const TS = "2026-05-04T00:00:00Z";
  let qc: QueryClient;
  beforeEach(() => {
    vi.clearAllMocks();
    qc = new QueryClient();
  });

  it("promotes agentctl status to 'ready' when session enters RUNNING and ready event was missed", () => {
    const store = makeStore();
    qc.setQueryData(qk.session.agentctl("s-1"), { status: "starting" });
    seedById(qc, { id: "s-1", task_id: "t-1", state: "STARTING" });
    const handler = registerTaskSessionHandlers(store, qc)[STATE_CHANGED_EVENT]!;

    handler({
      id: "m",
      type: "notification",
      action: STATE_CHANGED_EVENT,
      timestamp: TS,
      payload: { task_id: "t-1", session_id: "s-1", new_state: "RUNNING" },
    });

    expect(readAgentctl(qc, "s-1")).toMatchObject({ status: "ready" });
  });

  it("promotes agentctl status to 'ready' on WAITING_FOR_INPUT even when no prior entry exists", () => {
    const store = makeStore();
    seedById(qc, { id: "s-1", task_id: "t-1", state: "STARTING" });
    const handler = registerTaskSessionHandlers(store, qc)[STATE_CHANGED_EVENT]!;

    handler({
      id: "m",
      type: "notification",
      action: STATE_CHANGED_EVENT,
      timestamp: TS,
      payload: { task_id: "t-1", session_id: "s-1", new_state: "WAITING_FOR_INPUT" },
    });

    expect(readAgentctl(qc, "s-1")).toMatchObject({ status: "ready" });
  });

  it("does not re-set 'ready' when the session is already ready", () => {
    const store = makeStore();
    qc.setQueryData(qk.session.agentctl("s-1"), { status: "ready", agentExecutionId: "ae-keep" });
    seedById(qc, { id: "s-1", task_id: "t-1", state: "RUNNING" });
    const handler = registerTaskSessionHandlers(store, qc)[STATE_CHANGED_EVENT]!;

    handler({
      id: "m",
      type: "notification",
      action: STATE_CHANGED_EVENT,
      timestamp: TS,
      payload: { task_id: "t-1", session_id: "s-1", new_state: "WAITING_FOR_INPUT" },
    });

    // Unchanged: the existing ready entry (with its agentExecutionId) is preserved.
    expect(readAgentctl(qc, "s-1")).toMatchObject({ status: "ready", agentExecutionId: "ae-keep" });
  });

  // NOTE: agentctl_ready worktree indexing now lives entirely in the
  // session-state bridge (worktree_* fields → TaskSession TQ cache); the Zustand
  // worktree mirror was removed. See lib/query/bridge/session-state.test.ts
  // ("session.agentctl_ready merges worktree fields ...").

  it("does not promote on non-live states (STARTING, COMPLETED, FAILED)", () => {
    const store = makeStore();
    seedById(qc, { id: "s-1", task_id: "t-1", state: "CREATED" });
    const handler = registerTaskSessionHandlers(store, qc)[STATE_CHANGED_EVENT]!;

    for (const newState of ["STARTING", "COMPLETED", "FAILED", "CANCELLED"]) {
      handler({
        id: "m",
        type: "notification",
        action: STATE_CHANGED_EVENT,
        timestamp: TS,
        payload: { task_id: "t-1", session_id: "s-1", new_state: newState },
      });
    }

    expect(readAgentctl(qc, "s-1")).toBeUndefined();
  });
});
