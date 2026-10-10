import { cleanup, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { ConditionalJsonResult } from "@/lib/api/client";
import type { TaskSession } from "@/lib/types/http";

const mocks = vi.hoisted(() => {
  const state = {
    taskSessions: {
      items: {} as Record<string, TaskSession>,
      activityEpochBySession: {} as Record<string, number>,
      readCursorEpochBySession: {} as Record<string, number>,
    },
    taskSessionsByTask: { itemsByTaskId: {} as Record<string, TaskSession[]> },
    sessionAgentctl: { itemsBySessionId: {} as Record<string, { status: string }> },
    connection: { status: "connected" },
    setTaskSession: vi.fn(),
  };
  return {
    state,
    store: { current: { getState: () => state } },
    fetchTaskSessionConditional: vi.fn(),
    subscribeSession: vi.fn(() => vi.fn()),
  };
});

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: typeof mocks.state) => unknown) => selector(mocks.state),
  useAppStoreApi: () => mocks.store.current,
}));

vi.mock("@/lib/api", () => ({
  fetchTaskSessionConditional: (...args: unknown[]) => mocks.fetchTaskSessionConditional(...args),
}));

vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: () => ({ subscribeSession: mocks.subscribeSession }),
}));

import { useSession } from "./use-session";

const SESSION_ID = "session-1";
const TASK_ID = "task-1";
const RUNNING_SESSION_STATE = "RUNNING";
const INITIAL_SESSION_TIMESTAMP = "2026-07-31T08:00:00Z";
const FIRST_SESSION_TIMESTAMP = "2026-07-31T08:00:01Z";
const SECOND_SESSION_TIMESTAMP = "2026-07-31T08:00:02Z";
const LATER_SESSION_TIMESTAMP = "2026-07-31T08:00:05Z";
const SESSION_ETAG_V1 = '"session-v1"';
const SESSION_ETAG_V2 = '"session-v2"';
type ConditionalTaskSessionResult = ConditionalJsonResult<{ session: TaskSession }>;

function session(state: TaskSession["state"], updatedAt: string): TaskSession {
  return {
    id: SESSION_ID,
    task_id: TASK_ID,
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
  // Reconciler ownership is scoped to a store instance. Keep hook consumers
  // joined within a test while isolating any in-flight request from other tests.
  mocks.store.current = { getState: () => mocks.state };
  mocks.state.setTaskSession.mockImplementation((next: TaskSession) => {
    mocks.state.taskSessions.items[next.id] = next;
  });
  mocks.state.connection.status = "connected";
  mocks.state.taskSessions.items = {
    [SESSION_ID]: session(RUNNING_SESSION_STATE, INITIAL_SESSION_TIMESTAMP),
  };
  mocks.state.taskSessionsByTask.itemsByTaskId = {
    [TASK_ID]: [mocks.state.taskSessions.items[SESSION_ID]],
  };
  mocks.state.taskSessions.activityEpochBySession = {};
  mocks.state.taskSessions.readCursorEpochBySession = {};
  mocks.fetchTaskSessionConditional.mockResolvedValue({
    status: "ok",
    data: { session: session(RUNNING_SESSION_STATE, FIRST_SESSION_TIMESTAMP) },
    etag: null,
  });
});

afterEach(() => {
  cleanup();
  vi.useRealTimers();
});

