import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook } from "@testing-library/react";

const mockFocusSession = vi.fn();
const mockUnsubscribe = vi.fn();
let mockConnectionStatus = "connected";

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: { connection: { status: string } }) => unknown) =>
    selector({ connection: { status: mockConnectionStatus } }),
}));

vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: () => ({
    focusSession: mockFocusSession,
  }),
}));

import { useTaskFocus } from "./use-task-focus";

describe("useTaskFocus", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockFocusSession.mockReturnValue(mockUnsubscribe);
    mockConnectionStatus = "connected";
  });

  it("calls focusSession on mount when connected", () => {
    renderHook(() => useTaskFocus("sess-1"));
    expect(mockFocusSession).toHaveBeenCalledWith("sess-1");
  });

  it("calls the returned cleanup on unmount", () => {
    const { unmount } = renderHook(() => useTaskFocus("sess-1"));
    expect(mockUnsubscribe).not.toHaveBeenCalled();
    unmount();
    expect(mockUnsubscribe).toHaveBeenCalled();
  });

  it("does nothing when no sessionId is provided", () => {
    renderHook(() => useTaskFocus(null));
    expect(mockFocusSession).not.toHaveBeenCalled();
  });

  it("does nothing when WebSocket is not connected", () => {
    mockConnectionStatus = "connecting";
    renderHook(() => useTaskFocus("sess-1"));
    expect(mockFocusSession).not.toHaveBeenCalled();
  });

  it("re-focuses when sessionId changes", () => {
    const { rerender } = renderHook(({ id }: { id: string | null }) => useTaskFocus(id), {
      initialProps: { id: "sess-1" },
    });
    expect(mockFocusSession).toHaveBeenCalledTimes(1);
    expect(mockFocusSession).toHaveBeenLastCalledWith("sess-1");
    expect(mockUnsubscribe).not.toHaveBeenCalled();

    rerender({ id: "sess-2" });
    // Cleanup of previous focus should fire, then a new focus.
    expect(mockUnsubscribe).toHaveBeenCalledTimes(1);
    expect(mockFocusSession).toHaveBeenCalledTimes(2);
    expect(mockFocusSession).toHaveBeenLastCalledWith("sess-2");
  });
});
