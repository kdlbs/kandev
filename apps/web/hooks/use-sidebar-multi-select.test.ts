import { act, renderHook } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { createElement, type ReactNode } from "react";

const archiveTaskById = vi.fn();
const deleteTaskById = vi.fn();
const archiveAndSwitch = vi.fn();
const removeTaskFromBoard = vi.fn();
const moveTasks = vi.fn();
const toast = vi.fn();
let activeTaskId: string | null = null;
let queryClient: QueryClient;

vi.mock("./use-task-actions", () => ({
  useTaskActions: () => ({ archiveTaskById, deleteTaskById }),
  useArchiveAndSwitchTask: () => archiveAndSwitch,
}));
vi.mock("./use-task-removal", () => ({ useTaskRemoval: () => ({ removeTaskFromBoard }) }));
vi.mock("./use-task-workflow-move", () => ({ useTaskWorkflowMove: () => moveTasks }));
vi.mock("@/components/toast-provider", () => ({ useToast: () => ({ toast }) }));
vi.mock("@/components/state-provider", () => ({
  useAppStoreApi: () => ({
    getState: () => ({ tasks: { activeTaskId, activeSessionId: null } }),
  }),
}));

import { useSidebarMultiSelect } from "./use-sidebar-multi-select";

function wrapper({ children }: { children: ReactNode }) {
  return createElement(QueryClientProvider, { client: queryClient }, children);
}

function renderSidebarMultiSelect(workspaceId: string | null) {
  return renderHook(() => useSidebarMultiSelect(workspaceId), { wrapper });
}

function seedSnapshot(taskIds: string[]) {
  queryClient.setQueryData(["workflows", "wf1", "snapshot"], {
    workflow: { id: "wf1" },
    steps: [],
    tasks: taskIds.map((id) => ({ id })),
  });
}

function snapshotTaskIds(): string[] {
  const snapshot = queryClient.getQueryData<{ tasks: Array<{ id: string }> }>([
    "workflows",
    "wf1",
    "snapshot",
  ]);
  return snapshot?.tasks.map((task) => task.id) ?? [];
}

beforeEach(() => {
  queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  activeTaskId = null;
  archiveTaskById.mockReset().mockResolvedValue(undefined);
  deleteTaskById.mockReset().mockResolvedValue(undefined);
  archiveAndSwitch.mockReset().mockResolvedValue(undefined);
  removeTaskFromBoard.mockReset().mockResolvedValue(undefined);
  moveTasks.mockReset().mockResolvedValue(undefined);
  toast.mockReset();
});
afterEach(() => {
  vi.clearAllMocks();
});

describe("useSidebarMultiSelect", () => {
  it("toggles, ranges, and clears the selection", () => {
    const { result } = renderSidebarMultiSelect("ws1");
    expect(result.current.isSelecting).toBe(false);

    act(() => result.current.toggleSelect("a"));
    act(() => result.current.toggleSelect("b"));
    expect(result.current.selectedIds).toEqual(new Set(["a", "b"]));
    expect(result.current.isSelecting).toBe(true);

    act(() => result.current.clearSelection());
    expect(result.current.selectedIds.size).toBe(0);
  });

  it("pruneToVisible drops selected ids that are no longer visible", () => {
    const { result } = renderSidebarMultiSelect("ws1");
    act(() => result.current.toggleSelect("a"));
    act(() => result.current.toggleSelect("b"));

    act(() => result.current.pruneToVisible(["a"]));
    expect(result.current.selectedIds).toEqual(new Set(["a"]));

    // No-op when everything is still visible.
    act(() => result.current.pruneToVisible(["a", "b"]));
    expect(result.current.selectedIds).toEqual(new Set(["a"]));
  });

  it("pruneToVisible realigns the anchor so a later range starts from a visible id", () => {
    const { result } = renderSidebarMultiSelect("ws1");
    act(() => result.current.toggleSelect("a"));
    act(() => result.current.toggleSelect("b")); // anchor is now "b"
    act(() => result.current.pruneToVisible(["a"])); // drops "b", anchor must realign to "a"

    // Range from the realigned anchor "a" to "c" includes the in-between "x".
    // A stale "b" anchor (absent from orderedIds) would fall back to just {a, c}.
    act(() => result.current.selectRange("c", ["a", "x", "c"]));
    expect(result.current.selectedIds).toEqual(new Set(["a", "x", "c"]));
  });

  it("resets the selection when the workspace changes", () => {
    const { result, rerender } = renderHook(({ ws }) => useSidebarMultiSelect(ws), {
      initialProps: { ws: "ws1" },
      wrapper,
    });
    act(() => result.current.toggleSelect("a"));
    expect(result.current.selectedIds.size).toBe(1);

    rerender({ ws: "ws2" });
    expect(result.current.selectedIds.size).toBe(0);
  });
});

