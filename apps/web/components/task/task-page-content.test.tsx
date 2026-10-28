import { createElement, type ReactNode, useState } from "react";
import { act, cleanup, render, renderHook, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import * as api from "@/lib/api";
import { ApiError } from "@/lib/api/client";
import { taskId, workflowId, workspaceId, type Task } from "@/lib/types/http";
import { TaskLoadErrorState, useTaskDetails } from "./task-page-content";
import { TaskNavigationReadFeedback } from "./task-navigation-read-feedback";
import { TaskRouteSessionHydrationProvider } from "./task-route-session-hydration";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  vi.useRealTimers();
});

const TASK_A = "task-a";
const TASK_B = "task-b";
const TASK_CREATED_AT = "2026-07-18T00:00:00Z";
const CURRENT_TASK_TITLE = "Current task";
const REFRESHED_TASK_TITLE = "Refreshed task";
const RECOVERED_TASK_TITLE = "Recovered task";

function createStateWrapper(initialState: unknown) {
  return function StateTestWrapper({ children }: { children: ReactNode }) {
    return createElement(StateProvider, { initialState: initialState as never, children });
  };
}

function temporaryPersistenceFailure() {
  return new ApiError("busy", 503, { code: "persistence_unavailable" });
}

async function advanceAutomaticRetryCycle() {
  await vi.advanceTimersByTimeAsync(0);
  await vi.advanceTimersByTimeAsync(2_000);
  await vi.advanceTimersByTimeAsync(5_000);
  await vi.advanceTimersByTimeAsync(10_000);
  await vi.advanceTimersByTimeAsync(10_000);
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
    expect(link.className).toContain("h-7");
  });

  it("falls back to the unscoped overview when no workspace is active", () => {
    renderErrorState(null);

    expect(screen.getByTestId("task-unavailable-overview-link").getAttribute("href")).toBe(
      "/?home=overview",
    );
  });

  it("offers a localized manual retry for temporary task failures", () => {
    const onRetry = vi.fn();
    render(
      <StateProvider>
        <TaskLoadErrorState temporaryError onRetry={onRetry} />
      </StateProvider>,
    );

    expect(screen.getByText("Task temporarily unavailable")).toBeTruthy();
    expect(screen.getByTestId("task-read-retry").textContent).toBe("Retry");
    screen.getByTestId("task-read-retry").click();
    expect(onRetry).toHaveBeenCalledOnce();
  });

  it("keeps the Retry name stable and announces initial-read progress", () => {
    render(
      <StateProvider>
        <TaskLoadErrorState temporaryError retrying onRetry={vi.fn()} />
      </StateProvider>,
    );

    const retry = screen.getByRole("button", { name: "Retry" });
    expect(retry.hasAttribute("disabled")).toBe(true);
    expect(screen.getByRole("status").textContent).toContain("Retrying...");
  });

  it("keeps the Retry name stable and announces refresh progress", () => {
    render(
      <TaskNavigationReadFeedback
        recovery={{ temporaryError: true, retrying: true, onRetry: vi.fn() }}
      />,
    );

    const retry = screen.getByRole("button", { name: "Retry" });
    expect(retry.hasAttribute("disabled")).toBe(true);
    expect(screen.getByRole("status").textContent).toContain("Retrying...");
    const notice = screen.getByTestId("task-read-recovery-notice");
    expect(notice.className).not.toContain("absolute");
    expect(notice.className).toContain("w-full");
    expect(retry.className).toContain("max-md:h-11");
  });

  it("keeps routine background refreshes silent when task details are available", () => {
    render(
      <TaskNavigationReadFeedback
        recovery={{ temporaryError: false, retrying: true, onRetry: vi.fn() }}
      />,
    );

    expect(screen.queryByRole("status")).toBeNull();
  });
});

