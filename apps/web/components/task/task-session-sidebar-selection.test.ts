import { act, renderHook } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { useBulkConfirmDialog } from "./task-session-sidebar-selection";

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