describe("useSidebarMultiSelect — bulk actions", () => {
  it("bulkArchive removes all on full success and clears the selection", async () => {
    seedSnapshot(["a", "b"]);
    const { result } = renderSidebarMultiSelect("ws1");
    await act(async () => {
      await result.current.bulkArchive(["a", "b"]);
    });
    expect(archiveTaskById).toHaveBeenCalledTimes(2);
    expect(snapshotTaskIds()).toEqual([]);
    expect(result.current.selectedIds.size).toBe(0);
    expect(toast).not.toHaveBeenCalled();
  });

  it("bulkArchive keeps the failed ids selected and toasts on partial failure", async () => {
    archiveTaskById.mockImplementation((id) =>
      id === "b" ? Promise.reject(new Error("nope")) : Promise.resolve(),
    );
    seedSnapshot(["a", "b"]);
    const { result } = renderSidebarMultiSelect("ws1");
    await act(async () => {
      await result.current.bulkArchive(["a", "b"]);
    });
    expect(snapshotTaskIds()).toEqual(["b"]);
    expect(result.current.selectedIds).toEqual(new Set(["b"]));
    expect(toast).toHaveBeenCalledWith(expect.objectContaining({ variant: "error" }));
  });

  it("bulkArchive routes the active task through the switch-aware path", async () => {
    activeTaskId = "a";
    seedSnapshot(["a", "b"]);
    const { result } = renderSidebarMultiSelect("ws1");
    await act(async () => {
      await result.current.bulkArchive(["a", "b"]);
    });
    expect(archiveAndSwitch).toHaveBeenCalledWith("a", undefined);
    expect(archiveTaskById).toHaveBeenCalledTimes(1);
    expect(archiveTaskById).toHaveBeenCalledWith("b", undefined);
    expect(snapshotTaskIds()).toEqual(["a"]);
  });

  it("bulkArchive ignores an empty id list", async () => {
    const { result } = renderSidebarMultiSelect("ws1");
    await act(async () => {
      await result.current.bulkArchive([]);
    });
    expect(archiveTaskById).not.toHaveBeenCalled();
  });

  it("bulkDelete removes all on full success and clears the selection", async () => {
    seedSnapshot(["a", "b"]);
    const { result } = renderSidebarMultiSelect("ws1");
    await act(async () => {
      await result.current.bulkDelete(["a", "b"]);
    });
    expect(deleteTaskById).toHaveBeenCalledTimes(2);
    expect(snapshotTaskIds()).toEqual([]);
    expect(result.current.selectedIds.size).toBe(0);
    expect(toast).not.toHaveBeenCalled();
  });

  it("bulkDelete keeps failed ids selected and toasts on partial failure", async () => {
    deleteTaskById.mockImplementation((id: string) =>
      id === "b" ? Promise.reject(new Error("nope")) : Promise.resolve(),
    );
    const { result } = renderSidebarMultiSelect("ws1");
    await act(async () => {
      await result.current.bulkDelete(["a", "b"]);
    });
    expect(result.current.selectedIds).toEqual(new Set(["b"]));
    expect(toast).toHaveBeenCalledWith(expect.objectContaining({ variant: "error" }));
  });

  it("bulkDelete routes the active task through the switch-aware removal", async () => {
    activeTaskId = "a";
    const { result } = renderSidebarMultiSelect("ws1");
    await act(async () => {
      await result.current.bulkDelete(["a", "b"]);
    });
    // 'b' deleted directly; 'a' (active) deleted then removed-from-board to switch.
    expect(deleteTaskById).toHaveBeenCalledWith("b", undefined);
    expect(deleteTaskById).toHaveBeenCalledWith("a", undefined);
    expect(removeTaskFromBoard).toHaveBeenCalledWith(
      "a",
      expect.objectContaining({ wasActiveTaskId: "a" }),
    );
  });
});

describe("useSidebarMultiSelect — bulk move", () => {
  it("bulkMove classifies a same-workflow target as a step move", async () => {
    seedSnapshot(["a"]);
    const { result } = renderSidebarMultiSelect("ws1");
    act(() => result.current.toggleSelect("a"));
    await act(async () => {
      await result.current.bulkMove(["a"], "wf1", "s1");
    });
    expect(moveTasks).toHaveBeenCalledWith(["a"], "wf1", "s1", "step");
    expect(result.current.selectedIds.size).toBe(0);
  });

  it("bulkMove classifies a cross-workflow target as a workflow move", async () => {
    queryClient.setQueryData(["workflows", "wf2", "snapshot"], {
      workflow: { id: "wf2" },
      steps: [],
      tasks: [{ id: "a" }],
    });
    const { result } = renderSidebarMultiSelect("ws1");
    act(() => result.current.toggleSelect("a"));
    await act(async () => {
      await result.current.bulkMove(["a"], "wf1", "s1");
    });
    expect(moveTasks).toHaveBeenCalledWith(["a"], "wf1", "s1", "workflow");
    expect(result.current.selectedIds.size).toBe(0);
  });

  it("bulkMove keeps the selection and swallows the rejection on failure", async () => {
    moveTasks.mockRejectedValue(new Error("locked"));
    const { result } = renderSidebarMultiSelect("ws1");
    act(() => result.current.toggleSelect("a"));
    await act(async () => {
      await result.current.bulkMove(["a"], "wf1", "s1");
    });
    expect(result.current.selectedIds).toEqual(new Set(["a"]));
  });
});
