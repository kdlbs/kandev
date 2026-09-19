import { act, renderHook } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { useBulkConfirmDialog, useSelectionHandlers } from "./task-session-sidebar-selection";

const tasks = [
  { id: "a", remoteExecutorType: "docker" },
  { id: "b", remoteExecutorType: null },
  { id: "c", remoteExecutorType: "sprites" },
];

describe("useBulkConfirmDialog", () => {
  it("open() captures the ids and their executor types", () => {
    const bulkArchive = vi.fn().mockResolvedValue(undefined);
    const { result } = renderHook(() => useBulkConfirmDialog(tasks, bulkArchive));

    act(() => result.current.open(["a", "c"]));
    expect(result.current.state).toEqual({ ids: ["a", "c"], executorTypes: ["docker", "sprites"] });
  });

  it("confirm() archives the captured ids and clears the dialog", async () => {
    const bulkArchive = vi.fn().mockResolvedValue(undefined);
    const { result } = renderHook(() => useBulkConfirmDialog(tasks, bulkArchive));

    act(() => result.current.open(["a", "b"]));
    await act(async () => {
      await result.current.confirm({ cascade: true });
    });
    expect(bulkArchive).toHaveBeenCalledWith(["a", "b"], { cascade: true });
    expect(result.current.state).toBeNull();
  });

  it("confirm() is a no-op when no dialog is open", async () => {
    const bulkArchive = vi.fn().mockResolvedValue(undefined);
    const { result } = renderHook(() => useBulkConfirmDialog(tasks, bulkArchive));

    await act(async () => {
      await result.current.confirm({ cascade: false });
    });
    expect(bulkArchive).not.toHaveBeenCalled();
  });

  it("clears the dialog even when archiving rejects", async () => {
    const bulkArchive = vi.fn().mockRejectedValue(new Error("boom"));
    const errorSpy = vi.spyOn(console, "error").mockImplementation(() => {});
    try {
      const { result } = renderHook(() => useBulkConfirmDialog(tasks, bulkArchive));
      act(() => result.current.open(["a"]));
      await act(async () => {
        await result.current.confirm({ cascade: false });
      });
      expect(result.current.state).toBeNull();
    } finally {
      errorSpy.mockRestore();
    }
  });
});

describe("useSelectionHandlers", () => {
  it("keeps bulk move stable when the selection aggregate is recreated", () => {
    const clearSelection = vi.fn();
    const selectRange = vi.fn();
    const pruneToVisible = vi.fn();
    const bulkMove = vi.fn();
    const multiSelect = {
      isSelecting: false,
      clearSelection,
      selectRange,
      pruneToVisible,
      bulkMove,
    } as unknown as Parameters<typeof useSelectionHandlers>[0]["multiSelect"];
    const stableArgs = {
      pinTasks: vi.fn(),
      unpinTasks: vi.fn(),
      pinnedTaskIds: ["pinned"],
      visibleTaskIds: ["a", "b"],
      movableSelectedIds: new Set(["a", "b"]),
    };

    const { result, rerender } = renderHook(
      ({ selection }: { selection: typeof multiSelect }) =>
        useSelectionHandlers({ ...stableArgs, multiSelect: selection }),
      { initialProps: { selection: multiSelect } },
    );
    const firstBulkMove = result.current.onBulkMove;

    rerender({
      selection: {
        ...multiSelect,
      },
    });

    expect(result.current.onBulkMove).toBe(firstBulkMove);
  });
});
