import { describe, it, expect, vi, beforeEach } from "vitest";
import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";

import { registerTaskPlansHandlers } from "./task-plans";

const TASK_ID = "task-1";
const ACTION_CREATED = "task.plan.created";
const ACTION_UPDATED = "task.plan.updated";
const ACTION_DELETED = "task.plan.deleted";

function makePayload(overrides: Partial<Record<string, unknown>> = {}) {
  return {
    id: "plan-1",
    task_id: TASK_ID,
    title: "Plan",
    content: "# Plan",
    created_by: "agent",
    created_at: "2026-04-20T00:00:00Z",
    updated_at: "2026-04-20T00:00:00Z",
    ...overrides,
  };
}

function makeMessage(action: string, payload: Record<string, unknown>) {
  return { id: "msg-1", type: "notification", action, payload };
}

function makeStore(overrides: Record<string, unknown> = {}) {
  const state = {
    tasks: { activeTaskId: TASK_ID, activeSessionId: "s-1" },
    setTaskPlan: vi.fn(),
    markTaskPlanSeen: vi.fn(),
    ...overrides,
  };
  return {
    getState: () => state as unknown as AppState,
    setState: vi.fn(),
    subscribe: vi.fn(),
    destroy: vi.fn(),
    getInitialState: vi.fn(),
  } as unknown as StoreApi<AppState>;
}

describe("task.plan.* handlers", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("agent created: stores plan and does NOT mark seen", () => {
    const store = makeStore();
    const handlers = registerTaskPlansHandlers(store);

    handlers[ACTION_CREATED]!(
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      makeMessage(ACTION_CREATED, makePayload()) as any,
    );

    expect(store.getState().setTaskPlan).toHaveBeenCalledWith(TASK_ID, expect.any(Object));
    expect(store.getState().markTaskPlanSeen).not.toHaveBeenCalled();
  });

  it("agent updated: stores plan and does NOT mark seen", () => {
    const store = makeStore();
    const handlers = registerTaskPlansHandlers(store);

    handlers[ACTION_UPDATED]!(
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      makeMessage(ACTION_UPDATED, makePayload()) as any,
    );

    expect(store.getState().setTaskPlan).toHaveBeenCalled();
    expect(store.getState().markTaskPlanSeen).not.toHaveBeenCalled();
  });

  it("user-authored create: marks plan as seen", () => {
    const store = makeStore();
    const handlers = registerTaskPlansHandlers(store);

    handlers[ACTION_CREATED]!(
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      makeMessage(ACTION_CREATED, makePayload({ created_by: "user" })) as any,
    );

    expect(store.getState().setTaskPlan).toHaveBeenCalled();
    expect(store.getState().markTaskPlanSeen).toHaveBeenCalledWith(TASK_ID);
  });

  it("user-authored update: marks seen even when task is not active", () => {
    const store = makeStore({
      tasks: { activeTaskId: "other-task", activeSessionId: "s-1" },
    });
    const handlers = registerTaskPlansHandlers(store);

    handlers[ACTION_UPDATED]!(
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      makeMessage(ACTION_UPDATED, makePayload({ created_by: "user" })) as any,
    );

    expect(store.getState().markTaskPlanSeen).toHaveBeenCalledWith(TASK_ID);
  });

  it("agent update on plan originally created by user: stores plan without marking seen", () => {
    // Backend sets created_by to the last modifier on update, not the original
    // creator — so an agent update on a user-created plan emits created_by="agent".
    const store = makeStore();
    const handlers = registerTaskPlansHandlers(store);

    handlers[ACTION_UPDATED]!(
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      makeMessage(ACTION_UPDATED, makePayload({ created_by: "agent" })) as any,
    );

    expect(store.getState().setTaskPlan).toHaveBeenCalled();
    expect(store.getState().markTaskPlanSeen).not.toHaveBeenCalled();
  });

  it("delete: nulls plan and marks as seen", () => {
    const store = makeStore();
    const handlers = registerTaskPlansHandlers(store);

    handlers[ACTION_DELETED]!(
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      makeMessage(ACTION_DELETED, { task_id: TASK_ID }) as any,
    );

    expect(store.getState().setTaskPlan).toHaveBeenCalledWith(TASK_ID, null);
    expect(store.getState().markTaskPlanSeen).toHaveBeenCalledWith(TASK_ID);
  });
});
