import { act, cleanup, renderHook } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, expect, it } from "vitest";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import { useTaskReferenceDetails } from "./use-task-reference-details";

afterEach(cleanup);
// @covers AC-UI-AGENT-TASK-REFERENCE-001.3
it("reads sidebar-owned task details without opening a board or task", () => {
  const wrapper = ({ children }: { children: ReactNode }) => (
    <StateProvider>{children}</StateProvider>
  );
  const { result } = renderHook(
    () => ({ details: useTaskReferenceDetails("target"), store: useAppStoreApi() }),
    { wrapper },
  );
  const task = {
    id: "target",
    title: "Reconnect",
    description: "Keep terminal history when the connection returns.",
    workflowId: "workflow",
    workflowStepId: "step",
    position: 0,
    state: "COMPLETED" as const,
    parentTaskId: "parent",
    createdAt: "2026-11-16T12:30:00Z",
    sessionCount: 2,
  };
  act(() => {
    result.current.store
      .getState()
      .retainTaskOverviews("sidebar", [
        task,
        { ...task, id: "parent", title: "Terminal reliability", parentTaskId: null },
      ]);
    result.current.store.setState((state) => ({
      workflows: {
        ...state.workflows,
        items: [{ id: "workflow", name: "Reference testing", workspaceId: "workspace" }],
      },
    }));
  });
  expect(result.current.store.getState().kanban.tasks).toEqual([]);
  expect(result.current.details.task?.description).toBe(task.description);
  expect(result.current.details.task?.state).toBe("COMPLETED");
  expect(result.current.details.workflowName).toBe("Reference testing");
  expect(result.current.details.parentTask?.title).toBe("Terminal reliability");
  expect(result.current.details.createdTimestamp).toBe(Date.parse(task.createdAt));
  expect(result.current.details.task?.sessionCount).toBe(2);
  act(() =>
    result.current.store.getState().retainTaskOverviews("sidebar", [
      {
        ...task,
        title: "Reconnect fixed",
        description: "History now survives reconnects.",
        createdAt: "invalid",
      },
    ]),
  );
  expect(result.current.details.task?.title).toBe("Reconnect fixed");
  expect(result.current.details.task?.description).toBe("History now survives reconnects.");
  expect(result.current.details.parentTask).toBeNull();
  expect(result.current.details.createdTimestamp).toBeNull();
  act(() => result.current.store.getState().releaseTaskOverviews("sidebar"));
  expect(result.current.details.task).toBeNull();
});

// @covers AC-UI-AGENT-TASK-REFERENCE-001.3, AC-UI-AGENT-TASK-REFERENCE-001.5
it("reads loaded metadata and follows rename/removal without fetching", () => {
  const wrapper = ({ children }: { children: ReactNode }) => (
    <StateProvider>{children}</StateProvider>
  );
  const { result } = renderHook(
    () => ({ details: useTaskReferenceDetails("target"), store: useAppStoreApi() }),
    { wrapper },
  );
  expect(result.current.details.task).toBeNull();
  act(() =>
    result.current.store.setState((state) => ({
      kanban: {
        ...state.kanban,
        workflowId: "workflow",
        tasks: [
          {
            id: "target",
            title: "Original",
            workflowId: "workflow",
            workflowStepId: "step",
            position: 0,
            state: "COMPLETED",
          },
        ],
      },
      workflows: {
        ...state.workflows,
        items: [{ id: "workflow", name: "Review board", workspaceId: "workspace" }],
      },
    })),
  );
  expect(result.current.details.task?.title).toBe("Original");
  expect(result.current.details.workflowName).toBe("Review board");
  act(() =>
    result.current.store.setState((state) => ({
      kanban: {
        ...state.kanban,
        tasks: state.kanban.tasks.map((task) => ({ ...task, title: "Renamed" })),
      },
    })),
  );
  expect(result.current.details.task?.title).toBe("Renamed");
  act(() => result.current.store.setState((state) => ({ kanban: { ...state.kanban, tasks: [] } })));
  expect(result.current.details.task).toBeNull();
  expect(result.current.details.workflowName).toBeNull();
});

it.each(["workflow"])("resolves snapshot workflow names with workflow ID %s", (workflowId) => {
  const task = { id: "target", title: "Target", workflowId, workflowStepId: "step", position: 0 };
  const wrapper = ({ children }: { children: ReactNode }) => (
    <StateProvider>{children}</StateProvider>
  );
  const { result } = renderHook(
    () => ({ details: useTaskReferenceDetails("target"), store: useAppStoreApi() }),
    { wrapper },
  );
  act(() =>
    result.current.store.setState((state) => ({
      kanban: { ...state.kanban, tasks: [task] },
      kanbanMulti: {
        ...state.kanbanMulti,
        snapshots: {
          workflow: {
            workflowId: "workflow",
            workflowName: "Snapshot board",
            tasks: [task],
            steps: [],
          },
        },
      },
    })),
  );
  expect(result.current.details.workflowName).toBe("Snapshot board");
});
