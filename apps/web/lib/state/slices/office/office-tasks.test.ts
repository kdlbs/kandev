import { describe, expect, it } from "vitest";
import { create } from "zustand";
import { immer } from "zustand/middleware/immer";
import { createOfficeSlice } from "./office-slice";
import type { OfficeSlice, OfficeTask } from "./types";

function makeStore() {
  return create<OfficeSlice>()(
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    immer((...a) => ({ ...(createOfficeSlice as any)(...a) })),
  );
}

function makeTask(overrides?: Partial<OfficeTask>): OfficeTask {
  return {
    id: "task-1",
    workspaceId: "ws-1",
    identifier: "OFC-1",
    title: "Test Task",
    status: "todo",
    priority: "medium",
    createdAt: "2026-01-01T00:00:00Z",
    updatedAt: "2026-01-01T00:00:00Z",
    ...overrides,
  };
}

describe("office task actions — appendTasks", () => {
  it("appends to an empty store", () => {
    const store = makeStore();
    store.getState().appendTasks([makeTask(), makeTask({ id: "task-2" })]);
    expect(store.getState().office.tasks.items).toHaveLength(2);
    expect(store.getState().office.tasks.items.map((t) => t.id)).toEqual(["task-1", "task-2"]);
  });

  it("appends new tasks alongside existing ones", () => {
    const store = makeStore();
    store.getState().setTasks([makeTask({ id: "task-1" })]);
    store.getState().appendTasks([makeTask({ id: "task-2" }), makeTask({ id: "task-3" })]);
    expect(store.getState().office.tasks.items.map((t) => t.id)).toEqual([
      "task-1",
      "task-2",
      "task-3",
    ]);
  });

  it("skips tasks already present in the store", () => {
    const store = makeStore();
    store.getState().setTasks([makeTask({ id: "task-1", title: "Original" })]);
    store
      .getState()
      .appendTasks([makeTask({ id: "task-1", title: "Replacement" }), makeTask({ id: "task-2" })]);
    expect(store.getState().office.tasks.items).toHaveLength(2);
    expect(store.getState().office.tasks.items[0].title).toBe("Original");
    expect(store.getState().office.tasks.items[1].id).toBe("task-2");
  });

  it("skips duplicate ids within the incoming batch itself", () => {
    const store = makeStore();
    store
      .getState()
      .appendTasks([
        makeTask({ id: "task-1", title: "First" }),
        makeTask({ id: "task-1", title: "Duplicate" }),
        makeTask({ id: "task-2" }),
      ]);
    expect(store.getState().office.tasks.items).toHaveLength(2);
    expect(store.getState().office.tasks.items[0].title).toBe("First");
  });

  it("is a no-op for an empty incoming batch", () => {
    const store = makeStore();
    store.getState().setTasks([makeTask()]);
    store.getState().appendTasks([]);
    expect(store.getState().office.tasks.items).toHaveLength(1);
  });
});
