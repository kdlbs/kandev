import { describe, expect, it, vi } from "vitest";
import type { StoreApi } from "zustand";
import { registerTaskSessionHandlers } from "./agent-session";
import type { AppState } from "@/lib/state/store";
import type { TaskSessionStateChangedPayload } from "@/lib/types/backend";
import { sessionId, taskId, type TaskSessionState } from "@/lib/types/http";

const STATE_CHANGED_EVENT = "session.state_changed";
const TASK_TITLE = "Running task";

function makeStore(
  overrides: Partial<AppState> = {},
  beforeApplyState?: (state: AppState) => void,
) {
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
    beforeApplyState?.(state);
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

function makeKanbanTask(primarySessionId: string) {
  return {
    id: "t-1",
    workflowStepId: "step-1",
    title: TASK_TITLE,
    position: 0,
    primarySessionId,
    primarySessionState: "WAITING_FOR_INPUT",
  };
}

function makeTaskSession(id: string) {
  return {
    id: sessionId(id),
    task_id: taskId("t-1"),
    state: "WAITING_FOR_INPUT" as TaskSessionState,
    started_at: "",
    updated_at: "",
  };
}

function makeSnapshotTaskStore(primarySessionId: string) {
  const task = makeKanbanTask(primarySessionId);
  return {
    isLoading: false,
    snapshots: {
      "wf-1": {
        workflowId: "wf-1",
        workflowName: "Development",
        steps: [],
        tasks: [task],
      },
    },
  };
}

describe("session.state_changed -> primary kanban card state", () => {
  it("updates kanban card primary session state in workflow snapshots", () => {
    const task = makeKanbanTask("s-1");
    const store = makeStore({
      kanban: {
        workflowId: "wf-1",
        steps: [],
        tasks: [task],
      },
      kanbanMulti: makeSnapshotTaskStore("s-1"),
      taskSessions: {
        items: {
          "s-1": makeTaskSession("s-1"),
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

  it("logs the previous primary state when the task only exists in a workflow snapshot", () => {
    const debugSpy = vi.spyOn(console, "debug").mockImplementation(() => undefined);
    const store = makeStore({
      kanban: {
        workflowId: "wf-other",
        steps: [],
        tasks: [],
      },
      kanbanMulti: makeSnapshotTaskStore("s-1"),
      taskSessions: {
        items: {
          "s-1": makeTaskSession("s-1"),
        },
      },
    });
    const handler = registerTaskSessionHandlers(store)[STATE_CHANGED_EVENT]!;

    handler(makeMessage({ task_id: "t-1", session_id: "s-1", new_state: "RUNNING" }));

    expect(
      debugSpy.mock.calls.some((call) =>
        String(call[0]).includes("beforeTaskPrimaryState=WAITING_FOR_INPUT"),
      ),
    ).toBe(true);
    debugSpy.mockRestore();
  });

  it("preserves concurrent store changes while syncing kanban session state", () => {
    const task = makeKanbanTask("s-1");
    const store = makeStore(
      {
        kanban: {
          workflowId: "wf-1",
          steps: [],
          tasks: [task],
        },
        kanbanMulti: makeSnapshotTaskStore("s-1"),
        taskSessions: {
          items: {
            "s-1": makeTaskSession("s-1"),
          },
        },
      },
      (state) => {
        state.tasks = {
          ...state.tasks,
          activeTaskId: taskId("concurrent-task"),
        };
      },
    );
    const handler = registerTaskSessionHandlers(store)[STATE_CHANGED_EVENT]!;

    handler(makeMessage({ task_id: "t-1", session_id: "s-1", new_state: "RUNNING" }));

    expect(store.getState().kanban.tasks[0]?.primarySessionState).toBe("RUNNING");
    expect(store.getState().tasks.activeTaskId).toBe(taskId("concurrent-task"));
  });
});

describe("session.state_changed -> non-primary kanban card state", () => {
  it("does not update kanban cards for non-primary session transitions", () => {
    const store = makeStore({
      kanban: {
        workflowId: "wf-1",
        steps: [],
        tasks: [
          {
            id: "t-1",
            workflowStepId: "step-1",
            title: TASK_TITLE,
            position: 0,
            primarySessionId: "s-primary",
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
                title: TASK_TITLE,
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

    expect(store.getState().kanban.tasks[0]?.primarySessionState).toBe("WAITING_FOR_INPUT");
    expect(store.getState().kanbanMulti.snapshots["wf-1"]?.tasks[0]?.primarySessionState).toBe(
      "WAITING_FOR_INPUT",
    );
  });
});

describe("session.state_changed -> kanban sync guards", () => {
  it("does not write kanban state when the event has no new state", () => {
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

    handler(makeMessage({ task_id: "t-1", session_id: "s-1" }));

    expect(store.setState).not.toHaveBeenCalled();
  });

  it("keeps task arrays by reference when the primary state is already current", () => {
    const kanbanTasks = [
      {
        id: "t-1",
        workflowStepId: "step-1",
        title: TASK_TITLE,
        position: 0,
        primarySessionId: "s-1",
        primarySessionState: "RUNNING",
      },
    ];
    const snapshotTasks = [...kanbanTasks];
    const store = makeStore({
      kanban: { workflowId: "wf-1", steps: [], tasks: kanbanTasks },
      kanbanMulti: {
        isLoading: false,
        snapshots: {
          "wf-1": {
            workflowId: "wf-1",
            workflowName: "Development",
            steps: [],
            tasks: snapshotTasks,
          },
        },
      },
      taskSessions: {
        items: {
          "s-1": {
            id: sessionId("s-1"),
            task_id: taskId("t-1"),
            state: "RUNNING",
            started_at: "",
            updated_at: "",
          },
        },
      },
    });
    const handler = registerTaskSessionHandlers(store)[STATE_CHANGED_EVENT]!;

    handler(makeMessage({ task_id: "t-1", session_id: "s-1", new_state: "RUNNING" }));

    expect(store.getState().kanban.tasks).toBe(kanbanTasks);
    expect(store.getState().kanbanMulti.snapshots["wf-1"]?.tasks).toBe(snapshotTasks);
  });

  it("does not write kanban state for stale session events", () => {
    const store = makeStore({
      taskSessions: {
        items: {
          "s-1": {
            id: sessionId("s-1"),
            task_id: taskId("t-1"),
            state: "WAITING_FOR_INPUT",
            started_at: "",
            updated_at: "2026-06-18T12:00:00.000Z",
          },
        },
      },
    });
    const handler = registerTaskSessionHandlers(store)[STATE_CHANGED_EVENT]!;

    handler(
      makeMessage({
        task_id: "t-1",
        session_id: "s-1",
        new_state: "RUNNING",
        updated_at: "2026-06-18T11:00:00.000Z",
      }),
    );

    expect(store.setState).not.toHaveBeenCalled();
  });
});
