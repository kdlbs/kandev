import { beforeEach, describe, it, expect, vi } from "vitest";
import { create } from "zustand";
import { immer } from "zustand/middleware/immer";
import { createSessionSlice } from "./session-slice";
import type { SessionSlice } from "./types";

const mockGetPlanLastSeen = vi.fn();
const mockSetPlanLastSeen = vi.fn();
const mockGetWalkthroughLastSeen = vi.fn();
const mockSetWalkthroughLastSeen = vi.fn();

vi.mock("@/lib/local-storage", () => ({
  getPlanLastSeen: (...args: unknown[]) => mockGetPlanLastSeen(...args),
  setPlanLastSeen: (...args: unknown[]) => mockSetPlanLastSeen(...args),
}));

vi.mock("@/lib/walkthrough-notification-storage", () => ({
  getWalkthroughLastSeen: (...args: unknown[]) => mockGetWalkthroughLastSeen(...args),
  setWalkthroughLastSeen: (...args: unknown[]) => mockSetWalkthroughLastSeen(...args),
}));

function makeStore() {
  return create<SessionSlice>()(immer(createSessionSlice));
}

const TASK_ID = "task-1";
const TS_EPOCH = "2026-04-20T00:00:00Z";
const TS_LATER = "2026-04-20T01:00:00Z";

describe("walkthrough slice", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetPlanLastSeen.mockReturnValue(null);
    mockGetWalkthroughLastSeen.mockReturnValue(null);
  });

  it("starts without server-owned walkthrough payload state", () => {
    const store = makeStore();

    expect("byTaskId" in store.getState().walkthroughs).toBe(false);
    expect(store.getState().walkthroughs.activeStepByTaskId).toEqual({});
  });

  it("setWalkthroughActiveStep clamps within [0, stepCount-1]", () => {
    const store = makeStore();

    store.getState().setWalkthroughActiveStep(TASK_ID, 99, 3);
    expect(store.getState().walkthroughs.activeStepByTaskId[TASK_ID]).toBe(2);

    store.getState().setWalkthroughActiveStep(TASK_ID, -5, 3);
    expect(store.getState().walkthroughs.activeStepByTaskId[TASK_ID]).toBe(0);
  });

  it("setWalkthroughActiveStep allows non-negative values when no step count is known", () => {
    const store = makeStore();

    store.getState().setWalkthroughActiveStep(TASK_ID, 2);

    expect(store.getState().walkthroughs.activeStepByTaskId[TASK_ID]).toBe(2);
  });

  it("markWalkthroughSeen records the supplied updated_at", () => {
    const store = makeStore();

    store.getState().markWalkthroughSeen(TASK_ID, TS_LATER);

    expect(store.getState().walkthroughs.lastSeenUpdatedAtByTaskId[TASK_ID]).toBe(TS_LATER);
    expect(mockSetWalkthroughLastSeen).toHaveBeenCalledWith(TASK_ID, TS_LATER);
  });

  it("hydrateWalkthroughLastSeen hydrates stored lastSeenUpdatedAtByTaskId", () => {
    mockGetWalkthroughLastSeen.mockReturnValue(TS_LATER);
    const store = makeStore();

    store.getState().hydrateWalkthroughLastSeen(TASK_ID);

    expect(store.getState().walkthroughs.lastSeenUpdatedAtByTaskId[TASK_ID]).toBe(TS_LATER);
  });

  it("hydrateWalkthroughLastSeen does not overwrite a locally marked timestamp", () => {
    const store = makeStore();
    store.getState().markWalkthroughSeen(TASK_ID, TS_EPOCH);
    mockGetWalkthroughLastSeen.mockReturnValue(TS_LATER);

    store.getState().hydrateWalkthroughLastSeen(TASK_ID);

    expect(store.getState().walkthroughs.lastSeenUpdatedAtByTaskId[TASK_ID]).toBe(TS_EPOCH);
  });
});
