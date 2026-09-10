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
