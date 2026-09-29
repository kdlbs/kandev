import { act, cleanup, render, screen, waitFor } from "@testing-library/react";
import type { ReactElement } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { TaskDetailRoute } from "./task-detail-route";
import { StateProvider } from "@/components/state-provider";
import type { FetchedSessionData } from "@/lib/ssr/session-page-state";
import { taskId, workspaceId, workflowId } from "@/lib/types/ids";
import type { Task, TaskSession } from "@/lib/types/http";
import type { AppState } from "@/lib/state/store";
import type { TaskSessionHydrationEpoch } from "@/lib/state/slices/session/types";
import type { TaskNavigationIdentity } from "@/lib/state/task-navigation-reads";

const mocks = vi.hoisted(() => ({
  beginTaskNavigation: vi.fn(),
  isTaskNavigationCurrent: vi.fn(),
  readTaskNavigationIdentity: vi.fn(),
  fetchTaskNavigationEnrichment: vi.fn(),
  deferRouteHydration: false,
  onHydrated: null as (() => void) | null,
}));
const KANBAN_TASK_SHELL_TEST_ID = "kanban-task-shell";
const STATE_HYDRATOR_TEST_ID = "state-hydrator";
const FORCE_SESSION_ID_ATTRIBUTE = "data-force-session-id";
const ROUTE_READY_ATTRIBUTE = "data-route-ready";
const TASK_DATA_ATTRIBUTE = "data-task-id";
const SESSION_DATA_ATTRIBUTE = "data-session-id";
const READ_CURSOR_ATTRIBUTE = "data-read-cursor";
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
    fetchTaskNavigationEnrichment: mocks.fetchTaskNavigationEnrichment,
    extractInitialRepositories: vi.fn(() => []),
    extractInitialScripts: vi.fn(() => []),
  };
});

vi.mock("@/lib/state/task-navigation-reads", () => ({
  beginTaskNavigation: mocks.beginTaskNavigation,
  isTaskNavigationCurrent: mocks.isTaskNavigationCurrent,
  readTaskNavigationIdentity: mocks.readTaskNavigationIdentity,
}));

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((next) => {
    resolve = next;
  });
  return { promise, resolve };
}

