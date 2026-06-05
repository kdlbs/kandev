import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { renderHook, cleanup, waitFor } from "@testing-library/react";

const loadMoreMock = vi.fn<[], Promise<number>>();

vi.mock("@/hooks/use-lazy-load-messages", () => ({
  useLazyLoadMessages: () => ({ loadMore: loadMoreMock, hasMore: true, isLoading: false }),
}));

import { useDrainOlderMessages } from "./use-drain-older-messages";

beforeEach(() => {
  loadMoreMock.mockReset();
});

afterEach(() => {
  cleanup();
});

describe("useDrainOlderMessages", () => {
  it("is idle when inactive", () => {
    const { result } = renderHook(() => useDrainOlderMessages("s1", false));
    expect(result.current.isDraining).toBe(false);
    expect(loadMoreMock).not.toHaveBeenCalled();
  });

  it("is idle when sessionId is null", () => {
    const { result } = renderHook(() => useDrainOlderMessages(null, true));
    expect(result.current.isDraining).toBe(false);
    expect(loadMoreMock).not.toHaveBeenCalled();
  });

  it("drains batches until loadMore returns 0", async () => {
    loadMoreMock.mockResolvedValueOnce(20).mockResolvedValueOnce(20).mockResolvedValueOnce(0);
    const { result } = renderHook(() => useDrainOlderMessages("s1", true));
    expect(result.current.isDraining).toBe(true);
    await waitFor(() => expect(result.current.isDraining).toBe(false));
    expect(loadMoreMock).toHaveBeenCalledTimes(3);
  });

  it("stops at the batch cap when loadMore never empties", async () => {
    loadMoreMock.mockResolvedValue(20);
    const { result } = renderHook(() => useDrainOlderMessages("s1", true));
    await waitFor(() => expect(result.current.isDraining).toBe(false));
    expect(loadMoreMock).toHaveBeenCalledTimes(50);
  });

  it("clears isDraining when active flips to false mid-drain", async () => {
    let resolveFirst: (value: number) => void = () => {};
    loadMoreMock.mockImplementationOnce(
      () => new Promise<number>((resolve) => (resolveFirst = resolve)),
    );
    const { result, rerender } = renderHook(
      ({ active }: { active: boolean }) => useDrainOlderMessages("s1", active),
      { initialProps: { active: true } },
    );
    expect(result.current.isDraining).toBe(true);
    rerender({ active: false });
    expect(result.current.isDraining).toBe(false);
    resolveFirst(20);
  });

  it("clears isDraining if loadMore rejects", async () => {
    const errorSpy = vi.spyOn(console, "error").mockImplementation(() => {});
    loadMoreMock.mockRejectedValueOnce(new Error("boom"));
    const { result } = renderHook(() => useDrainOlderMessages("s1", true));
    await waitFor(() => expect(result.current.isDraining).toBe(false));
    errorSpy.mockRestore();
  });
});
