import { describe, it, expect } from "vitest";
import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import { registerKanbanHandlers } from "./kanban";

// Mirrors kanban-auto-start-failed.test.ts's regression shape for the third
// board marker: kanban.update's two producer sites (the main kanban.tasks
// rebuild and the kanbanMulti snapshot merge) must both carry workspaceOrphaned
// forward, or a badge set by a live task.updated -> kanban.update replay
// disappears the next time the board resynced its step/task list.

const WORKFLOW_ID = "wf1";
const TASK_ID = "t1";
const STEP_ID = "s1";
const TASK_TITLE = "T1";

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

describe("kanban.update handler — workspaceOrphaned preservation", () => {
  it("preserves workspaceOrphaned from existing tasks", () => {
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
            workspaceOrphaned: true,
          },
        ],
      },
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
                workspaceOrphaned: true,
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

    const task = store.getState().kanban.tasks.find((t) => t.id === TASK_ID);
    expect(task?.workspaceOrphaned).toBe(true);

    const snapshotTask = store
      .getState()
      .kanbanMulti.snapshots[WORKFLOW_ID]?.tasks.find((t) => t.id === TASK_ID);
    expect(snapshotTask?.workspaceOrphaned).toBe(true);
  });

  it("does not restore stale snapshot value when a kanban.update run clears workspaceOrphaned", () => {
    // A snapshot's own cached workspaceOrphaned (true) must not resurrect the
    // badge once the primary kanban list has moved on without it (false): the
    // kanbanMulti merge only falls back to the snapshot's value when the
    // primary lookup is `undefined` (task absent), not when it resolves to an
    // explicit false.
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
            workspaceOrphaned: false,
          },
        ],
      },
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
                workspaceOrphaned: true,
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

    const snapshotTask = store
      .getState()
      .kanbanMulti.snapshots[WORKFLOW_ID]?.tasks.find((t) => t.id === TASK_ID);
    expect(snapshotTask?.workspaceOrphaned).toBe(false);
  });
});
