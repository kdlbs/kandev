import { cleanup, render, screen, waitFor } from "@testing-library/react";
import type { ReactElement } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { TaskDetailRoute } from "./task-detail-route";
import { StateProvider } from "@/components/state-provider";
import type { FetchedSessionData } from "@/lib/ssr/session-page-state";
import { taskId, workspaceId, workflowId } from "@/lib/types/ids";
import type { Task, TaskSession } from "@/lib/types/http";
import type { AppState } from "@/lib/state/store";

const mocks = vi.hoisted(() => ({
  fetchSessionDataForTask: vi.fn(),
}));
const KANBAN_TASK_SHELL_TEST_ID = "kanban-task-shell";
const STATE_HYDRATOR_TEST_ID = "state-hydrator";
const TASK_DATA_ATTRIBUTE = "data-task-id";
const LOADING_TASK_COPY = "Loading task";
const TASK_ONE_ID = "task-1";
const TASK_TWO_ID = "task-2";

vi.mock("@/components/state-hydrator", () => ({
  StateHydrator: ({ sessionId }: { sessionId?: string }) => (
    <div data-testid={STATE_HYDRATOR_TEST_ID} data-force-session-id={sessionId ?? ""} />
  ),
}));

vi.mock("@/app/tasks/[id]/kanban-task-shell", async () => {
  const { useAppStore } = await import("@/components/state-provider");
  const { useTaskRouteSessionHydrated } =
    await import("@/components/task/task-route-session-hydration");
  return {
    KanbanTaskShell: ({
      task,
      taskId,
      sessionId,
    }: {
      task: Task | null;
      taskId: string;
      sessionId: string | null;
    }) => {
      const readCursor = useAppStore((state) =>
        sessionId ? (state.taskSessions.items[sessionId]?.last_read_message_id ?? "") : "",
      );
      const routeReady = useTaskRouteSessionHydrated();
      return (
        <div
          data-testid={KANBAN_TASK_SHELL_TEST_ID}
          data-route-task-id={taskId}
          data-route-ready={routeReady}
          data-task-id={task?.id ?? ""}
          data-session-id={sessionId ?? ""}
          data-read-cursor={readCursor}
        />
      );
    },
  };
});

vi.mock("@/lib/ssr/session-page-state", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/ssr/session-page-state")>();
  return {
    ...actual,
    fetchSessionDataForTask: mocks.fetchSessionDataForTask,
    extractInitialRepositories: vi.fn(() => []),
    extractInitialScripts: vi.fn(() => []),
  };
});

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((next) => {
    resolve = next;
  });
  return { promise, resolve };
}

function makeFetchedData(): FetchedSessionData {
  return {
    task: {
      id: taskId(TASK_ONE_ID),
      title: "Task one",
      description: "",
      workspace_id: workspaceId("workspace-1"),
      workflow_id: workflowId("workflow-1"),
      workflow_step_id: "step-1",
      state: "CREATED",
      priority: "medium",
      position: 0,
      repositories: [],
      created_at: "2026-06-16T00:00:00Z",
      updated_at: "2026-06-16T00:00:00Z",
    },
    sessionId: "session-1",
    initialState: {},
    initialTerminals: [],
  };
}

function makeTaskSession(lastReadMessageId: string): TaskSession {
  return {
    id: "session-1",
    task_id: TASK_ONE_ID,
    last_read_message_id: lastReadMessageId,
  } as TaskSession;
}

function makeSessionHydrationState(session: TaskSession): Partial<AppState> {
  return {
    taskSessions: { items: { [session.id]: session } },
    taskSessionsByTask: {
      itemsByTaskId: { [session.task_id]: [session] },
      loadingByTaskId: { [session.task_id]: false },
      loadedByTaskId: { [session.task_id]: true },
    },
  } as unknown as Partial<AppState>;
}

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

function renderTaskRoute(element: ReactElement, initialState?: Partial<AppState>) {
  return render(element, {
    wrapper: ({ children }) => (
      <StateProvider initialState={initialState}>{children}</StateProvider>
    ),
  });
}

