import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";

const mockFetchTaskSession = vi.hoisted(() => vi.fn());
const mockState = vi.hoisted(() => ({ value: {} as Record<string, unknown> }));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: typeof mockState.value) => unknown) => selector(mockState.value),
}));

vi.mock("@/lib/api/domains/session-api", () => ({
  fetchTaskSession: (...args: unknown[]) => mockFetchTaskSession(...args),
}));

import { useEnsureTaskSession } from "./use-ensure-task-session";

const setTaskSession = vi.fn();

function setStoredSessions(items: Record<string, unknown>) {
  mockState.value = { taskSessions: { items }, setTaskSession };
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((resolvePromise) => {
    resolve = resolvePromise;
  });
  return { promise, resolve };
}

beforeEach(() => {
  vi.clearAllMocks();
  setStoredSessions({});
  mockFetchTaskSession.mockResolvedValue({ session: { id: "session-1", task_id: "task-1" } });
});

describe("useEnsureTaskSession", () => {
  // A tab learned from a task event has no session row, and without one the
  // chat renders but cannot subscribe or accept input.
  it("fetches and stores a session the client has never seen", async () => {
    renderHook(() => useEnsureTaskSession("session-1"));

    await waitFor(() => expect(mockFetchTaskSession).toHaveBeenCalledWith("session-1"));
    expect(setTaskSession).toHaveBeenCalledWith({ id: "session-1", task_id: "task-1" });
  });

  it("merges the full row when a placeholder arrives while hydration is pending", async () => {
    const response = deferred<{
      session: { id: string; task_id: string; agent_profile_id: string };
    }>();
    mockFetchTaskSession.mockReturnValue(response.promise);

    const { rerender } = renderHook(() => useEnsureTaskSession("session-1"));
    await waitFor(() => expect(mockFetchTaskSession).toHaveBeenCalledTimes(1));

    // A status response can insert a placeholder before the authoritative HTTP row arrives.
    setStoredSessions({
      "session-1": {
        id: "session-1",
        state: "WAITING_FOR_INPUT",
        updated_at: "2026-08-29T09:00:00Z",
      },
    });
    rerender();

    response.resolve({
      session: {
        id: "session-1",
        task_id: "task-1",
        agent_profile_id: "profile-1",
      },
    });

    await waitFor(() =>
      expect(setTaskSession).toHaveBeenCalledWith({
        id: "session-1",
        task_id: "task-1",
        agent_profile_id: "profile-1",
      }),
    );
  });

  it("does not refetch a session already in the store", () => {
    setStoredSessions({ "session-1": { id: "session-1" } });

    renderHook(() => useEnsureTaskSession("session-1"));

    expect(mockFetchTaskSession).not.toHaveBeenCalled();
  });

  it("does nothing without a session id", () => {
    renderHook(() => useEnsureTaskSession(null));

    expect(mockFetchTaskSession).not.toHaveBeenCalled();
  });

  it("requests a given session only once, even if it never arrives", async () => {
    mockFetchTaskSession.mockRejectedValue(new Error("deleted elsewhere"));

    const { rerender } = renderHook(() => useEnsureTaskSession("session-1"));
    await waitFor(() => expect(mockFetchTaskSession).toHaveBeenCalledTimes(1));
    rerender();
    rerender();

    expect(mockFetchTaskSession).toHaveBeenCalledTimes(1);
    expect(setTaskSession).not.toHaveBeenCalled();
  });

  it("retries hydration when the active tab changes before the request settles", async () => {
    const firstResponse = deferred<{ session: { id: string; task_id: string } }>();
    const secondResponse = deferred<{ session: { id: string; task_id: string } }>();
    let sessionOneCalls = 0;
    mockFetchTaskSession.mockImplementation((sessionId: string) => {
      if (sessionId === "session-1") {
        sessionOneCalls += 1;
        return sessionOneCalls === 1 ? firstResponse.promise : secondResponse.promise;
      }
      return Promise.resolve({ session: { id: sessionId, task_id: "task-2" } });
    });

    const { rerender } = renderHook(
      ({ sessionId }: { sessionId: string }) => useEnsureTaskSession(sessionId),
      { initialProps: { sessionId: "session-1" } },
    );
    await waitFor(() => expect(mockFetchTaskSession).toHaveBeenCalledTimes(1));

    rerender({ sessionId: "session-2" });
    rerender({ sessionId: "session-1" });

    await waitFor(() => expect(mockFetchTaskSession).toHaveBeenCalledTimes(3));
    secondResponse.resolve({ session: { id: "session-1", task_id: "task-after-return" } });

    await waitFor(() =>
      expect(setTaskSession).toHaveBeenCalledWith({
        id: "session-1",
        task_id: "task-after-return",
      }),
    );
    firstResponse.resolve({ session: { id: "session-1", task_id: "task-stale" } });
  });
});
