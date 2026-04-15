import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook } from "@testing-library/react";
import type { SessionFailureNotification } from "@/lib/state/slices/ui/types";

let mockNotification: SessionFailureNotification | null = null;
const mockClearNotification = vi.fn();
const mockToast = vi.fn();

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: Record<string, unknown>) => unknown) =>
    selector({
      sessionFailureNotification: mockNotification,
      setSessionFailureNotification: mockClearNotification,
    }),
}));

vi.mock("@/components/toast-provider", () => ({
  useToast: () => ({ toast: mockToast }),
}));

import { useSessionFailureToast } from "./use-session-failure-toast";

describe("useSessionFailureToast", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockNotification = null;
  });

  it("shows toast when notification is set", () => {
    mockNotification = { sessionId: "s-1", taskId: "t-1", message: "boom" };
    renderHook(() => useSessionFailureToast());

    expect(mockToast).toHaveBeenCalledWith({
      title: "Task failed to start",
      description: "boom",
      variant: "error",
    });
    expect(mockClearNotification).toHaveBeenCalledWith(null);
  });

  it("does not show toast when notification is null", () => {
    mockNotification = null;
    renderHook(() => useSessionFailureToast());

    expect(mockToast).not.toHaveBeenCalled();
    expect(mockClearNotification).not.toHaveBeenCalled();
  });

  it("deduplicates toasts for the same sessionId across rerenders", () => {
    mockNotification = { sessionId: "s-1", taskId: "t-1", message: "first" };
    const { rerender } = renderHook(() => useSessionFailureToast());
    expect(mockToast).toHaveBeenCalledTimes(1);

    mockToast.mockClear();
    mockClearNotification.mockClear();

    mockNotification = { sessionId: "s-1", taskId: "t-1", message: "duplicate" };
    rerender();

    expect(mockToast).not.toHaveBeenCalled();
    expect(mockClearNotification).toHaveBeenCalledWith(null);
  });

  it("shows toast for a different sessionId", () => {
    mockNotification = { sessionId: "s-1", taskId: "t-1", message: "first" };
    const { rerender } = renderHook(() => useSessionFailureToast());
    expect(mockToast).toHaveBeenCalledTimes(1);

    mockToast.mockClear();

    mockNotification = { sessionId: "s-2", taskId: "t-1", message: "second" };
    rerender();

    expect(mockToast).toHaveBeenCalledWith({
      title: "Task failed to start",
      description: "second",
      variant: "error",
    });
  });
});
