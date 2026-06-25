import { describe, expect, it, vi } from "vitest";
import type { StoreApi } from "zustand";
import { registerTaskSessionHandlers } from "./agent-session";
import type { AppState } from "@/lib/state/store";
import type { TaskSessionStateChangedPayload } from "@/lib/types/backend";
import { sessionId, taskId } from "@/lib/types/http";

const STATE_CHANGED_EVENT = "session.state_changed";

function makeStore(overrides: Partial<AppState> = {}) {
  const state = {
    tasks: {
      activeTaskId: null,
      activeSessionId: null,
      pinnedSessionId: null,
      lastSessionByTaskId: {},
    },
    taskSessions: { items: {} },
    taskSessionsByTask: { itemsByTaskId: {} },
    upsertTaskSessionFromEvent: vi.fn(),
    setActiveSessionAuto: vi.fn(),
    setSessionAgentctlStatus: vi.fn(),
    setSessionFailureNotification: vi.fn(),
    setContextWindow: vi.fn(),
    ...overrides,
  } as unknown as AppState;
  const setState = vi.fn((updater: unknown) => {
    const next = typeof updater === "function" ? updater(state) : updater;
    if (next && next !== state) Object.assign(state, next);
  });
  return {
    getState: () => state,
    setState,
    subscribe: vi.fn(),
    destroy: vi.fn(),
    getInitialState: vi.fn(),
  } as unknown as StoreApi<AppState>;
}

function makeMessage(payload: TaskSessionStateChangedPayload) {
  return {
    id: "msg-1",
    type: "notification" as const,
    action: "session.state_changed" as const,
    payload,
  };
}

describe("session.state_changed legacy kanban mirrors", () => {
  it("does not expose kanban or kanbanMulti; Query bridge owns primary card state", () => {
    const store = makeStore({
      taskSessions: {
        items: {
          "s-1": {
            id: sessionId("s-1"),
            task_id: taskId("t-1"),
            state: "WAITING_FOR_INPUT",
            started_at: "",
            updated_at: "",
          },
        },
      },
    });
    const handler = registerTaskSessionHandlers(store)[STATE_CHANGED_EVENT]!;

    handler(makeMessage({ task_id: "t-1", session_id: "s-1", new_state: "RUNNING" }));

    expect("kanban" in store.getState()).toBe(false);
    expect("kanbanMulti" in store.getState()).toBe(false);
  });
});
