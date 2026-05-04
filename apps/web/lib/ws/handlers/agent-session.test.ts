import { describe, it, expect, vi, beforeEach } from "vitest";
import {
  isTerminalSessionState,
  pickReplacementSessionId,
  registerTaskSessionHandlers,
  shouldAdoptNewSession,
} from "./agent-session";
import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import type { TaskSessionState } from "@/lib/types/http";

function makeStore(overrides: Record<string, unknown> = {}) {
  const state: Record<string, unknown> = {
    tasks: { activeTaskId: null, activeSessionId: null, pinnedSessionId: null },
    taskSessions: { items: {} },
    taskSessionsByTask: { itemsByTaskId: {} },
    setTaskSession: vi.fn(),
    setTaskSessionsForTask: vi.fn(),
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

const STATE_CHANGED_EVENT = "session.state_changed";

function makeMessage(payload: Record<string, unknown>) {
  return { id: "msg-1", type: "notification", action: STATE_CHANGED_EVENT, payload };
}

describe("session.state_changed handler", () => {
  let store: ReturnType<typeof makeStore>;
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  let handler: (msg: any) => void;

  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("sets failure notification on first FAILED event", () => {
    store = makeStore({
      taskSessions: {
        items: { "s-1": { id: "s-1", task_id: "t-1", state: "STARTING" } },
      },
    });
    handler = registerTaskSessionHandlers(store)[STATE_CHANGED_EVENT]!;

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
    store = makeStore({
      taskSessions: {
        items: { "s-1": { id: "s-1", task_id: "t-1", state: "FAILED" } },
      },
    });
    handler = registerTaskSessionHandlers(store)[STATE_CHANGED_EVENT]!;

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
    handler = registerTaskSessionHandlers(store)[STATE_CHANGED_EVENT]!;

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
    store = makeStore({
      taskSessions: {
        items: { "s-1": { id: "s-1", task_id: "t-1", state: "STARTING" } },
      },
    });
    handler = registerTaskSessionHandlers(store)[STATE_CHANGED_EVENT]!;

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

function makeAppState(partial: Partial<AppState>): AppState {
  return {
    tasks: { activeTaskId: null, activeSessionId: null, pinnedSessionId: null },
    taskSessions: { items: {} },
    taskSessionsByTask: { itemsByTaskId: {} },
    ...partial,
  } as unknown as AppState;
}

describe("isTerminalSessionState", () => {
  it.each<[TaskSessionState | undefined, boolean]>([
    ["COMPLETED", true],
    ["FAILED", true],
    ["CANCELLED", true],
    ["RUNNING", false],
    ["STARTING", false],
    ["CREATED", false],
    ["WAITING_FOR_INPUT", false],
    [undefined, false],
  ])("returns %o → %s", (input, expected) => {
    expect(isTerminalSessionState(input)).toBe(expected);
  });
});

describe("shouldAdoptNewSession", () => {
  it("adopts when there is no active session for the task", () => {
    const state = makeAppState({
      tasks: { activeTaskId: "t-1", activeSessionId: null, pinnedSessionId: null },
    });
    expect(shouldAdoptNewSession(state, "t-1", "STARTING")).toBe(true);
  });

  it("adopts when active session belongs to a different task", () => {
    const state = makeAppState({
      tasks: { activeTaskId: "t-1", activeSessionId: "s-other", pinnedSessionId: null },
      taskSessions: {
        items: { "s-other": { id: "s-other", task_id: "t-2", state: "RUNNING" } },
      } as unknown as AppState["taskSessions"],
    });
    expect(shouldAdoptNewSession(state, "t-1", "STARTING")).toBe(true);
  });

  it("adopts when active session is already terminal", () => {
    const state = makeAppState({
      tasks: { activeTaskId: "t-1", activeSessionId: "s-old", pinnedSessionId: null },
      taskSessions: {
        items: { "s-old": { id: "s-old", task_id: "t-1", state: "COMPLETED" } },
      } as unknown as AppState["taskSessions"],
    });
    expect(shouldAdoptNewSession(state, "t-1", "STARTING")).toBe(true);
  });

  it("does NOT adopt while the current active session is still running", () => {
    const state = makeAppState({
      tasks: { activeTaskId: "t-1", activeSessionId: "s-old", pinnedSessionId: null },
      taskSessions: {
        items: { "s-old": { id: "s-old", task_id: "t-1", state: "RUNNING" } },
      } as unknown as AppState["taskSessions"],
    });
    expect(shouldAdoptNewSession(state, "t-1", "STARTING")).toBe(false);
  });

  it("does NOT adopt when the event is for a non-active task", () => {
    const state = makeAppState({
      tasks: { activeTaskId: "t-1", activeSessionId: null, pinnedSessionId: null },
    });
    expect(shouldAdoptNewSession(state, "t-2", "STARTING")).toBe(false);
  });

  it("does NOT adopt terminal state events", () => {
    const state = makeAppState({
      tasks: { activeTaskId: "t-1", activeSessionId: null, pinnedSessionId: null },
    });
    expect(shouldAdoptNewSession(state, "t-1", "COMPLETED")).toBe(false);
  });
});

describe("pickReplacementSessionId", () => {
  it("returns the newest non-terminal session in the per-task list", () => {
    const state = makeAppState({
      taskSessionsByTask: {
        itemsByTaskId: {
          "t-1": [
            { id: "s-1", task_id: "t-1", state: "COMPLETED", started_at: "", updated_at: "" },
            { id: "s-2", task_id: "t-1", state: "RUNNING", started_at: "", updated_at: "" },
            { id: "s-3", task_id: "t-1", state: "CANCELLED", started_at: "", updated_at: "" },
          ],
        },
      } as unknown as AppState["taskSessionsByTask"],
    });
    expect(pickReplacementSessionId(state, "t-1")).toBe("s-2");
  });

  it("returns null when all sessions are terminal", () => {
    const state = makeAppState({
      taskSessionsByTask: {
        itemsByTaskId: {
          "t-1": [
            { id: "s-1", task_id: "t-1", state: "COMPLETED", started_at: "", updated_at: "" },
            { id: "s-2", task_id: "t-1", state: "FAILED", started_at: "", updated_at: "" },
          ],
        },
      } as unknown as AppState["taskSessionsByTask"],
    });
    expect(pickReplacementSessionId(state, "t-1")).toBeNull();
  });

  it("returns null when the task has no sessions tracked", () => {
    expect(pickReplacementSessionId(makeAppState({}), "t-missing")).toBeNull();
  });
});

describe("session.state_changed → active session switching", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("adopts a newly-created session for the active task", () => {
    const store = makeStore({
      tasks: { activeTaskId: "t-1", activeSessionId: null, pinnedSessionId: null },
    });
    const handler = registerTaskSessionHandlers(store)[STATE_CHANGED_EVENT]!;

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
      tasks: { activeTaskId: "other-task", activeSessionId: null, pinnedSessionId: null },
    });
    const handler = registerTaskSessionHandlers(store)[STATE_CHANGED_EVENT]!;

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
      tasks: { activeTaskId: "t-1", activeSessionId: "s-old", pinnedSessionId: null },
      taskSessions: {
        items: { "s-old": { id: "s-old", task_id: "t-1", state: "RUNNING" } },
      },
    });
    const handler = registerTaskSessionHandlers(store)[STATE_CHANGED_EVENT]!;

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
      tasks: { activeTaskId: "t-1", activeSessionId: "s-old", pinnedSessionId: "s-old" },
      taskSessions: {
        items: { "s-old": { id: "s-old", task_id: "t-1", state: "COMPLETED" } },
      },
    });
    const handler = registerTaskSessionHandlers(store)[STATE_CHANGED_EVENT]!;

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
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("hands off when the current active session transitions to terminal", () => {
    const store = makeStore({
      tasks: { activeTaskId: "t-1", activeSessionId: "s-old", pinnedSessionId: null },
      taskSessions: {
        items: { "s-old": { id: "s-old", task_id: "t-1", state: "RUNNING" } },
      },
      taskSessionsByTask: {
        itemsByTaskId: {
          "t-1": [
            { id: "s-old", task_id: "t-1", state: "RUNNING", started_at: "", updated_at: "" },
            { id: "s-new", task_id: "t-1", state: "STARTING", started_at: "", updated_at: "" },
          ],
        },
      },
    });
    const handler = registerTaskSessionHandlers(store)[STATE_CHANGED_EVENT]!;

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
      tasks: { activeTaskId: "t-1", activeSessionId: "s-old", pinnedSessionId: null },
      taskSessions: {
        items: { "s-old": { id: "s-old", task_id: "t-1", state: "RUNNING" } },
      },
      taskSessionsByTask: {
        itemsByTaskId: {
          "t-1": [
            { id: "s-old", task_id: "t-1", state: "RUNNING", started_at: "", updated_at: "" },
          ],
        },
      },
    });
    const handler = registerTaskSessionHandlers(store)[STATE_CHANGED_EVENT]!;

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
      tasks: { activeTaskId: "t-1", activeSessionId: "s-old", pinnedSessionId: null },
      taskSessions: {
        items: { "s-old": { id: "s-old", task_id: "t-1", state: "RUNNING" } },
      },
      taskSessionsByTask: {
        itemsByTaskId: {
          "t-1": [
            { id: "s-done", task_id: "t-1", state: "COMPLETED", started_at: "", updated_at: "" },
            { id: "s-old", task_id: "t-1", state: "RUNNING", started_at: "", updated_at: "" },
          ],
        },
      },
    });
    const handler = registerTaskSessionHandlers(store)[STATE_CHANGED_EVENT]!;

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
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("does NOT hand off when the user pinned the session that just terminated", () => {
    // User manually clicked s-old, so pinnedSessionId === "s-old".
    // When s-old terminates we should respect the pin and stay on it.
    const store = makeStore({
      tasks: { activeTaskId: "t-1", activeSessionId: "s-old", pinnedSessionId: "s-old" },
      taskSessions: {
        items: { "s-old": { id: "s-old", task_id: "t-1", state: "RUNNING" } },
      },
      taskSessionsByTask: {
        itemsByTaskId: {
          "t-1": [
            { id: "s-old", task_id: "t-1", state: "RUNNING", started_at: "", updated_at: "" },
            { id: "s-new", task_id: "t-1", state: "STARTING", started_at: "", updated_at: "" },
          ],
        },
      },
    });
    const handler = registerTaskSessionHandlers(store)[STATE_CHANGED_EVENT]!;

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