function makeFetchedData(id = TASK_ONE_ID): FetchedSessionData {
  return {
    task: {
      id: taskId(id),
      title: id === TASK_ONE_ID ? "Task one" : "Task two",
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

function makeTaskSession(
  lastReadMessageId: string,
  id = "session-1",
  ownerTaskId = TASK_ONE_ID,
): TaskSession {
  return {
    id,
    task_id: ownerTaskId,
    last_read_message_id: lastReadMessageId,
  } as TaskSession;
}

function makeNavigationIdentity(id = TASK_ONE_ID, includeSession = true): TaskNavigationIdentity {
  return {
    task: makeFetchedData(id).task,
    allSessionsResponse: {
      sessions: includeSession ? [makeTaskSession("message-1", `session-${id.slice(-1)}`, id)] : [],
      total: includeSession ? 1 : 0,
    },
  };
}

function makeEnrichedData(identity: TaskNavigationIdentity): FetchedSessionData {
  const data = makeFetchedData(identity.task.id);
  return {
    ...data,
    task: identity.task,
    sessionId: identity.allSessionsResponse.sessions[0]?.id ?? null,
    initialState: identity.allSessionsResponse.sessions[0]
      ? makeSessionHydrationState({
          ...identity.allSessionsResponse.sessions[0],
          last_read_message_id: "message-2",
        })
      : {},
  };
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

beforeEach(() => {
  let generation = 0;
  const ownerToken = {};
  let lastRouteKey: string | null = null;
  let currentContext: { ownerToken: object; identity: string; generation: number } | null = null;
  mocks.beginTaskNavigation.mockImplementation((_, __, routeKey: string) => {
    if (lastRouteKey !== routeKey) {
      lastRouteKey = routeKey;
      currentContext = { ownerToken, identity: "test-scope", generation: ++generation };
    }
    return currentContext;
  });
  mocks.isTaskNavigationCurrent.mockReturnValue(true);
  mocks.readTaskNavigationIdentity.mockImplementation((_, id: string) =>
    Promise.resolve(makeNavigationIdentity(id)),
  );
  mocks.fetchTaskNavigationEnrichment.mockImplementation((identity: TaskNavigationIdentity) =>
    Promise.resolve(makeEnrichedData(identity)),
  );
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
    const identity = makeNavigationIdentity();
    mocks.readTaskNavigationIdentity.mockResolvedValueOnce(identity);
    mocks.fetchTaskNavigationEnrichment.mockResolvedValueOnce(fetchedData);

    renderTaskRoute(<TaskDetailRoute taskId={TASK_ONE_ID} />, initialState);

    await waitFor(() => {
      expect(
        screen.getByTestId(KANBAN_TASK_SHELL_TEST_ID).getAttribute(READ_CURSOR_ATTRIBUTE),
      ).toBe("message-2");
    });
  });

  it("shows accessible task-loading progress while route data is pending", async () => {
    const routeData = deferred<TaskNavigationIdentity>();
    mocks.readTaskNavigationIdentity.mockReturnValueOnce(routeData.promise);

    renderTaskRoute(<TaskDetailRoute taskId={TASK_ONE_ID} />);

    expect(screen.queryByTestId(KANBAN_TASK_SHELL_TEST_ID)).toBeNull();
    expect(screen.getByRole("status").textContent).toContain(LOADING_TASK_COPY);
    expect(screen.getByRole("status").parentElement?.className).toContain("h-full");
    expect(screen.getByRole("status").parentElement?.className).toContain("min-h-0");
    expect(screen.getByRole("status").parentElement?.className).not.toContain("h-dvh");
    expect(screen.getByRole("status").parentElement?.className).not.toContain("h-screen");

    routeData.resolve(makeNavigationIdentity());

    await waitFor(() => {
      expect(screen.getByTestId(KANBAN_TASK_SHELL_TEST_ID).getAttribute(TASK_DATA_ATTRIBUTE)).toBe(
        TASK_ONE_ID,
      );
    });
  });

  it("reveals the destination shell while optional enrichment is held", async () => {
    const enrichment = deferred<FetchedSessionData>();
    mocks.fetchTaskNavigationEnrichment.mockReturnValueOnce(enrichment.promise);

    renderTaskRoute(<TaskDetailRoute taskId={TASK_ONE_ID} />);

    await waitFor(() => {
      expect(screen.getByTestId(KANBAN_TASK_SHELL_TEST_ID).getAttribute(TASK_DATA_ATTRIBUTE)).toBe(
        TASK_ONE_ID,
      );
    });
    expect(screen.getByTestId(KANBAN_TASK_SHELL_TEST_ID).getAttribute(ROUTE_READY_ATTRIBUTE)).toBe(
      "true",
    );
    expect(screen.queryByRole("status")).toBeNull();

    enrichment.resolve(makeEnrichedData(makeNavigationIdentity()));
    await waitFor(() => {
      expect(
        screen.getByTestId(KANBAN_TASK_SHELL_TEST_ID).getAttribute(READ_CURSOR_ATTRIBUTE),
      ).toBe("message-2");
    });
  });

  it("reveals a sessionless destination from task identity", async () => {
    mocks.readTaskNavigationIdentity.mockResolvedValueOnce(
      makeNavigationIdentity(TASK_ONE_ID, false),
    );

    renderTaskRoute(<TaskDetailRoute taskId={TASK_ONE_ID} />);

    await waitFor(() => {
      expect(screen.getByTestId(KANBAN_TASK_SHELL_TEST_ID).getAttribute(TASK_DATA_ATTRIBUTE)).toBe(
        TASK_ONE_ID,
      );
    });
    expect(screen.getByTestId(KANBAN_TASK_SHELL_TEST_ID).getAttribute("data-session-id")).toBe("");
  });

  it("uses boot route data without fetching again", async () => {
    renderTaskRoute(<TaskDetailRoute taskId={TASK_ONE_ID} initialData={makeFetchedData()} />);

    expect(mocks.readTaskNavigationIdentity).not.toHaveBeenCalled();
    expect(screen.getByTestId(KANBAN_TASK_SHELL_TEST_ID).getAttribute(TASK_DATA_ATTRIBUTE)).toBe(
      TASK_ONE_ID,
    );
    expect(screen.getByTestId(STATE_HYDRATOR_TEST_ID)).toBeTruthy();
    expect(
      screen.getByTestId(STATE_HYDRATOR_TEST_ID).getAttribute(FORCE_SESSION_ID_ATTRIBUTE),
    ).toBe("session-1");
  });

  it("does not force-merge a client-fetched session over the live session cache", async () => {
    mocks.readTaskNavigationIdentity.mockResolvedValueOnce(makeNavigationIdentity());

    renderTaskRoute(<TaskDetailRoute taskId={TASK_ONE_ID} />);

    await waitFor(() => expect(screen.getByTestId(KANBAN_TASK_SHELL_TEST_ID)).toBeTruthy());
    expect(
      screen.getByTestId(STATE_HYDRATOR_TEST_ID).getAttribute(FORCE_SESSION_ID_ATTRIBUTE),
    ).toBe("");
  });

  it("keeps route session consumers gated until client data hydration completes", async () => {
    mocks.deferRouteHydration = true;
    mocks.readTaskNavigationIdentity.mockResolvedValueOnce(makeNavigationIdentity());

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

describe("TaskDetailRoute client navigation", () => {
  it("renders a cached destination immediately while its shared identity read is pending", () => {
    const routeData = deferred<TaskNavigationIdentity>();
    mocks.readTaskNavigationIdentity.mockReturnValueOnce(routeData.promise);
    const { rerender } = renderTaskRoute(
      <TaskDetailRoute taskId={TASK_ONE_ID} initialData={makeFetchedData()} />,
      makeKnownTaskState(),
    );

    rerender(<TaskDetailRoute taskId={TASK_TWO_ID} />);

    const shell = screen.getByTestId(KANBAN_TASK_SHELL_TEST_ID);
    expect(shell.getAttribute(TASK_DATA_ATTRIBUTE)).toBe(TASK_TWO_ID);
    expect(shell.getAttribute(SESSION_DATA_ATTRIBUTE)).toBe("session-2");
    expect(shell.closest("[inert]")).toBeNull();
    expect(screen.queryByRole("status")).toBeNull();
    expect(shell.getAttribute(ROUTE_READY_ATTRIBUTE)).toBe("false");

    routeData.resolve(makeNavigationIdentity(TASK_TWO_ID));
  });

  it("keeps the existing task shell mounted while client route data loads", async () => {
    const routeData = deferred<TaskNavigationIdentity>();
    mocks.readTaskNavigationIdentity.mockReturnValueOnce(routeData.promise);
    const { rerender } = renderTaskRoute(
      <TaskDetailRoute taskId={TASK_ONE_ID} initialData={makeFetchedData()} />,
    );
    const initialShell = screen.getByTestId(KANBAN_TASK_SHELL_TEST_ID);

    rerender(<TaskDetailRoute taskId={TASK_TWO_ID} initialData={makeFetchedData()} />);

    expect(screen.getByRole("status").textContent).toContain(LOADING_TASK_COPY);
    expect(screen.getByTestId(KANBAN_TASK_SHELL_TEST_ID)).toBe(initialShell);
    expect(initialShell.getAttribute(ROUTE_READY_ATTRIBUTE)).toBe("false");
    routeData.resolve(makeNavigationIdentity(TASK_TWO_ID));
    await waitFor(() => {
      expect(screen.getByTestId(KANBAN_TASK_SHELL_TEST_ID)).toBe(initialShell);
      expect(initialShell.getAttribute(TASK_DATA_ATTRIBUTE)).toBe(TASK_TWO_ID);
      expect(initialShell.getAttribute(ROUTE_READY_ATTRIBUTE)).toBe("true");
    });
  });

  it("reloads a boot task after client navigation instead of reusing its stale boot snapshot", async () => {
    mocks.readTaskNavigationIdentity
      .mockResolvedValueOnce(makeNavigationIdentity(TASK_TWO_ID))
      .mockResolvedValueOnce(makeNavigationIdentity());
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
    expect(mocks.readTaskNavigationIdentity).toHaveBeenNthCalledWith(
      1,
      expect.anything(),
      TASK_TWO_ID,
      { context: expect.objectContaining({ generation: 2 }) },
    );
    expect(mocks.readTaskNavigationIdentity).toHaveBeenNthCalledWith(
      2,
      expect.anything(),
      TASK_ONE_ID,
      { context: expect.objectContaining({ generation: 3 }) },
    );
    expect(
      screen.getByTestId(STATE_HYDRATOR_TEST_ID).getAttribute(FORCE_SESSION_ID_ATTRIBUTE),
    ).toBe("");
  });
});

describe("TaskDetailRoute session selection", () => {
  it("uses the session selected by route loading when the requested session is unavailable", async () => {
    mocks.readTaskNavigationIdentity.mockResolvedValueOnce(makeNavigationIdentity());

    renderTaskRoute(<TaskDetailRoute taskId={TASK_ONE_ID} sessionId="missing-session" />);

    await waitFor(() => {
      expect(screen.getByTestId(KANBAN_TASK_SHELL_TEST_ID).getAttribute("data-session-id")).toBe(
        "session-1",
      );
    });
    expect(mocks.fetchTaskNavigationEnrichment).toHaveBeenCalledWith(
      expect.anything(),
      "session-1",
      { store: expect.anything() },
    );
  });

  it("rejects late enrichment from an earlier A-B-A navigation", async () => {
    const oldEnrichment = deferred<FetchedSessionData>();
    const newEnrichment = deferred<FetchedSessionData>();
    mocks.fetchTaskNavigationEnrichment
      .mockReturnValueOnce(oldEnrichment.promise)
      .mockImplementationOnce((identity: TaskNavigationIdentity) =>
        Promise.resolve(makeEnrichedData(identity)),
      )
      .mockReturnValueOnce(newEnrichment.promise);
    const { rerender } = renderTaskRoute(<TaskDetailRoute taskId={TASK_ONE_ID} />);

    await waitFor(() => expect(screen.getByTestId(KANBAN_TASK_SHELL_TEST_ID)).toBeTruthy());
    rerender(<TaskDetailRoute taskId={TASK_TWO_ID} />);
    await waitFor(() => {
      expect(screen.getByTestId(KANBAN_TASK_SHELL_TEST_ID).getAttribute(TASK_DATA_ATTRIBUTE)).toBe(
        TASK_TWO_ID,
      );
    });
    rerender(<TaskDetailRoute taskId={TASK_ONE_ID} />);
    await waitFor(() => {
      expect(screen.getByTestId(KANBAN_TASK_SHELL_TEST_ID).getAttribute(TASK_DATA_ATTRIBUTE)).toBe(
        TASK_ONE_ID,
      );
    });

    oldEnrichment.resolve({
      ...makeEnrichedData(makeNavigationIdentity()),
      initialState: makeSessionHydrationState(makeTaskSession("stale-message")),
    });
    await waitFor(() => {
      expect(
        screen.getByTestId(KANBAN_TASK_SHELL_TEST_ID).getAttribute(READ_CURSOR_ATTRIBUTE),
      ).toBe("message-1");
    });

    newEnrichment.resolve({
      ...makeEnrichedData(makeNavigationIdentity()),
      initialState: makeSessionHydrationState(makeTaskSession("fresh-message")),
    });
    await waitFor(() => {
      expect(
        screen.getByTestId(KANBAN_TASK_SHELL_TEST_ID).getAttribute(READ_CURSOR_ATTRIBUTE),
      ).toBe("fresh-message");
    });
  });

  it("preserves the selected owned conversation when the route omits a session id", async () => {
    const state = makeKnownTaskState();
    const secondary = makeTaskSession("message-1", "secondary", TASK_TWO_ID);
    state.tasks = { activeTaskId: TASK_TWO_ID, activeSessionId: "secondary" } as AppState["tasks"];
    state.taskSessions!.items.secondary = secondary;
    const identity = makeNavigationIdentity(TASK_TWO_ID);
    identity.allSessionsResponse.sessions = [identity.allSessionsResponse.sessions![0], secondary];
    mocks.readTaskNavigationIdentity.mockResolvedValueOnce(identity);

    renderTaskRoute(<TaskDetailRoute taskId={TASK_TWO_ID} />, state);

    expect(screen.getByTestId(KANBAN_TASK_SHELL_TEST_ID).getAttribute(SESSION_DATA_ATTRIBUTE)).toBe(
      "secondary",
    );
    await waitFor(() => {
      expect(mocks.fetchTaskNavigationEnrichment).toHaveBeenCalledWith(identity, "secondary", {
        store: expect.anything(),
      });
    });
  });
});

describe("TaskDetailRoute return navigation", () => {
  it("waits for fresh hydration when returning to a previously visited route", async () => {
    const taskB = makeNavigationIdentity(TASK_TWO_ID);
    mocks.readTaskNavigationIdentity
      .mockResolvedValueOnce(taskB)
      .mockResolvedValueOnce(makeNavigationIdentity());
    const { rerender } = renderTaskRoute(
      <TaskDetailRoute taskId={TASK_ONE_ID} initialData={makeFetchedData()} />,
    );
    mocks.deferRouteHydration = true;

    rerender(<TaskDetailRoute taskId={TASK_TWO_ID} />);
    await waitFor(() => {
      expect(screen.getByTestId(KANBAN_TASK_SHELL_TEST_ID).getAttribute(TASK_DATA_ATTRIBUTE)).toBe(
        TASK_TWO_ID,
      );
    });
    expect(screen.getByTestId(KANBAN_TASK_SHELL_TEST_ID).getAttribute(ROUTE_READY_ATTRIBUTE)).toBe(
      "false",
    );
    act(() => mocks.onHydrated?.());

    rerender(<TaskDetailRoute taskId={TASK_ONE_ID} />);
    await waitFor(() => {
      expect(screen.getByTestId(KANBAN_TASK_SHELL_TEST_ID).getAttribute(TASK_DATA_ATTRIBUTE)).toBe(
        TASK_ONE_ID,
      );
    });
    expect(screen.getByTestId(KANBAN_TASK_SHELL_TEST_ID).getAttribute(ROUTE_READY_ATTRIBUTE)).toBe(
      "false",
    );
    act(() => mocks.onHydrated?.());
    expect(screen.getByTestId(KANBAN_TASK_SHELL_TEST_ID).getAttribute(ROUTE_READY_ATTRIBUTE)).toBe(
      "true",
    );
  });
});

describe("TaskDetailRoute fallback", () => {
  it("keeps the previous shell inert behind loading progress on the first frame after route reuse", async () => {
    const routeData = deferred<TaskNavigationIdentity>();
    mocks.readTaskNavigationIdentity.mockReturnValueOnce(routeData.promise);
    const { rerender } = renderTaskRoute(
      <TaskDetailRoute taskId={TASK_ONE_ID} initialData={makeFetchedData()} />,
    );

    rerender(<TaskDetailRoute taskId={TASK_TWO_ID} />);

    const previousShell = screen.getByTestId(KANBAN_TASK_SHELL_TEST_ID);
    expect(screen.getByRole("status").textContent).toContain(LOADING_TASK_COPY);
    expect(previousShell.closest("[inert]")).toBeTruthy();
    expect(mocks.readTaskNavigationIdentity).toHaveBeenCalledWith(expect.anything(), TASK_TWO_ID, {
      context: expect.objectContaining({ generation: 2 }),
    });

    routeData.resolve(makeNavigationIdentity(TASK_TWO_ID));
    await waitFor(() => {
      expect(screen.getByTestId(KANBAN_TASK_SHELL_TEST_ID).getAttribute(TASK_DATA_ATTRIBUTE)).toBe(
        TASK_TWO_ID,
      );
    });
  });

  it("reaches the unavailable task shell when route data fails", async () => {
    mocks.readTaskNavigationIdentity.mockRejectedValueOnce(new Error("task not found"));

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
