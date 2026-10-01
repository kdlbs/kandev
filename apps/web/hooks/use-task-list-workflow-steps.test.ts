import { act, renderHook, waitFor, cleanup } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import type { Task, Workflow } from "@/lib/types/http";

const mocks = vi.hoisted(() => ({
  listWorkflowSteps: vi.fn(),
  handlers: new Map<string, (message: { payload: { step: { workflow_id: string } } }) => void>(),
  state: {
    workflows: {
      items: [] as Array<{ id: string; workspaceId: string; name: string; sortOrder: number }>,
    },
    connection: { status: "connected" },
    kanban: {
      workflowId: "wf",
      steps: [] as Array<{ id: string; title: string; position: number }>,
    },
    kanbanMulti: { snapshots: {} },
  },
}));
vi.mock("@/lib/ws/connection", () => ({
  useWebSocketClient: () => ({
    on: (
      action: string,
      handler: (message: { payload: { step: { workflow_id: string } } }) => void,
    ) => {
      mocks.handlers.set(action, handler);
      return () => mocks.handlers.delete(action);
    },
  }),
}));
vi.mock("@/lib/api/domains/workflow-api", () => ({ listWorkflowSteps: mocks.listWorkflowSteps }));
vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: typeof mocks.state) => unknown) => selector(mocks.state),
}));
import { useTaskListWorkflowSteps, useTasksListStepRefresh } from "./use-task-list-workflow-steps";

const tasks = [
  { id: "task", workspace_id: "ws", workflow_id: "wf", workflow_step_id: "step" },
] as Task[];
const workflows = [{ id: "wf", workspace_id: "ws" }] as Workflow[];
const response = (name: string) => ({
  steps: [{ id: "step", name, color: "#123456", position: 0 }],
});
beforeEach(() => {
  mocks.listWorkflowSteps.mockReset();
  mocks.handlers.clear();
  mocks.state.workflows.items = [{ id: "wf", workspaceId: "ws", name: "Delivery", sortOrder: 0 }];
  mocks.state.kanban.steps = [];
  mocks.listWorkflowSteps.mockResolvedValue(response("Build"));
});
afterEach(cleanup);

// @covers AC-UI-LIST-STEP-GROUPING-001.4
it("reads only workflows represented by authorized tasks in this workspace", async () => {
  const foreign = { ...tasks[0], workspace_id: "other", workflow_id: "foreign" } as Task;
  const { result } = renderHook(() =>
    useTaskListWorkflowSteps("ws", [tasks[0], foreign], workflows, true),
  );
  await waitFor(() => expect(result.current.previews.wf?.status).toBe("success"));
  expect(mocks.listWorkflowSteps.mock.calls.map(([id]) => id)).toEqual(["wf"]);
  expect(result.current.previews.wf).toMatchObject({ steps: [{ title: "Build" }] });
});

it("does not read step metadata for another grouping", () => {
  const { result } = renderHook(() => useTaskListWorkflowSteps("ws", tasks, workflows, false));
  expect(result.current.previews).toEqual({});
  expect(mocks.listWorkflowSteps).not.toHaveBeenCalled();
});

it("retries failed metadata when the list refreshes", async () => {
  mocks.listWorkflowSteps.mockRejectedValueOnce(new Error("unavailable"));
  const { result } = renderHook(() => useTaskListWorkflowSteps("ws", tasks, workflows, true));
  await waitFor(() => expect(result.current.previews.wf?.status).toBe("error"));
  act(() => result.current.refresh());
  await waitFor(() =>
    expect(result.current.previews.wf).toMatchObject({
      status: "success",
      steps: [{ title: "Build" }],
    }),
  );
});