describe("useTaskDetails temporary refresh failure", () => {
  it("keeps loaded task details and allows a bounded manual retry", async () => {
    vi.useFakeTimers();
    const initialTask = { id: taskId(TASK_A), title: CURRENT_TASK_TITLE } as Task;
    const refreshedTask = { ...initialTask, title: REFRESHED_TASK_TITLE };
    const fetchTask = vi
      .spyOn(api, "fetchTask")
      .mockRejectedValueOnce(temporaryPersistenceFailure())
      .mockRejectedValueOnce(temporaryPersistenceFailure())
      .mockRejectedValueOnce(temporaryPersistenceFailure())
      .mockRejectedValueOnce(temporaryPersistenceFailure())
      .mockRejectedValueOnce(temporaryPersistenceFailure())
      .mockResolvedValueOnce(refreshedTask);
    vi.spyOn(api, "listTaskSessions").mockResolvedValue({ sessions: [], total: 0 });
    const { result } = renderHook(() => useTaskDetails(TASK_A, initialTask), {
      wrapper: createStateWrapper({ tasks: { activeTaskId: TASK_A } }),
    });

    await act(async () => {
      const refresh = result.current.refreshTask();
      await advanceAutomaticRetryCycle();
      await refresh;
    });
    expect(result.current.taskLoadError).toMatchObject({ status: 503 });
    expect(result.current.task?.title).toBe(CURRENT_TASK_TITLE);
    expect(result.current.isTemporaryTaskReadError).toBe(true);

    await act(async () => {
      await result.current.retryTaskRead();
    });
    expect(result.current.task?.title).toBe(REFRESHED_TASK_TITLE);
    expect(result.current.taskLoadError).toBeNull();
    expect(fetchTask).toHaveBeenCalledTimes(6);
  });

  // @covers AC-PLATFORM-INTERACTIVE-READS-005.7
  it("reports a temporary read while the shared cycle is still retrying", async () => {
    vi.useFakeTimers();
    const initialTask = { id: taskId(TASK_A), title: CURRENT_TASK_TITLE } as Task;
    const refreshedTask = { ...initialTask, title: REFRESHED_TASK_TITLE };
    vi.spyOn(api, "fetchTask")
      .mockRejectedValueOnce(temporaryPersistenceFailure())
      .mockResolvedValueOnce(refreshedTask);
    vi.spyOn(api, "listTaskSessions").mockResolvedValue({ sessions: [], total: 0 });
    const { result } = renderHook(() => useTaskDetails(TASK_A, initialTask), {
      wrapper: createStateWrapper({ tasks: { activeTaskId: TASK_A } }),
    });

    await act(async () => {
      const refresh = result.current.refreshTask();
      await vi.advanceTimersByTimeAsync(0);
      expect(result.current.navigationReadState.phase).toBe("retrying");
      expect(result.current.isTemporaryTaskReadError).toBe(true);
      expect(result.current.task?.title).toBe(CURRENT_TASK_TITLE);
      await vi.advanceTimersByTimeAsync(2_000);
      await refresh;
    });
    expect(result.current.task?.title).toBe(REFRESHED_TASK_TITLE);
    expect(result.current.isTemporaryTaskReadError).toBe(false);
  });

  it("retries one shared recovery cycle on a foreground burst after exhaustion", async () => {
    vi.useFakeTimers();
    const initialTask = { id: taskId(TASK_A), title: CURRENT_TASK_TITLE } as Task;
    const recoveredTask = { ...initialTask, title: RECOVERED_TASK_TITLE };
    const fetchTask = vi
      .spyOn(api, "fetchTask")
      .mockRejectedValueOnce(temporaryPersistenceFailure())
      .mockRejectedValueOnce(temporaryPersistenceFailure())
      .mockRejectedValueOnce(temporaryPersistenceFailure())
      .mockRejectedValueOnce(temporaryPersistenceFailure())
      .mockRejectedValueOnce(temporaryPersistenceFailure())
      .mockResolvedValueOnce(recoveredTask);
    vi.spyOn(api, "listTaskSessions").mockResolvedValue({ sessions: [], total: 0 });
    const { result, rerender } = renderHook(() => useTaskDetails(TASK_A, initialTask), {
      wrapper: createStateWrapper({ tasks: { activeTaskId: TASK_A } }),
    });

    await act(async () => {
      window.dispatchEvent(new Event("online"));
      await advanceAutomaticRetryCycle();
    });
    expect(fetchTask).toHaveBeenCalledTimes(5);
    expect(result.current.taskLoadError).toMatchObject({ status: 503 });

    await act(async () => result.current.refreshTask());
    rerender();
    expect(fetchTask).toHaveBeenCalledTimes(5);

    await act(async () => {
      window.dispatchEvent(new Event("online"));
      window.dispatchEvent(new Event("focus"));
      window.dispatchEvent(new Event("pageshow"));
      document.dispatchEvent(new Event("visibilitychange"));
      await vi.advanceTimersByTimeAsync(0);
    });
    expect(result.current.task?.title).toBe(RECOVERED_TASK_TITLE);
    expect(fetchTask).toHaveBeenCalledTimes(6);
    expect(result.current.taskLoadError).toBeNull();
  });
});

