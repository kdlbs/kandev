import { describe, it, expect, vi, beforeEach } from "vitest";
import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";

import { registerTaskPlansHandlers } from "./task-plans";

const TASK_ID = "task-1";
const ACTION_CREATED = "task.plan.created";
const ACTION_UPDATED = "task.plan.updated";
const ACTION_DELETED = "task.plan.deleted";
const ACTION_REVISION_CREATED = "task.plan.revision.created";
const BASE_TIMESTAMP = "2026-04-20T00:00:00Z";

function makePayload(overrides: Partial<Record<string, unknown>> = {}) {
  return {
    id: "plan-1",
    task_id: TASK_ID,
    title: "Plan",
    content: "# Plan",
    created_by: "agent",
    created_at: BASE_TIMESTAMP,
    updated_at: BASE_TIMESTAMP,
    ...overrides,
  };
}

function makeMessage(action: string, payload: Record<string, unknown>) {
  return { id: "msg-1", type: "notification", action, payload };
}

function makeRevisionPayload(overrides: Partial<Record<string, unknown>> = {}) {
  return {
    id: "rev-1",
    task_id: TASK_ID,
    revision_number: 1,
    title: "Plan",
    author_kind: "agent",
    author_name: "Agent",
    content_length: 42,
    created_at: BASE_TIMESTAMP,
    updated_at: BASE_TIMESTAMP,
    ...overrides,
  };
}

function makeStore(
  overrides: Record<string, unknown> = {},
  prevPlan: Record<string, unknown> | null = null,
) {
  const state = {
    tasks: { activeTaskId: TASK_ID, activeSessionId: "s-1" },
    taskPlans: { byTaskId: prevPlan ? { [TASK_ID]: prevPlan } : {} },
    setTaskPlan: vi.fn(),
    markTaskPlanSeen: vi.fn(),
    upsertPlanRevision: vi.fn(),
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

  it("user-authored update with UNCHANGED content does NOT mark seen", () => {
    // Editor auto-save round-trips the agent's plan content through TipTap
    // and saves it as user-authored — same content, new updated_at. Without
    // a content-change check this would erase the agent's unseen indicator.
    const store = makeStore({}, makePayload({ content: "# Plan", created_by: "agent" }));
    const handlers = registerTaskPlansHandlers(store);

    const payload = makePayload({
      content: "# Plan",
      created_by: "user",
      updated_at: "2026-04-20T01:00:00Z",
    });
    handlers[ACTION_UPDATED]!(
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      makeMessage(ACTION_UPDATED, payload) as any,
    );

    expect(store.getState().setTaskPlan).toHaveBeenCalled();
    expect(store.getState().markTaskPlanSeen).not.toHaveBeenCalled();
  });

  it("user-authored update with CHANGED content marks seen", () => {
    const store = makeStore({}, makePayload({ content: "# Old", created_by: "agent" }));
    const handlers = registerTaskPlansHandlers(store);

    const payload = makePayload({
      content: "# New",
      created_by: "user",
      updated_at: "2026-04-20T01:00:00Z",
    });
    handlers[ACTION_UPDATED]!(
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      makeMessage(ACTION_UPDATED, payload) as any,
    );

    expect(store.getState().markTaskPlanSeen).toHaveBeenCalledWith(TASK_ID);
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

describe("task.plan.revision.created handler", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("carries content_length and workflow step badge into the store", () => {
    const store = makeStore();
    const handlers = registerTaskPlansHandlers(store);

    const payload = makeRevisionPayload({
      content_length: 123,
      workflow_step_id: "step-1",
      workflow_step_name: "In Review",
      workflow_step_color: "#00ff00",
    });
    handlers[ACTION_REVISION_CREATED]!(
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      makeMessage(ACTION_REVISION_CREATED, payload) as any,
    );

    expect(store.getState().upsertPlanRevision).toHaveBeenCalledWith(
      TASK_ID,
      expect.objectContaining({
        content_length: 123,
        workflow_step_id: "step-1",
        workflow_step_name: "In Review",
        workflow_step_color: "#00ff00",
      }),
    );
  });

  it("omits step keys entirely when the task has no workflow step, so a coalesce merge cannot clobber an existing badge", () => {
    const store = makeStore();
    const handlers = registerTaskPlansHandlers(store);

    const payload = makeRevisionPayload();
    handlers[ACTION_REVISION_CREATED]!(
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      makeMessage(ACTION_REVISION_CREATED, payload) as any,
    );

    const [, rev] = (store.getState().upsertPlanRevision as ReturnType<typeof vi.fn>).mock
      .calls[0]!;
    expect(Object.prototype.hasOwnProperty.call(rev, "workflow_step_id")).toBe(false);
    expect(Object.prototype.hasOwnProperty.call(rev, "workflow_step_name")).toBe(false);
    expect(Object.prototype.hasOwnProperty.call(rev, "workflow_step_color")).toBe(false);
    expect(rev.content_length).toBe(42);
  });
});
