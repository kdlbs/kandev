import { act, renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { useNestTask } from "./use-nest-task";

const updateTaskMock = vi.hoisted(() => vi.fn());
const detachTaskMock = vi.hoisted(() => vi.fn());
const setWorkflowSnapshotMock = vi.hoisted(() => vi.fn());
const store = vi.hoisted(() => ({ state: {} as Record<string, unknown> }));

vi.mock("@/lib/api", () => ({ updateTask: updateTaskMock, detachTask: detachTaskMock }));
vi.mock("@/components/state-provider", () => ({
  useAppStoreApi: () => ({ getState: () => store.state }),
}));
vi.mock("sonner", () => ({ toast: { error: vi.fn() } }));

describe("useNestTask", () => {
  beforeEach(() => {
    updateTaskMock.mockReset().mockResolvedValue(undefined);
    detachTaskMock.mockReset().mockResolvedValue(undefined);
    setWorkflowSnapshotMock.mockReset();
  });

  it("still sends the reparent request when the multi snapshot is missing", async () => {
    // Initial-load fallback state: no snapshot yet for this workflow.
    store.state = { kanbanMulti: { snapshots: {} }, setWorkflowSnapshot: setWorkflowSnapshotMock };
    const { result } = renderHook(() => useNestTask());

    await act(async () => {
      await result.current("task-1", "wf-1", "parent-1");
    });

    expect(updateTaskMock).toHaveBeenCalledWith("task-1", { parent_id: "parent-1" });
    // No optimistic snapshot write is attempted without a snapshot.
    expect(setWorkflowSnapshotMock).not.toHaveBeenCalled();
  });

  it("un-nests through the detach endpoint so workspace mode is normalized", async () => {
    store.state = { kanbanMulti: { snapshots: {} }, setWorkflowSnapshot: setWorkflowSnapshotMock };
    const { result } = renderHook(() => useNestTask());

    await act(async () => {
      await result.current("task-1", "wf-1", null);
    });

    // Detach (not a plain parent_id clear) also resets an inherit_parent
    // subtask's workspace mode to shared_group and emits the Office event.
    expect(detachTaskMock).toHaveBeenCalledWith("task-1");
    expect(updateTaskMock).not.toHaveBeenCalled();
  });

  it("is a no-op when the parent is unchanged and the snapshot is present", async () => {
    const snapshot = { tasks: [{ id: "task-1", parentTaskId: "parent-1" }] };
    store.state = {
      kanbanMulti: { snapshots: { "wf-1": snapshot } },
      setWorkflowSnapshot: setWorkflowSnapshotMock,
    };
    const { result } = renderHook(() => useNestTask());

    await act(async () => {
      await result.current("task-1", "wf-1", "parent-1"); // already nested there
    });

    expect(updateTaskMock).not.toHaveBeenCalled();
    expect(setWorkflowSnapshotMock).not.toHaveBeenCalled();
  });

  it("optimistically patches the snapshot when present, then persists", async () => {
    const snapshot = { tasks: [{ id: "task-1", parentTaskId: undefined }] };
    store.state = {
      kanbanMulti: { snapshots: { "wf-1": snapshot } },
      setWorkflowSnapshot: setWorkflowSnapshotMock,
    };
    const { result } = renderHook(() => useNestTask());

    await act(async () => {
      await result.current("task-1", "wf-1", "parent-1");
    });

    expect(setWorkflowSnapshotMock).toHaveBeenCalled();
    expect(updateTaskMock).toHaveBeenCalledWith("task-1", { parent_id: "parent-1" });
  });
});
