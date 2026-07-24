import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { TaskSession } from "@/lib/types/http";

const mockMarkSessionRead = vi.fn();
const mockSetTaskSession = vi.fn();

type MockState = {
  taskSessions: { items: Record<string, TaskSession> };
  setTaskSession: typeof mockSetTaskSession;
};

let mockState: MockState;

function session(overrides: Partial<TaskSession> = {}): TaskSession {
  return {
    id: "session-1",
    task_id: "task-1",
    state: "RUNNING",
    started_at: "2026-06-27T00:00:00Z",
    updated_at: "2026-06-27T00:00:00Z",
    ...overrides,
  } as TaskSession;
}

const mockStoreApi = { getState: () => mockState };

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: MockState) => unknown) => selector(mockState),
  useAppStoreApi: () => mockStoreApi,
}));

vi.mock("@/lib/api/domains/session-api", () => ({
  markSessionRead: (...args: unknown[]) => mockMarkSessionRead(...args),
}));

import { useSessionReadTracking } from "./use-session-read-tracking";

afterEach(cleanup);

beforeEach(() => {
  vi.clearAllMocks();
  mockState = {
    taskSessions: { items: {} },
    setTaskSession: mockSetTaskSession,
  };
  mockMarkSessionRead.mockResolvedValue({ session: session({ last_read_message_id: "m2" }) });
});

describe("useSessionReadTracking", () => {
  it("returns null while not visible and does not mark the session read", () => {
    mockState.taskSessions.items["session-1"] = session({ last_read_message_id: "m1" });
    const { result } = renderHook(() => useSessionReadTracking("session-1", false, "m2"));

    expect(result.current).toBeNull();
    expect(mockMarkSessionRead).not.toHaveBeenCalled();
  });

  it("freezes the divider anchor at the cursor value from before this visit's advance", async () => {
    mockState.taskSessions.items["session-1"] = session({ last_read_message_id: "m1" });
    const { result } = renderHook(() => useSessionReadTracking("session-1", true, "m3"));

    // Anchor is the value that was current the instant the session became
    // visible — "m1" — not wherever the cursor ends up after advancing.
    expect(result.current).toBe("m1");
    await waitFor(() => expect(mockMarkSessionRead).toHaveBeenCalledWith("session-1", "m3"));
  });

  it("advances the cursor as new messages arrive while still visible, without moving the divider", async () => {
    mockState.taskSessions.items["session-1"] = session({ last_read_message_id: "m1" });
    const { result, rerender } = renderHook(
      ({ latest }: { latest: string }) => useSessionReadTracking("session-1", true, latest),
      { initialProps: { latest: "m2" } },
    );
    expect(result.current).toBe("m1");
    await waitFor(() => expect(mockMarkSessionRead).toHaveBeenCalledWith("session-1", "m2"));

    // Simulate the store reflecting the server's response to the first call.
    mockState.taskSessions.items["session-1"] = session({ last_read_message_id: "m2" });
    rerender({ latest: "m4" });

    await waitFor(() => expect(mockMarkSessionRead).toHaveBeenCalledWith("session-1", "m4"));
    // Divider anchor is unchanged — still the value from when the visit started.
    expect(result.current).toBe("m1");
  });

  it("does not call markSessionRead again once the cursor already matches the latest message", async () => {
    mockState.taskSessions.items["session-1"] = session({ last_read_message_id: "m2" });
    renderHook(() => useSessionReadTracking("session-1", true, "m2"));

    await act(async () => {
      await Promise.resolve();
    });
    expect(mockMarkSessionRead).not.toHaveBeenCalled();
  });

  it("re-captures a fresh anchor after leaving and re-entering the session", async () => {
    mockState.taskSessions.items["session-1"] = session({ last_read_message_id: "m1" });
    const { result, rerender } = renderHook(
      ({ visible, latest }: { visible: boolean; latest: string }) =>
        useSessionReadTracking("session-1", visible, latest),
      { initialProps: { visible: true, latest: "m2" } },
    );
    expect(result.current).toBe("m1");
    await waitFor(() => expect(mockMarkSessionRead).toHaveBeenCalledTimes(1));

    // Leave: cursor is now advanced to m2 server-side.
    mockState.taskSessions.items["session-1"] = session({ last_read_message_id: "m2" });
    rerender({ visible: false, latest: "m2" });
    expect(result.current).toBeNull();

    // More messages arrive while away, then the user navigates back in.
    mockState.taskSessions.items["session-1"] = session({ last_read_message_id: "m2" });
    rerender({ visible: true, latest: "m4" });

    // New anchor reflects where the user actually left off (m2), not the
    // stale m1 from the first visit.
    expect(result.current).toBe("m2");
    await waitFor(() => expect(mockMarkSessionRead).toHaveBeenCalledWith("session-1", "m4"));
  });

  it("logs and swallows a failed mark-read call instead of throwing", async () => {
    mockState.taskSessions.items["session-1"] = session({ last_read_message_id: "m1" });
    mockMarkSessionRead.mockRejectedValue(new Error("network error"));
    const consoleError = vi.spyOn(console, "error").mockImplementation(() => {});

    renderHook(() => useSessionReadTracking("session-1", true, "m2"));

    await waitFor(() => expect(consoleError).toHaveBeenCalled());
    consoleError.mockRestore();
  });

  it("discards a stale mark-read response that resolves after a newer one, so the local cursor never regresses", async () => {
    mockState.taskSessions.items["session-1"] = session({ last_read_message_id: "m1" });

    // Two overlapping requests: an older m2 whose response we control via a
    // deferred promise, and a newer m3 that resolves immediately. m3's
    // response lands first, then m2's stale response resolves last.
    let resolveM2: ((value: { session: TaskSession }) => void) | undefined;
    const m2Response = new Promise<{ session: TaskSession }>((resolve) => {
      resolveM2 = resolve;
    });
    mockMarkSessionRead.mockImplementation((_sessionId: string, messageId: string) => {
      if (messageId === "m2") return m2Response;
      return Promise.resolve({ session: session({ last_read_message_id: messageId }) });
    });

    const { rerender } = renderHook(
      ({ latest }: { latest: string }) => useSessionReadTracking("session-1", true, latest),
      { initialProps: { latest: "m2" } },
    );
    await waitFor(() => expect(mockMarkSessionRead).toHaveBeenCalledWith("session-1", "m2"));

    // A newer message arrives while m2's request is still in flight — this
    // dispatches and resolves the m3 request before m2 settles.
    mockState.taskSessions.items["session-1"] = session({ last_read_message_id: "m1" });
    rerender({ latest: "m3" });
    await waitFor(() => expect(mockMarkSessionRead).toHaveBeenCalledWith("session-1", "m3"));
    await waitFor(() =>
      expect(mockSetTaskSession).toHaveBeenCalledWith(
        expect.objectContaining({ last_read_message_id: "m3" }),
      ),
    );
    mockSetTaskSession.mockClear();

    // The delayed, now-stale m2 response finally resolves. It must be
    // discarded rather than regressing the store back to m2.
    resolveM2?.({ session: session({ last_read_message_id: "m2" }) });
    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });
    expect(mockSetTaskSession).not.toHaveBeenCalled();
  });
});
