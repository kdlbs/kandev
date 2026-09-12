import { act, cleanup, render, screen, waitFor } from "@testing-library/react";
import { useEffect } from "react";
import { afterEach, describe, expect, it } from "vitest";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import { TaskLoadErrorState } from "./task-page-content";
import { TaskRemovalBoundary } from "./task-removal-boundary";

afterEach(cleanup);

const TASK_A = "task-a";
const TASK_B = "task-b";
const REMOVAL_STATUS_TEST_ID = "task-removal-status";

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
