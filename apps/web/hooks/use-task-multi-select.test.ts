import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, renderHook } from "@testing-library/react";
import { createElement, type ReactNode } from "react";
import { afterEach, beforeEach, describe, it, expect, vi } from "vitest";
import { qk } from "@/lib/query/keys";
import type { WorkflowSnapshot } from "@/lib/types/http";
import type { WorkflowSnapshotData } from "@/lib/state/slices/kanban/types";
import { multiSelectReducer, INITIAL_STATE } from "./use-task-multi-select";
import { useTaskMultiSelect } from "./use-task-multi-select";

const archiveTaskById = vi.fn(async () => {});
const deleteTaskById = vi.fn(async () => {});
const moveTaskById = vi.fn(async () => {});

vi.mock("@/hooks/use-task-actions", () => ({
  useTaskActions: () => ({
    archiveTaskById,
    deleteTaskById,
    moveTaskById,
  }),
}));

const WORKFLOW_ID = "workflow-1";

function createQueryClient() {
  return new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Infinity } },
  });
}

function wrapperFor(queryClient: QueryClient) {
  return function Wrapper({ children }: { children: ReactNode }) {
    return createElement(QueryClientProvider, { client: queryClient }, children);
  };
}

function rawSnapshot(): WorkflowSnapshot {
  return {
    workflow: {
      id: WORKFLOW_ID,
      workspace_id: "workspace-1",
      name: "Build",
      sort_order: 0,
      hidden: false,
    },
    steps: [
      {
        id: "step-1",
        workflow_id: WORKFLOW_ID,
        name: "Todo",
        position: 0,
        color: "bg-blue-500",
        allow_manual_move: true,
      },
      {
        id: "step-2",
        workflow_id: WORKFLOW_ID,
        name: "Done",
        position: 1,
        color: "bg-green-500",
        allow_manual_move: true,
      },
    ],
    tasks: [
      {
        id: "task-1",
        workspace_id: "workspace-1",
        workflow_id: WORKFLOW_ID,
        workflow_step_id: "step-1",
        position: 0,
        title: "Task 1",
        description: "",
        state: "TODO",
        priority: 0,
        repositories: [],
        created_at: "2026-06-24T00:00:00Z",
        updated_at: "2026-06-24T00:00:00Z",
      },
    ],
  } as unknown as WorkflowSnapshot;
}

function convertedSnapshots(): Record<string, WorkflowSnapshotData> {
  return {
    [WORKFLOW_ID]: {
      workflowId: WORKFLOW_ID,
      workflowName: "Build",
      steps: [
        { id: "step-1", title: "Todo", color: "bg-blue-500", position: 0 },
        { id: "step-2", title: "Done", color: "bg-green-500", position: 1 },
      ],
      tasks: [{ id: "task-1", workflowStepId: "step-1", title: "Task 1", position: 0 }],
    },
  };
}

describe("useTaskMultiSelect query cache updates", () => {
  beforeEach(() => {
    archiveTaskById.mockClear();
    deleteTaskById.mockClear();
    moveTaskById.mockClear();
  });

  afterEach(() => {
    cleanup();
  });

  it("removes successfully archived tasks from workflow snapshot Query cache without a Zustand store", async () => {
    const queryClient = createQueryClient();
    queryClient.setQueryData(qk.workflows.snapshot(WORKFLOW_ID), rawSnapshot());
    const { result } = renderHook(() => useTaskMultiSelect(WORKFLOW_ID, convertedSnapshots()), {
      wrapper: wrapperFor(queryClient),
    });

    act(() => result.current.toggleSelect("task-1"));
    await act(async () => {
      await result.current.bulkArchive();
    });

    expect(archiveTaskById).toHaveBeenCalledWith("task-1", undefined);
    expect(
      queryClient.getQueryData<WorkflowSnapshot>(qk.workflows.snapshot(WORKFLOW_ID))?.tasks,
    ).toEqual([]);
  });

  it("moves successfully moved tasks in workflow snapshot Query cache without a Zustand store", async () => {
    const queryClient = createQueryClient();
    queryClient.setQueryData(qk.workflows.snapshot(WORKFLOW_ID), rawSnapshot());
    const { result } = renderHook(() => useTaskMultiSelect(WORKFLOW_ID, convertedSnapshots()), {
      wrapper: wrapperFor(queryClient),
    });

    act(() => result.current.toggleSelect("task-1"));
    await act(async () => {
      await result.current.bulkMove("step-2");
    });

    expect(moveTaskById).toHaveBeenCalledWith("task-1", {
      workflow_id: WORKFLOW_ID,
      workflow_step_id: "step-2",
      position: 0,
    });
    expect(
      queryClient.getQueryData<WorkflowSnapshot>(qk.workflows.snapshot(WORKFLOW_ID))?.tasks[0]
        ?.workflow_step_id,
    ).toBe("step-2");
  });
});

