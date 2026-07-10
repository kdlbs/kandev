import { describe, it, expect, vi, beforeEach } from "vitest";
import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";

import { registerWalkthroughsHandlers } from "./walkthroughs";

const TASK_ID = "task-1";
const ACTION_CREATED = "task.walkthrough.created";
const ACTION_UPDATED = "task.walkthrough.updated";
const ACTION_DELETED = "task.walkthrough.deleted";

function makePayload(overrides: Partial<Record<string, unknown>> = {}) {
  return {
    id: "wt-1",
    task_id: TASK_ID,
    title: "Walkthrough",
    steps: [{ file: "a.go", line: 1, text: "first" }],
    created_by: "agent",
    created_at: "2026-04-20T00:00:00Z",
    updated_at: "2026-04-20T00:00:00Z",
    ...overrides,
  };
}

function makeMessage(action: string, payload: Record<string, unknown>) {
  return { id: "msg-1", type: "notification", action, payload };
}

function makeStore() {
  const state = {
    tasks: { activeTaskId: TASK_ID, activeSessionId: "s-1" },
    walkthroughs: { byTaskId: {}, activeStepByTaskId: {}, lastSeenUpdatedAtByTaskId: {} },
    setWalkthrough: vi.fn(),
    markWalkthroughSeen: vi.fn(),
  };
  return {
    getState: () => state as unknown as AppState,
    setState: vi.fn(),
    subscribe: vi.fn(),
    destroy: vi.fn(),
    getInitialState: vi.fn(),
  } as unknown as StoreApi<AppState>;
}

describe("task.walkthrough.* handlers", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("created: stores the walkthrough with its steps", () => {
    const store = makeStore();
    const handlers = registerWalkthroughsHandlers(store);

    handlers[ACTION_CREATED]!(
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      makeMessage(ACTION_CREATED, makePayload()) as any,
    );

    expect(store.getState().setWalkthrough).toHaveBeenCalledWith(
      TASK_ID,
      expect.objectContaining({ id: "wt-1", steps: expect.any(Array) }),
    );
    expect(store.getState().markWalkthroughSeen).not.toHaveBeenCalled();
  });

  it("updated: re-stores the walkthrough", () => {
    const store = makeStore();
    const handlers = registerWalkthroughsHandlers(store);

    handlers[ACTION_UPDATED]!(
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      makeMessage(ACTION_UPDATED, makePayload({ title: "v2" })) as any,
    );

    expect(store.getState().setWalkthrough).toHaveBeenCalledWith(
      TASK_ID,
      expect.objectContaining({ title: "v2" }),
    );
  });

  it("deleted: clears the walkthrough and marks seen", () => {
    const store = makeStore();
    const handlers = registerWalkthroughsHandlers(store);

    handlers[ACTION_DELETED]!(
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      makeMessage(ACTION_DELETED, makePayload()) as any,
    );

    expect(store.getState().setWalkthrough).toHaveBeenCalledWith(TASK_ID, null);
    expect(store.getState().markWalkthroughSeen).toHaveBeenCalledWith(TASK_ID);
  });
});
