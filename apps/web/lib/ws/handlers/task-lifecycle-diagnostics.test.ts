import { describe, expect, it, vi } from "vitest";
import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import { didPreservePrimaryState, registerTasksHandlers } from "./tasks";

type Listener = (state: AppState) => void;

function makeStore(initial: Partial<AppState> = {}) {
  let state = {
    kanban: { workflowId: "wf1", steps: [], tasks: [] },
    kanbanMulti: { snapshots: {}, isLoading: false },
    tasks: {
      activeTaskId: null,
      activeSessionId: null,
      pinnedSessionId: null,
      lastSessionByTaskId: {},
    },
    taskSessionsByTask: { itemsByTaskId: {}, loadedByTaskId: {}, loadingByTaskId: {} },
    environmentIdBySessionId: {},
    setActiveSessionAuto: vi.fn(),
    removeTaskFromSidebarPrefs: vi.fn(),
    ...initial,
  } as unknown as AppState;

  const listeners = new Set<Listener>();
  return {
    getState: () => state,
    setState: (updater: AppState | ((s: AppState) => AppState)) => {
      const next =
        typeof updater === "function" ? (updater as (s: AppState) => AppState)(state) : updater;
      state = { ...state, ...next };
      for (const l of listeners) l(state);
    },
    subscribe: (l: Listener) => {
      listeners.add(l);
      return () => listeners.delete(l);
    },
    destroy: vi.fn(),
    getInitialState: vi.fn(),
  } as unknown as StoreApi<AppState> & { getState: () => AppState };
}

function makeTask(id: string, primarySessionId: string | null, workflowId = "wf1") {
  return {
    task_id: id,
    workflow_id: workflowId,
    workflow_step_id: "step1",
    title: "Test",
    description: "",
    state: "IN_PROGRESS",
    primary_session_id: primarySessionId,
    is_ephemeral: false,
  } as Record<string, unknown>;
}

function makeUpdatedMessage(payload: Record<string, unknown>) {
  return {
    id: "msg-1",
    type: "notification" as const,
    action: "task.updated" as const,
    payload,
  } as Parameters<NonNullable<ReturnType<typeof registerTasksHandlers>["task.updated"]>>[0];
}

function makeCreatedMessage(payload: Record<string, unknown>) {
  return {
    id: "msg-1",
    type: "notification" as const,
    action: "task.created" as const,
    payload,
  } as Parameters<NonNullable<ReturnType<typeof registerTasksHandlers>["task.created"]>>[0];
}

function makeStateChangedMessage(payload: Record<string, unknown>) {
  return {
    id: "msg-1",
    type: "notification" as const,
    action: "task.state_changed" as const,
    payload,
  } as Parameters<NonNullable<ReturnType<typeof registerTasksHandlers>["task.state_changed"]>>[0];
}

const existingTask = {
  id: "t1",
  workflowStepId: "step1",
  title: "Old",
  position: 0,
  state: "IN_PROGRESS" as const,
  primarySessionId: "sess-1",
  primarySessionState: "RUNNING",
};

function taskWithPrimaryState(primarySessionState: string | undefined) {
  return {
    ...existingTask,
    primarySessionState,
  };
}

describe("didPreservePrimaryState", () => {
  it("detects only omitted payload primary state that stayed preserved", () => {
    expect(
      didPreservePrimaryState(
        { workflow_id: "wf1", primary_session_state: "RUNNING" },
        taskWithPrimaryState("COMPLETED"),
        taskWithPrimaryState("COMPLETED"),
      ),
    ).toBe(false);
    expect(
      didPreservePrimaryState(
        { workflow_id: "wf1" },
        taskWithPrimaryState(undefined),
        taskWithPrimaryState("RUNNING"),
      ),
    ).toBe(false);
    expect(
      didPreservePrimaryState(
        { workflow_id: "wf1" },
        taskWithPrimaryState("RUNNING"),
        taskWithPrimaryState("RUNNING"),
      ),
    ).toBe(true);
    expect(
      didPreservePrimaryState(
        { workflow_id: "wf1" },
        taskWithPrimaryState("RUNNING"),
        taskWithPrimaryState("COMPLETED"),
      ),
    ).toBe(false);
  });
});

