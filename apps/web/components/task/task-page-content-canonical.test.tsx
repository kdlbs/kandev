import { createElement, type ReactNode } from "react";
import { act, cleanup, renderHook } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import * as api from "@/lib/api";
import { useNextWorkflowStep } from "@/hooks/domains/kanban/use-plan-actions";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import { taskId, workflowId, workspaceId, type Task } from "@/lib/types/http";
import { useTaskDetails } from "./task-page-content";
const TASK_A = "task-a";
vi.mock("@/components/toast-provider", () => ({ useToast: () => ({ toast: vi.fn() }) }));
afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});
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

it.each([false, true])(
  "refreshes workflow actions from detail reads while preserving newer live moves (%s)",
  async (liveMove) => {
    const initialTask = {
      id: taskId(TASK_A),
      title: "Task",
      workspace_id: workspaceId("workspace"),
      workflow_id: workflowId("workflow"),
      workflow_step_id: "working",
      updated_at: "2026-10-15T10:00:00Z",
    } as Task;
    let finish!: (task: Task) => void;
    vi.spyOn(api, "fetchTask").mockImplementation(
      () =>
        new Promise((resolve) => {
          finish = resolve;
        }),
    );
    vi.spyOn(api, "listTaskSessions").mockResolvedValue({ sessions: [], total: 0 });
    const wrapper = createStateWrapper({
      workspaces: { activeId: "workspace", items: [] },
      tasks: { activeTaskId: TASK_A },
      kanban: {
        workflowId: "workflow",
        steps: [
          {
            id: "working",
            title: "Working",
            position: 0,
            events: { on_turn_complete: [{ type: "move_to_next" }] },
          },
          { id: "review", title: "Review", position: 1 },
          { id: "done", title: "Done", position: 2 },
        ],
        tasks: [
          {
            id: TASK_A,
            title: "Task",
            workflowId: "workflow",
            workflowStepId: "working",
            workspaceId: "workspace",
            updatedAt: initialTask.updated_at,
          },
        ],
      },
    });
    const { result } = renderHook(
      () => ({
        details: useTaskDetails(TASK_A, initialTask),
        nextStep: useNextWorkflowStep(TASK_A),
        store: useAppStoreApi(),
      }),
      { wrapper },
    );
    expect(result.current.nextStep.proceedStepName).toBeNull();
    await act(async () => {
      const refresh = result.current.details.refreshTask();
      if (liveMove)
        result.current.store.getState().retainTaskOverviews("detail", [
          {
            id: TASK_A,
            title: "Task",
            workflowId: "workflow",
            workflowStepId: "done",
            workspaceId: "workspace",
            position: 0,
            updatedAt: "2026-10-15T10:02:00Z",
          },
        ]);
      finish({ ...initialTask, workflow_step_id: "review", updated_at: "2026-10-15T10:01:00Z" });
      await refresh;
    });
    expect(result.current.nextStep.proceedStepName).toBe(liveMove ? null : "Done");
    expect(result.current.store.getState().taskOverview.byId[TASK_A]?.workflowStepId).toBe(
      liveMove ? "done" : "review",
    );
    expect(result.current.store.getState().taskOverview.reads).toEqual({});
  },
);
