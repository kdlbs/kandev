import { createElement, type ReactNode } from "react";
import { act, cleanup, renderHook } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import * as api from "@/lib/api";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import { ToastProvider } from "@/components/toast-provider";
import { useNextWorkflowStep } from "@/hooks/domains/kanban/use-plan-actions";
import { toKanbanTask } from "@/lib/kanban/map-task";
import { applyTaskOverviewEvent } from "@/lib/ws/handlers/task-overview";
import { taskId, workflowId, workspaceId, type Task } from "@/lib/types/http";
import { useTaskDetails } from "./task-page-content";
const TASK_A = "task-a";
afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
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

const WORKSPACE = "workspace";
const WORKFLOW = "workflow";

function makeTask(step = "working", id = TASK_A): Task {
  return {
    id: taskId(id),
    title: "Current task",
    description: "",
    state: "CREATED",
    priority: "medium",
    workspace_id: workspaceId(WORKSPACE),
    workflow_id: workflowId(WORKFLOW),
    workflow_step_id: step,
    updated_at: "2026-10-15T10:00:00Z",
    created_at: "2026-10-15T09:00:00Z",
    repositories: [],
    position: 0,
  };
}

function mountRefreshingDetail() {
  const initialTask = makeTask();
  const otherTask = makeTask("review", "task-b");
  const initialState = {
    workspaces: { activeId: WORKSPACE, items: [] },
    tasks: { activeTaskId: TASK_A },
    kanban: {
      workflowId: WORKFLOW,
      tasks: [toKanbanTask(initialTask), toKanbanTask(otherTask)],
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
    },
  };
  const StateWrapper = createStateWrapper(initialState);
  function Wrapper({ children }: { children: ReactNode }) {
    return createElement(StateWrapper, { children: createElement(ToastProvider, { children }) });
  }
  let resolve!: (response: Response) => void;
  const taskResponse = new Promise<Response>((yes) => {
    resolve = yes;
  });
  vi.stubGlobal(
    "fetch",
    vi.fn((url: string) =>
      String(url).endsWith("/sessions")
        ? Promise.resolve(Response.json({ sessions: [], total: 0 }))
        : taskResponse,
    ),
  );
  const rendered = renderHook(
    ({ task }) => ({
      details: useTaskDetails(task.id, task),
      store: useAppStoreApi(),
      workflow: useNextWorkflowStep(task.id),
    }),
    { wrapper: Wrapper, initialProps: { task: initialTask } },
  );
  return { ...rendered, resolve, initialTask, otherTask };
}

const LIVE_TIMESTAMPS = [
  { name: "equal timestamp", value: "2026-10-15T10:00:00Z" },
  { name: "later nanosecond", value: "2026-10-15T10:00:00.000000001Z" },
];

function moveLiveTaskToDone(view: ReturnType<typeof mountRefreshingDetail>, updatedAt: string) {
  view.result.current.store.setState((state) =>
    applyTaskOverviewEvent(state, {
      ...view.initialTask,
      workflow_step_id: "done",
      updated_at: updatedAt,
    }),
  );
}

function expectSettledLivePlacement(view: ReturnType<typeof mountRefreshingDetail>) {
  const state = view.result.current.store.getState();
  expect(state.taskOverview.byId[TASK_A].workflowStepId).toBe("done");
  expect(view.result.current.workflow.proceedStepName).toBeNull();
  expect(state.taskOverview.reads).toEqual({});
  const taskReads = vi
    .mocked(fetch)
    .mock.calls.filter(([url]) => String(url).endsWith(`/tasks/${TASK_A}`));
  expect(taskReads).toHaveLength(1);
}

it.each(LIVE_TIMESTAMPS)(
  "keeps a live move when a second refresh joins an older HTTP read ($name)",
  async ({ value }) => {
    const view = mountRefreshingDetail();
    await act(async () => {
      const first = view.result.current.details.refreshTask();
      moveLiveTaskToDone(view, value);
      const second = view.result.current.details.refreshTask();
      view.resolve(Response.json({ ...view.initialTask, workflow_step_id: "review" }));
      await Promise.all([first, second]);
    });
    expectSettledLivePlacement(view);
  },
);