describe("task lifecycle diagnostics", () => {
  it("upserts task state and primary session state into both kanban stores", () => {
    const store = makeStore({
      kanban: {
        workflowId: "wf1",
        steps: [],
        tasks: [existingTask],
      } as unknown as AppState["kanban"],
      kanbanMulti: {
        isLoading: false,
        snapshots: {
          wf1: { workflowId: "wf1", workflowName: "WF1", steps: [], tasks: [existingTask] },
        },
      } as unknown as AppState["kanbanMulti"],
    });

    registerTasksHandlers(store)["task.state_changed"]!(
      makeStateChangedMessage({
        ...makeTask("t1", "sess-1"),
        state: "REVIEW",
        primary_session_state: "WAITING_FOR_INPUT",
      }),
    );

    const state = store.getState();
    const kanbanTask = state.kanban.tasks.find((task) => task.id === "t1");
    const snapshotTask = state.kanbanMulti.snapshots.wf1.tasks.find((task) => task.id === "t1");
    expect(kanbanTask?.state).toBe("REVIEW");
    expect(kanbanTask?.primarySessionState).toBe("WAITING_FOR_INPUT");
    expect(snapshotTask?.state).toBe("REVIEW");
    expect(snapshotTask?.primarySessionState).toBe("WAITING_FOR_INPUT");
  });

  it("skips ephemeral task state changes", () => {
    const store = makeStore();

    registerTasksHandlers(store)["task.state_changed"]!(
      makeStateChangedMessage({
        ...makeTask("t-ephemeral", "sess-1"),
        is_ephemeral: true,
        state: "REVIEW",
      }),
    );

    expect(store.getState().kanban.tasks).toEqual([]);
  });

  it("skips ephemeral task creation", () => {
    const store = makeStore();

    registerTasksHandlers(store)["task.created"]!(
      makeCreatedMessage({
        ...makeTask("t-ephemeral", "sess-1"),
        is_ephemeral: true,
      }),
    );

    expect(store.getState().kanban.tasks).toEqual([]);
  });

  it("logs whether a task update preserved an omitted primary session state", () => {
    const debugSpy = vi.spyOn(console, "debug").mockImplementation(() => undefined);
    const store = makeStore({
      kanban: {
        workflowId: "wf1",
        steps: [],
        tasks: [existingTask],
      } as unknown as AppState["kanban"],
    });
    const handlers = registerTasksHandlers(store);

    handlers["task.updated"]!(makeUpdatedMessage(makeTask("t1", "sess-1")));
    handlers["task.updated"]!(
      makeUpdatedMessage({
        ...makeTask("t1", "sess-1"),
        primary_session_state: null,
      }),
    );

    const logs = debugSpy.mock.calls.map((call) => String(call[0]));
    expect(logs.some((line) => line.includes("preservedPrimaryState=true"))).toBe(true);
    expect(logs.some((line) => line.includes("payloadPrimarySessionState=null"))).toBe(true);
    expect(logs.some((line) => line.includes("preservedPrimaryState=false"))).toBe(true);
    debugSpy.mockRestore();
  });
});

describe("task primary-session clears", () => {
  it("does not preserve primary session fields when the payload explicitly clears them", () => {
    const store = makeStore({
      kanban: {
        workflowId: "wf1",
        steps: [],
        tasks: [existingTask],
      } as unknown as AppState["kanban"],
      kanbanMulti: {
        isLoading: false,
        snapshots: {
          wf1: { workflowId: "wf1", workflowName: "WF1", steps: [], tasks: [existingTask] },
        },
      } as unknown as AppState["kanbanMulti"],
    });

    registerTasksHandlers(store)["task.updated"]!(
      makeUpdatedMessage({
        ...makeTask("t1", null),
        primary_session_state: null,
      }),
    );

    const state = store.getState();
    const kanbanTask = state.kanban.tasks.find((task) => task.id === "t1");
    const snapshotTask = state.kanbanMulti.snapshots.wf1.tasks.find((task) => task.id === "t1");
    expect(kanbanTask?.primarySessionId).toBeUndefined();
    expect(kanbanTask?.primarySessionState).toBeUndefined();
    expect(snapshotTask?.primarySessionId).toBeUndefined();
    expect(snapshotTask?.primarySessionState).toBeUndefined();
  });
});
