import { act, cleanup, renderHook, screen } from "@testing-library/react";
import { createElement, type ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import { ToastProvider } from "@/components/toast-provider";
import type { AppState } from "@/lib/state/store";
import type { TaskPR } from "@/lib/types/github";

const deleteTaskPRMock = vi.hoisted(() => vi.fn());
const invalidateTaskPRSyncMock = vi.hoisted(() => vi.fn());

vi.mock("@/lib/api/domains/github-api", () => ({
  deleteTaskPR: deleteTaskPRMock,
}));

vi.mock("./task-pr-sync-resource", () => ({
  getTaskPRSyncResource: () => ({ invalidate: invalidateTaskPRSyncMock }),
}));

import { useTaskPRUnlink } from "./use-task-pr-unlink";

const linkedPR = { id: "association-1", task_id: "task-1" } as TaskPR;

function initialState(workspaceId: string | null): Partial<AppState> {
  return {
    workspaces: { items: [], activeId: workspaceId },
    taskPRs: { byTaskId: { "task-1": [linkedPR] } },
  };
}

function createWrapper(workspaceId: string | null) {
  return function TestWrapper({ children }: { children: ReactNode }) {
    return createElement(StateProvider, {
      initialState: initialState(workspaceId),
      children: createElement(ToastProvider, null, children),
    });
  };
}

function useUnlinkWithStore() {
  const unlink = useTaskPRUnlink("task-1");
  const store = useAppStoreApi();
  return { unlink, store };
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((resolvePromise) => {
    resolve = resolvePromise;
  });
  return { promise, resolve };
}

beforeEach(() => {
  deleteTaskPRMock.mockReset();
  invalidateTaskPRSyncMock.mockReset();
});
afterEach(() => cleanup());

describe("useTaskPRUnlink", () => {
  // @covers AC-INTEGRATIONS-GITHUB-PR-UNLINK-MENUS-001.4, .5
  it("guards duplicate requests for one association and removes it after success", async () => {
    const pending = deferred<void>();
    deleteTaskPRMock.mockReturnValue(pending.promise);
    const view = renderHook(() => useUnlinkWithStore(), {
      wrapper: createWrapper("ws-1"),
    });
    let first!: Promise<boolean>;
    let duplicate!: Promise<boolean>;

    act(() => {
      first = view.result.current.unlink.unlink(linkedPR.id);
      duplicate = view.result.current.unlink.unlink(linkedPR.id);
    });

    expect(deleteTaskPRMock).toHaveBeenCalledOnce();
    expect(view.result.current.unlink.pendingIds.has(linkedPR.id)).toBe(true);
    await expect(duplicate).resolves.toBe(false);

    await act(async () => {
      pending.resolve();
      await first;
    });

    expect(deleteTaskPRMock).toHaveBeenCalledWith(linkedPR.id, "ws-1");
    expect(view.result.current.store.getState().taskPRs.byTaskId["task-1"]).toBeUndefined();
    expect(view.result.current.unlink.pendingIds.has(linkedPR.id)).toBe(false);
  });

  // @covers AC-INTEGRATIONS-GITHUB-PR-UNLINK-MENUS-001.5
  it("keeps the association on failure, shows an error, and permits retry", async () => {
    deleteTaskPRMock
      .mockRejectedValueOnce(new Error("Network unavailable"))
      .mockResolvedValueOnce(undefined);
    const view = renderHook(() => useUnlinkWithStore(), {
      wrapper: createWrapper("ws-1"),
    });

    await act(async () => {
      await view.result.current.unlink.unlink(linkedPR.id);
    });

    expect(view.result.current.store.getState().taskPRs.byTaskId["task-1"]).toEqual([linkedPR]);
    expect(screen.getByText("Failed to unlink pull request")).toBeTruthy();
    expect(screen.getByText("Network unavailable")).toBeTruthy();

    await act(async () => {
      await view.result.current.unlink.unlink(linkedPR.id);
    });

    expect(deleteTaskPRMock).toHaveBeenCalledTimes(2);
    expect(view.result.current.store.getState().taskPRs.byTaskId["task-1"]).toBeUndefined();
  });

  it("disables mutation when the task has no active workspace", async () => {
    const view = renderHook(() => useTaskPRUnlink("task-1"), {
      wrapper: createWrapper(null),
    });

    expect(view.result.current.canUnlink).toBe(false);
    await expect(view.result.current.unlink(linkedPR.id)).resolves.toBe(false);
    expect(deleteTaskPRMock).not.toHaveBeenCalled();
  });

  it("does not submit an association that disappeared while its menu was open", async () => {
    const view = renderHook(() => useUnlinkWithStore(), {
      wrapper: createWrapper("ws-1"),
    });
    const state = view.result.current.store.getState();
    act(() => {
      state.removeTaskPR("task-1", linkedPR.id, {
        workspaceId: "ws-1",
        workspaceContextGeneration: state.workspaceContextGeneration,
      });
    });

    await act(async () => {
      await view.result.current.unlink.unlink(linkedPR.id);
    });

    expect(deleteTaskPRMock).not.toHaveBeenCalled();
  });
});

describe("useTaskPRUnlink workspace changes", () => {
  it("keeps the newly active workspace cache when an old unlink resolves late", async () => {
    const pending = deferred<void>();
    deleteTaskPRMock.mockReturnValue(pending.promise);
    const view = renderHook(() => useUnlinkWithStore(), {
      wrapper: createWrapper("ws-1"),
    });
    let unlink!: Promise<boolean>;

    act(() => {
      unlink = view.result.current.unlink.unlink(linkedPR.id);
    });
    expect(deleteTaskPRMock).toHaveBeenCalledWith(linkedPR.id, "ws-1");

    const workspaceBPR = { ...linkedPR, id: "association-b", task_id: "task-b" };
    act(() => {
      const state = view.result.current.store.getState();
      state.setActiveWorkspace("ws-2");
      const next = view.result.current.store.getState();
      next.setTaskPRs(
        { "task-b": [workspaceBPR] },
        {
          workspaceId: "ws-2",
          workspaceContextGeneration: next.workspaceContextGeneration,
        },
      );
    });

    await act(async () => {
      pending.resolve();
      await unlink;
    });

    const state = view.result.current.store.getState();
    expect(state.workspaces.activeId).toBe("ws-2");
    expect(state.taskPRs.workspaceId).toBe("ws-2");
    expect(state.taskPRs.byTaskId).toEqual({ "task-b": [workspaceBPR] });
    expect(invalidateTaskPRSyncMock).not.toHaveBeenCalled();
  });
});
