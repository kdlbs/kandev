import { act, cleanup, render, screen, waitFor } from "@testing-library/react";
import type { ReactElement } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { TaskDetailRoute } from "./task-detail-route";
import { StateProvider } from "@/components/state-provider";
import type { FetchedSessionData } from "@/lib/ssr/session-page-state";
import { taskId, workspaceId, workflowId } from "@/lib/types/ids";
import type { Task, TaskSession } from "@/lib/types/http";
import type { AppState } from "@/lib/state/store";
import type { TaskSessionHydrationEpoch } from "@/lib/state/slices/session/types";

const mocks = vi.hoisted(() => ({
  fetchTaskNavigationData: vi.fn(),
  deferRouteHydration: false,
  onHydrated: null as (() => void) | null,
}));
const KANBAN_TASK_SHELL_TEST_ID = "kanban-task-shell";
const STATE_HYDRATOR_TEST_ID = "state-hydrator";
const FORCE_SESSION_ID_ATTRIBUTE = "data-force-session-id";
const ROUTE_READY_ATTRIBUTE = "data-route-ready";
const TASK_DATA_ATTRIBUTE = "data-task-id";
const SESSION_DATA_ATTRIBUTE = "data-session-id";
const LOADING_TASK_COPY = "Loading task";
const TASK_ONE_ID = "task-1";
const TASK_TWO_ID = "task-2";

vi.mock("@/components/state-hydrator", async () => {
  const { useLayoutEffect, useRef } = await import("react");
  const { useAppStoreApi } = await import("@/components/state-provider");
  return {
    StateHydrator: ({
      initialState,
      sessionId,
      taskSessionHydrationEpochsAtRequestStart,
      onHydrated,
    }: {
      initialState: Partial<AppState>;
      sessionId?: string;
      taskSessionHydrationEpochsAtRequestStart?: Readonly<
        Record<string, TaskSessionHydrationEpoch>
      >;
      onHydrated?: () => void;
    }) => {
      const store = useAppStoreApi();
      const onHydratedRef = useRef(onHydrated);
      onHydratedRef.current = onHydrated;
      useLayoutEffect(() => {
        if (Object.keys(initialState).length) {
          store.getState().hydrate(initialState, {
            forceMergeSessionId: sessionId,
            taskSessionHydrationEpochsAtRequestStart,
          });
        }
        const markHydrated = () => onHydratedRef.current?.();
        mocks.onHydrated = markHydrated;
        if (!mocks.deferRouteHydration) markHydrated();
      }, [initialState, sessionId, store, taskSessionHydrationEpochsAtRequestStart]);
      return <div data-testid={STATE_HYDRATOR_TEST_ID} data-force-session-id={sessionId ?? ""} />;
    },
  };
});

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
    fetchTaskNavigationData: mocks.fetchTaskNavigationData,
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
  mocks.deferRouteHydration = false;
  mocks.onHydrated = null;
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
    mocks.fetchTaskNavigationData.mockResolvedValueOnce(fetchedData);

    renderTaskRoute(<TaskDetailRoute taskId={TASK_ONE_ID} />, initialState);

    await waitFor(() => {
      expect(screen.getByTestId(KANBAN_TASK_SHELL_TEST_ID).getAttribute("data-read-cursor")).toBe(
        "message-2",
      );
    });
  });

  it("shows accessible task-loading progress while route data is pending", async () => {
    const routeData = deferred<FetchedSessionData>();
    mocks.fetchTaskNavigationData.mockReturnValueOnce(routeData.promise);

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

    expect(mocks.fetchTaskNavigationData).not.toHaveBeenCalled();
    expect(screen.getByTestId(KANBAN_TASK_SHELL_TEST_ID).getAttribute(TASK_DATA_ATTRIBUTE)).toBe(
      TASK_ONE_ID,
    );
    expect(screen.getByTestId(STATE_HYDRATOR_TEST_ID)).toBeTruthy();
    expect(
      screen.getByTestId(STATE_HYDRATOR_TEST_ID).getAttribute(FORCE_SESSION_ID_ATTRIBUTE),
    ).toBe("session-1");
  });

  it("does not force-merge a client-fetched session over the live session cache", async () => {
    mocks.fetchTaskNavigationData.mockResolvedValueOnce(makeFetchedData());

    renderTaskRoute(<TaskDetailRoute taskId={TASK_ONE_ID} />);

    await waitFor(() => expect(screen.getByTestId(KANBAN_TASK_SHELL_TEST_ID)).toBeTruthy());
    expect(
      screen.getByTestId(STATE_HYDRATOR_TEST_ID).getAttribute(FORCE_SESSION_ID_ATTRIBUTE),
    ).toBe("");
  });

  it("keeps route session consumers gated until client data hydration completes", async () => {
    mocks.deferRouteHydration = true;
    mocks.fetchTaskNavigationData.mockResolvedValueOnce(makeFetchedData());

    renderTaskRoute(<TaskDetailRoute taskId={TASK_ONE_ID} />);

    await waitFor(() => expect(screen.getByTestId(KANBAN_TASK_SHELL_TEST_ID)).toBeTruthy());
    expect(screen.getByTestId(KANBAN_TASK_SHELL_TEST_ID).getAttribute(ROUTE_READY_ATTRIBUTE)).toBe(
      "false",
    );

    act(() => mocks.onHydrated?.());

    expect(screen.getByTestId(KANBAN_TASK_SHELL_TEST_ID).getAttribute(ROUTE_READY_ATTRIBUTE)).toBe(
      "true",
    );
  });
});

