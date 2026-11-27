import { act, renderHook, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { createAppStore } from "@/lib/state/store";
import { useSheetArchiveActions } from "./session-task-switcher-sheet-hooks";

const WORKSPACE_ID = "workspace-1";
const TASK_ONE = "task-1";
const TASK_TWO = "task-2";

function makeStore() {
  const store = createAppStore();
  store.setState((state) => ({
    ...state,
    workspaces: { ...state.workspaces, activeId: WORKSPACE_ID },
  }));
  return store;
}

function beginRemoval(
  store: ReturnType<typeof createAppStore>,
  taskId: string,
  action: "archive" | "delete" = "archive",
): string {
  const token = store.getState().beginTaskRemoval({
    action,
    workspaceId: WORKSPACE_ID,
    taskIds: [taskId],
    requestIds: [taskId],
    departure: null,
  });
  if (!token) throw new Error("removal was not accepted");
  return token;
}

describe("useSheetArchiveActions", () => {
  let store: ReturnType<typeof createAppStore>;

  beforeEach(() => {
    store = makeStore();
  });

  it("tracks its confirmation target through that target's accepted removal", () => {
    const archiveAndSwitch = vi.fn();
    const { result } = renderHook(() =>
      useSheetArchiveActions(
        store as Parameters<typeof useSheetArchiveActions>[0],
        archiveAndSwitch as unknown as Parameters<typeof useSheetArchiveActions>[1],
        WORKSPACE_ID,
      ),
    );

    act(() => result.current.handleArchiveTask(TASK_ONE));
    expect(result.current.archivingTask?.id).toBe(TASK_ONE);
    expect(result.current.isArchiving).toBe(false);

    let token!: string;
    act(() => {
      token = beginRemoval(store, TASK_ONE);
    });
    expect(result.current.isArchiving).toBe(true);

    act(() => {
      store.getState().releaseTaskRemoval(token);
    });
    expect(result.current.isArchiving).toBe(false);
    expect(result.current.archivingTask?.id).toBe(TASK_ONE);
  });

  it("keeps this target busy while an independent archive settles", () => {
    const archiveAndSwitch = vi.fn();
    const { result } = renderHook(() =>
      useSheetArchiveActions(
        store as Parameters<typeof useSheetArchiveActions>[0],
        archiveAndSwitch as unknown as Parameters<typeof useSheetArchiveActions>[1],
        WORKSPACE_ID,
      ),
    );
    act(() => result.current.handleArchiveTask(TASK_ONE));
    let ownToken!: string;
    let otherToken!: string;
    act(() => {
      ownToken = beginRemoval(store, TASK_ONE);
      otherToken = beginRemoval(store, TASK_TWO, "delete");
    });
    expect(result.current.isArchiving).toBe(true);

    act(() => {
      store.getState().recordTaskRemovalResult(otherToken, [TASK_TWO], "failed");
      store.getState().releaseTaskRemoval(otherToken);
    });
    expect(result.current.isArchiving).toBe(true);

    act(() => {
      store.getState().releaseTaskRemoval(ownToken);
    });
    expect(result.current.isArchiving).toBe(false);
  });

  it("does not mark the confirmation busy for another target's removal", () => {
    const archiveAndSwitch = vi.fn();
    const { result } = renderHook(() =>
      useSheetArchiveActions(
        store as Parameters<typeof useSheetArchiveActions>[0],
        archiveAndSwitch as unknown as Parameters<typeof useSheetArchiveActions>[1],
        WORKSPACE_ID,
      ),
    );
    act(() => result.current.handleArchiveTask(TASK_ONE));

    let otherToken!: string;
    act(() => {
      otherToken = beginRemoval(store, TASK_TWO);
    });
    expect(result.current.isArchiving).toBe(false);

    act(() => {
      store.getState().releaseTaskRemoval(otherToken);
    });
    expect(result.current.isArchiving).toBe(false);
  });

  it("clears the confirmation target after a direct archive run", async () => {
    let releaseArchive!: () => void;
    const archiveAndSwitch = vi.fn(
      () =>
        new Promise<void>((resolve) => {
          releaseArchive = resolve;
        }),
    );
    const { result } = renderHook(() =>
      useSheetArchiveActions(
        store as Parameters<typeof useSheetArchiveActions>[0],
        archiveAndSwitch as unknown as Parameters<typeof useSheetArchiveActions>[1],
        WORKSPACE_ID,
      ),
    );

    act(() => result.current.handleArchiveTask(TASK_ONE, { cascade: false }));
    await act(async () => {
      releaseArchive();
    });
    await waitFor(() =>
      expect(archiveAndSwitch).toHaveBeenCalledWith(TASK_ONE, { cascade: false }),
    );
    expect(result.current.archivingTask).toBeNull();
  });
});