describe("TaskDetailRoute", () => {
  it("hydrates client route session data before mounting the selected task shell", async () => {
    const initialState = makeSessionHydrationState(makeTaskSession("message-1"));
    const fetchedData = makeFetchedData();
    fetchedData.initialState = makeSessionHydrationState(makeTaskSession("message-2"));
    mocks.fetchSessionDataForTask.mockResolvedValueOnce(fetchedData);

    renderTaskRoute(<TaskDetailRoute taskId={TASK_ONE_ID} />, initialState);

    await waitFor(() => {
      expect(screen.getByTestId(KANBAN_TASK_SHELL_TEST_ID).getAttribute("data-read-cursor")).toBe(
        "message-2",
      );
    });
  });

  it("shows accessible task-loading progress while route data is pending", async () => {
    const routeData = deferred<FetchedSessionData>();
    mocks.fetchSessionDataForTask.mockReturnValueOnce(routeData.promise);

    renderTaskRoute(<TaskDetailRoute taskId={TASK_ONE_ID} />);

    expect(screen.queryByTestId(KANBAN_TASK_SHELL_TEST_ID)).toBeNull();
    expect(screen.getByRole("status").textContent).toContain(LOADING_TASK_COPY);
    expect(screen.getByRole("status").parentElement?.className).toContain("h-full");
    expect(screen.getByRole("status").parentElement?.className).toContain("min-h-0");
    expect(screen.getByRole("status").parentElement?.className).not.toContain("h-dvh");
    expect(screen.getByRole("status").parentElement?.className).not.toContain("h-screen");

    routeData.resolve(makeFetchedData());

    await waitFor(() => {
      expect(screen.getByTestId(KANBAN_TASK_SHELL_TEST_ID).getAttribute(TASK_DATA_ATTRIBUTE)).toBe(
        TASK_ONE_ID,
      );
    });
  });

  it("uses boot route data without fetching again", async () => {
    renderTaskRoute(<TaskDetailRoute taskId={TASK_ONE_ID} initialData={makeFetchedData()} />);

    expect(mocks.fetchSessionDataForTask).not.toHaveBeenCalled();
    expect(screen.getByTestId(KANBAN_TASK_SHELL_TEST_ID).getAttribute(TASK_DATA_ATTRIBUTE)).toBe(
      TASK_ONE_ID,
    );
    expect(screen.getByTestId(STATE_HYDRATOR_TEST_ID)).toBeTruthy();
    expect(screen.getByTestId(STATE_HYDRATOR_TEST_ID).getAttribute("data-force-session-id")).toBe(
      "session-1",
    );
  });

  it("does not force-merge a client-fetched session over the live session cache", async () => {
    mocks.fetchSessionDataForTask.mockResolvedValueOnce(makeFetchedData());

    renderTaskRoute(<TaskDetailRoute taskId={TASK_ONE_ID} />);

    await waitFor(() => expect(screen.getByTestId(STATE_HYDRATOR_TEST_ID)).toBeTruthy());
    expect(screen.getByTestId(STATE_HYDRATOR_TEST_ID).getAttribute("data-force-session-id")).toBe(
      "",
    );
  });
});