describe("useSession polling ownership", () => {
  it("shares one polling loop across simultaneous consumers of the same session", () => {
    const first = renderHook(() => useSession(SESSION_ID));
    const second = renderHook(() => useSession(SESSION_ID));

    expect(mocks.fetchTaskSessionConditional).toHaveBeenCalledTimes(1);

    first.unmount();
    second.unmount();
  });

  it("reuses an in-flight poll when a consumer is replaced", async () => {
    let resolveFetch: ((value: ConditionalTaskSessionResult) => void) | undefined;
    mocks.fetchTaskSessionConditional.mockImplementation(
      () =>
        new Promise<ConditionalTaskSessionResult>((resolve) => {
          resolveFetch = resolve;
        }),
    );

    const first = renderHook(() => useSession(SESSION_ID));
    first.unmount();
    const replacement = renderHook(() => useSession(SESSION_ID));

    expect(mocks.fetchTaskSessionConditional).toHaveBeenCalledTimes(1);

    resolveFetch?.({
      status: "ok",
      data: { session: session("WAITING_FOR_INPUT", LATER_SESSION_TIMESTAMP) },
      etag: null,
    });
    await flushPromises();
    replacement.unmount();
  });

  it("polls while the authoritative session remains busy", async () => {
    const hook = renderHook(() => useSession(SESSION_ID));
    await flushPromises();

    await vi.advanceTimersByTimeAsync(750);

    expect(mocks.fetchTaskSessionConditional).toHaveBeenCalledTimes(2);
    hook.unmount();
  });
});

describe("useSession snapshot ordering", () => {
  it("uses a full snapshot after the WebSocket reconnects", async () => {
    let resolveConditionalRead:
      | ((value: { status: "not-modified"; etag: string }) => void)
      | undefined;
    mocks.fetchTaskSessionConditional
      .mockResolvedValueOnce({
        status: "ok",
        data: { session: session(RUNNING_SESSION_STATE, FIRST_SESSION_TIMESTAMP) },
        etag: SESSION_ETAG_V1,
      })
      .mockImplementationOnce(
        () =>
          new Promise((resolve) => {
            resolveConditionalRead = resolve;
          }),
      )
      .mockResolvedValueOnce({
        status: "ok",
        data: { session: session(RUNNING_SESSION_STATE, SECOND_SESSION_TIMESTAMP) },
        etag: SESSION_ETAG_V2,
      });

    const hook = renderHook(() => useSession(SESSION_ID));
    await flushPromises();
    await vi.advanceTimersByTimeAsync(750);
    expect(mocks.fetchTaskSessionConditional).toHaveBeenNthCalledWith(
      2,
      SESSION_ID,
      SESSION_ETAG_V1,
    );

    mocks.state.connection.status = "reconnecting";
    hook.rerender();
    mocks.state.connection.status = "connected";
    hook.rerender();
    resolveConditionalRead?.({ status: "not-modified", etag: SESSION_ETAG_V1 });
    await flushPromises();
    await vi.advanceTimersByTimeAsync(750);

    expect(mocks.fetchTaskSessionConditional).toHaveBeenNthCalledWith(3, SESSION_ID);
    hook.unmount();
  });

  it("rejects an HTTP snapshot older than the current live state", async () => {
    mocks.state.taskSessions.items[SESSION_ID] = session(
      "WAITING_FOR_INPUT",
      LATER_SESSION_TIMESTAMP,
    );
    mocks.fetchTaskSessionConditional.mockResolvedValue({
      status: "ok",
      data: { session: session(RUNNING_SESSION_STATE, FIRST_SESSION_TIMESTAMP) },
      etag: null,
    });

    const hook = renderHook(() => useSession(SESSION_ID));
    await flushPromises();

    expect(mocks.state.setTaskSession).not.toHaveBeenCalled();
    hook.unmount();
  });
});

describe("useSession hydration generations", () => {
  it("passes request-start epochs to the HTTP snapshot merge", async () => {
    mocks.state.taskSessions.items[SESSION_ID].last_read_message_id = "message-2";
    mocks.state.taskSessions.readCursorEpochBySession[SESSION_ID] = 7;
    mocks.fetchTaskSessionConditional.mockResolvedValue({
      status: "ok",
      data: {
        session: {
          ...session(RUNNING_SESSION_STATE, "2026-07-31T08:00:06Z"),
          last_read_message_id: "message-1",
        },
      },
      etag: null,
    });

    const hook = renderHook(() => useSession(SESSION_ID));
    await flushPromises();

    expect(mocks.state.setTaskSession).toHaveBeenCalledWith(
      expect.objectContaining({ last_read_message_id: "message-1" }),
      { activity: 0, readCursor: 7, workspaceRecovery: 0 },
    );
    hook.unmount();
  });
});

