import { createElement, type ReactNode } from "react";
import { act, cleanup, renderHook } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import { taskId, workflowId, workspaceId, type Task } from "@/lib/types/http";
import { useTaskDetails } from "./task-page-content";
const TASK_A = "task-a";
afterEach(cleanup);
function createStateWrapper(initialState: unknown) {
  return function Wrapper({ children }: { children: ReactNode }) {
    return createElement(StateProvider, { initialState: initialState as never, children });
  };
}

it("keeps a moved detail task current after it leaves the loaded source board", () => {
  const initialTask = {
    id: taskId(TASK_A),
    title: "Moved task",
    workflow_id: workflowId("source"),
    workflow_step_id: "old",
    workspace_id: workspaceId("workspace"),
    updated_at: "2026-10-10T10:00:00Z",
  } as Task;
  const wrapper = createStateWrapper({
    workspaces: { activeId: "workspace", items: [] },
    tasks: { activeTaskId: TASK_A },
    kanban: {
      workflowId: "source",
      tasks: [
        {
          id: TASK_A,
          title: "Moved task",
          workflowId: "source",
          workflowStepId: "old",
          workspaceId: "workspace",
          updatedAt: initialTask.updated_at,
        },
      ],
    },
  });
  const { result } = renderHook(
    () => ({ details: useTaskDetails(TASK_A, initialTask), store: useAppStoreApi() }),
    { wrapper },
  );
  act(() =>
    result.current.store.getState().retainTaskOverviews("sidebar:fixture", [
      {
        id: TASK_A,
        title: "Moved task",
        workflowId: "destination",
        workflowStepId: "analysis",
        workspaceId: "workspace",
        updatedAt: "2026-10-10T11:00:00Z",
        position: 0,
      },
    ]),
  );
  expect(result.current.store.getState().kanban.tasks).toEqual([]);
  expect(result.current.details.task?.workflow_id).toBe("destination");
  expect(result.current.details.task?.workflow_step_id).toBe("analysis");
});