describe("TaskDetailRoute client navigation", () => {
  it("keeps the existing task shell mounted while client route data loads", async () => {
    const routeData = deferred<FetchedSessionData>();
    const taskTwoData = {
      ...makeFetchedData(),
      task: { ...makeFetchedData().task, id: taskId(TASK_TWO_ID), title: "Task two" },
      sessionId: "session-2",
    };
    mocks.fetchSessionDataForTask.mockReturnValueOnce(routeData.promise);
    const { rerender } = renderTaskRoute(
      <TaskDetailRoute taskId={TASK_ONE_ID} initialData={makeFetchedData()} />,
    );
    const initialShell = screen.getByTestId(KANBAN_TASK_SHELL_TEST_ID);

    rerender(<TaskDetailRoute taskId={TASK_TWO_ID} initialData={makeFetchedData()} />);

    expect(screen.getByRole("status").textContent).toContain(LOADING_TASK_COPY);
    expect(screen.getByTestId(KANBAN_TASK_SHELL_TEST_ID)).toBe(initialShell);
    expect(initialShell.getAttribute("data-route-ready")).toBe("false");
    routeData.resolve(taskTwoData);
    await waitFor(() => {
      expect(screen.getByTestId(KANBAN_TASK_SHELL_TEST_ID)).toBe(initialShell);
      expect(initialShell.getAttribute(TASK_DATA_ATTRIBUTE)).toBe(TASK_TWO_ID);
      expect(initialShell.getAttribute("data-route-ready")).toBe("true");
    });
  });

  it("reloads a boot task after client navigation instead of reusing its stale boot snapshot", async () => {
    const taskTwoData = {
      ...makeFetchedData(),
      task: { ...makeFetchedData().task, id: taskId(TASK_TWO_ID), title: "Task two" },
      sessionId: "session-2",
    };
    mocks.fetchSessionDataForTask
      .mockResolvedValueOnce(taskTwoData)
      .mockResolvedValueOnce(makeFetchedData());
    const { rerender } = renderTaskRoute(
      <TaskDetailRoute taskId={TASK_ONE_ID} initialData={makeFetchedData()} />,
    );

    rerender(<TaskDetailRoute taskId={TASK_TWO_ID} initialData={makeFetchedData()} />);
    await waitFor(() => {
      expect(screen.getByTestId(KANBAN_TASK_SHELL_TEST_ID).getAttribute(TASK_DATA_ATTRIBUTE)).toBe(
        TASK_TWO_ID,
      );
    });

    rerender(<TaskDetailRoute taskId={TASK_ONE_ID} initialData={makeFetchedData()} />);
    expect(screen.getByRole("status").textContent).toContain(LOADING_TASK_COPY);
    await waitFor(() => {
      expect(screen.getByTestId(KANBAN_TASK_SHELL_TEST_ID).getAttribute(TASK_DATA_ATTRIBUTE)).toBe(
        TASK_ONE_ID,
      );
    });
    expect(mocks.fetchSessionDataForTask).toHaveBeenNthCalledWith(1, TASK_TWO_ID, undefined);
    expect(mocks.fetchSessionDataForTask).toHaveBeenNthCalledWith(2, TASK_ONE_ID, undefined);
    expect(screen.getByTestId(STATE_HYDRATOR_TEST_ID).getAttribute("data-force-session-id")).toBe(
      "",
    );
  });

  it("uses the session selected by route loading when the requested session is unavailable", async () => {
    mocks.fetchSessionDataForTask.mockResolvedValueOnce(makeFetchedData());

    renderTaskRoute(<TaskDetailRoute taskId={TASK_ONE_ID} sessionId="missing-session" />);

    await waitFor(() => {
      expect(screen.getByTestId(KANBAN_TASK_SHELL_TEST_ID).getAttribute("data-session-id")).toBe(
        "session-1",
      );
    });
    expect(mocks.fetchSessionDataForTask).toHaveBeenCalledWith(TASK_ONE_ID, "missing-session");
  });
});

describe("TaskDetailRoute fallback", () => {
  it("keeps the previous shell inert behind loading progress on the first frame after route reuse", async () => {
    const routeData = deferred<FetchedSessionData>();
    mocks.fetchSessionDataForTask.mockReturnValueOnce(routeData.promise);
    const { rerender } = renderTaskRoute(
      <TaskDetailRoute taskId={TASK_ONE_ID} initialData={makeFetchedData()} />,
    );

    rerender(<TaskDetailRoute taskId={TASK_TWO_ID} />);

    const previousShell = screen.getByTestId(KANBAN_TASK_SHELL_TEST_ID);
    expect(screen.getByRole("status").textContent).toContain(LOADING_TASK_COPY);
    expect(previousShell.closest("[inert]")).toBeTruthy();
    expect(mocks.fetchSessionDataForTask).toHaveBeenCalledWith(TASK_TWO_ID, undefined);

    routeData.resolve({
      ...makeFetchedData(),
      task: { ...makeFetchedData().task, id: taskId(TASK_TWO_ID), title: "Task two" },
    });
    await waitFor(() => {
      expect(screen.getByTestId(KANBAN_TASK_SHELL_TEST_ID).getAttribute(TASK_DATA_ATTRIBUTE)).toBe(
        TASK_TWO_ID,
      );
    });
  });

  it("reaches the unavailable task shell when route data fails", async () => {
    mocks.fetchSessionDataForTask.mockRejectedValueOnce(new Error("task not found"));

    renderTaskRoute(<TaskDetailRoute taskId="missing-task" />);

    await waitFor(() => {
      expect(screen.getByTestId(KANBAN_TASK_SHELL_TEST_ID)).toBeTruthy();
    });
    expect(screen.getByTestId(KANBAN_TASK_SHELL_TEST_ID).getAttribute(TASK_DATA_ATTRIBUTE)).toBe(
      "",
    );
    expect(screen.queryByTestId(STATE_HYDRATOR_TEST_ID)).toBeNull();
  });
});
