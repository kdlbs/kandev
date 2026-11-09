import { act, cleanup, render, screen, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import { ToastProvider } from "@/components/toast-provider";
import { TaskPageContent } from "@/components/task/task-page-content";
import type { AppState } from "@/lib/state/store";
import type { Task, TaskSession } from "@/lib/types/http";
import {
  sessionId as toSessionId,
  taskId as toTaskId,
  workflowId,
  workspaceId,
} from "@/lib/types/ids";

const PARENT_TASK_ID = "parent-task";
const PARENT_SESSION_ID = "parent-session";
const CHILD_TASK_ID = "child-task";
const CHILD_SESSION_ID = "child-session";
const OLDER_CHILD_SESSION_ID = "older-child-session";
const ACTIVE_SESSION_ID = "active-session";
const FOREIGN_SESSION_ID = "foreign-session";
const PARENT_TITLE = "Parent task";
const CHILD_TITLE = "Child task";

type SessionStatusRequest = { taskId: string; sessionId: string };

const mocks = vi.hoisted(() => {
  const requests: SessionStatusRequest[] = [];
  const listTaskSessions = vi.fn();
  const socket = {
    request: vi.fn(async (action: string, payload: Record<string, unknown>) => {
      if (action === "task.session.status") {
        requests.push({
          taskId: String(payload.task_id ?? ""),
          sessionId: String(payload.session_id ?? ""),
        });
        return { is_agent_running: true, state: "RUNNING" };
      }
      return {};
    }),
    subscribeSession: vi.fn(() => () => undefined),
    focusSession: vi.fn(() => () => undefined),
  };
  return { requests, listTaskSessions, socket };
});

vi.mock("@/lib/api", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/api")>();
  return { ...actual, listTaskSessions: mocks.listTaskSessions };
});

vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: () => mocks.socket,
  setWebSocketClient: vi.fn(),
  subscribeWebSocketClient: () => () => undefined,
  useWebSocketClient: () => mocks.socket,
}));

vi.mock("@/components/task/task-page-inner", () => ({
  TaskPageInner: ({
    task,
    effectiveSessionId,
  }: {
    task: Task | null;
    effectiveSessionId: string | null;
  }) => (
    <div
      data-testid="task-page-inner"
      data-task-id={task?.id ?? ""}
      data-session-id={effectiveSessionId ?? ""}
    />
  ),
}));

vi.mock("@/hooks/domains/workspace/use-repositories", () => ({
  useRepositories: () => ({ repositories: [], isLoading: false, isLoaded: true }),
}));

vi.mock("@/hooks/domains/workspace/use-external-vcs-file-link", () => ({
  useExternalVcsFileLinkHydration: () => undefined,
}));

vi.mock("@/hooks/domains/kanban/use-workflow-steps-by-id", () => ({
  useWorkflowStepsByID: () => [],
  useWorkflowStepsById: () => [],
}));

vi.mock("@/hooks/domains/task/use-task-canvases", () => ({
  useTaskCanvasesStateForTask: () => ({ key: null, canvases: [], status: "idle" }),
}));

vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => ({
    isMobile: false,
    isTablet: false,
    isCompactDesktop: false,
    isFullDesktop: true,
    isCoarsePointer: false,
    breakpoint: "desktop",
  }),
}));

function makeTask(id: string, title: string): Task {
  return {
    id: toTaskId(id),
    title,
    description: "",
    state: "IN_PROGRESS",
    priority: "medium",
    workspace_id: workspaceId("workspace-1"),
    workflow_id: workflowId("workflow-1"),
    workflow_step_id: "working",
    position: 0,
    repositories: [],
    created_at: "2026-11-03T00:00:00Z",
    updated_at: "2026-11-03T00:00:00Z",
  } as unknown as Task;
}

function makeSession(id: string, taskId: string): TaskSession {
  return {
    id: toSessionId(id),
    task_id: toTaskId(taskId),
    state: "WAITING_FOR_INPUT",
  } as unknown as TaskSession;
}

function makeState({
  activeTaskId,
  activeSessionId,
  sessions,
  sessionsByTask,
}: {
  activeTaskId: string | null;
  activeSessionId: string | null;
  sessions: TaskSession[];
  sessionsByTask?: Record<string, TaskSession[]>;
}): Partial<AppState> {
  const items = Object.fromEntries(sessions.map((session) => [session.id, session]));
  const itemsByTaskId: Record<string, TaskSession[]> = {};
  for (const session of sessions) {
    (itemsByTaskId[session.task_id] ??= []).push(session);
  }
  Object.assign(itemsByTaskId, sessionsByTask ?? {});
  return {
    workspaces: { activeId: "workspace-1", items: [] },
    connection: { status: "connected" },
    tasks: { activeTaskId, activeSessionId },
    taskSessions: { items },
    taskSessionsByTask: { itemsByTaskId },
    kanban: { workflowId: "workflow-1", tasks: [], steps: [] },
  } as unknown as Partial<AppState>;
}