it("refreshes metadata and tasks together while preserving foreground task refresh", async () => {
  const fetchTasks = vi.fn().mockResolvedValue(undefined);
  const { result, unmount } = renderHook(() =>
    useTasksListStepRefresh({ activeWorkspaceId: "ws", fetchTasks }, tasks, "workflow_step"),
  );
  await waitFor(() => expect(result.current.previews.wf?.status).toBe("success"));
  mocks.listWorkflowSteps.mockResolvedValue(response("Updated"));
  await act(() => result.current.refresh());
  await waitFor(() =>
    expect(result.current.previews.wf).toMatchObject({ steps: [{ title: "Updated" }] }),
  );
  expect(fetchTasks).toHaveBeenCalledWith();
  act(() => window.dispatchEvent(new Event("focus")));
  await waitFor(() => expect(fetchTasks).toHaveBeenCalledWith(true));
  unmount();
  expect(mocks.handlers.size).toBe(0);
});

it("reads a newly created workflow from the store after a task-list refresh", async () => {
  const fetchTasks = vi.fn().mockResolvedValue(undefined);
  const newTask = { ...tasks[0], workflow_id: "new" } as Task;
  const { result, rerender } = renderHook(() =>
    useTasksListStepRefresh({ activeWorkspaceId: "ws", fetchTasks }, [newTask], "workflow_step"),
  );
  expect(mocks.listWorkflowSteps).not.toHaveBeenCalled();
  mocks.state.workflows.items = [
    ...mocks.state.workflows.items,
    { id: "new", workspaceId: "ws", name: "New delivery", sortOrder: 2 },
  ];
  rerender();
  await waitFor(() =>
    expect(mocks.listWorkflowSteps).toHaveBeenCalledWith("new", expect.anything()),
  );
  await waitFor(() => expect(result.current.previews.new?.status).toBe("success"));
  expect(result.current.workflows).toContainEqual({
    id: "new",
    workspace_id: "ws",
    name: "New delivery",
    sort_order: 2,
  });
});

it("reads new workspace workflows while old page props remain unchanged", async () => {
  const fetchTasks = vi.fn().mockResolvedValue(undefined);
  const nextTasks = [{ ...tasks[0], workspace_id: "next", workflow_id: "next-wf" }] as Task[];
  const { result, rerender } = renderHook(() =>
    useTasksListStepRefresh({ activeWorkspaceId: "next", fetchTasks }, nextTasks, "workflow_step"),
  );
  expect(mocks.listWorkflowSteps).not.toHaveBeenCalled();
  mocks.state.workflows.items = [
    ...mocks.state.workflows.items,
    { id: "next-wf", workspaceId: "next", name: "Support", sortOrder: 1 },
  ];
  rerender();
  await waitFor(() =>
    expect(mocks.listWorkflowSteps).toHaveBeenCalledWith("next-wf", expect.anything()),
  );
  await waitFor(() => expect(result.current.previews["next-wf"]?.status).toBe("success"));
  expect(result.current.workflows).toEqual([
    { id: "next-wf", workspace_id: "next", name: "Support", sort_order: 1 },
  ]);
});

it("refreshes relevant step notifications on a cold list but not task rerenders or foreign workflows", async () => {
  const { result, rerender } = renderHook(() =>
    useTaskListWorkflowSteps("ws", tasks, workflows, true),
  );
  await waitFor(() => expect(result.current.previews.wf?.status).toBe("success"));
  rerender();
  expect(mocks.listWorkflowSteps).toHaveBeenCalledTimes(1);
  expect(mocks.handlers.get("workflow.step.updated")).toBeDefined();
  act(() =>
    mocks.handlers.get("workflow.step.updated")?.({
      payload: { step: { workflow_id: "foreign" } },
    }),
  );
  expect(mocks.listWorkflowSteps).toHaveBeenCalledTimes(1);
  mocks.listWorkflowSteps.mockResolvedValue(response("Renamed"));
  act(() =>
    mocks.handlers.get("workflow.step.updated")?.({ payload: { step: { workflow_id: "wf" } } }),
  );
  await waitFor(() =>
    expect(result.current.previews.wf).toMatchObject({ steps: [{ title: "Renamed" }] }),
  );
});
