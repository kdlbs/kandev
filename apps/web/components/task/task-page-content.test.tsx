import { createElement, type ReactNode, useEffect } from "react";
import { act, cleanup, render, renderHook, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import * as api from "@/lib/api";
import { taskId, workflowId, workspaceId, type Task } from "@/lib/types/http";
import { TaskLoadErrorState, useTaskDetails } from "./task-page-content";
import { TaskRemovalBoundary } from "./task-removal-boundary";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

const TASK_A = "task-a";
const TASK_B = "task-b";
const REMOVAL_STATUS_TEST_ID = "task-removal-status";

function createStateWrapper(initialState: unknown) {
  return function StateTestWrapper({ children }: { children: ReactNode }) {
    return createElement(StateProvider, { initialState: initialState as never, children });
  };
}

function renderErrorState(activeId: string | null) {
  render(
    <StateProvider initialState={{ workspaces: { items: [], activeId } }}>
      <TaskLoadErrorState />
    </StateProvider>,
  );
}

describe("TaskLoadErrorState", () => {
  it("preserves the active workspace in the overview destination", () => {
    renderErrorState("ws-1");

    const link = screen.getByTestId("task-unavailable-overview-link");
    expect(link.getAttribute("href")).toBe("/?home=overview&workspaceId=ws-1");
    expect(link.className).toContain("min-h-11");
  });

  it("falls back to the unscoped overview when no workspace is active", () => {
    renderErrorState(null);

    expect(screen.getByTestId("task-unavailable-overview-link").getAttribute("href")).toBe(
      "/?home=overview",
    );
  });
});

describe("useTaskDetails reconnect refresh", () => {
  it("reloads task placement after the websocket reconnects", async () => {
    const initialTask = {
      id: taskId(TASK_A),
      title: "Workflow migration task",
      description: "Task details",
      workflow_id: workflowId("workflow-source"),
      workflow_step_id: "step-source",
      position: 0,
      state: "TODO",
      workspace_id: workspaceId("workspace-1"),
      priority: "medium",
      repositories: [],
      created_at: "2026-07-18T00:00:00Z",
      updated_at: "2026-07-18T00:00:00Z",
    } as Task;
    const movedTask = {
      ...initialTask,
      workflow_id: workflowId("workflow-destination"),
      workflow_step_id: "step-analysis",
      updated_at: "2026-07-19T00:00:00Z",
    };
    const fetchTask = vi.spyOn(api, "fetchTask").mockResolvedValue(movedTask);
    const wrapper = createStateWrapper({
      tasks: { activeTaskId: TASK_A },
      connection: { status: "disconnected" },
      kanban: {
        tasks: [
          {
            id: TASK_A,
            title: initialTask.title,
            description: initialTask.description,
            workflowId: "workflow-source",
            workflowStepId: "step-source",
            position: initialTask.position,
            state: initialTask.state,
            updatedAt: initialTask.updated_at,
          },
        ],
      } as never,
    });
    const { result } = renderHook(
      () => ({
        details: useTaskDetails(TASK_A, initialTask),
        store: useAppStoreApi(),
      }),
      { wrapper },
    );

    expect(fetchTask).not.toHaveBeenCalled();
    act(() => result.current.store.getState().setConnectionStatus("connected"));

    await waitFor(() => expect(fetchTask).toHaveBeenCalledWith(TASK_A, { cache: "no-store" }));
    await waitFor(() => {
      expect(result.current.details.task).toMatchObject({
        workflow_id: "workflow-destination",
        workflow_step_id: "step-analysis",
      });
    });
  });
});

function StartRemoval({
  removalTaskId = "task-1",
  activeTaskId,
}: {
  removalTaskId?: string;
  activeTaskId?: string;
}) {
  const store = useAppStoreApi();
  useEffect(() => {
    if (activeTaskId) store.getState().setActiveTask(activeTaskId);
    store.getState().beginTaskRemoval({
      action: "delete",
      workspaceId: "ws-1",
      taskIds: [removalTaskId],
      requestIds: [removalTaskId],
      departure: null,
    });
  }, [activeTaskId, removalTaskId, store]);
  return null;
}

function StoreCapture({
  onStore,
}: {
  onStore: (store: ReturnType<typeof useAppStoreApi>) => void;
}) {
  onStore(useAppStoreApi());
  return null;
}

describe("TaskRemovalBoundary pending presentation", () => {
  it("unmounts outgoing content while a removal operation is pending", async () => {
    render(
      <StateProvider>
        <StartRemoval />
        <TaskRemovalBoundary taskId="task-1">
          <div data-testid="outgoing-task-content">Outgoing task</div>
        </TaskRemovalBoundary>
      </StateProvider>,
    );

    await waitFor(() => expect(screen.getByTestId(REMOVAL_STATUS_TEST_ID)).toBeTruthy());
    expect(screen.queryByTestId("outgoing-task-content")).toBeNull();
  });

  it("does not let a stale active task hide an explicit route task", async () => {
    render(
      <StateProvider initialState={{ tasks: { activeTaskId: TASK_A } } as never}>
        <StartRemoval removalTaskId={TASK_A} />
        <TaskRemovalBoundary taskId={TASK_B}>
          <div data-testid="explicit-task-content">Explicit task</div>
        </TaskRemovalBoundary>
      </StateProvider>,
    );

    await waitFor(() => expect(screen.getByTestId("explicit-task-content")).toBeTruthy());
    expect(screen.queryByTestId(REMOVAL_STATUS_TEST_ID)).toBeNull();
  });
});

describe("TaskRemovalBoundary displayed identity", () => {
  it("lets a newly committed route supersede the pending old route", async () => {
    let store!: ReturnType<typeof useAppStoreApi>;
    const view = render(
      <StateProvider initialState={{ tasks: { activeTaskId: TASK_A } } as never}>
        <StoreCapture onStore={(value) => (store = value)} />
        <TaskRemovalBoundary taskId={TASK_A}>
          <div data-testid="new-route-content">New route</div>
        </TaskRemovalBoundary>
      </StateProvider>,
    );

    let token: string | null = null;
    act(() => {
      token = store.getState().beginTaskRemoval({
        action: "delete",
        workspaceId: "ws-1",
        taskIds: [TASK_A],
        requestIds: [TASK_A],
        departure: null,
      });
    });

    view.rerender(
      <StateProvider initialState={{ tasks: { activeTaskId: TASK_A } } as never}>
        <StoreCapture onStore={(value) => (store = value)} />
        <TaskRemovalBoundary taskId={TASK_B}>
          <div data-testid="new-route-content">New route</div>
        </TaskRemovalBoundary>
      </StateProvider>,
    );

    await waitFor(() => expect(screen.getByTestId("new-route-content")).toBeTruthy());
    expect(screen.queryByTestId(REMOVAL_STATUS_TEST_ID)).toBeNull();

    act(() => {
      store.getState().releaseTaskRemoval(token!);
    });
  });

  it("gates an in-place sidebar selection while the route still names the original task", async () => {
    let store!: ReturnType<typeof useAppStoreApi>;
    render(
      <StateProvider initialState={{ tasks: { activeTaskId: TASK_A } } as never}>
        <StoreCapture onStore={(value) => (store = value)} />
        <TaskRemovalBoundary taskId={TASK_A}>
          <div data-testid="selected-task-content">Selected task</div>
        </TaskRemovalBoundary>
      </StateProvider>,
    );

    act(() => {
      store.getState().setActiveTask(TASK_B);
    });

    let token: string | null = null;
    act(() => {
      token = store.getState().beginTaskRemoval({
        action: "delete",
        workspaceId: "ws-1",
        taskIds: [TASK_B],
        requestIds: [TASK_B],
        departure: null,
      });
    });

    await waitFor(() => expect(screen.getByTestId(REMOVAL_STATUS_TEST_ID)).toBeTruthy());
    expect(screen.queryByTestId("selected-task-content")).toBeNull();

    act(() => {
      store.getState().recordTaskRemovalResult(token!, [TASK_B], "succeeded");
    });
    expect(screen.getByTestId(REMOVAL_STATUS_TEST_ID)).toBeTruthy();

    act(() => {
      store.getState().releaseTaskRemoval(token!);
    });
    await waitFor(() => expect(screen.getByTestId("selected-task-content")).toBeTruthy());
  });

  it("does not gate the displayed task when an unselected route task is removed", async () => {
    let store!: ReturnType<typeof useAppStoreApi>;
    render(
      <StateProvider initialState={{ tasks: { activeTaskId: TASK_A } } as never}>
        <StoreCapture onStore={(value) => (store = value)} />
        <TaskRemovalBoundary taskId={TASK_A}>
          <div data-testid="displayed-task-content">Displayed task</div>
        </TaskRemovalBoundary>
      </StateProvider>,
    );

    act(() => {
      store.getState().setActiveTask(TASK_B);
      store.getState().beginTaskRemoval({
        action: "archive",
        workspaceId: "ws-1",
        taskIds: [TASK_A],
        requestIds: [TASK_A],
        departure: null,
      });
    });

    await waitFor(() => expect(screen.getByTestId("displayed-task-content")).toBeTruthy());
    expect(screen.queryByTestId(REMOVAL_STATUS_TEST_ID)).toBeNull();
  });
});
