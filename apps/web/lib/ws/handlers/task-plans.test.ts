import { describe, it, expect, vi, beforeEach } from "vitest";
import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";

const dockviewState = {
  api: { getPanel: vi.fn() } as { getPanel: ReturnType<typeof vi.fn> },
  addPlanPanel: vi.fn(),
  isRestoringLayout: false,
};

vi.mock("@/lib/state/dockview-store", () => ({
  useDockviewStore: {
    getState: () => dockviewState,
  },
}));

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
    dockviewState.api = { getPanel: vi.fn().mockReturnValue(null) };
    dockviewState.isRestoringLayout = false;
  });

  it("agent created on active task: stores plan and opens panel quietly in center", () => {
    const store = makeStore();
    const handlers = registerTaskPlansHandlers(store);

    handlers[ACTION_CREATED]!(
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      makeMessage(ACTION_CREATED, makePayload()) as any,
    );

    expect(store.getState().setTaskPlan).toHaveBeenCalledWith(TASK_ID, expect.any(Object));
    expect(dockviewState.addPlanPanel).toHaveBeenCalledWith({ quiet: true, inCenter: true });
    expect(store.getState().markTaskPlanSeen).not.toHaveBeenCalled();
  });

  it("agent updated with plan panel already open: does not re-open, does not mark seen", () => {
    const store = makeStore();
    dockviewState.api.getPanel.mockReturnValue({ id: "plan" });
    const handlers = registerTaskPlansHandlers(store);

    handlers[ACTION_UPDATED]!(
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      makeMessage(ACTION_UPDATED, makePayload()) as any,
    );

    expect(store.getState().setTaskPlan).toHaveBeenCalled();
    expect(dockviewState.addPlanPanel).not.toHaveBeenCalled();
    expect(store.getState().markTaskPlanSeen).not.toHaveBeenCalled();
  });

  it("agent event for non-active task: does not open panel", () => {
    const store = makeStore({
      tasks: { activeTaskId: "other-task", activeSessionId: "s-1" },
    });
    const handlers = registerTaskPlansHandlers(store);

    handlers[ACTION_CREATED]!(
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      makeMessage(ACTION_CREATED, makePayload()) as any,
    );

    expect(store.getState().setTaskPlan).toHaveBeenCalled();
    expect(dockviewState.addPlanPanel).not.toHaveBeenCalled();
  });

  it("agent event while layout is restoring: does not open panel", () => {
    const store = makeStore();
    dockviewState.isRestoringLayout = true;
    const handlers = registerTaskPlansHandlers(store);

    handlers[ACTION_CREATED]!(
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      makeMessage(ACTION_CREATED, makePayload()) as any,
    );

    expect(dockviewState.addPlanPanel).not.toHaveBeenCalled();
  });

  it("user-authored create: marks plan as seen, does not open panel", () => {
    const store = makeStore();
    const handlers = registerTaskPlansHandlers(store);

    handlers[ACTION_CREATED]!(
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      makeMessage(ACTION_CREATED, makePayload({ created_by: "user" })) as any,
    );

    expect(store.getState().setTaskPlan).toHaveBeenCalled();
    expect(store.getState().markTaskPlanSeen).toHaveBeenCalledWith(TASK_ID);
    expect(dockviewState.addPlanPanel).not.toHaveBeenCalled();
  });

  it("user-authored update: marks seen even when task is not active", () => {
    // Reviewer concern: if user saves the plan for a non-active task, the
    // indicator could fire spuriously on task switch. We unconditionally mark
    // user-authored writes as seen.
    const store = makeStore({
      tasks: { activeTaskId: "other-task", activeSessionId: "s-1" },
    });
    const handlers = registerTaskPlansHandlers(store);

    handlers[ACTION_UPDATED]!(
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      makeMessage(ACTION_UPDATED, makePayload({ created_by: "user" })) as any,
    );

    expect(store.getState().markTaskPlanSeen).toHaveBeenCalledWith(TASK_ID);
    expect(dockviewState.addPlanPanel).not.toHaveBeenCalled();
  });

  it("agent update on plan originally created by user: still arms indicator", () => {
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
    expect(dockviewState.addPlanPanel).toHaveBeenCalledWith({ quiet: true, inCenter: true });
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
