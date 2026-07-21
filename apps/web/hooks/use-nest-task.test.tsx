import { act, renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { useNestTask } from "./use-nest-task";

const updateTaskMock = vi.hoisted(() => vi.fn());
const setWorkflowSnapshotMock = vi.hoisted(() => vi.fn());
const store = vi.hoisted(() => ({ state: {} as Record<string, unknown> }));

vi.mock("@/lib/api", () => ({ updateTask: updateTaskMock }));
vi.mock("@/components/state-provider", () => ({
  useAppStoreApi: () => ({ getState: () => store.state }),
}));
vi.mock("sonner", () => ({ toast: { error: vi.fn() } }));

describe("useNestTask", () => {
  beforeEach(() => {
    updateTaskMock.mockReset().mockResolvedValue(undefined);
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

  it("clears the parent by sending an empty parent_id (un-nest)", async () => {
    store.state = { kanbanMulti: { snapshots: {} }, setWorkflowSnapshot: setWorkflowSnapshotMock };
    const { result } = renderHook(() => useNestTask());

    await act(async () => {
      await result.current("task-1", "wf-1", null);
    });

    expect(updateTaskMock).toHaveBeenCalledWith("task-1", { parent_id: "" });
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