it.each(LIVE_TIMESTAMPS)(
  "keeps a live move when a detail read reuses a cached identity ($name)",
  async ({ value }) => {
    const view = mountRefreshingDetail();
    await act(async () => {
      const pending = view.result.current.details.refreshTask();
      view.resolve(Response.json({ ...view.initialTask, workflow_step_id: "review" }));
      await pending;
    });
    await act(async () => {
      moveLiveTaskToDone(view, value);
      await view.result.current.details.refreshTask(false);
    });
    expectSettledLivePlacement(view);
  },
);

it.each(LIVE_TIMESTAMPS)(
  "keeps a live move when a foreground recovery episode reuses its result ($name)",
  async ({ value }) => {
    const view = mountRefreshingDetail();
    const episode = {};
    await act(async () => {
      const pending = view.result.current.details.refreshTask(true, TASK_A, false, false, episode);
      view.resolve(Response.json({ ...view.initialTask, workflow_step_id: "review" }));
      await pending;
    });
    await act(async () => {
      moveLiveTaskToDone(view, value);
      await view.result.current.details.refreshTask(true, TASK_A, false, false, episode);
    });
    expectSettledLivePlacement(view);
  },
);

it("publishes a refreshed detail placement to the canonical workflow action", async () => {
  const view = mountRefreshingDetail();
  expect(view.result.current.workflow.proceedStepName).toBeNull();
  await act(async () => {
    const pending = view.result.current.details.refreshTask();
    view.resolve(Response.json({ ...view.initialTask, workflow_step_id: "review" }));
    await pending;
  });
  expect(view.result.current.details.task?.workflow_step_id).toBe("review");
  expect(view.result.current.store.getState().taskOverview.byId[TASK_A].workflowStepId).toBe(
    "review",
  );
  expect(view.result.current.workflow.proceedStepName).toBe("Done");
});

it("preserves a live workflow move while an older detail read completes", async () => {
  const view = mountRefreshingDetail();
  await act(async () => {
    const pending = view.result.current.details.refreshTask();
    view.result.current.store.setState((state) =>
      applyTaskOverviewEvent(state, {
        ...view.initialTask,
        workflow_step_id: "done",
      }),
    );
    view.resolve(Response.json({ ...view.initialTask, workflow_step_id: "review" }));
    await pending;
  });
  expect(view.result.current.store.getState().taskOverview.byId[TASK_A].workflowStepId).toBe(
    "done",
  );
  expect(view.result.current.workflow.proceedStepName).toBeNull();
});

it("does not publish a superseded task read after navigation", async () => {
  const view = mountRefreshingDetail();
  let pending!: Promise<void>;
  act(() => {
    pending = view.result.current.details.refreshTask();
    view.result.current.store.getState().setActiveTask("task-b");
  });
  view.rerender({ task: view.otherTask });
  await act(async () => {
    view.resolve(Response.json({ ...view.initialTask, workflow_step_id: "review" }));
    await pending;
  });
  expect(view.result.current.details.task?.id).toBe("task-b");
  expect(view.result.current.store.getState().taskOverview.byId[TASK_A].workflowStepId).toBe(
    "working",
  );
  expect(view.result.current.workflow.proceedStepName).toBe("Done");
});

function createToastStateWrapper(initialState: unknown) {
  const StateWrapper = createStateWrapper(initialState);
  return function Wrapper({ children }: { children: ReactNode }) {
    return createElement(StateWrapper, { children: createElement(ToastProvider, { children }) });
  };
}

it.each([false, true])(
  "refreshes workflow actions from detail reads while preserving newer live moves (%s)",
  async (liveMove) => {
    const initialTask = { ...makeTask(), title: "Task" };
    let finish!: (task: Task) => void;
    vi.spyOn(api, "fetchTask").mockImplementation(
      () =>
        new Promise((resolve) => {
          finish = resolve;
        }),
    );
    vi.spyOn(api, "listTaskSessions").mockResolvedValue({ sessions: [], total: 0 });
    const wrapper = createToastStateWrapper({
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
