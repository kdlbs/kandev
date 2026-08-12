import { describe, it, expect } from "vitest";
import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import { registerKanbanHandlers } from "./kanban";

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

// AC-58a: a kanban.update payload never carries parked_on_background_work (it
// doesn't carry primarySessionId/foregroundActivity either — see the comment
// in kanban.ts), so the parked affordance must survive the rebuild in BOTH
// projections, exactly as foregroundActivity already does
// (kanban.test.ts's "foregroundActivity preservation" describe block).
describe("kanban.update handler — parkedOnBackgroundWork preservation (AC-58a)", () => {
  it("preserves parkedOnBackgroundWork from existing tasks in both projections", () => {
    const store = makeStore({
      kanban: {
        workflowId: WORKFLOW_ID,
        steps: [],
        tasks: [
          {
            id: TASK_ID,
            workflowStepId: STEP_ID,
            title: TASK_TITLE,
            position: 0,
            parkedOnBackgroundWork: true,
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
                workflowStepId: STEP_ID,
                title: TASK_TITLE,
                position: 0,
                parkedOnBackgroundWork: true,
              },
            ],
          },
        },
      },
    } as unknown as Partial<AppState>);

    const handler = registerKanbanHandlers(store)["kanban.update"]!;
    handler(
      makeUpdateMessage(WORKFLOW_ID, [
        { id: TASK_ID, workflowStepId: STEP_ID, title: TASK_TITLE, position: 0 },
      ]),
    );

    const task = store.getState().kanban.tasks.find((t) => t.id === TASK_ID);
    expect(task?.parkedOnBackgroundWork).toBe(true);

    const snapshotTask = store
      .getState()
      .kanbanMulti.snapshots[WORKFLOW_ID]?.tasks.find((t) => t.id === TASK_ID);
    expect(snapshotTask?.parkedOnBackgroundWork).toBe(true);
  });

  it("falls back to the multi-snapshot's own value only when the main lookup is undefined", () => {
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
                workflowStepId: STEP_ID,
                title: TASK_TITLE,
                position: 0,
                parkedOnBackgroundWork: true,
              },
            ],
          },
        },
      },
    } as unknown as Partial<AppState>);

    const handler = registerKanbanHandlers(store)["kanban.update"]!;
    handler(
      makeUpdateMessage(WORKFLOW_ID, [
        { id: TASK_ID, workflowStepId: STEP_ID, title: TASK_TITLE, position: 0 },
      ]),
    );

    const snapshotTask = store
      .getState()
      .kanbanMulti.snapshots[WORKFLOW_ID]?.tasks.find((t) => t.id === TASK_ID);
    expect(snapshotTask?.parkedOnBackgroundWork).toBe(true);
  });
});