function renderPage(
  state: Partial<AppState>,
  props: { task: Task; sessionId: string | null },
  onStore?: (store: ReturnType<typeof useAppStoreApi>) => void,
) {
  const rows = Object.values(
    (state.taskSessions as { items?: Record<string, TaskSession> } | undefined)?.items ?? {},
  );
  mocks.listTaskSessions.mockImplementation(async (taskId: string) => {
    const owned = rows.filter((row) => row.task_id === taskId);
    return { sessions: owned, total: owned.length };
  });
  function StoreCapture({ children }: { children: ReactNode }) {
    onStore?.(useAppStoreApi());
    return <>{children}</>;
  }
  return (
    <StateProvider initialState={state}>
      <ToastProvider>
        <StoreCapture>
          <TaskPageContent
            task={props.task}
            taskId={props.task.id}
            sessionId={props.sessionId}
            initialRepositories={[]}
            initialScripts={[]}
            defaultLayouts={{}}
          />
        </StoreCapture>
      </ToastProvider>
    </StateProvider>
  );
}

function renderedPair() {
  const panel = screen.getByTestId("task-page-inner");
  return {
    taskId: panel.getAttribute("data-task-id"),
    sessionId: panel.getAttribute("data-session-id"),
  };
}

function statusPairs(): SessionStatusRequest[] {
  return mocks.requests;
}

function expectOnlyOwnedStatusPairs(sessionsById: Record<string, TaskSession>) {
  for (const pair of statusPairs()) {
    expect(
      sessionsById[pair.sessionId]?.task_id,
      `status request used a foreign pair: ${pair.taskId}/${pair.sessionId}`,
    ).toBe(pair.taskId);
  }
}

beforeEach(() => {
  mocks.requests.length = 0;
  vi.clearAllMocks();
});

afterEach(() => {
  cleanup();
});