describe("multiSelectReducer", () => {
  it("reset returns initial state", () => {
    const dirty = {
      selectedIds: new Set(["a", "b"]),
      isMultiSelectEnabled: true,
      isDeleting: true,
      isArchiving: false,
    };
    expect(multiSelectReducer(dirty, { type: "reset" })).toBe(INITIAL_STATE);
  });

  it("toggle_select adds a task", () => {
    const next = multiSelectReducer(INITIAL_STATE, { type: "toggle_select", taskId: "t1" });
    expect(next.selectedIds).toEqual(new Set(["t1"]));
  });

  it("toggle_select removes an already-selected task", () => {
    const state = { ...INITIAL_STATE, selectedIds: new Set(["t1", "t2"]) };
    const next = multiSelectReducer(state, { type: "toggle_select", taskId: "t1" });
    expect(next.selectedIds).toEqual(new Set(["t2"]));
  });

  it("set_selected replaces the selection set", () => {
    const state = { ...INITIAL_STATE, selectedIds: new Set(["old"]) };
    const next = multiSelectReducer(state, { type: "set_selected", ids: new Set(["a", "b"]) });
    expect(next.selectedIds).toEqual(new Set(["a", "b"]));
  });

  it("set_enabled controls isMultiSelectEnabled", () => {
    const on = multiSelectReducer(INITIAL_STATE, { type: "set_enabled", value: true });
    expect(on.isMultiSelectEnabled).toBe(true);
    const off = multiSelectReducer(on, { type: "set_enabled", value: false });
    expect(off.isMultiSelectEnabled).toBe(false);
  });

  describe("bulk operation state flags", () => {
    it("set_deleting toggles isDeleting", () => {
      const on = multiSelectReducer(INITIAL_STATE, { type: "set_deleting", value: true });
      expect(on.isDeleting).toBe(true);
      const off = multiSelectReducer(on, { type: "set_deleting", value: false });
      expect(off.isDeleting).toBe(false);
    });

    it("set_archiving toggles isArchiving", () => {
      const on = multiSelectReducer(INITIAL_STATE, { type: "set_archiving", value: true });
      expect(on.isArchiving).toBe(true);
      const off = multiSelectReducer(on, { type: "set_archiving", value: false });
      expect(off.isArchiving).toBe(false);
    });
  });

  describe("bulk action scenarios (reducer-level)", () => {
    it("all succeed: selectedIds empty + enabled false", () => {
      const state = {
        ...INITIAL_STATE,
        selectedIds: new Set(["t1", "t2"]),
        isMultiSelectEnabled: true,
      };
      // Simulate: set_selected with empty failed set, then set_enabled false
      const afterSelect = multiSelectReducer(state, {
        type: "set_selected",
        ids: new Set<string>(),
      });
      const afterDisable = multiSelectReducer(afterSelect, { type: "set_enabled", value: false });
      expect(afterDisable.selectedIds.size).toBe(0);
      expect(afterDisable.isMultiSelectEnabled).toBe(false);
    });

    it("some fail: selectedIds contains failed IDs, enabled stays true", () => {
      const state = {
        ...INITIAL_STATE,
        selectedIds: new Set(["t1", "t2", "t3"]),
        isMultiSelectEnabled: true,
      };
      // Simulate: set_selected with failed IDs only, no set_enabled call
      const afterSelect = multiSelectReducer(state, {
        type: "set_selected",
        ids: new Set(["t2"]),
      });
      expect(afterSelect.selectedIds).toEqual(new Set(["t2"]));
      expect(afterSelect.isMultiSelectEnabled).toBe(true);
    });

    it("all fail: selectedIds unchanged, enabled stays true", () => {
      const state = {
        ...INITIAL_STATE,
        selectedIds: new Set(["t1", "t2"]),
        isMultiSelectEnabled: true,
      };
      const afterSelect = multiSelectReducer(state, {
        type: "set_selected",
        ids: new Set(["t1", "t2"]),
      });
      expect(afterSelect.selectedIds).toEqual(new Set(["t1", "t2"]));
      expect(afterSelect.isMultiSelectEnabled).toBe(true);
    });
  });
});