describe("useTaskDetails reconnect refresh", () => {
  it("retains a reconnect refresh until route hydration finishes", async () => {
    const initialTask = { id: taskId(TASK_A), title: "Before reconnect" } as Task;
    const refreshedTask = { ...initialTask, title: "After reconnect" };
    const fetchTask = vi.spyOn(api, "fetchTask").mockResolvedValue(refreshedTask);
    let setReady!: (ready: boolean) => void;
    function Wrapper({ children }: { children: ReactNode }) {
      const [isReady, updateReady] = useState(false);
      setReady = updateReady;
      return (
        <StateProvider initialState={{ connection: { status: "disconnected" } } as never}>
          <TaskRouteSessionHydrationProvider isReady={isReady}>
            {children}
          </TaskRouteSessionHydrationProvider>
        </StateProvider>
      );
    }
    const { result } = renderHook(
      () => ({ details: useTaskDetails(TASK_A, initialTask), store: useAppStoreApi() }),
      { wrapper: Wrapper },
    );
    act(() => result.current.store.getState().setConnectionStatus("connected"));
    expect(fetchTask).not.toHaveBeenCalled();
    act(() => setReady(true));
    await waitFor(() => expect(result.current.details.task?.title).toBe("After reconnect"));
    expect(fetchTask).toHaveBeenCalledTimes(1);
  });

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
      created_at: TASK_CREATED_AT,
      updated_at: TASK_CREATED_AT,
    } as Task;
    const movedTask = {
      ...initialTask,
      workflow_id: workflowId("workflow-destination"),
      workflow_step_id: "step-analysis",
      updated_at: "2026-07-19T00:00:00Z",
    };
    const fetchTask = vi.spyOn(api, "fetchTask").mockResolvedValue(movedTask);
    const listTaskSessions = vi
      .spyOn(api, "listTaskSessions")
      .mockResolvedValue({ sessions: [], total: 0 });
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

    await waitFor(() =>
      expect(fetchTask).toHaveBeenCalledWith(TASK_A, {
        cache: "no-store",
        init: { signal: expect.any(AbortSignal) },
      }),
    );
    expect(listTaskSessions).toHaveBeenCalledWith(TASK_A, {
      cache: "no-store",
      init: { signal: expect.any(AbortSignal) },
    });
    await waitFor(() => {
      expect(result.current.details.task).toMatchObject({
        workflow_id: "workflow-destination",
        workflow_step_id: "step-analysis",
      });
    });
  });
});

describe("useTaskDetails delayed unarchive navigation", () => {
  it("keeps the new route load when an old unarchive callback completes", async () => {
    const archivedTask = {
      id: taskId(TASK_A),
      title: "Archived task",
      archived_at: TASK_CREATED_AT,
    } as Task;
    const newTask = { id: taskId(TASK_B), title: "New route task" } as Task;
    let resolveNewRoute!: (task: Task) => void;
    const newRouteResponse = new Promise<Task>((resolve) => {
      resolveNewRoute = resolve;
    });
    const fetchTask = vi
      .spyOn(api, "fetchTask")
      .mockImplementation((id) =>
        id === TASK_B ? newRouteResponse : Promise.resolve(archivedTask),
      );
    const { result, rerender } = renderHook(
      ({ activeId, initialTask }) => useTaskDetails(activeId, initialTask),
      {
        wrapper: createStateWrapper({}),
        initialProps: { activeId: TASK_A, initialTask: archivedTask as Task | null },
      },
    );
    const oldUnarchiveCallback = result.current.onTaskUnarchived;

    rerender({ activeId: TASK_B, initialTask: null });
    await waitFor(() =>
      expect(fetchTask).toHaveBeenCalledWith(TASK_B, {
        cache: "no-store",
        init: { signal: expect.any(AbortSignal) },
      }),
    );
    act(() => oldUnarchiveCallback(TASK_A));

    await act(async () => {
      resolveNewRoute(newTask);
      await newRouteResponse;
    });
    await waitFor(() => expect(result.current.task?.id).toBe(TASK_B));
    expect(fetchTask.mock.calls.map(([id]) => id)).toEqual([TASK_B]);
  });
});

