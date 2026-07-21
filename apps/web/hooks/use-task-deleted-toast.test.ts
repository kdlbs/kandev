import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook } from "@testing-library/react";
import type { TaskDeletedNotification } from "@/lib/state/slices/ui/types";

let mockNotification: TaskDeletedNotification | null = null;
const mockClearNotification = vi.fn();
const mockToast = vi.fn();

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: Record<string, unknown>) => unknown) =>
    selector({
      taskDeletedNotification: mockNotification,
      setTaskDeletedNotification: mockClearNotification,
    }),
}));

vi.mock("@/components/toast-provider", () => ({
  useToast: () => ({ toast: mockToast }),
}));

import { useTaskDeletedToast } from "./use-task-deleted-toast";

describe("useTaskDeletedToast", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockNotification = null;
  });

  it("shows a reason-specific toast for an approved PR", () => {
    mockNotification = { taskId: "t-1", title: "Review PR #11259", reason: "pr_approved_by_user" };
    renderHook(() => useTaskDeletedToast());

    expect(mockToast).toHaveBeenCalledWith({
      title: '"Review PR #11259" was closed',
      description: "Its pull request was approved, so this review task was closed automatically.",
    });
    expect(mockClearNotification).toHaveBeenCalledWith(null);
  });

  it("falls back to a generic message when no reason is provided", () => {
    mockNotification = { taskId: "t-2" };
    renderHook(() => useTaskDeletedToast());

    expect(mockToast).toHaveBeenCalledWith({
      title: "Task closed",
      description: "This task was closed automatically.",
    });
  });

  it("does not show a toast when notification is null", () => {
    mockNotification = null;
    renderHook(() => useTaskDeletedToast());

    expect(mockToast).not.toHaveBeenCalled();
    expect(mockClearNotification).not.toHaveBeenCalled();
  });

  it("deduplicates toasts for the same taskId across rerenders", () => {
    mockNotification = { taskId: "t-1", reason: "pr_approved_by_user" };
    const { rerender } = renderHook(() => useTaskDeletedToast());
    expect(mockToast).toHaveBeenCalledTimes(1);

    mockToast.mockClear();
    mockClearNotification.mockClear();

    mockNotification = { taskId: "t-1", reason: "pr_approved_by_user" };
    rerender();

    expect(mockToast).not.toHaveBeenCalled();
    expect(mockClearNotification).toHaveBeenCalledWith(null);
  });
});
