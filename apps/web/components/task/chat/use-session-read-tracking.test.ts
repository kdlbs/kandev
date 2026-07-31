import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { TaskSession } from "@/lib/types/http";

const mockMarkSessionRead = vi.fn();
const mockUpdateSessionReadCursor = vi.fn();

type MockState = {
  userSettings: { unreadDivider: boolean };
  taskSessions: { items: Record<string, TaskSession> };
  updateSessionReadCursor: typeof mockUpdateSessionReadCursor;
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
    userSettings: { unreadDivider: true },
    taskSessions: { items: {} },
    updateSessionReadCursor: mockUpdateSessionReadCursor,
  };
  mockMarkSessionRead.mockResolvedValue({ session_id: "session-1", last_read_message_id: "m2" });
});

describe("useSessionReadTracking", () => {
  it("returns null while not visible and does not mark the session read", () => {
    mockState.taskSessions.items["session-1"] = session({ last_read_message_id: "m1" });
    const { result } = renderHook(() => useSessionReadTracking("session-1", false, "m2"));

    expect(result.current).toBeNull();
    expect(mockMarkSessionRead).not.toHaveBeenCalled();
  });

  it("returns null and does not mark the session read when the user disables the divider", async () => {
    mockState.userSettings.unreadDivider = false;
    mockState.taskSessions.items["session-1"] = session({ last_read_message_id: "m1" });
    const { result } = renderHook(() => useSessionReadTracking("session-1", true, "m2"));

    expect(result.current).toBeNull();
    await act(async () => {
      await Promise.resolve();
    });
    expect(mockMarkSessionRead).not.toHaveBeenCalled();
  });
  it("does not capture an anchor before the session record has loaded into the store, capturing correctly once it does", async () => {
    // Session absent entirely (still fetching) — must not treat this the
    // same as "loaded, and legitimately has no prior cursor" (see the
    // next test). Locking in a capture here would use undefined?.last_
    // read_message_id ?? null as the answer, when the real answer isn't
    // known yet.
    const { result, rerender } = renderHook(
      ({ latest }: { latest: string | null }) => useSessionReadTracking("session-1", true, latest),
      { initialProps: { latest: null as string | null } },
    );
    expect(result.current).toBeNull();
    expect(mockMarkSessionRead).not.toHaveBeenCalled();

    // The session finishes loading with a real prior cursor already set
    // (e.g. a fetch that resolved after this hook's first render).
    mockState.taskSessions.items["session-1"] = session({ last_read_message_id: "m1" });
    rerender({ latest: "m3" });

    expect(result.current).toBe("m1");
    await waitFor(() => expect(mockMarkSessionRead).toHaveBeenCalledWith("session-1", "m3"));
  });

  it("does not dispatch mark-read while the session hasn't loaded yet, even if latestMessageId already resolved", async () => {
    // Reproduces a real mobile bug: unlike the dockview panel path (isVisible
    // lags behind usePanelActive's async resolution, giving the session
    // fetch a head start), mobile's isVisible is true from render one — so
    // if the *messages* list resolves before the *session metadata* fetch
    // does, latestMessageId can already be non-null while the session
    // record itself is still missing from the store. The mark-read
    // dispatch effect must not race ahead of the capture above and advance
    // the cursor before this visit's real "prior" boundary is ever read —
    // that would silently swallow the divider with no error.
    const { rerender } = renderHook(
      ({ latest }: { latest: string | null }) => useSessionReadTracking("session-1", true, latest),
      { initialProps: { latest: "m5" as string | null } },
    );
    await act(async () => {
      await Promise.resolve();
    });
    expect(mockMarkSessionRead).not.toHaveBeenCalled();

    // Session loads with the real prior cursor — the capture (during
    // render, before this commit's effects) must see this value, not
    // whatever a premature dispatch might have already advanced it to.
    mockState.taskSessions.items["session-1"] = session({ last_read_message_id: "m1" });
    rerender({ latest: "m5" });

    await waitFor(() => expect(mockMarkSessionRead).toHaveBeenCalledWith("session-1", "m5"));
  });

  it("captures null when the session has loaded but genuinely has no prior cursor (first-ever visit)", () => {
    mockState.taskSessions.items["session-1"] = session({ last_read_message_id: undefined });
    const { result } = renderHook(() => useSessionReadTracking("session-1", true, "m1"));

    expect(result.current).toBeNull();
  });

  it("does not activate a divider when a new message arrives after opening at the cursor", () => {
    mockState.taskSessions.items["session-1"] = session({ last_read_message_id: "m1" });
    const { result, rerender } = renderHook(
      ({ latest }: { latest: string }) => useSessionReadTracking("session-1", true, latest),
      { initialProps: { latest: "m1" } },
    );

    // Opening at the transcript tail has no unread boundary.
    expect(result.current).toBeNull();

    // The user stays on this chat panel and sends a new prompt. It must not
    // turn the tail cursor captured at visit start into a "New" divider.
    rerender({ latest: "m2" });
    expect(result.current).toBeNull();
  });

  it("waits for the initial message load before preserving a genuine unread boundary", () => {
    mockState.taskSessions.items["session-1"] = session({ last_read_message_id: "m1" });
    const trackRead = useSessionReadTracking as unknown as (
      sessionId: string | null,
      isVisible: boolean,
      latestMessageId: string | null,
      initialMessagesLoading: boolean,
    ) => string | null;
    const { result, rerender } = renderHook(
      ({
        latest,
        initialMessagesLoading,
      }: {
        latest: string | null;
        initialMessagesLoading: boolean;
      }) => trackRead("session-1", true, latest, initialMessagesLoading),
      { initialProps: { latest: null as string | null, initialMessagesLoading: true } },
    );

    expect(result.current).toBeNull();

    // The first complete transcript includes messages that pre-date this visit.
    rerender({ latest: "m3", initialMessagesLoading: false });
    expect(result.current).toBe("m1");
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
});

describe("useSessionReadTracking — mark-read dispatch", () => {
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

    // Leave: cursor is now advanced to m2 server-side. Advance the clock
    // past the hide-debounce (see use-session-read-tracking.ts) so the
    // reset that lets the next show re-capture actually commits — a real
    // navigate-away is far slower than the debounce window, and fake
    // timers drive that deterministically rather than a real sleep.
    vi.useFakeTimers();
    mockState.taskSessions.items["session-1"] = session({ last_read_message_id: "m2" });
    rerender({ visible: false, latest: "m2" });
    act(() => {
      vi.advanceTimersByTime(350);
    });
    vi.useRealTimers();
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
    type MarkReadResult = { session_id: string; last_read_message_id: string };
    let resolveM2: ((value: MarkReadResult) => void) | undefined;
    const m2Response = new Promise<MarkReadResult>((resolve) => {
      resolveM2 = resolve;
    });
    mockMarkSessionRead.mockImplementation((_sessionId: string, messageId: string) => {
      if (messageId === "m2") return m2Response;
      return Promise.resolve({ session_id: "session-1", last_read_message_id: messageId });
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
      expect(mockUpdateSessionReadCursor).toHaveBeenCalledWith("session-1", "m3"),
    );
    mockUpdateSessionReadCursor.mockClear();

    // The delayed, now-stale m2 response finally resolves. It must be
    // discarded rather than regressing the store back to m2.
    resolveM2?.({ session_id: "session-1", last_read_message_id: "m2" });
    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });
    expect(mockUpdateSessionReadCursor).not.toHaveBeenCalled();
  });
});
