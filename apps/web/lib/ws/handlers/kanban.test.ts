import { describe, it, expect } from "vitest";
import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import { registerKanbanHandlers } from "./kanban";

function makeStore(initial: Partial<AppState> = {}) {
  let state = {
    kanban: { workflowId: null, steps: [], tasks: [] },
    kanbanMulti: { snapshots: {}, isLoading: false },
    ...initial,
  } as unknown as AppState;

  return {
    getState: () => state,
    setState: (updater: AppState | ((s: AppState) => AppState)) => {
      state =
        typeof updater === "function" ? (updater as (s: AppState) => AppState)(state) : updater;
    },
    subscribe: () => () => {},
    destroy: () => {},
    getInitialState: () => state,
  } as unknown as StoreApi<AppState>;
}

// eslint-disable-next-line @typescript-eslint/no-explicit-any
function makeUpdateMessage(workflowId: string, tasks: unknown[], steps: unknown[] = []): any {
  return {
    id: "msg-1",
    type: "notification",
    action: "kanban.update",
    payload: { workflowId, tasks, steps },
  };
}

describe("kanban.update handler — primarySessionId preservation", () => {
  it("preserves primarySessionId from existing tasks", () => {
    const store = makeStore({
      kanban: {
        workflowId: "wf1",
        steps: [],
        tasks: [
          {
            id: "t1",
            workflowStepId: "s1",
            title: "T1",
            position: 0,
            primarySessionId: "sess-primary",
          },
        ],
      },
    } as Partial<AppState>);

    const handler = registerKanbanHandlers(store)["kanban.update"]!;
    handler(
      makeUpdateMessage("wf1", [
        { id: "t1", workflowStepId: "s1", title: "T1 updated", position: 0, state: "IN_PROGRESS" },
      ]),
    );

    const task = store.getState().kanban.tasks.find((t) => t.id === "t1");
    expect(task?.primarySessionId).toBe("sess-primary");
    expect(task?.title).toBe("T1 updated");
  });

  it("preserves primarySessionState from existing tasks", () => {
    const store = makeStore({
      kanban: {
        workflowId: "wf1",
        steps: [],
        tasks: [
          {
            id: "t1",
            workflowStepId: "s1",
            title: "T1",
            position: 0,
            primarySessionId: "sess-primary",
            primarySessionState: "RUNNING",
          },
        ],
      },
    } as Partial<AppState>);

    const handler = registerKanbanHandlers(store)["kanban.update"]!;
    handler(
      makeUpdateMessage("wf1", [{ id: "t1", workflowStepId: "s1", title: "T1", position: 0 }]),
    );

    const task = store.getState().kanban.tasks.find((t) => t.id === "t1");
    expect(task?.primarySessionState).toBe("RUNNING");
  });

  it("new tasks start with undefined primarySessionId", () => {
    const store = makeStore({
      kanban: { workflowId: "wf1", steps: [], tasks: [] },
    } as Partial<AppState>);

    const handler = registerKanbanHandlers(store)["kanban.update"]!;
    handler(
      makeUpdateMessage("wf1", [
        { id: "t-new", workflowStepId: "s1", title: "New Task", position: 0 },
      ]),
    );

    const task = store.getState().kanban.tasks.find((t) => t.id === "t-new");
    expect(task).toBeDefined();
    expect(task?.primarySessionId).toBeUndefined();
  });
});

describe("kanban.update handler — explicit-null primary preservation", () => {
  it("does not restore stale snapshot value when primarySessionId is explicitly cleared", () => {
    // Repro for the multi-snapshot null-preservation bug: when task.updated
    // clears primarySessionId to null in kanban.tasks, the multi-snapshot must
    // accept the null rather than fall back to a stale value.
    const store = makeStore({
      kanban: {
        workflowId: "wf1",
        steps: [],
        tasks: [
          {
            id: "t1",
            workflowStepId: "s1",
            title: "T1",
            position: 0,
            primarySessionId: null,
          },
        ],
      },
      kanbanMulti: {
        isLoading: false,
        snapshots: {
          wf1: {
            workflowId: "wf1",
            workflowName: "WF1",
            steps: [],
            tasks: [
              {
                id: "t1",
                workflowStepId: "s1",
                title: "T1",
                position: 0,
                primarySessionId: "stale-session",
              },
            ],
          },
        },
      },
    } as Partial<AppState>);

    const handler = registerKanbanHandlers(store)["kanban.update"]!;
    handler(
      makeUpdateMessage("wf1", [{ id: "t1", workflowStepId: "s1", title: "T1", position: 0 }]),
    );

    const snapshot = store.getState().kanbanMulti.snapshots["wf1"];
    const task = snapshot?.tasks.find((t) => t.id === "t1");
    expect(task?.primarySessionId).toBeNull();
  });
});

describe("kanban.update handler — multi-snapshot primary lookup", () => {
  it("preserves primarySessionId in kanbanMulti snapshot", () => {
    const store = makeStore({
      kanban: { workflowId: "wf1", steps: [], tasks: [] },
      kanbanMulti: {
        isLoading: false,
        snapshots: {
          wf1: {
            workflowId: "wf1",
            workflowName: "WF1",
            steps: [],
            tasks: [
              {
                id: "t1",
                workflowStepId: "s1",
                title: "T1",
                position: 0,
                primarySessionId: "sess-multi-primary",
              },
            ],
          },
        },
      },
    } as Partial<AppState>);

    const handler = registerKanbanHandlers(store)["kanban.update"]!;
    handler(
      makeUpdateMessage("wf1", [{ id: "t1", workflowStepId: "s1", title: "T1", position: 0 }]),
    );

    const snapshot = store.getState().kanbanMulti.snapshots["wf1"];
    const task = snapshot?.tasks.find((t) => t.id === "t1");
    expect(task?.primarySessionId).toBe("sess-multi-primary");
  });
});

describe("kanban.update handler — task filtering", () => {
  it("skips ephemeral tasks", () => {
    const store = makeStore({
      kanban: { workflowId: "wf1", steps: [], tasks: [] },
    } as Partial<AppState>);

    const handler = registerKanbanHandlers(store)["kanban.update"]!;
    handler(
      makeUpdateMessage("wf1", [
        { id: "t1", workflowStepId: "s1", title: "Real", position: 0 },
        {
          id: "t-ephemeral",
          workflowStepId: "s1",
          title: "Ephemeral",
          position: 1,
          is_ephemeral: true,
        },
      ]),
    );

    expect(store.getState().kanban.tasks).toHaveLength(1);
    expect(store.getState().kanban.tasks[0].id).toBe("t1");
  });
});
