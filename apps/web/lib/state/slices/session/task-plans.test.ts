import { beforeEach, describe, it, expect, vi } from "vitest";
import { create } from "zustand";
import { immer } from "zustand/middleware/immer";
import { createSessionSlice } from "./session-slice";
import type { SessionSlice } from "./types";
import type { TaskPlan } from "@/lib/types/http";

const mockGetPlanLastSeen = vi.fn();
const mockSetPlanLastSeen = vi.fn();

vi.mock("@/lib/local-storage", () => ({
  getPlanLastSeen: (...args: unknown[]) => mockGetPlanLastSeen(...args),
  setPlanLastSeen: (...args: unknown[]) => mockSetPlanLastSeen(...args),
}));

function makeStore() {
  return create<SessionSlice>()(immer(createSessionSlice));
}

const TASK_ID = "task-1";
const TS_EPOCH = "2026-04-20T00:00:00Z";
const TS_LATER = "2026-04-20T01:00:00Z";
const TS_LATEST = "2026-04-20T02:00:00Z";

function makePlan(overrides: Partial<TaskPlan> = {}): TaskPlan {
  return {
    id: "plan-1",
    task_id: TASK_ID,
    title: "Plan",
    content: "# Plan",
    created_by: "agent",
    created_at: TS_EPOCH,
    updated_at: TS_EPOCH,
    ...overrides,
  };
}

describe("task plan slice", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetPlanLastSeen.mockReturnValue(null);
  });

  it("markTaskPlanSeen writes the current plan updated_at", () => {
    const store = makeStore();
    store.getState().setTaskPlan(TASK_ID, makePlan({ updated_at: TS_LATER }));

    store.getState().markTaskPlanSeen(TASK_ID);

    expect(store.getState().taskPlans.lastSeenUpdatedAtByTaskId[TASK_ID]).toBe(TS_LATER);
    expect(mockSetPlanLastSeen).toHaveBeenCalledWith(TASK_ID, TS_LATER);
  });

  it("markTaskPlanSeen with no plan writes an empty-string sentinel", () => {
    const store = makeStore();

    store.getState().markTaskPlanSeen("task-missing");

    expect(store.getState().taskPlans.lastSeenUpdatedAtByTaskId["task-missing"]).toBe("");
    expect(mockSetPlanLastSeen).toHaveBeenCalledWith("task-missing", "");
  });

  it("setTaskPlan hydrates stored lastSeenUpdatedAtByTaskId", () => {
    mockGetPlanLastSeen.mockReturnValue(TS_LATER);
    const store = makeStore();

    store.getState().setTaskPlan(TASK_ID, makePlan({ updated_at: TS_LATER }));

    expect(store.getState().taskPlans.lastSeenUpdatedAtByTaskId[TASK_ID]).toBe(TS_LATER);
  });

  it("setTaskPlan does not change lastSeenUpdatedAtByTaskId", () => {
    const store = makeStore();
    store.getState().setTaskPlan(TASK_ID, makePlan({ updated_at: TS_EPOCH }));
    store.getState().markTaskPlanSeen(TASK_ID);

    // New update arrives — seen should NOT advance automatically
    store.getState().setTaskPlan(TASK_ID, makePlan({ updated_at: TS_LATEST }));

    expect(store.getState().taskPlans.lastSeenUpdatedAtByTaskId[TASK_ID]).toBe(TS_EPOCH);
  });

  it("clearTaskPlan removes the lastSeen entry", () => {
    const store = makeStore();
    store.getState().setTaskPlan(TASK_ID, makePlan());
    store.getState().markTaskPlanSeen(TASK_ID);

    store.getState().clearTaskPlan(TASK_ID);

    expect(store.getState().taskPlans.lastSeenUpdatedAtByTaskId[TASK_ID]).toBeUndefined();
    expect(store.getState().taskPlans.byTaskId[TASK_ID]).toBeUndefined();
    expect(mockSetPlanLastSeen).toHaveBeenCalledWith(TASK_ID, null);
  });
});
