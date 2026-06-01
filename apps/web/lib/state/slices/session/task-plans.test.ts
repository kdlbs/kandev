import { describe, it, expect } from "vitest";
import { create } from "zustand";
import { immer } from "zustand/middleware/immer";
import { createSessionSlice } from "./session-slice";
import type { SessionSlice } from "./types";

function makeStore() {
  return create<SessionSlice>()(immer(createSessionSlice));
}

const TASK_ID = "task-1";
const TS_EPOCH = "2026-04-20T00:00:00Z";
const TS_LATER = "2026-04-20T01:00:00Z";

describe("task plan client-state slice", () => {
  it("markTaskPlanSeen writes the supplied updated_at", () => {
    const store = makeStore();

    store.getState().markTaskPlanSeen(TASK_ID, TS_LATER);

    expect(store.getState().taskPlans.lastSeenUpdatedAtByTaskId[TASK_ID]).toBe(TS_LATER);
  });

  it("markTaskPlanSeen with no updated_at writes an empty-string sentinel", () => {
    const store = makeStore();

    store.getState().markTaskPlanSeen("task-missing");

    expect(store.getState().taskPlans.lastSeenUpdatedAtByTaskId["task-missing"]).toBe("");
  });

  it("markTaskPlanSeen only advances when called with a new value", () => {
    const store = makeStore();
    store.getState().markTaskPlanSeen(TASK_ID, TS_EPOCH);

    // No second mark — seen should NOT advance automatically
    expect(store.getState().taskPlans.lastSeenUpdatedAtByTaskId[TASK_ID]).toBe(TS_EPOCH);
  });

  it("clearTaskPlan removes the per-task client entries", () => {
    const store = makeStore();
    store.getState().markTaskPlanSeen(TASK_ID, TS_EPOCH);
    store.getState().setTaskPlanSaving(TASK_ID, true);

    store.getState().clearTaskPlan(TASK_ID);

    expect(store.getState().taskPlans.lastSeenUpdatedAtByTaskId[TASK_ID]).toBeUndefined();
    expect(store.getState().taskPlans.savingByTaskId[TASK_ID]).toBeUndefined();
  });
});
