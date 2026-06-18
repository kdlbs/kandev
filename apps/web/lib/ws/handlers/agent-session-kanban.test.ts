import { describe, expect, it, vi } from "vitest";
import type { StoreApi } from "zustand";
import { registerTaskSessionHandlers } from "./agent-session";
import type { AppState } from "@/lib/state/store";
import type { TaskSessionStateChangedPayload } from "@/lib/types/backend";
import { sessionId, taskId } from "@/lib/types/http";

const STATE_CHANGED_EVENT = "session.state_changed";

function makeStore(overrides: Partial<AppState> = {}) {
  const state = {
    kanban: { workflowId: null, steps: [], tasks: [] },
    kanbanMulti: { snapshots: {}, isLoading: false },
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

describe("session.state_changed -> kanban primary session state", () => {
  it("updates kanban card primary session state in workflow snapshots", () => {
    const store = makeStore({
      kanban: {
        workflowId: "wf-1",
        steps: [],
        tasks: [
          {
            id: "t-1",
            workflowStepId: "step-1",
            title: "Running task",
            position: 0,
            primarySessionId: "s-1",
            primarySessionState: "WAITING_FOR_INPUT",
          },
        ],
      },
      kanbanMulti: {
        isLoading: false,
        snapshots: {
          "wf-1": {
            workflowId: "wf-1",
            workflowName: "Development",
            steps: [],
            tasks: [
              {
                id: "t-1",
                workflowStepId: "step-1",
                title: "Running task",
                position: 0,
                primarySessionId: "s-1",
                primarySessionState: "WAITING_FOR_INPUT",
              },
            ],
          },
        },
      },
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

    expect(store.getState().kanban.tasks[0]?.primarySessionState).toBe("RUNNING");
    expect(store.getState().kanbanMulti.snapshots["wf-1"]?.tasks[0]?.primarySessionState).toBe(
      "RUNNING",
    );
  });

  it("does not update kanban cards for non-primary session transitions", () => {
    const store = makeStore({
      kanbanMulti: {
        isLoading: false,
        snapshots: {
          "wf-1": {
            workflowId: "wf-1",
            workflowName: "Development",
            steps: [],
            tasks: [
              {
                id: "t-1",
                workflowStepId: "step-1",
                title: "Running task",
                position: 0,
                primarySessionId: "s-primary",
                primarySessionState: "WAITING_FOR_INPUT",
              },
            ],
          },
        },
      },
      taskSessions: {
        items: {
          "s-secondary": {
            id: sessionId("s-secondary"),
            task_id: taskId("t-1"),
            state: "WAITING_FOR_INPUT",
            started_at: "",
            updated_at: "",
          },
        },
      },
    });
    const handler = registerTaskSessionHandlers(store)[STATE_CHANGED_EVENT]!;

    handler(makeMessage({ task_id: "t-1", session_id: "s-secondary", new_state: "RUNNING" }));

    expect(store.getState().kanbanMulti.snapshots["wf-1"]?.tasks[0]?.primarySessionState).toBe(
      "WAITING_FOR_INPUT",
    );
  });
});
