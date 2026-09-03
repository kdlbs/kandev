import { describe, it, expect } from "vitest";
import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import { registerKanbanHandlers } from "./kanban";

// kanban.test.ts is already at the file-length limit, so this regression gets
// its own file rather than growing that one further (see
// kanban-auto-start-failed.test.ts for the same convention).

const WORKFLOW_ID = "wf1";
const TASK_ID = "t1";
const STEP_ID = "s1";
const TASK_TITLE = "T1";
const UPDATED_TITLE = "T1 updated";

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

describe("kanban.update handler — priority preservation", () => {
  it("preserves priority from existing tasks when the update omits it", () => {
    const store = makeStore({
      kanban: {
        workflowId: WORKFLOW_ID,
        steps: [],
        tasks: [
          {
            id: TASK_ID,
            workflowId: WORKFLOW_ID,
            workflowStepId: STEP_ID,
            title: TASK_TITLE,
            position: 0,
            priority: "critical",
          },
        ],
      },
    } as Partial<AppState>);

    const handler = registerKanbanHandlers(store)["kanban.update"]!;
    handler(
      makeUpdateMessage(WORKFLOW_ID, [
        { id: TASK_ID, workflowStepId: STEP_ID, title: UPDATED_TITLE, position: 0 },
      ]),
    );

    const task = store.getState().kanban.tasks.find((t) => t.id === TASK_ID);
    expect(task?.priority).toBe("critical");
  });

  it("applies a new priority carried by the update", () => {
    const store = makeStore({
      kanban: {
        workflowId: WORKFLOW_ID,
        steps: [],
        tasks: [
          {
            id: TASK_ID,
            workflowId: WORKFLOW_ID,
            workflowStepId: STEP_ID,
            title: TASK_TITLE,
            position: 0,
            priority: "low",
          },
        ],
      },
    } as Partial<AppState>);

    const handler = registerKanbanHandlers(store)["kanban.update"]!;
    handler(
      makeUpdateMessage(WORKFLOW_ID, [
        {
          id: TASK_ID,
          workflowStepId: STEP_ID,
          title: TASK_TITLE,
          position: 0,
          priority: "high",
        },
      ]),
    );

    const task = store.getState().kanban.tasks.find((t) => t.id === TASK_ID);
    expect(task?.priority).toBe("high");
  });

  it("applies an explicit null priority instead of falling back to the existing value", () => {
    const store = makeStore({
      kanban: {
        workflowId: WORKFLOW_ID,
        steps: [],
        tasks: [
          {
            id: TASK_ID,
            workflowId: WORKFLOW_ID,
            workflowStepId: STEP_ID,
            title: TASK_TITLE,
            position: 0,
            priority: "critical",
          },
        ],
      },
    } as Partial<AppState>);

    const handler = registerKanbanHandlers(store)["kanban.update"]!;
    handler(
      makeUpdateMessage(WORKFLOW_ID, [
        { id: TASK_ID, workflowStepId: STEP_ID, title: UPDATED_TITLE, position: 0, priority: null },
      ]),
    );

    const task = store.getState().kanban.tasks.find((t) => t.id === TASK_ID);
    expect(task?.priority).toBeNull();
  });

  it("preserves priority in kanbanMulti snapshot", () => {
    const store = makeStore({
      kanban: { workflowId: WORKFLOW_ID, steps: [], tasks: [] },
      kanbanMulti: {
        isLoading: false,
        snapshots: {
          [WORKFLOW_ID]: {
            workflowId: WORKFLOW_ID,
            workflowName: "WF1",
            steps: [],
            tasks: [
              {
                id: TASK_ID,
                workflowId: WORKFLOW_ID,
                workflowStepId: STEP_ID,
                title: TASK_TITLE,
                position: 0,
                priority: "critical",
              },
            ],
          },
        },
      },
    } as Partial<AppState>);

    const handler = registerKanbanHandlers(store)["kanban.update"]!;
    handler(
      makeUpdateMessage(WORKFLOW_ID, [
        { id: TASK_ID, workflowStepId: STEP_ID, title: TASK_TITLE, position: 0 },
      ]),
    );

    const snapshot = store.getState().kanbanMulti.snapshots[WORKFLOW_ID];
    const task = snapshot?.tasks.find((t) => t.id === TASK_ID);
    expect(task?.priority).toBe("critical");
  });

  it("preserves priority from the snapshot's own cached task when the update omits it and the single-workflow lookup misses", () => {
    const store = makeStore({
      kanban: { workflowId: null, steps: [], tasks: [] },
      kanbanMulti: {
        snapshots: {
          [WORKFLOW_ID]: {
            workflowId: WORKFLOW_ID,
            workflowName: "Workflow",
            steps: [],
            tasks: [
              {
                id: TASK_ID,
                workflowId: WORKFLOW_ID,
                workflowStepId: STEP_ID,
                title: TASK_TITLE,
                position: 0,
                priority: "critical",
              },
            ],
          },
        },
        isLoading: false,
      },
    } as Partial<AppState>);

    const handler = registerKanbanHandlers(store)["kanban.update"]!;
    handler(
      makeUpdateMessage(WORKFLOW_ID, [
        { id: TASK_ID, workflowStepId: STEP_ID, title: UPDATED_TITLE, position: 0 },
      ]),
    );

    const task = store
      .getState()
      .kanbanMulti.snapshots[WORKFLOW_ID]?.tasks.find((t) => t.id === TASK_ID);
    expect(task?.priority).toBe("critical");
  });
});
