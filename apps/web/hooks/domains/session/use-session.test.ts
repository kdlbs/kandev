import { cleanup, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { TaskSession } from "@/lib/types/http";

const mocks = vi.hoisted(() => {
  const state = {
    taskSessions: { items: {} as Record<string, TaskSession> },
    sessionAgentctl: { itemsBySessionId: {} as Record<string, { status: string }> },
    connection: { status: "connected" },
    setTaskSession: vi.fn(),
  };
  return {
    state,
    store: { getState: () => state },
    fetchTaskSession: vi.fn(),
    subscribeSession: vi.fn(() => vi.fn()),
  };
});

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: typeof mocks.state) => unknown) => selector(mocks.state),
  useAppStoreApi: () => mocks.store,
}));

vi.mock("@/lib/api", () => ({
  fetchTaskSession: (...args: unknown[]) => mocks.fetchTaskSession(...args),
}));

vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: () => ({ subscribeSession: mocks.subscribeSession }),
}));

import { useSession } from "./use-session";

const SESSION_ID = "session-1";

function session(state: TaskSession["state"], updatedAt: string): TaskSession {
  return {
    id: SESSION_ID,
    task_id: "task-1",
    state,
    updated_at: updatedAt,
  } as TaskSession;
}

async function flushPromises(): Promise<void> {
  await Promise.resolve();
  await Promise.resolve();
}

beforeEach(() => {
  vi.useFakeTimers();
  vi.clearAllMocks();
  mocks.state.connection.status = "connected";
  mocks.state.taskSessions.items = {
    [SESSION_ID]: session("RUNNING", "2026-07-31T08:00:00Z"),
  };
  mocks.fetchTaskSession.mockResolvedValue({
    session: session("RUNNING", "2026-07-31T08:00:01Z"),
  });
});

afterEach(() => {
  cleanup();
  vi.useRealTimers();
});

describe("useSession reconciliation", () => {
  it("shares one polling loop across simultaneous consumers of the same session", () => {
    const first = renderHook(() => useSession(SESSION_ID));
    const second = renderHook(() => useSession(SESSION_ID));

    expect(mocks.fetchTaskSession).toHaveBeenCalledTimes(1);

    first.unmount();
    second.unmount();
  });

  it("reuses an in-flight poll when a consumer is replaced", async () => {
    let resolveFetch: ((value: { session: TaskSession }) => void) | undefined;
    mocks.fetchTaskSession.mockImplementation(
      () =>
        new Promise<{ session: TaskSession }>((resolve) => {
          resolveFetch = resolve;
        }),
    );

    const first = renderHook(() => useSession(SESSION_ID));
    first.unmount();
    const replacement = renderHook(() => useSession(SESSION_ID));

    expect(mocks.fetchTaskSession).toHaveBeenCalledTimes(1);

    resolveFetch?.({ session: session("WAITING_FOR_INPUT", "2026-07-31T08:00:05Z") });
    await flushPromises();
    replacement.unmount();
  });

  it("polls while the authoritative session remains busy", async () => {
    const hook = renderHook(() => useSession(SESSION_ID));
    await flushPromises();

    await vi.advanceTimersByTimeAsync(750);

    expect(mocks.fetchTaskSession).toHaveBeenCalledTimes(2);
    hook.unmount();
  });

  it("rejects an HTTP snapshot older than the current live state", async () => {
    mocks.state.taskSessions.items[SESSION_ID] = session(
      "WAITING_FOR_INPUT",
      "2026-07-31T08:00:05Z",
    );
    mocks.fetchTaskSession.mockResolvedValue({
      session: session("RUNNING", "2026-07-31T08:00:01Z"),
    });

    const hook = renderHook(() => useSession(SESSION_ID));
    await flushPromises();

    expect(mocks.state.setTaskSession).not.toHaveBeenCalled();
    hook.unmount();
  });

  it("ignores a fetch that resolves after the last consumer unmounts", async () => {
    let resolveFetch: ((value: { session: TaskSession }) => void) | undefined;
    mocks.fetchTaskSession.mockImplementation(
      () =>
        new Promise<{ session: TaskSession }>((resolve) => {
          resolveFetch = resolve;
        }),
    );
    const hook = renderHook(() => useSession(SESSION_ID));

    hook.unmount();
    resolveFetch?.({ session: session("WAITING_FOR_INPUT", "2026-07-31T08:00:05Z") });
    await flushPromises();

    expect(mocks.state.setTaskSession).not.toHaveBeenCalled();
    expect(vi.getTimerCount()).toBe(0);
  });
});
