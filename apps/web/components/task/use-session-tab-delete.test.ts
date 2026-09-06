import { act, renderHook } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { useSessionTabDelete } from "./use-session-tab-delete";

describe("useSessionTabDelete", () => {
  it("confirms context-menu deletion with toast feedback", async () => {
    const setConfirmDelete = vi.fn();
    const handleDelete = vi.fn().mockResolvedValue(true);
    const { result } = renderHook(() => useSessionTabDelete(setConfirmDelete, handleDelete));

    act(() => result.current.handleMenuDelete());
    await act(async () => {
      await result.current.handleConfirmDelete();
    });

    expect(handleDelete).toHaveBeenCalledWith({ feedback: "toast" });
  });
});
