import { describe, it, expect, vi, beforeEach } from "vitest";
import { registerTaskSessionHandlers } from "./agent-session";
import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";

function makeStore(overrides: Record<string, unknown> = {}) {
  const state: Record<string, unknown> = {
    taskSessions: { items: {} },
    taskSessionsByTask: { itemsByTaskId: {} },
    setTaskSession: vi.fn(),
    setTaskSessionsForTask: vi.fn(),
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

function makeMessage(payload: Record<string, unknown>) {
  return { id: "msg-1", type: "notification", action: "session.state_changed", payload };
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
    handler = registerTaskSessionHandlers(store)["session.state_changed"]!;

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
    handler = registerTaskSessionHandlers(store)["session.state_changed"]!;

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

  it("sets failure notification for unknown session (first event)", () => {
    store = makeStore();
    handler = registerTaskSessionHandlers(store)["session.state_changed"]!;

    handler(
      makeMessage({
        task_id: "t-1",
        session_id: "s-new",
        new_state: "FAILED",
        error_message: "timeout",
      }),
    );

    expect(store.getState().setSessionFailureNotification).toHaveBeenCalledWith({
      sessionId: "s-new",
      taskId: "t-1",
      message: "timeout",
    });
  });

  it("respects suppress_toast flag", () => {
    store = makeStore({
      taskSessions: {
        items: { "s-1": { id: "s-1", task_id: "t-1", state: "STARTING" } },
      },
    });
    handler = registerTaskSessionHandlers(store)["session.state_changed"]!;

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
