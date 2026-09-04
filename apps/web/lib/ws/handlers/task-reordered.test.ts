import { describe, it, expect } from "vitest";
import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import { registerKanbanHandlers } from "./kanban";

// kanban.test.ts is already at the file-length limit, so task.reordered gets
// its own file rather than growing that one further.

const WORKFLOW_ID = "wf1";
const STEP_ID = "step-1";

type FakeTask = { id: string; workflowStepId: string; position: number };

function makeTask(id: string, position: number): FakeTask {
  return { id, workflowStepId: STEP_ID, position };
}

function makeStore(overrides: {
  tasks?: FakeTask[];
  orderRevisionByStepId?: Record<string, number>;
  pendingReorderBandKeys?: Record<string, true>;
}) {
  const tasks = overrides.tasks ?? [makeTask("a", 0), makeTask("b", 1), makeTask("c", 2)];
  let state = {
    kanban: { workflowId: WORKFLOW_ID, steps: [], tasks },
    kanbanMulti: {
      isLoading: false,
      orderRevisionByStepId: overrides.orderRevisionByStepId ?? {},
      pendingReorderBandKeys: overrides.pendingReorderBandKeys ?? {},
      snapshots: {
        [WORKFLOW_ID]: { workflowId: WORKFLOW_ID, workflowName: "WF1", steps: [], tasks },
      },
    },
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

function makeReorderedMessage(
  revision: number,
  tasks: Array<{ id: string; position: number }>,
  band: "admitted" | "queued" = "admitted",
) {
  return {
    id: "msg-1",
    type: "notification" as const,
    action: "task.reordered" as const,
    payload: { workflow_step_id: STEP_ID, band, revision, tasks },
  };
}

describe("task.reordered handler", () => {
  it("applies positions to kanban.tasks and the matching snapshot, and records the revision", () => {
    const store = makeStore({});
    const handler = registerKanbanHandlers(store)["task.reordered"]!;

    handler(
      makeReorderedMessage(1, [
        { id: "c", position: 0 },
        { id: "a", position: 1 },
        { id: "b", position: 2 },
      ]),
    );

    const tasks = store.getState().kanban.tasks;
    expect(tasks.find((t) => t.id === "c")?.position).toBe(0);
    expect(tasks.find((t) => t.id === "a")?.position).toBe(1);
    expect(tasks.find((t) => t.id === "b")?.position).toBe(2);
    const snapshotTasks = store.getState().kanbanMulti.snapshots[WORKFLOW_ID]?.tasks;
    expect(snapshotTasks?.find((t) => t.id === "c")?.position).toBe(0);
    expect(store.getState().kanbanMulti.orderRevisionByStepId[STEP_ID]).toBe(1);
  });

  it("ignores an event whose revision is not strictly greater than the recorded one", () => {
    const store = makeStore({ orderRevisionByStepId: { [STEP_ID]: 5 } });
    const handler = registerKanbanHandlers(store)["task.reordered"]!;

    handler(makeReorderedMessage(5, [{ id: "a", position: 2 }]));

    expect(store.getState().kanban.tasks.find((t) => t.id === "a")?.position).toBe(0);
    expect(store.getState().kanbanMulti.orderRevisionByStepId[STEP_ID]).toBe(5);
  });

  it("applies the first order it ever receives for a step with no recorded revision", () => {
    const store = makeStore({});
    const handler = registerKanbanHandlers(store)["task.reordered"]!;

    handler(makeReorderedMessage(0, [{ id: "a", position: 9 }]));

    expect(store.getState().kanban.tasks.find((t) => t.id === "a")?.position).toBe(9);
    expect(store.getState().kanbanMulti.orderRevisionByStepId[STEP_ID]).toBe(0);
  });

  it("does not apply an event for a band with a request currently in flight (AC.27)", () => {
    const store = makeStore({ pendingReorderBandKeys: { [`${STEP_ID}:admitted`]: true } });
    const handler = registerKanbanHandlers(store)["task.reordered"]!;

    handler(makeReorderedMessage(1, [{ id: "a", position: 9 }]));

    expect(store.getState().kanban.tasks.find((t) => t.id === "a")?.position).toBe(0);
    expect(store.getState().kanbanMulti.orderRevisionByStepId[STEP_ID]).toBeUndefined();
  });

  it("still applies to a different band on the same step while the other band is pending", () => {
    const store = makeStore({ pendingReorderBandKeys: { [`${STEP_ID}:queued`]: true } });
    const handler = registerKanbanHandlers(store)["task.reordered"]!;

    handler(makeReorderedMessage(1, [{ id: "a", position: 9 }], "admitted"));

    expect(store.getState().kanban.tasks.find((t) => t.id === "a")?.position).toBe(9);
  });
});
