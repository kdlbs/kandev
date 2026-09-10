import { act, renderHook, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

const mockGetTaskDeletePreflight = vi.fn();

vi.mock("@/lib/api", () => ({
  getTaskDeletePreflight: (...args: unknown[]) => mockGetTaskDeletePreflight(...args),
}));

import { useTaskDeletePreflight } from "./use-task-delete-preflight";

beforeEach(() => {
  mockGetTaskDeletePreflight.mockReset();
});

describe("useTaskDeletePreflight", () => {
  it("resolves clean workspaces without discard consent", async () => {
    mockGetTaskDeletePreflight.mockResolvedValue({ requires_discard_consent: false });
    const { result } = renderHook(() => useTaskDeletePreflight(true, "task-1"));

    await waitFor(() => expect(result.current.status).toBe("resolved"));
    expect(result.current.requiresDiscardConsent).toBe(false);
    expect(mockGetTaskDeletePreflight).toHaveBeenCalledWith(["task-1"], false);
  });

  it("fails closed and retries an unavailable inspection", async () => {
    mockGetTaskDeletePreflight.mockRejectedValueOnce(new Error("unavailable"));
    mockGetTaskDeletePreflight.mockResolvedValueOnce({ requires_discard_consent: true });
    const { result } = renderHook(() => useTaskDeletePreflight(true, "task-1"));

    await waitFor(() => expect(result.current.status).toBe("error"));
    expect(result.current.requiresDiscardConsent).toBe(false);
    act(() => result.current.retry());
    await waitFor(() => expect(result.current.status).toBe("resolved"));
    expect(result.current.requiresDiscardConsent).toBe(true);
    expect(mockGetTaskDeletePreflight).toHaveBeenCalledTimes(2);
  });

  it("ignores a stale response after the delete scope changes", async () => {
    let resolveFirst!: (value: { requires_discard_consent: boolean }) => void;
    mockGetTaskDeletePreflight
      .mockImplementationOnce(
        () =>
          new Promise((resolve) => {
            resolveFirst = resolve;
          }),
      )
      .mockResolvedValueOnce({ requires_discard_consent: true });
    const { result, rerender } = renderHook(({ taskId }) => useTaskDeletePreflight(true, taskId), {
      initialProps: { taskId: "task-1" },
    });

    rerender({ taskId: "task-2" });
    await waitFor(() => expect(result.current.status).toBe("resolved"));
    expect(result.current.requiresDiscardConsent).toBe(true);
    resolveFirst({ requires_discard_consent: false });
    await new Promise((resolve) => setTimeout(resolve, 0));
    expect(result.current.requiresDiscardConsent).toBe(true);
  });

  it("does not enable deletion for an empty selection", async () => {
    const { result } = renderHook(() => useTaskDeletePreflight(true));

    await waitFor(() => expect(result.current.status).toBe("error"));
    expect(mockGetTaskDeletePreflight).not.toHaveBeenCalled();
  });

  it("starts a fresh inspection when the dialog is reopened", async () => {
    mockGetTaskDeletePreflight.mockResolvedValue({ requires_discard_consent: false });
    const { result, rerender } = renderHook(({ open }) => useTaskDeletePreflight(open, "task-1"), {
      initialProps: { open: true },
    });

    await waitFor(() => expect(result.current.status).toBe("resolved"));
    rerender({ open: false });
    rerender({ open: true });
    await waitFor(() => expect(mockGetTaskDeletePreflight).toHaveBeenCalledTimes(2));
    expect(result.current.status).toBe("resolved");
  });
});