// eslint-disable-next-line max-lines-per-function -- the ownership cases share one page harness.
describe("mounted task page session ownership", () => {
  it("keeps the rendered parent's own session when the global selection belongs to the child", async () => {
    const parentSession = makeSession(PARENT_SESSION_ID, PARENT_TASK_ID);
    const childSession = makeSession(CHILD_SESSION_ID, CHILD_TASK_ID);
    const state = makeState({
      activeTaskId: CHILD_TASK_ID,
      activeSessionId: CHILD_SESSION_ID,
      sessions: [parentSession, childSession],
      sessionsByTask: {
        [PARENT_TASK_ID]: [parentSession],
        [CHILD_TASK_ID]: [childSession],
      },
    });
    const parentTask = makeTask(PARENT_TASK_ID, PARENT_TITLE);

    render(renderPage(state, { task: parentTask, sessionId: PARENT_SESSION_ID }));

    await waitFor(() =>
      expect(renderedPair()).toEqual({
        taskId: PARENT_TASK_ID,
        sessionId: PARENT_SESSION_ID,
      }),
    );
    await waitFor(() => expect(statusPairs().length).toBeGreaterThan(0));
    expect(statusPairs()).toContainEqual({
      taskId: PARENT_TASK_ID,
      sessionId: PARENT_SESSION_ID,
    });
    expectOnlyOwnedStatusPairs({
      [PARENT_SESSION_ID]: parentSession,
      [CHILD_SESSION_ID]: childSession,
    });
    expect(statusPairs()).not.toContainEqual({
      taskId: PARENT_TASK_ID,
      sessionId: CHILD_SESSION_ID,
    });
  });

  it("opens the child conversation after the retained parent hands over", async () => {
    const parentSession = makeSession(PARENT_SESSION_ID, PARENT_TASK_ID);
    const childSession = makeSession(CHILD_SESSION_ID, CHILD_TASK_ID);
    const state = makeState({
      activeTaskId: PARENT_TASK_ID,
      activeSessionId: PARENT_SESSION_ID,
      sessions: [parentSession, childSession],
      sessionsByTask: {
        [PARENT_TASK_ID]: [parentSession],
        [CHILD_TASK_ID]: [childSession],
      },
    });
    const parentTask = makeTask(PARENT_TASK_ID, PARENT_TITLE);
    const childTask = makeTask(CHILD_TASK_ID, CHILD_TITLE);
    let store: ReturnType<typeof useAppStoreApi> | null = null;
    const page = (props: { task: Task; sessionId: string | null }) =>
      renderPage(state, props, (captured) => {
        store = captured;
      });

    const { rerender } = render(page({ task: parentTask, sessionId: PARENT_SESSION_ID }));
    await waitFor(() =>
      expect(renderedPair()).toEqual({
        taskId: PARENT_TASK_ID,
        sessionId: PARENT_SESSION_ID,
      }),
    );

    mocks.requests.length = 0;
    // The creation handoff publishes the child selection before the route
    // commits, while the previous route is still rendered.
    act(() => store?.getState().setActiveSession(CHILD_TASK_ID, CHILD_SESSION_ID));
    await waitFor(() =>
      expect(renderedPair()).toEqual({
        taskId: PARENT_TASK_ID,
        sessionId: PARENT_SESSION_ID,
      }),
    );
    expect(statusPairs()).not.toContainEqual({
      taskId: PARENT_TASK_ID,
      sessionId: CHILD_SESSION_ID,
    });

    mocks.requests.length = 0;
    rerender(page({ task: childTask, sessionId: CHILD_SESSION_ID }));

    await waitFor(() =>
      expect(renderedPair()).toEqual({
        taskId: CHILD_TASK_ID,
        sessionId: CHILD_SESSION_ID,
      }),
    );
    await waitFor(() =>
      expect(statusPairs()).toContainEqual({
        taskId: CHILD_TASK_ID,
        sessionId: CHILD_SESSION_ID,
      }),
    );
    expectOnlyOwnedStatusPairs({
      [PARENT_SESSION_ID]: parentSession,
      [CHILD_SESSION_ID]: childSession,
    });
  });

  it("waits for delayed child ownership instead of guessing or launching another session", async () => {
    const parentSession = makeSession(PARENT_SESSION_ID, PARENT_TASK_ID);
    const state = makeState({
      activeTaskId: CHILD_TASK_ID,
      activeSessionId: CHILD_SESSION_ID,
      sessions: [parentSession],
      sessionsByTask: { [PARENT_TASK_ID]: [parentSession] },
    });
    const childTask = makeTask(CHILD_TASK_ID, CHILD_TITLE);
    let store: ReturnType<typeof useAppStoreApi> | null = null;

    render(
      renderPage(state, { task: childTask, sessionId: CHILD_SESSION_ID }, (captured) => {
        store = captured;
      }),
    );

    await waitFor(() => expect(renderedPair()).toEqual({ taskId: CHILD_TASK_ID, sessionId: "" }));
    await waitFor(() =>
      expect(store?.getState().taskSessionsByTask.loadedByTaskId[CHILD_TASK_ID]).toBe(true),
    );
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 100));
    });
    expect(statusPairs()).toHaveLength(0);

    const childSession = makeSession(CHILD_SESSION_ID, CHILD_TASK_ID);
    act(() => store?.getState().upsertTaskSessionFromEvent(CHILD_TASK_ID, childSession));

    await waitFor(() =>
      expect(renderedPair()).toEqual({
        taskId: CHILD_TASK_ID,
        sessionId: CHILD_SESSION_ID,
      }),
    );
    await waitFor(() =>
      expect(statusPairs()).toContainEqual({
        taskId: CHILD_TASK_ID,
        sessionId: CHILD_SESSION_ID,
      }),
    );
    expectOnlyOwnedStatusPairs({ [CHILD_SESSION_ID]: childSession });
    const unexpectedActions = mocks.socket.request.mock.calls
      .map(([action]) => action)
      .filter((action) => action !== "task.session.status");
    expect(unexpectedActions).toEqual([]);
  });

  it("keeps an unhydrated route session pending instead of showing an older task session", async () => {
    const olderSession = makeSession(OLDER_CHILD_SESSION_ID, CHILD_TASK_ID);
    const state = makeState({
      activeTaskId: CHILD_TASK_ID,
      activeSessionId: null,
      sessions: [olderSession],
      sessionsByTask: { [CHILD_TASK_ID]: [olderSession] },
    });
    const childTask = makeTask(CHILD_TASK_ID, CHILD_TITLE);
    let store: ReturnType<typeof useAppStoreApi> | null = null;

    render(
      renderPage(state, { task: childTask, sessionId: CHILD_SESSION_ID }, (captured) => {
        store = captured;
      }),
    );

    await waitFor(() => expect(renderedPair()).toEqual({ taskId: CHILD_TASK_ID, sessionId: "" }));
    expect(statusPairs()).toHaveLength(0);
    expect(statusPairs()).not.toContainEqual({
      taskId: CHILD_TASK_ID,
      sessionId: OLDER_CHILD_SESSION_ID,
    });

    const childSession = makeSession(CHILD_SESSION_ID, CHILD_TASK_ID);
    act(() => store?.getState().upsertTaskSessionFromEvent(CHILD_TASK_ID, childSession));

    await waitFor(() =>
      expect(renderedPair()).toEqual({
        taskId: CHILD_TASK_ID,
        sessionId: CHILD_SESSION_ID,
      }),
    );
    await waitFor(() =>
      expect(statusPairs()).toEqual([{ taskId: CHILD_TASK_ID, sessionId: CHILD_SESSION_ID }]),
    );
  });

  it("prefers an owned active session over a different owned route session", async () => {
    const routeSession = makeSession(PARENT_SESSION_ID, PARENT_TASK_ID);
    const activeSession = makeSession(ACTIVE_SESSION_ID, PARENT_TASK_ID);
    const state = makeState({
      activeTaskId: PARENT_TASK_ID,
      activeSessionId: PARENT_SESSION_ID,
      sessions: [routeSession, activeSession],
      sessionsByTask: { [PARENT_TASK_ID]: [routeSession, activeSession] },
    });
    const parentTask = makeTask(PARENT_TASK_ID, PARENT_TITLE);
    let store: ReturnType<typeof useAppStoreApi> | null = null;

    render(
      renderPage(state, { task: parentTask, sessionId: PARENT_SESSION_ID }, (captured) => {
        store = captured;
      }),
    );
    await waitFor(() =>
      expect(renderedPair()).toEqual({
        taskId: PARENT_TASK_ID,
        sessionId: PARENT_SESSION_ID,
      }),
    );

    // A session-tab change updates the active selection without changing the
    // route; the owned active session must win over the route session.
    act(() => store?.getState().setActiveSession(PARENT_TASK_ID, ACTIVE_SESSION_ID));

    await waitFor(() =>
      expect(renderedPair()).toEqual({
        taskId: PARENT_TASK_ID,
        sessionId: ACTIVE_SESSION_ID,
      }),
    );
    await waitFor(() =>
      expect(statusPairs()).toContainEqual({
        taskId: PARENT_TASK_ID,
        sessionId: ACTIVE_SESSION_ID,
      }),
    );
  });

  it("rejects foreign active and route candidates and admits the owned fallback", async () => {
    const parentSession = makeSession(PARENT_SESSION_ID, PARENT_TASK_ID);
    const foreignSession = makeSession(FOREIGN_SESSION_ID, CHILD_TASK_ID);
    const state = makeState({
      activeTaskId: PARENT_TASK_ID,
      activeSessionId: FOREIGN_SESSION_ID,
      sessions: [parentSession, foreignSession],
      sessionsByTask: { [PARENT_TASK_ID]: [parentSession, foreignSession] },
    });
    const parentTask = makeTask(PARENT_TASK_ID, PARENT_TITLE);

    render(renderPage(state, { task: parentTask, sessionId: FOREIGN_SESSION_ID }));

    await waitFor(() =>
      expect(renderedPair()).toEqual({
        taskId: PARENT_TASK_ID,
        sessionId: PARENT_SESSION_ID,
      }),
    );
    await waitFor(() =>
      expect(statusPairs()).toEqual([{ taskId: PARENT_TASK_ID, sessionId: PARENT_SESSION_ID }]),
    );
    expect(statusPairs()).not.toContainEqual({
      taskId: PARENT_TASK_ID,
      sessionId: FOREIGN_SESSION_ID,
    });
  });

  it("stays sessionless when no candidate belongs to the rendered task", async () => {
    const childSession = makeSession(CHILD_SESSION_ID, CHILD_TASK_ID);
    const state = makeState({
      activeTaskId: CHILD_TASK_ID,
      activeSessionId: CHILD_SESSION_ID,
      sessions: [childSession],
      sessionsByTask: { [CHILD_TASK_ID]: [childSession] },
    });
    const parentTask = makeTask(PARENT_TASK_ID, PARENT_TITLE);

    render(renderPage(state, { task: parentTask, sessionId: CHILD_SESSION_ID }));

    await waitFor(() => expect(renderedPair()).toEqual({ taskId: PARENT_TASK_ID, sessionId: "" }));
    expect(statusPairs()).toHaveLength(0);
  });
});
