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
});