function makeKnownTaskState(): Partial<AppState> {
  return {
    workspaces: { activeId: "workspace-1", items: [] },
    kanban: {
      workflowId: "workflow-1",
      steps: [],
      tasks: [
        {
          id: TASK_TWO_ID,
          title: "Task two",
          workspaceId: "workspace-1",
          workflowId: "workflow-1",
          workflowStepId: "step-1",
          position: 0,
          primarySessionId: "session-2",
        },
      ],
    },
    ...makeSessionHydrationState({ id: "session-2", task_id: TASK_TWO_ID } as TaskSession),
  } as Partial<AppState>;
}

describe("TaskDetailRoute client navigation", () => {
  it("renders a known selected task immediately while its route refresh is pending", () => {
    const routeData = deferred<FetchedSessionData>();
    mocks.fetchTaskNavigationData.mockReturnValueOnce(routeData.promise);
    const initialState = makeKnownTaskState();
    const { rerender } = renderTaskRoute(
      <TaskDetailRoute taskId={TASK_ONE_ID} initialData={makeFetchedData()} />,
      initialState,
    );

    rerender(<TaskDetailRoute taskId={TASK_TWO_ID} />);

    const shell = screen.getByTestId(KANBAN_TASK_SHELL_TEST_ID);
    expect(shell.getAttribute(TASK_DATA_ATTRIBUTE)).toBe(TASK_TWO_ID);
    expect(shell.getAttribute(SESSION_DATA_ATTRIBUTE)).toBe("session-2");
    expect(shell.closest("[inert]")).toBeNull();
    expect(screen.queryByRole("status")).toBeNull();
    expect(shell.getAttribute(ROUTE_READY_ATTRIBUTE)).toBe("false");
  });

  it("keeps the existing task shell mounted while client route data loads", async () => {
    const routeData = deferred<FetchedSessionData>();
    const taskTwoData = {
      ...makeFetchedData(),
      task: { ...makeFetchedData().task, id: taskId(TASK_TWO_ID), title: "Task two" },
      sessionId: "session-2",
    };
    mocks.fetchTaskNavigationData.mockReturnValueOnce(routeData.promise);
    const { rerender } = renderTaskRoute(
      <TaskDetailRoute taskId={TASK_ONE_ID} initialData={makeFetchedData()} />,
    );
    const initialShell = screen.getByTestId(KANBAN_TASK_SHELL_TEST_ID);

    rerender(<TaskDetailRoute taskId={TASK_TWO_ID} initialData={makeFetchedData()} />);

    expect(screen.getByRole("status").textContent).toContain(LOADING_TASK_COPY);
    expect(screen.getByTestId(KANBAN_TASK_SHELL_TEST_ID)).toBe(initialShell);
    expect(initialShell.getAttribute(ROUTE_READY_ATTRIBUTE)).toBe("false");
    routeData.resolve(taskTwoData);
    await waitFor(() => {
      expect(screen.getByTestId(KANBAN_TASK_SHELL_TEST_ID)).toBe(initialShell);
      expect(initialShell.getAttribute(TASK_DATA_ATTRIBUTE)).toBe(TASK_TWO_ID);
      expect(initialShell.getAttribute(ROUTE_READY_ATTRIBUTE)).toBe("true");
    });
  });

  it("reloads a boot task after client navigation instead of reusing its stale boot snapshot", async () => {
    const taskTwoData = {
      ...makeFetchedData(),
      task: { ...makeFetchedData().task, id: taskId(TASK_TWO_ID), title: "Task two" },
      sessionId: "session-2",
    };
    mocks.fetchTaskNavigationData
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
    expect(mocks.fetchTaskNavigationData).toHaveBeenNthCalledWith(1, TASK_TWO_ID, undefined);
    expect(mocks.fetchTaskNavigationData).toHaveBeenNthCalledWith(2, TASK_ONE_ID, undefined);
    expect(
      screen.getByTestId(STATE_HYDRATOR_TEST_ID).getAttribute(FORCE_SESSION_ID_ATTRIBUTE),
    ).toBe("");
  });

  it("uses the session selected by route loading when the requested session is unavailable", async () => {
    mocks.fetchTaskNavigationData.mockResolvedValueOnce(makeFetchedData());

    renderTaskRoute(<TaskDetailRoute taskId={TASK_ONE_ID} sessionId="missing-session" />);

    await waitFor(() => {
      expect(
        screen.getByTestId(KANBAN_TASK_SHELL_TEST_ID).getAttribute(SESSION_DATA_ATTRIBUTE),
      ).toBe("session-1");
    });
    expect(mocks.fetchTaskNavigationData).toHaveBeenCalledWith(TASK_ONE_ID, "missing-session");
  });
});

