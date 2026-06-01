import { describe, it, expect, vi, beforeEach } from "vitest";
import type { StoreApi } from "zustand";
import type { QueryClient } from "@tanstack/react-query";
import type { AppState } from "@/lib/state/store";
import type { TaskPlanData } from "@/lib/query/query-options/session";

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

function makeStore() {
  const state = {
    tasks: { activeTaskId: TASK_ID, activeSessionId: "s-1" },
    markTaskPlanSeen: vi.fn(),
  };
  return {
    getState: () => state as unknown as AppState,
    setState: vi.fn(),
    subscribe: vi.fn(),
    destroy: vi.fn(),
    getInitialState: vi.fn(),
  } as unknown as StoreApi<AppState>;
}

// The handler reads the previous plan from the TQ cache (the bridge owns the
// write); model that with a getQueryData stub.
function makeQueryClient(prevPlanContent: string | null = null) {
  const data: TaskPlanData | undefined =
    prevPlanContent === null
      ? undefined
      : { plan: makePayload({ content: prevPlanContent }) as never, lastSeenUpdatedAt: null };
  return {
    getQueryData: vi.fn(() => data),
  } as unknown as QueryClient;
}

describe("task.plan.* handlers", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("agent created: does NOT mark seen", () => {
    const store = makeStore();
    const handlers = registerTaskPlansHandlers(store, makeQueryClient());

    handlers[ACTION_CREATED]!(
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      makeMessage(ACTION_CREATED, makePayload()) as any,
    );

    expect(store.getState().markTaskPlanSeen).not.toHaveBeenCalled();
  });

  it("agent updated: does NOT mark seen", () => {
    const store = makeStore();
    const handlers = registerTaskPlansHandlers(store, makeQueryClient());

    handlers[ACTION_UPDATED]!(
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      makeMessage(ACTION_UPDATED, makePayload()) as any,
    );

    expect(store.getState().markTaskPlanSeen).not.toHaveBeenCalled();
  });

  it("user-authored create (no prior plan): marks plan as seen", () => {
    const store = makeStore();
    const handlers = registerTaskPlansHandlers(store, makeQueryClient());

    handlers[ACTION_CREATED]!(
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      makeMessage(ACTION_CREATED, makePayload({ created_by: "user" })) as any,
    );

    expect(store.getState().markTaskPlanSeen).toHaveBeenCalledWith(TASK_ID, "2026-04-20T00:00:00Z");
  });

  it("agent update on plan originally created by user: does not mark seen", () => {
    // Backend sets created_by to the last modifier on update, not the original
    // creator — so an agent update on a user-created plan emits created_by="agent".
    const store = makeStore();
    const handlers = registerTaskPlansHandlers(store, makeQueryClient());

    handlers[ACTION_UPDATED]!(
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      makeMessage(ACTION_UPDATED, makePayload({ created_by: "agent" })) as any,
    );

    expect(store.getState().markTaskPlanSeen).not.toHaveBeenCalled();
  });

  it("user-authored update with UNCHANGED content does NOT mark seen", () => {
    // Editor auto-save round-trips the agent's plan content through TipTap
    // and saves it as user-authored — same content, new updated_at. Without
    // a content-change check this would erase the agent's unseen indicator.
    const store = makeStore();
    const handlers = registerTaskPlansHandlers(store, makeQueryClient("# Plan"));

    const payload = makePayload({
      content: "# Plan",
      created_by: "user",
      updated_at: "2026-04-20T01:00:00Z",
    });
    handlers[ACTION_UPDATED]!(
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      makeMessage(ACTION_UPDATED, payload) as any,
    );

    expect(store.getState().markTaskPlanSeen).not.toHaveBeenCalled();
  });

  it("user-authored update with CHANGED content marks seen", () => {
    const store = makeStore();
    const handlers = registerTaskPlansHandlers(store, makeQueryClient("# Old"));

    const payload = makePayload({
      content: "# New",
      created_by: "user",
      updated_at: "2026-04-20T01:00:00Z",
    });
    handlers[ACTION_UPDATED]!(
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      makeMessage(ACTION_UPDATED, payload) as any,
    );

    expect(store.getState().markTaskPlanSeen).toHaveBeenCalledWith(TASK_ID, "2026-04-20T01:00:00Z");
  });

  it("delete: marks as seen with empty sentinel", () => {
    const store = makeStore();
    const handlers = registerTaskPlansHandlers(store, makeQueryClient());

    handlers[ACTION_DELETED]!(
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      makeMessage(ACTION_DELETED, { task_id: TASK_ID }) as any,
    );

    expect(store.getState().markTaskPlanSeen).toHaveBeenCalledWith(TASK_ID, "");
  });
});
