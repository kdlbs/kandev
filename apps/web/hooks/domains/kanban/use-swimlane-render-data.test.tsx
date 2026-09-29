import { act, renderHook } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import type { WorkflowSnapshotData } from "@/lib/state/slices/kanban/types";
import { useSwimlaneRenderData } from "./use-swimlane-render-data";

const REPOSITORIES: string[] = [];

function snapshot(id: string): WorkflowSnapshotData {
  return {
    workflowId: id,
    workflowName: id,
    steps: [{ id: `${id}-step`, title: "Ready", color: "blue", position: 0 }],
    tasks: [
      { id: `${id}-task`, title: id, workflowId: id, workflowStepId: `${id}-step`, position: 0 },
    ],
  };
}

function renderOverview() {
  return renderHook(
    () => ({ store: useAppStoreApi(), data: useSwimlaneRenderData(null, REPOSITORIES, "") }),
    {
      wrapper: ({ children }) => <StateProvider>{children}</StateProvider>,
    },
  );
}

// @covers AC-UI-TASK-NAVIGATION-RESPONSIVENESS-001.1
describe("overview snapshot hydration", () => {
  it("renders a missing snapshot beside a loaded workflow", () => {
    const { result } = renderOverview();
    act(() => {
      result.current.store.setState((state) => ({
        kanbanMulti: { ...state.kanbanMulti, snapshots: { loaded: snapshot("loaded") } },
      }));
    });

    expect(result.current.data.getFilteredTasks("loaded").map((task) => task.id)).toEqual([
      "loaded-task",
    ]);
    expect(result.current.data.getFilteredTasks("unloaded")).toEqual([]);
  });

  it("keeps filters scoped as another snapshot arrives and is removed", () => {
    const { result } = renderOverview();
    act(() => {
      result.current.store.setState((state) => ({
        kanbanMulti: { ...state.kanbanMulti, snapshots: { first: snapshot("first") } },
        userSettings: {
          ...state.userSettings,
          hiddenWorkflowStepIds: { first: ["first-step"] },
        },
      }));
    });
    expect(result.current.data.getFilteredTasks("first")).toEqual([]);
    expect(result.current.data.getFilteredTasks("second")).toEqual([]);

    act(() => {
      result.current.store.setState((state) => ({
        kanbanMulti: {
          ...state.kanbanMulti,
          snapshots: { ...state.kanbanMulti.snapshots, second: snapshot("second") },
        },
      }));
    });
    expect(result.current.data.getFilteredTasks("second").map((task) => task.id)).toEqual([
      "second-task",
    ]);
    expect(result.current.data.getFilteredTasks("first")).toEqual([]);

    act(() => {
      result.current.store.setState((state) => ({
        kanbanMulti: { ...state.kanbanMulti, snapshots: { first: snapshot("first") } },
        userSettings: { ...state.userSettings, hiddenWorkflowStepIds: {} },
      }));
    });
    expect(result.current.data.getFilteredTasks("second")).toEqual([]);
    expect(result.current.data.getFilteredTasks("first").map((task) => task.id)).toEqual([
      "first-task",
    ]);
  });

  it("reuses the projection while its inputs are unchanged", () => {
    const { result, rerender } = renderOverview();
    act(() => {
      result.current.store.setState((state) => ({
        kanbanMulti: { ...state.kanbanMulti, snapshots: { first: snapshot("first") } },
      }));
    });
    const tasks = result.current.data.getFilteredTasks("first");
    rerender();
    expect(result.current.data.getFilteredTasks("first")).toBe(tasks);
  });
});