describe("TaskDetailRoute fallback", () => {
  it("keeps the previous shell inert behind loading progress on the first frame after route reuse", async () => {
    const routeData = deferred<FetchedSessionData>();
    mocks.fetchTaskNavigationData.mockReturnValueOnce(routeData.promise);
    const { rerender } = renderTaskRoute(
      <TaskDetailRoute taskId={TASK_ONE_ID} initialData={makeFetchedData()} />,
    );

    rerender(<TaskDetailRoute taskId={TASK_TWO_ID} />);

    const previousShell = screen.getByTestId(KANBAN_TASK_SHELL_TEST_ID);
    expect(screen.getByRole("status").textContent).toContain(LOADING_TASK_COPY);
    expect(previousShell.closest("[inert]")).toBeTruthy();
    expect(mocks.fetchTaskNavigationData).toHaveBeenCalledWith(TASK_TWO_ID, undefined);

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
    mocks.fetchTaskNavigationData.mockRejectedValueOnce(new Error("task not found"));

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

describe("TaskDetailRoute saved conversation", () => {
  it("preserves the selected owned conversation through route refresh without a session URL", async () => {
    const state = makeKnownTaskState();
    state.tasks = { activeTaskId: TASK_TWO_ID, activeSessionId: "secondary" } as AppState["tasks"];
    state.taskSessions!.items.secondary = {
      id: "secondary",
      task_id: TASK_TWO_ID,
    } as TaskSession;
    const pending = deferred<FetchedSessionData>();
    mocks.fetchTaskNavigationData.mockReturnValueOnce(pending.promise);
    renderTaskRoute(<TaskDetailRoute taskId={TASK_TWO_ID} />, state);
    expect(screen.getByTestId(KANBAN_TASK_SHELL_TEST_ID).getAttribute(SESSION_DATA_ATTRIBUTE)).toBe(
      "secondary",
    );
    expect(mocks.fetchTaskNavigationData).toHaveBeenCalledWith(TASK_TWO_ID, "secondary");
    await act(async () =>
      pending.resolve({
        ...makeFetchedData(),
        task: { ...makeFetchedData().task, id: taskId(TASK_TWO_ID) },
        sessionId: "secondary",
      }),
    );
    expect(screen.getByTestId(KANBAN_TASK_SHELL_TEST_ID).getAttribute(SESSION_DATA_ATTRIBUTE)).toBe(
      "secondary",
    );
  });
});

describe("TaskDetailRoute cached presentation boundaries", () => {
  it("waits for fresh hydration on a return visit before enabling read tracking", async () => {
    const taskB = {
      ...makeFetchedData(),
      task: { ...makeFetchedData().task, id: taskId(TASK_TWO_ID) },
      sessionId: "session-2",
    };
    mocks.fetchTaskNavigationData
      .mockResolvedValueOnce(taskB)
      .mockResolvedValueOnce(makeFetchedData());
    const { rerender } = renderTaskRoute(
      <TaskDetailRoute taskId={TASK_ONE_ID} initialData={makeFetchedData()} />,
    );
    mocks.deferRouteHydration = true;
    rerender(<TaskDetailRoute taskId={TASK_TWO_ID} />);
    await waitFor(() =>
      expect(screen.getByTestId(KANBAN_TASK_SHELL_TEST_ID).getAttribute(TASK_DATA_ATTRIBUTE)).toBe(
        TASK_TWO_ID,
      ),
    );
    rerender(<TaskDetailRoute taskId={TASK_ONE_ID} />);
    await waitFor(() =>
      expect(screen.getByTestId(KANBAN_TASK_SHELL_TEST_ID).getAttribute(TASK_DATA_ATTRIBUTE)).toBe(
        TASK_ONE_ID,
      ),
    );
    expect(screen.getByTestId(KANBAN_TASK_SHELL_TEST_ID).getAttribute(ROUTE_READY_ATTRIBUTE)).toBe(
      "false",
    );
    act(() => mocks.onHydrated?.());
    expect(screen.getByTestId(KANBAN_TASK_SHELL_TEST_ID).getAttribute(ROUTE_READY_ATTRIBUTE)).toBe(
      "true",
    );
  });

  it.each(["workspace", "authentication", "requested-session", "archived-task"])(
    "waits for authoritative loading across the %s boundary",
    (boundary) => {
      const state = makeKnownTaskState();
      if (boundary === "workspace") state.workspaces!.activeId = "other-workspace";
      if (boundary === "archived-task") state.kanban!.tasks[0].isArchived = true;
      if (boundary === "authentication")
        state.auth = { mode: "enabled", authenticated: false, user: null };
      mocks.fetchTaskNavigationData.mockReturnValueOnce(new Promise(() => {}));
      renderTaskRoute(
        <TaskDetailRoute
          taskId={TASK_TWO_ID}
          sessionId={boundary === "requested-session" ? "foreign-session" : undefined}
        />,
        state,
      );
      expect(screen.queryByTestId(KANBAN_TASK_SHELL_TEST_ID)).toBeNull();
      expect(screen.getByRole("status").textContent).toContain(LOADING_TASK_COPY);
    },
  );

  it("does not apply an obsolete response after A to B to A navigation", async () => {
    const taskB = deferred<FetchedSessionData>();
    const taskA = deferred<FetchedSessionData>();
    mocks.fetchTaskNavigationData
      .mockReturnValueOnce(taskB.promise)
      .mockReturnValueOnce(taskA.promise);
    const { rerender } = renderTaskRoute(
      <TaskDetailRoute taskId={TASK_ONE_ID} initialData={makeFetchedData()} />,
    );
    rerender(<TaskDetailRoute taskId={TASK_TWO_ID} />);
    rerender(<TaskDetailRoute taskId={TASK_ONE_ID} />);
    await act(async () => taskA.resolve(makeFetchedData()));
    await act(async () =>
      taskB.resolve({
        ...makeFetchedData(),
        task: { ...makeFetchedData().task, id: taskId(TASK_TWO_ID) },
        sessionId: "session-2",
      }),
    );
    expect(screen.getByTestId(KANBAN_TASK_SHELL_TEST_ID).getAttribute(TASK_DATA_ATTRIBUTE)).toBe(
      TASK_ONE_ID,
    );
    expect(screen.queryByRole("status")).toBeNull();
  });
});
