import { act, renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

const deleteTaskById = vi.fn();
const archiveTaskById = vi.fn();
const moveTasks = vi.fn();

vi.mock("./use-task-actions", () => ({
  useTaskActions: () => ({ deleteTaskById, archiveTaskById }),
}));
vi.mock("./use-task-removal", () => ({
  useTaskRemovalSuccessNotifier: () => vi.fn(),
  useTaskRemoval: () => ({ removeTaskFromBoard: vi.fn(), runTaskRemovalBatch: vi.fn() }),
}));
vi.mock("./use-task-workflow-move", () => ({ useTaskWorkflowMove: () => moveTasks }));

type FakeTask = { id: string; workflowStepId: string };

let storeState: {
  kanban: { workflowId: string | null; tasks: FakeTask[] };
  kanbanMulti: { snapshots: Record<string, { tasks: FakeTask[] }> };
  hydrate: (patch: Record<string, unknown>) => void;
  setWorkflowSnapshot: (wfId: string, snapshot: { tasks: FakeTask[] }) => void;
};

function resetStore() {
  storeState = {
    kanban: {
      workflowId: "wf1",
      tasks: [
        { id: "a", workflowStepId: "step-1" },
        { id: "b", workflowStepId: "step-1" },
      ],
    },
    kanbanMulti: { snapshots: {} },
    hydrate(patch) {
      storeState = { ...storeState, ...(patch as typeof storeState) };
    },
    setWorkflowSnapshot(wfId, snapshot) {
      storeState.kanbanMulti.snapshots[wfId] = snapshot;
    },
  };
}

const store = { getState: () => storeState };
vi.mock("@/components/state-provider", () => ({ useAppStoreApi: () => store }));

import { useTaskMultiSelect } from "./use-task-multi-select";

beforeEach(() => {
  resetStore();
  deleteTaskById.mockReset().mockResolvedValue(undefined);
  archiveTaskById.mockReset().mockResolvedValue(undefined);
  moveTasks.mockReset().mockResolvedValue(undefined);
});

describe("useTaskMultiSelect — bulk move", () => {
  it("routes the whole selection through the batch endpoint in one call", async () => {
    const { result } = renderHook(() => useTaskMultiSelect("wf1"));
    act(() => result.current.toggleSelect("a"));
    act(() => result.current.toggleSelect("b"));

    await act(async () => {
      await result.current.bulkMove("step-2");
    });

    // A single batched call, not one moveTaskById per selected task — the
    // previous Promise.allSettled fan-out raced each task's arrival position
    // against the others' (REQ-TASKS-KANBAN-TASK-REORDERING-001.29).
    expect(moveTasks).toHaveBeenCalledTimes(1);
    expect(moveTasks).toHaveBeenCalledWith(["a", "b"], "wf1", "step-2", "step");
    expect(storeState.kanban.tasks.map((t) => t.workflowStepId)).toEqual(["step-2", "step-2"]);
  });

  it("does nothing for an empty selection", async () => {
    const { result } = renderHook(() => useTaskMultiSelect("wf1"));
    await act(async () => {
      await result.current.bulkMove("step-2");
    });
    expect(moveTasks).not.toHaveBeenCalled();
  });

  it("does nothing without a workflow id", async () => {
    const { result } = renderHook(() => useTaskMultiSelect(null));
    act(() => result.current.toggleSelect("a"));
    await act(async () => {
      await result.current.bulkMove("step-2");
    });
    expect(moveTasks).not.toHaveBeenCalled();
  });

  it("leaves the store unchanged and swallows the rejection on failure", async () => {
    moveTasks.mockRejectedValue(new Error("locked"));
    const { result } = renderHook(() => useTaskMultiSelect("wf1"));
    act(() => result.current.toggleSelect("a"));

    await act(async () => {
      await result.current.bulkMove("step-2");
    });

    expect(storeState.kanban.tasks.find((t) => t.id === "a")?.workflowStepId).toBe("step-1");
    // The selection itself is left as-is by bulkMove; useTaskWorkflowMove
    // already surfaced the failure toast.
    expect(result.current.selectedIds).toEqual(new Set(["a"]));
  });
});
