import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, act, waitFor } from "@testing-library/react";

const mockToast = vi.fn();
const mockTriggerAll = vi.fn();
let mockItems: unknown[] = [];
let mockLoaded = false;
let mockWorkspaceId: string | null = "ws-1";

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: Record<string, unknown>) => unknown) =>
    selector({
      workspaces: { activeId: mockWorkspaceId },
    }),
}));

vi.mock("@/components/toast-provider", () => ({
  useToast: () => ({ toast: mockToast }),
}));

vi.mock("./use-review-watches", () => ({
  useReviewWatches: () => ({
    items: mockItems,
    loaded: mockLoaded,
    triggerAll: mockTriggerAll,
  }),
}));

import { useRefreshReviews } from "./use-refresh-reviews";

describe("useRefreshReviews", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockItems = [];
    mockLoaded = false;
    mockWorkspaceId = "ws-1";
  });

  describe("available", () => {
    it("is false while watches are unloaded", () => {
      mockLoaded = false;
      mockItems = [{ id: "w-1" }];
      const { result } = renderHook(() => useRefreshReviews());
      expect(result.current.available).toBe(false);
    });

    it("is false when loaded but no watches exist", () => {
      mockLoaded = true;
      mockItems = [];
      const { result } = renderHook(() => useRefreshReviews());
      expect(result.current.available).toBe(false);
    });

    it("is true when loaded and watches exist", () => {
      mockLoaded = true;
      mockItems = [{ id: "w-1" }];
      const { result } = renderHook(() => useRefreshReviews());
      expect(result.current.available).toBe(true);
    });
  });

  describe("trigger", () => {
    it("shows the success toast when new PRs are found", async () => {
      mockTriggerAll.mockResolvedValue({ new_prs_found: 2 });
      const { result } = renderHook(() => useRefreshReviews());

      await act(async () => {
        await result.current.trigger();
      });

      expect(mockToast).toHaveBeenCalledWith({
        description: "Found 2 new PRs to review",
        variant: "success",
      });
    });

    it("singularizes the success toast for a single PR", async () => {
      mockTriggerAll.mockResolvedValue({ new_prs_found: 1 });
      const { result } = renderHook(() => useRefreshReviews());

      await act(async () => {
        await result.current.trigger();
      });

      expect(mockToast).toHaveBeenCalledWith({
        description: "Found 1 new PR to review",
        variant: "success",
      });
    });

    it("shows the empty-state toast when no new PRs are found", async () => {
      mockTriggerAll.mockResolvedValue({ new_prs_found: 0 });
      const { result } = renderHook(() => useRefreshReviews());

      await act(async () => {
        await result.current.trigger();
      });

      expect(mockToast).toHaveBeenCalledWith({ description: "No new PRs to review" });
    });

    it("shows the error toast when triggerAll rejects", async () => {
      mockTriggerAll.mockRejectedValue(new Error("network"));
      const { result } = renderHook(() => useRefreshReviews());

      await act(async () => {
        await result.current.trigger();
      });

      expect(mockToast).toHaveBeenCalledWith({
        description: "Failed to check for review PRs",
        variant: "error",
      });
    });

    it("toggles loading true while in flight and false when settled", async () => {
      let resolveTriggerAll: (value: { new_prs_found: number }) => void = () => {};
      mockTriggerAll.mockImplementation(
        () =>
          new Promise((resolve) => {
            resolveTriggerAll = resolve;
          }),
      );

      const { result } = renderHook(() => useRefreshReviews());
      expect(result.current.loading).toBe(false);

      let triggerPromise: Promise<void> = Promise.resolve();
      act(() => {
        triggerPromise = result.current.trigger();
      });

      await waitFor(() => expect(result.current.loading).toBe(true));

      await act(async () => {
        resolveTriggerAll({ new_prs_found: 0 });
        await triggerPromise;
      });

      expect(result.current.loading).toBe(false);
    });
  });
});
