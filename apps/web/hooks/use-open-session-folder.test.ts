import { act, renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { useOpenSessionFolder } from "./use-open-session-folder";

const { openSessionFolder, toast, availability } = vi.hoisted(() => ({
  openSessionFolder: vi.fn(),
  toast: vi.fn(),
  availability: { value: true },
}));
vi.mock("@/hooks/domains/settings/use-editors", () => ({
  useEditors: () => ({ folderOpeningAvailable: availability.value }),
}));
vi.mock("@/lib/api", () => ({ openSessionFolder }));
vi.mock("@/components/toast-provider", () => ({ useToast: () => ({ toast }) }));

beforeEach(() => {
  vi.clearAllMocks();
  availability.value = true;
  openSessionFolder.mockResolvedValue({ success: true });
});

// @covers AC-TASKS-OPEN-FOLDER-001.2, AC-TASKS-OPEN-FOLDER-001.3
describe("useOpenSessionFolder", () => {
  it("does not send requests when the host opener is unavailable", async () => {
    availability.value = false;
    const { result } = renderHook(() => useOpenSessionFolder("session-1"));
    await act(async () => {
      expect(await result.current.open("wt-2")).toBeNull();
    });
    expect(openSessionFolder).not.toHaveBeenCalled();
    expect(toast).not.toHaveBeenCalled();
  });

  it("sends the selected worktree without replacing request options", async () => {
    const { result } = renderHook(() => useOpenSessionFolder("session-1"));
    await act(async () => {
      await result.current.open("wt-2");
    });
    expect(openSessionFolder).toHaveBeenCalledWith(
      "session-1",
      { cache: "no-store" },
      { worktree_id: "wt-2" },
    );
  });

  it("keeps the default folder request for existing callers", async () => {
    const { result } = renderHook(() => useOpenSessionFolder("session-1"));
    await act(async () => {
      await result.current.open();
    });
    expect(openSessionFolder).toHaveBeenCalledWith("session-1", { cache: "no-store" }, undefined);
  });

  it("does nothing without a session", async () => {
    const { result } = renderHook(() => useOpenSessionFolder(null));
    await act(async () => {
      expect(await result.current.open()).toBeNull();
    });
    expect(openSessionFolder).not.toHaveBeenCalled();
  });

  it("surfaces a localized failure and permits retry", async () => {
    openSessionFolder.mockRejectedValueOnce(new Error("workspace path not found"));
    const { result } = renderHook(() => useOpenSessionFolder("session-1"));
    await act(async () => {
      await expect(result.current.open()).resolves.toBeNull();
    });
    expect(toast).toHaveBeenCalledWith(
      expect.objectContaining({ title: "Failed to open folder", variant: "error" }),
    );
    expect(result.current.isLoading).toBe(false);
    await act(async () => {
      expect(await result.current.open()).toEqual({ success: true });
    });
  });

  it("prevents duplicate requests before React renders the loading state", async () => {
    let finish!: (value: { success: boolean }) => void;
    openSessionFolder.mockReturnValueOnce(
      new Promise((resolve) => {
        finish = resolve;
      }),
    );
    const { result } = renderHook(() => useOpenSessionFolder("session-1"));
    let pending!: Promise<unknown>;
    act(() => {
      pending = result.current.open();
      void result.current.open();
    });
    expect(openSessionFolder).toHaveBeenCalledTimes(1);
    expect(result.current.isLoading).toBe(true);
    await act(async () => {
      finish({ success: true });
      await pending;
    });
    expect(result.current.isLoading).toBe(false);
  });

  it("uses the current session after switching tasks", async () => {
    const { result, rerender } = renderHook(({ session }) => useOpenSessionFolder(session), {
      initialProps: { session: "session-1" },
    });
    rerender({ session: "session-2" });
    await act(async () => {
      await result.current.open("wt-new");
    });
    expect(openSessionFolder).toHaveBeenCalledWith(
      "session-2",
      { cache: "no-store" },
      { worktree_id: "wt-new" },
    );
  });
});

describe("shared folder launches", () => {
  it("shares pending state across surfaces while allowing other sessions", async () => {
    let finish!: (value: { success: boolean }) => void;
    openSessionFolder.mockReturnValueOnce(
      new Promise((resolve) => {
        finish = resolve;
      }),
    );
    const { result } = renderHook(() => [
      useOpenSessionFolder("session-1"),
      useOpenSessionFolder("session-1"),
      useOpenSessionFolder("session-2"),
    ]);
    let pending!: Promise<unknown>;
    act(() => {
      pending = result.current[0].open();
    });
    const sharedLoading = result.current[1].isLoading;
    const otherLoading = result.current[2].isLoading;
    await act(async () => {
      await result.current[1].open();
      await result.current[2].open();
    });
    const count = openSessionFolder.mock.calls.length;
    await act(async () => {
      finish({ success: true });
      await pending;
    });
    expect(sharedLoading).toBe(true);
    expect(otherLoading).toBe(false);
    expect(count).toBe(2);
    expect(result.current.every((folder) => !folder.isLoading)).toBe(true);
    await act(async () => {
      await result.current[1].open();
    });
    expect(openSessionFolder).toHaveBeenCalledTimes(3);
  });
});
