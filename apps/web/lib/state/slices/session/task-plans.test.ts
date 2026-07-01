import { beforeEach, describe, it, expect, vi } from "vitest";
import { create } from "zustand";
import { immer } from "zustand/middleware/immer";
import { createSessionSlice } from "./session-slice";
import type { SessionSlice } from "./types";

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
const TS_LATER = "2026-04-20T01:00:00Z";

describe("task plan slice", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetPlanLastSeen.mockReturnValue(null);
  });

  it("markTaskPlanSeen writes the provided plan updated_at", () => {
    const store = makeStore();

    store.getState().markTaskPlanSeen(TASK_ID, TS_LATER);

    expect(store.getState().taskPlans.lastSeenUpdatedAtByTaskId[TASK_ID]).toBe(TS_LATER);
    expect(mockSetPlanLastSeen).toHaveBeenCalledWith(TASK_ID, TS_LATER);
  });

  it("markTaskPlanSeen with no plan writes an empty-string sentinel", () => {
    const store = makeStore();

    store.getState().markTaskPlanSeen("task-missing");

    expect(store.getState().taskPlans.lastSeenUpdatedAtByTaskId["task-missing"]).toBe("");
    expect(mockSetPlanLastSeen).toHaveBeenCalledWith("task-missing", "");
  });

  it("hydrates stored lastSeenUpdatedAtByTaskId", () => {
    mockGetPlanLastSeen.mockReturnValue(TS_LATER);
    const store = makeStore();

    store.getState().hydrateTaskPlanLastSeen(TASK_ID);

    expect(store.getState().taskPlans.lastSeenUpdatedAtByTaskId[TASK_ID]).toBe(TS_LATER);
  });

  it("does not rehydrate over explicit lastSeenUpdatedAtByTaskId", () => {
    const store = makeStore();
    store.getState().markTaskPlanSeen(TASK_ID, TS_LATER);
    mockGetPlanLastSeen.mockReturnValue("2026-04-20T02:00:00Z");

    store.getState().hydrateTaskPlanLastSeen(TASK_ID);

    expect(store.getState().taskPlans.lastSeenUpdatedAtByTaskId[TASK_ID]).toBe(TS_LATER);
  });

  it("does not expose task-plan DTO server-state through the session slice", () => {
    const state = makeStore().getState() as unknown as {
      taskPlans: Record<string, unknown>;
    } & Record<string, unknown>;

    expect("byTaskId" in state.taskPlans).toBe(false);
    expect("loadingByTaskId" in state.taskPlans).toBe(false);
    expect("loadedByTaskId" in state.taskPlans).toBe(false);
    expect("savingByTaskId" in state.taskPlans).toBe(false);
    expect("setTaskPlan" in state).toBe(false);
    expect("setTaskPlanLoading" in state).toBe(false);
    expect("setTaskPlanSaving" in state).toBe(false);
    expect("revisionsByTaskId" in state.taskPlans).toBe(false);
    expect("revisionsLoadingByTaskId" in state.taskPlans).toBe(false);
    expect("revisionsLoadedByTaskId" in state.taskPlans).toBe(false);
    expect("revisionContentCache" in state.taskPlans).toBe(false);
    expect("setPlanRevisions" in state).toBe(false);
    expect("upsertPlanRevision" in state).toBe(false);
    expect("setPlanRevisionsLoading" in state).toBe(false);
    expect("cachePlanRevisionContent" in state).toBe(false);
  });
});