describe("useTaskDetails unarchive refresh", () => {
  it("refreshes the route task after unarchive before active-task hydration", async () => {
    const archivedTask = {
      id: taskId(TASK_A),
      title: "Archived route task",
      description: "Task details",
      workflow_id: workflowId("workflow-1"),
      workflow_step_id: "step-1",
      position: 0,
      state: "TODO",
      workspace_id: workspaceId("workspace-1"),
      priority: "medium",
      repositories: [],
      created_at: TASK_CREATED_AT,
      updated_at: TASK_CREATED_AT,
      archived_at: TASK_CREATED_AT,
    } as Task;
    const unarchivedTask = { ...archivedTask, archived_at: null };
    const fetchTask = vi.spyOn(api, "fetchTask").mockResolvedValue(unarchivedTask);
    const wrapper = createStateWrapper({ tasks: { activeTaskId: null } });
    const { result } = renderHook(() => useTaskDetails(null, archivedTask), { wrapper });

    act(() => result.current.onTaskUnarchived(TASK_A));

    await waitFor(() =>
      expect(fetchTask).toHaveBeenCalledWith(TASK_A, {
        cache: "no-store",
        init: { signal: expect.any(AbortSignal) },
      }),
    );
    await waitFor(() => expect(result.current.task?.archived_at).toBeNull());
  });

  it("refreshes the route task when the global selection is stale after unarchive", async () => {
    const archivedTask = {
      id: taskId(TASK_A),
      title: "Archived route task",
      archived_at: "2026-07-18T00:00:00Z",
    } as Task;
    const fetchTask = vi
      .spyOn(api, "fetchTask")
      .mockResolvedValue({ ...archivedTask, archived_at: null });
    const wrapper = createStateWrapper({ tasks: { activeTaskId: TASK_B } });
    const { result } = renderHook(() => useTaskDetails(TASK_B, archivedTask), { wrapper });

    act(() => result.current.onTaskUnarchived(TASK_A));

    await waitFor(() =>
      expect(fetchTask).toHaveBeenCalledWith(TASK_A, {
        cache: "no-store",
        init: { signal: expect.any(AbortSignal) },
      }),
    );
    await waitFor(() => expect(result.current.task?.archived_at).toBeNull());
  });

  it("does not let an older task refresh restore the archived state", async () => {
    const archivedTask = {
      id: taskId(TASK_A),
      title: "Archived route task",
      archived_at: TASK_CREATED_AT,
    } as Task;
    const unarchivedTask = { ...archivedTask, archived_at: null };
    let resolveOlder!: (task: Task) => void;
    let resolveUnarchive!: (task: Task) => void;
    const olderResponse = new Promise<Task>((resolve) => {
      resolveOlder = resolve;
    });
    const unarchiveResponse = new Promise<Task>((resolve) => {
      resolveUnarchive = resolve;
    });
    const fetchTask = vi
      .spyOn(api, "fetchTask")
      .mockReturnValueOnce(olderResponse)
      .mockReturnValueOnce(unarchiveResponse);
    const wrapper = createStateWrapper({ tasks: { activeTaskId: TASK_A } });
    const { result } = renderHook(() => useTaskDetails(TASK_A, archivedTask), { wrapper });

    let olderRequest!: Promise<void>;
    act(() => {
      olderRequest = result.current.refreshTask();
    });
    await waitFor(() => expect(fetchTask).toHaveBeenCalledTimes(1));

    act(() => result.current.onTaskUnarchived(TASK_A));
    await waitFor(() => expect(fetchTask).toHaveBeenCalledTimes(2));

    await act(async () => {
      resolveUnarchive(unarchivedTask);
      await Promise.resolve();
    });
    await waitFor(() => expect(result.current.task?.archived_at).toBeNull());

    await act(async () => {
      resolveOlder(archivedTask);
      await olderRequest;
    });
    expect(result.current.task?.archived_at).toBeNull();
  });
});

describe("useTaskDetails route loading", () => {
  it("does not duplicate task details while the route owns their load", () => {
    const fetchTask = vi.spyOn(api, "fetchTask").mockImplementation(() => new Promise(() => {}));
    const wrapper = ({ children }: { children: ReactNode }) => (
      <StateProvider>
        <TaskRouteSessionHydrationProvider isReady={false}>
          {children}
        </TaskRouteSessionHydrationProvider>
      </StateProvider>
    );
    renderHook(() => useTaskDetails(TASK_B, null), { wrapper });
    expect(fetchTask).not.toHaveBeenCalled();
  });
});

it("uses newly hydrated route details without retaining the provisional task", () => {
  const initial = { id: taskId(TASK_A), title: "Provisional task" } as Task;
  const authoritative = { ...initial, title: "Authoritative task", repositories: [] };
  const fetchTask = vi.spyOn(api, "fetchTask");
  const { result, rerender } = renderHook(({ task }) => useTaskDetails(TASK_A, task), {
    wrapper: createStateWrapper({}),
    initialProps: { task: initial },
  });
  rerender({ task: authoritative });
  expect(result.current.task).toBe(authoritative);
  expect(fetchTask).not.toHaveBeenCalled();
});
