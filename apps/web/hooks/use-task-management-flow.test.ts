import { act, renderHook } from "@testing-library/react";
import { useStore } from "zustand";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { createAppStore, type AppState } from "@/lib/state/store";
import { useTaskManagementFlow } from "./use-task-management-flow";

let store: ReturnType<typeof createAppStore>;
vi.mock("@/components/state-provider", () => ({
  useAppStoreApi: () => store,
  useAppStore: (selector: (state: AppState) => unknown) => useStore(store, selector),
}));
beforeEach(() => {
  store = createAppStore();
  store.setState((state) => ({
    workspaces: { ...state.workspaces, activeId: "workspace" },
    workflows: {
      ...state.workflows,
      items: [{ id: "workflow", name: "Flow", workspaceId: "workspace" }],
    },
    kanbanMulti: {
      ...state.kanbanMulti,
      snapshots: {
        workflow: {
          workflowId: "workflow",
          workflowName: "Flow",
          steps: [],
          tasks: ["A", "B"].map((id) => ({
            id,
            title: id,
            workflowId: "workflow",
            workflowStepId: "step",
            position: 0,
          })),
        },
      },
    },
  }));
});

describe("captured task flow", () => {
  // @covers AC-TASKS-THREADS-ACTIONS-001.3
  it.each([true, false])(
    "uses hydrated active steps only when the multi-workflow snapshot is a placeholder (%s)",
    (isPlaceholder) => {
      const steps = [{ id: "next", title: "Next", position: 1, color: "" }];
      store.setState((state) => ({
        kanban: { ...state.kanban, workflowId: "workflow", steps },
        kanbanMulti: {
          ...state.kanbanMulti,
          snapshots: {
            workflow: { ...state.kanbanMulti.snapshots.workflow, isPlaceholder },
          },
        },
      }));
      const { result } = renderHook(() => useTaskManagementFlow());
      act(() => result.current.open("A"));
      expect(result.current.stepsByWorkflowId.workflow).toEqual(isPlaceholder ? steps : []);
    },
  );
  it("does not borrow another workflow's steps for a placeholder", () => {
    store.setState((state) => ({
      kanban: {
        ...state.kanban,
        workflowId: "other",
        steps: [{ id: "other-step", title: "Other", position: 0, color: "" }],
      },
      kanbanMulti: {
        ...state.kanbanMulti,
        snapshots: {
          workflow: { ...state.kanbanMulti.snapshots.workflow, isPlaceholder: true },
        },
      },
    }));
    const { result } = renderHook(() => useTaskManagementFlow());
    act(() => result.current.open("A"));
    expect(result.current.stepsByWorkflowId.workflow).toEqual([]);
  });
  it("does not revive a dismissed flow when the previous workspace becomes active again", () => {
    const { result } = renderHook(() => useTaskManagementFlow());
    act(() => result.current.open("A"));
    act(() =>
      store.setState((state) => ({ workspaces: { ...state.workspaces, activeId: "elsewhere" } })),
    );
    expect(result.current.stage).toBe("closed");
    act(() =>
      store.setState((state) => ({ workspaces: { ...state.workspaces, activeId: "workspace" } })),
    );
    expect(result.current.stage).toBe("closed");
  });
  // @covers AC-TASKS-THREADS-ACTIONS-002.1, AC-TASKS-THREADS-ACTIONS-002.2
  it("keeps A through selection and session changes until another menu explicitly opens", () => {
    const { result } = renderHook(() => useTaskManagementFlow());
    act(() => {
      result.current.open("A");
    });
    act(() => {
      store.getState().setActiveSession("B", "session-B");
    });
    expect(result.current.identity).toEqual({ taskId: "A", workspaceId: "workspace" });
    act(() => {
      result.current.open("B");
    });
    expect(result.current.identity?.taskId).toBe("B");
  });
  // @covers AC-TASKS-THREADS-ACTIONS-002.3
  it("does not open a menu for an unresolved target", () => {
    const { result } = renderHook(() => useTaskManagementFlow());
    act(() => {
      result.current.open("missing");
    });
    expect(result.current.identity).toBeNull();
  });
});
