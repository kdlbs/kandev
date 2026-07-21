import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";

const mockGetSubtaskCount = vi.fn();

vi.mock("@/lib/api", () => ({
  getSubtaskCount: (...args: unknown[]) => mockGetSubtaskCount(...args),
}));

import { useSubtaskCount } from "./use-subtask-count";

beforeEach(() => {
  mockGetSubtaskCount.mockReset();
});

describe("useSubtaskCount", () => {
  it("returns 0 when the dialog is closed", () => {
    const { result } = renderHook(() => useSubtaskCount(false, "task-1"));
    expect(result.current).toBe(0);
    expect(mockGetSubtaskCount).not.toHaveBeenCalled();
  });

  it("returns 0 while the fetch is in flight, then the resolved total", async () => {
    let resolveFetch: (v: { count: number }) => void = () => {};
    mockGetSubtaskCount.mockReturnValue(
      new Promise<{ count: number }>((res) => {
        resolveFetch = res;
      }),
    );
    const { result } = renderHook(() => useSubtaskCount(true, "task-1"));
    expect(result.current).toBe(0);
    resolveFetch({ count: 7 });
    await waitFor(() => expect(result.current).toBe(7));
  });

  it("returns 0 immediately when reopened for a different task until the fetch lands", async () => {
    mockGetSubtaskCount.mockImplementation((id: string) =>
      Promise.resolve({ count: id === "a" ? 3 : 0 }),
    );
    const { result, rerender } = renderHook(
      ({ open, taskId }: { open: boolean; taskId: string }) => useSubtaskCount(open, taskId),
      { initialProps: { open: true, taskId: "a" } },
    );
    await waitFor(() => expect(result.current).toBe(3));

    rerender({ open: false, taskId: "a" });
    expect(result.current).toBe(0);

    // Reopen for a different task — should return 0 until the new
    // fetch resolves (no stale 3 flashing through).
    rerender({ open: true, taskId: "b" });
    expect(result.current).toBe(0);
    await waitFor(() => expect(result.current).toBe(0));
  });

  it("sums counts across multiple task ids for bulk operations", async () => {
    mockGetSubtaskCount.mockImplementation((id: string) =>
      Promise.resolve({ count: id === "a" ? 2 : 5 }),
    );
    const { result } = renderHook(() => useSubtaskCount(true, undefined, ["a", "b"]));
    await waitFor(() => expect(result.current).toBe(7));
  });
});
