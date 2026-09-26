import { act, renderHook } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { useSessionTabDelete } from "./use-session-tab-delete";

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((nextResolve) => {
    resolve = nextResolve;
  });
  return { promise, resolve };
}

describe("useSessionTabDelete", () => {
  it("uses inline feedback and clears the spinner after a tab delete settles", async () => {
    const setConfirmDelete = vi.fn();
    const pending = deferred<boolean>();
    const handleDelete = vi.fn(() => pending.promise);
    const { result } = renderHook(() => useSessionTabDelete(setConfirmDelete, handleDelete));

    act(() => result.current.handleCloseTab());
    expect(setConfirmDelete).toHaveBeenCalledWith(true);

    let deletePromise!: Promise<void>;
    act(() => {
      deletePromise = result.current.handleConfirmDelete();
    });

    expect(result.current.isDeletingFromTab).toBe(true);
    expect(handleDelete).toHaveBeenCalledWith({ feedback: "inline" });

    await act(async () => {
      pending.resolve(true);
      await deletePromise;
    });

    expect(result.current.isDeletingFromTab).toBe(false);
  });

  it("clears the spinner when a tab delete fails", async () => {
    const setConfirmDelete = vi.fn();
    const handleDelete = vi.fn().mockResolvedValue(false);
    const { result } = renderHook(() => useSessionTabDelete(setConfirmDelete, handleDelete));

    act(() => result.current.handleCloseTab());
    await act(async () => {
      await result.current.handleConfirmDelete();
    });

    expect(result.current.isDeletingFromTab).toBe(false);
    expect(handleDelete).toHaveBeenCalledWith({ feedback: "inline" });
  });

  it("keeps context-menu deletion on toast feedback without the tab spinner", async () => {
    const setConfirmDelete = vi.fn();
    const handleDelete = vi.fn().mockResolvedValue(true);
    const { result } = renderHook(() => useSessionTabDelete(setConfirmDelete, handleDelete));

    act(() => result.current.handleMenuDelete());
    await act(async () => {
      await result.current.handleConfirmDelete();
    });

    expect(result.current.isDeletingFromTab).toBe(false);
    expect(handleDelete).toHaveBeenCalledWith({ feedback: "toast" });
  });
});
