import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
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
  navigationReadState: {
    phase: "idle",
    attempt: 0,
    temporary: false,
    revision: 0,
    cycle: 0,
  },
}));
const KANBAN_TASK_SHELL_TEST_ID = "kanban-task-shell";
const STATE_HYDRATOR_TEST_ID = "state-hydrator";
const TASK_DATA_ATTRIBUTE = "data-task-id";
const SESSION_DATA_ATTRIBUTE = "data-session-id";
const READ_CURSOR_ATTRIBUTE = "data-read-cursor";
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

vi.mock("@/lib/state/task-navigation-reads", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/state/task-navigation-reads")>();
  return {
    ...actual,
    beginTaskNavigation: mocks.beginTaskNavigation,
    isTaskNavigationCurrent: mocks.isTaskNavigationCurrent,
    readTaskNavigationIdentity: mocks.readTaskNavigationIdentity,
    retainTaskNavigationRead: vi.fn(() => () => {}),
    useTaskNavigationReadState: vi.fn(() => mocks.navigationReadState),
  };
});

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
  mocks.readTaskNavigationIdentity.mockReset();
  mocks.navigationReadState = {
    phase: "idle",
    attempt: 0,
    temporary: false,
    revision: 0,
    cycle: 0,
  };
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

describe("TaskDetailRoute session selection", () => {
  // @covers AC-AGENTS-MCP-MANAGEMENT-001.2
  it("never opens a fallback conversation for an unavailable explicit session", async () => {
    mocks.readTaskNavigationIdentity.mockResolvedValueOnce(makeNavigationIdentity());
    renderTaskRoute(<TaskDetailRoute taskId={TASK_ONE_ID} sessionId="missing-session" />);
    await screen.findByTestId("task-session-unavailable");
    expect(screen.queryByTestId(KANBAN_TASK_SHELL_TEST_ID)).toBeNull();
    expect(mocks.fetchTaskNavigationEnrichment).not.toHaveBeenCalled();
  });

  it("retries an unavailable session list without opening another conversation", async () => {
    const identity = makeNavigationIdentity();
    mocks.readTaskNavigationIdentity
      .mockResolvedValueOnce({
        ...identity,
        allSessionsResponse: { sessions: [], total: 0 },
        sessionListUnavailable: true,
      })
      .mockResolvedValueOnce(identity);
    renderTaskRoute(<TaskDetailRoute taskId={TASK_ONE_ID} sessionId="session-1" />);
    fireEvent.click(await screen.findByRole("button", { name: "Retry" }));
    await waitFor(() =>
      expect(
        screen.getByTestId(KANBAN_TASK_SHELL_TEST_ID).getAttribute(SESSION_DATA_ATTRIBUTE),
      ).toBe("session-1"),
    );
    expect(mocks.readTaskNavigationIdentity).toHaveBeenLastCalledWith(
      expect.anything(),
      TASK_ONE_ID,
      expect.objectContaining({ refresh: true }),
    );
  });

  it("preserves the existing recovery shell for ordinary explicit-route errors", async () => {
    mocks.readTaskNavigationIdentity.mockRejectedValueOnce(new Error("temporary network error"));
    renderTaskRoute(<TaskDetailRoute taskId={TASK_ONE_ID} sessionId="session-1" />);
    await screen.findByTestId(KANBAN_TASK_SHELL_TEST_ID);
    expect(screen.queryByTestId("task-session-unavailable")).toBeNull();
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
