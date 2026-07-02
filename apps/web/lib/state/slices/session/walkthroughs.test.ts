import { describe, it, expect } from "vitest";
import { create } from "zustand";
import { immer } from "zustand/middleware/immer";
import { createSessionSlice } from "./session-slice";
import type { SessionSlice } from "./types";
import type { TaskWalkthrough, WalkthroughStep } from "@/lib/types/http";

function makeStore() {
  return create<SessionSlice>()(immer(createSessionSlice));
}

const TASK_ID = "task-1";
const TS_EPOCH = "2026-04-20T00:00:00Z";
const TS_LATER = "2026-04-20T01:00:00Z";

function steps(n: number): WalkthroughStep[] {
  return Array.from({ length: n }, (_, i) => ({
    file: `f${i}.go`,
    line: i + 1,
    text: `step ${i}`,
  }));
}

function makeWalkthrough(overrides: Partial<TaskWalkthrough> = {}): TaskWalkthrough {
  return {
    id: "wt-1",
    task_id: TASK_ID,
    title: "Walkthrough",
    steps: steps(3),
    created_by: "agent",
    created_at: TS_EPOCH,
    updated_at: TS_EPOCH,
    ...overrides,
  };
}

describe("walkthrough slice", () => {
  it("setWalkthrough stores the tour and defaults the active step to 0", () => {
    const store = makeStore();
    store.getState().setWalkthrough(TASK_ID, makeWalkthrough());

    expect(store.getState().walkthroughs.byTaskId[TASK_ID]?.steps).toHaveLength(3);
    expect(store.getState().walkthroughs.activeStepByTaskId[TASK_ID]).toBe(0);
  });

  it("setWalkthroughActiveStep clamps within [0, len-1]", () => {
    const store = makeStore();
    store.getState().setWalkthrough(TASK_ID, makeWalkthrough());

    store.getState().setWalkthroughActiveStep(TASK_ID, 99);
    expect(store.getState().walkthroughs.activeStepByTaskId[TASK_ID]).toBe(2);

    store.getState().setWalkthroughActiveStep(TASK_ID, -5);
    expect(store.getState().walkthroughs.activeStepByTaskId[TASK_ID]).toBe(0);
  });

  it("replacing with a shorter tour clamps the active step back into range", () => {
    const store = makeStore();
    store.getState().setWalkthrough(TASK_ID, makeWalkthrough());
    store.getState().setWalkthroughActiveStep(TASK_ID, 2);

    store.getState().setWalkthrough(TASK_ID, makeWalkthrough({ steps: steps(1) }));

    expect(store.getState().walkthroughs.activeStepByTaskId[TASK_ID]).toBe(0);
  });

  it("setWalkthrough(null) resets the active step to 0", () => {
    const store = makeStore();
    store.getState().setWalkthrough(TASK_ID, makeWalkthrough());
    store.getState().setWalkthroughActiveStep(TASK_ID, 2);

    store.getState().setWalkthrough(TASK_ID, null);

    expect(store.getState().walkthroughs.byTaskId[TASK_ID]).toBeNull();
    expect(store.getState().walkthroughs.activeStepByTaskId[TASK_ID]).toBe(0);
  });

  it("markWalkthroughSeen records the current updated_at", () => {
    const store = makeStore();
    store.getState().setWalkthrough(TASK_ID, makeWalkthrough({ updated_at: TS_LATER }));

    store.getState().markWalkthroughSeen(TASK_ID);

    expect(store.getState().walkthroughs.lastSeenUpdatedAtByTaskId[TASK_ID]).toBe(TS_LATER);
  });
});