describe("useSession late responses", () => {
  it("ignores a fetch that resolves after the last consumer unmounts", async () => {
    let resolveFetch: ((value: ConditionalTaskSessionResult) => void) | undefined;
    mocks.fetchTaskSessionConditional.mockImplementation(
      () =>
        new Promise<ConditionalTaskSessionResult>((resolve) => {
          resolveFetch = resolve;
        }),
    );
    const hook = renderHook(() => useSession(SESSION_ID));

    hook.unmount();
    resolveFetch?.({
      status: "ok",
      data: { session: session("WAITING_FOR_INPUT", LATER_SESSION_TIMESTAMP) },
      etag: null,
    });
    await flushPromises();

    expect(mocks.state.setTaskSession).not.toHaveBeenCalled();
    expect(vi.getTimerCount()).toBe(0);
  });

  it("keeps the stored session and skips publication when the server returns 304", async () => {
    mocks.fetchTaskSessionConditional
      .mockResolvedValueOnce({
        status: "ok",
        data: { session: session(RUNNING_SESSION_STATE, FIRST_SESSION_TIMESTAMP) },
        etag: SESSION_ETAG_V1,
      })
      .mockResolvedValueOnce({ status: "not-modified", etag: SESSION_ETAG_V1 });

    const hook = renderHook(() => useSession(SESSION_ID));
    await flushPromises();
    expect(mocks.state.setTaskSession).toHaveBeenCalledTimes(1);
    const retained = mocks.state.taskSessions.items[SESSION_ID];

    await vi.advanceTimersByTimeAsync(750);

    expect(mocks.fetchTaskSessionConditional).toHaveBeenNthCalledWith(
      2,
      SESSION_ID,
      SESSION_ETAG_V1,
    );
    expect(mocks.state.setTaskSession).toHaveBeenCalledTimes(1);
    expect(mocks.state.taskSessions.items[SESSION_ID]).toBe(retained);
    hook.unmount();
  });

  it("forces an unconditional read when a 304 arrives after the local session was removed", async () => {
    let resolveConditionalRead:
      | ((value: { status: "not-modified"; etag: string }) => void)
      | undefined;
    mocks.fetchTaskSessionConditional
      .mockResolvedValueOnce({
        status: "ok",
        data: { session: session(RUNNING_SESSION_STATE, FIRST_SESSION_TIMESTAMP) },
        etag: SESSION_ETAG_V1,
      })
      .mockImplementationOnce(
        () =>
          new Promise((resolve) => {
            resolveConditionalRead = resolve;
          }),
      )
      .mockResolvedValueOnce({
        status: "ok",
        data: { session: session(RUNNING_SESSION_STATE, SECOND_SESSION_TIMESTAMP) },
        etag: SESSION_ETAG_V2,
      });

    const hook = renderHook(() => useSession(SESSION_ID));
    await flushPromises();
    expect(mocks.state.taskSessions.items[SESSION_ID]?.updated_at).toBe(FIRST_SESSION_TIMESTAMP);
    mocks.state.setTaskSession.mockClear();

    await vi.advanceTimersByTimeAsync(750);
    expect(mocks.fetchTaskSessionConditional).toHaveBeenCalledTimes(2);
    expect(mocks.fetchTaskSessionConditional).toHaveBeenNthCalledWith(
      2,
      SESSION_ID,
      SESSION_ETAG_V1,
    );
    mocks.state.taskSessions.items = {};
    resolveConditionalRead?.({ status: "not-modified", etag: SESSION_ETAG_V1 });
    await flushPromises();

    expect(mocks.fetchTaskSessionConditional).toHaveBeenNthCalledWith(3, SESSION_ID);
    expect(mocks.state.setTaskSession).toHaveBeenCalledTimes(1);
    expect(mocks.state.taskSessions.items[SESSION_ID]?.updated_at).toBe(SECOND_SESSION_TIMESTAMP);
    hook.unmount();
  });
});
