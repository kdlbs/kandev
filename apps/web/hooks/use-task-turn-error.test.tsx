import { afterEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, renderHook } from "@testing-library/react";
import { useTaskTurnError } from "./use-task-turn-error";

const dismissMock = vi.hoisted(() => vi.fn());
const toastMock = vi.hoisted(() => vi.fn());

vi.mock("@/lib/api/domains/task-turn-error-api", () => ({
  dismissTaskTurnError: dismissMock,
}));
vi.mock("@/components/toast-provider", () => ({
  useToast: () => ({ toast: toastMock }),
}));

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

describe("useTaskTurnError", () => {
  it("dismisses the observed stamp without optimistic state", async () => {
    dismissMock.mockResolvedValue({ dismissed: true });
    const { result } = renderHook(() => useTaskTurnError());
    await act(async () => {
      await result.current.dismiss("task-1", "failure-1");
    });
    expect(dismissMock).toHaveBeenCalledWith("task-1", "failure-1");
    expect(result.current.pendingStamp).toBeNull();
    expect(toastMock).not.toHaveBeenCalled();
  });

  it("keeps attention visible and reports a localized failure", async () => {
    dismissMock.mockRejectedValue(new Error("network"));
    const { result } = renderHook(() => useTaskTurnError());
    await act(async () => {
      await result.current.dismiss("task-1", "failure-1");
    });
    expect(toastMock).toHaveBeenCalledTimes(1);
    expect(toastMock.mock.calls[0][0]).toMatchObject({ variant: "error" });
  });

  it("ignores an empty task or stamp", async () => {
    const { result } = renderHook(() => useTaskTurnError());
    await act(async () => {
      await result.current.dismiss("", "failure-1");
      await result.current.dismiss("task-1", "");
    });
    expect(dismissMock).not.toHaveBeenCalled();
  });
});
