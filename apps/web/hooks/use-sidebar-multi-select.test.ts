import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const archiveTaskById = vi.fn();
const archiveAndSwitch = vi.fn();
const removeTasksFromStore = vi.fn();
const moveTasks = vi.fn();
const toast = vi.fn();
let activeTaskId: string | null = null;

vi.mock("./use-task-actions", () => ({
  useTaskActions: () => ({ archiveTaskById }),
  useArchiveAndSwitchTask: () => archiveAndSwitch,
}));
vi.mock("./use-task-workflow-move", () => ({ useTaskWorkflowMove: () => moveTasks }));
vi.mock("@/components/toast-provider", () => ({ useToast: () => ({ toast }) }));
vi.mock("@/components/state-provider", () => ({
  useAppStoreApi: () => ({ getState: () => ({ tasks: { activeTaskId } }) }),
}));
vi.mock("./use-task-multi-select", async (importOriginal) => {
  const actual = await importOriginal<typeof import("./use-task-multi-select")>();
  return { ...actual, useTaskMultiSelectStore: () => ({ removeTasksFromStore }) };
});

import { useSidebarMultiSelect } from "./use-sidebar-multi-select";

beforeEach(() => {
  activeTaskId = null;
  archiveTaskById.mockReset().mockResolvedValue(undefined);
  archiveAndSwitch.mockReset().mockResolvedValue(undefined);
  removeTasksFromStore.mockReset();
  moveTasks.mockReset().mockResolvedValue(undefined);
  toast.mockReset();
});
afterEach(() => {
  vi.clearAllMocks();
});

describe("useSidebarMultiSelect", () => {
  it("toggles, ranges, and clears the selection", () => {
    const { result } = renderHook(() => useSidebarMultiSelect("ws1"));
    expect(result.current.isSelecting).toBe(false);

    act(() => result.current.toggleSelect("a"));
    act(() => result.current.toggleSelect("b"));
    expect(result.current.selectedIds).toEqual(new Set(["a", "b"]));
    expect(result.current.isSelecting).toBe(true);

    act(() => result.current.clearSelection());
    expect(result.current.selectedIds.size).toBe(0);
  });

  it("resets the selection when the workspace changes", () => {
    const { result, rerender } = renderHook(({ ws }) => useSidebarMultiSelect(ws), {
      initialProps: { ws: "ws1" },
    });
    act(() => result.current.toggleSelect("a"));
    expect(result.current.selectedIds.size).toBe(1);

    rerender({ ws: "ws2" });
    expect(result.current.selectedIds.size).toBe(0);
  });

  it("bulkArchive removes all on full success and clears the selection", async () => {
    const { result } = renderHook(() => useSidebarMultiSelect("ws1"));
    await act(async () => {
      await result.current.bulkArchive(["a", "b"]);
    });
    expect(archiveTaskById).toHaveBeenCalledTimes(2);
    expect(removeTasksFromStore).toHaveBeenCalledWith(new Set(["a", "b"]));
    expect(result.current.selectedIds.size).toBe(0);
    expect(toast).not.toHaveBeenCalled();
  });

  it("bulkArchive keeps the failed ids selected and toasts on partial failure", async () => {
    archiveTaskById.mockImplementation((id) =>
      id === "b" ? Promise.reject(new Error("nope")) : Promise.resolve(),
    );
    const { result } = renderHook(() => useSidebarMultiSelect("ws1"));
    await act(async () => {
      await result.current.bulkArchive(["a", "b"]);
    });
    expect(removeTasksFromStore).toHaveBeenCalledWith(new Set(["a"]));
    expect(result.current.selectedIds).toEqual(new Set(["b"]));
    expect(toast).toHaveBeenCalledWith(expect.objectContaining({ variant: "error" }));
  });

  it("bulkArchive routes the active task through the switch-aware path", async () => {
    activeTaskId = "a";
    const { result } = renderHook(() => useSidebarMultiSelect("ws1"));
    await act(async () => {
      await result.current.bulkArchive(["a", "b"]);
    });
    expect(archiveAndSwitch).toHaveBeenCalledWith("a", undefined);
    expect(archiveTaskById).toHaveBeenCalledTimes(1);
    expect(archiveTaskById).toHaveBeenCalledWith("b", undefined);
    expect(removeTasksFromStore).toHaveBeenCalledWith(new Set(["b"]));
  });

  it("bulkArchive ignores an empty id list", async () => {
    const { result } = renderHook(() => useSidebarMultiSelect("ws1"));
    await act(async () => {
      await result.current.bulkArchive([]);
    });
    expect(archiveTaskById).not.toHaveBeenCalled();
  });

  it("bulkMove clears the selection on success", async () => {
    const { result } = renderHook(() => useSidebarMultiSelect("ws1"));
    act(() => result.current.toggleSelect("a"));
    await act(async () => {
      await result.current.bulkMove(["a"], "wf1", "s1");
    });
    expect(moveTasks).toHaveBeenCalledWith(["a"], "wf1", "s1");
    expect(result.current.selectedIds.size).toBe(0);
  });

  it("bulkMove keeps the selection and swallows the rejection on failure", async () => {
    moveTasks.mockRejectedValue(new Error("locked"));
    const { result } = renderHook(() => useSidebarMultiSelect("ws1"));
    act(() => result.current.toggleSelect("a"));
    await act(async () => {
      await result.current.bulkMove(["a"], "wf1", "s1");
    });
    expect(result.current.selectedIds).toEqual(new Set(["a"]));
  });
});
