import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import type { ReactElement } from "react";
import type { StoreApi } from "zustand";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import { ToastProvider } from "@/components/toast-provider";
import { ApiError } from "@/lib/api/client";
import type { TaskSessionHydrationEpoch } from "@/lib/state/slices/session/types";
import type { AppState } from "@/lib/state/store";
import type {
  TaskNavigationContext,
  TaskNavigationIdentity,
  TaskNavigationReadSnapshot,
} from "@/lib/state/task-navigation-reads";
import { createTaskNavigationReads } from "@/lib/state/task-navigation-reads";
import { sessionId, taskId, workspaceId, workflowId } from "@/lib/types/ids";
import type { Task, TaskSession } from "@/lib/types/http";
import type { FetchedSessionData } from "@/lib/ssr/session-page-state";
import { TaskDetailRoute } from "./task-detail-route";

const TASK_ID = taskId("task-recovery");
const PRIMARY_SESSION_ID = sessionId("session-primary");
const SECONDARY_SESSION_ID = sessionId("session-secondary");

const mocks = vi.hoisted(() => ({
  beginTaskNavigation: vi.fn(),
  isTaskNavigationCurrent: vi.fn(),
  readTaskNavigationIdentity: vi.fn(),
  retainTaskNavigationRead: vi.fn(() => () => {}),
  fetchTaskNavigationEnrichment: vi.fn(),
  navigationReadState: {
    phase: "idle" as TaskNavigationReadSnapshot["phase"],
    attempt: 0,
    temporary: false,
    revision: 0,
    cycle: 0,
    identity: undefined as TaskNavigationIdentity | undefined,
  } as TaskNavigationReadSnapshot,
}));

vi.mock("@/components/state-hydrator", async () => {
  const { useLayoutEffect, useRef } = await import("react");
  const { useAppStoreApi } = await import("@/components/state-provider");
  return {
    StateHydrator: ({
      initialState,
      sessionId: selectedSessionId,
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
            forceMergeSessionId: selectedSessionId,
            taskSessionHydrationEpochsAtRequestStart,
          });
        }
        onHydratedRef.current?.();
      }, [initialState, selectedSessionId, store, taskSessionHydrationEpochsAtRequestStart]);
      return null;
    },
  };
});

vi.mock("@/components/task/task-page-inner", () => ({
  TaskPageInner: () => <div data-testid="real-task-page-inner" />,
}));

vi.mock("@/app/tasks/[id]/kanban-task-shell", async () => {
  const { TaskPageContent } = await import("@/components/task/task-page-content");
  return {
    KanbanTaskShell: ({
      task,
      taskId: fallbackTaskId,
      sessionId: selectedSessionId,
    }: {
      task: Task | null;
      taskId: string;
      sessionId: string | null;
    }) => <TaskPageContent task={task} taskId={fallbackTaskId} sessionId={selectedSessionId} />,
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
    retainTaskNavigationRead: mocks.retainTaskNavigationRead,
    useTaskNavigationReadState: vi.fn(() => mocks.navigationReadState),
  };
});

function makeTask(): Task {
  return {
    id: TASK_ID,
    title: "Task navigation recovery",
    description: "",
    workspace_id: workspaceId("workspace-1"),
    workflow_id: workflowId("workflow-1"),
    workflow_step_id: "step-1",
    primary_session_id: PRIMARY_SESSION_ID,
    state: "COMPLETED",
    archived_at: "2026-10-06T00:00:00Z",
    priority: "medium",
    position: 0,
    repositories: [],
    created_at: "2026-10-06T00:00:00Z",
    updated_at: "2026-10-06T00:00:00Z",
  } as Task;
}

function makeSession(id: string): TaskSession {
  return {
    id: sessionId(id),
    task_id: TASK_ID,
    last_read_message_id: `read-${id}`,
  } as TaskSession;
}

function makeIdentity(): TaskNavigationIdentity {
  return {
    task: makeTask(),
    allSessionsResponse: {
      sessions: [makeSession(PRIMARY_SESSION_ID), makeSession(SECONDARY_SESSION_ID)],
      total: 2,
    },
  };
}

function makeEnrichedData(identity: TaskNavigationIdentity): FetchedSessionData {
  return {
    task: identity.task,
    sessionId: SECONDARY_SESSION_ID,
    initialState: {},
    initialTerminals: [],
  } as FetchedSessionData;
}

function createNavigationHarness(loader: (taskId: string) => Promise<TaskNavigationIdentity>) {
  const reads = createTaskNavigationReads((id) => loader(id));
  let currentContext: TaskNavigationContext | undefined;
  let currentOwner: object | undefined;
  let currentRouteKey: string | undefined;
  mocks.beginTaskNavigation.mockImplementation((_store, owner: object, routeKey: string) => {
    const generation = reads.beginNavigation(owner, routeKey);
    if (
      currentContext &&
      currentOwner === owner &&
      currentRouteKey === routeKey &&
      currentContext.generation === generation
    ) {
      return currentContext;
    }
    currentOwner = owner;
    currentRouteKey = routeKey;
    currentContext = { ownerToken: {}, identity: "test-scope", generation };
    return currentContext;
  });
  mocks.readTaskNavigationIdentity.mockImplementation(
    (_store, requestedTaskId: string, options: Record<string, unknown> = {}) => {
      const context = options.context as TaskNavigationContext | undefined;
      const generation = context?.generation ?? reads.currentGeneration();
      const pending = reads.read(requestedTaskId, generation, {
        refresh: options.refresh as boolean | undefined,
        manualRetry: options.manualRetry as boolean | undefined,
        supersede: options.supersede as boolean | undefined,
        foregroundRecoveryEpisode: options.foregroundRecoveryEpisode as object | undefined,
      });
      return pending.then(
        (identity) => {
          mocks.navigationReadState = reads.getSnapshot(requestedTaskId, generation);
          return identity;
        },
        (error) => {
          mocks.navigationReadState = reads.getSnapshot(requestedTaskId, generation);
          throw error;
        },
      );
    },
  );
  return reads;
}

function TaskRouteStoreCapture({ storeRef }: { storeRef: { current: StoreApi<AppState> | null } }) {
  storeRef.current = useAppStoreApi();
  return null;
}

function renderTaskRoute(
  element: ReactElement,
  initialState: Partial<AppState>,
  storeRef: { current: StoreApi<AppState> | null },
) {
  return render(element, {
    wrapper: ({ children }) => (
      <StateProvider initialState={initialState}>
        <ToastProvider>
          <TaskRouteStoreCapture storeRef={storeRef} />
          {children}
        </ToastProvider>
      </StateProvider>
    ),
  });
}

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
  vi.useRealTimers();
});

beforeEach(() => {
  mocks.isTaskNavigationCurrent.mockReturnValue(true);
  mocks.fetchTaskNavigationEnrichment.mockImplementation((identity: TaskNavigationIdentity) =>
    Promise.resolve(makeEnrichedData(identity)),
  );
  mocks.navigationReadState = {
    phase: "idle",
    attempt: 0,
    temporary: false,
    revision: 0,
    cycle: 0,
    identity: undefined,
  };
});

describe("task-detail route recovery", () => {
  it("keeps the selected owned session through real task-page sync and manual recovery", async () => {
    vi.useFakeTimers();
    let attempts = 0;
    const identity = makeIdentity();
    const reads = createNavigationHarness(async () => {
      attempts++;
      if (attempts <= 3) {
        throw new ApiError("busy", 503, { code: "persistence_unavailable" });
      }
      return identity;
    });
    const secondary = makeSession(SECONDARY_SESSION_ID);
    const initialState = {
      tasks: { activeTaskId: TASK_ID, activeSessionId: SECONDARY_SESSION_ID },
      taskSessions: { items: { [SECONDARY_SESSION_ID]: secondary } },
    } as unknown as Partial<AppState>;
    const storeRef: { current: StoreApi<AppState> | null } = { current: null };
    const { rerender } = renderTaskRoute(
      <TaskDetailRoute taskId={TASK_ID} />,
      initialState,
      storeRef,
    );

    await act(async () => {
      await vi.advanceTimersByTimeAsync(0);
      await vi.advanceTimersByTimeAsync(2_000);
      await vi.advanceTimersByTimeAsync(5_000);
    });
    expect(screen.getByTestId("task-load-error-state")).toBeTruthy();
    expect(attempts).toBe(3);
    expect(storeRef.current?.getState().tasks.activeSessionId).toBe(SECONDARY_SESSION_ID);

    const enriched = makeEnrichedData(identity);
    mocks.fetchTaskNavigationEnrichment.mockResolvedValueOnce(enriched);
    await act(async () => {
      fireEvent.click(screen.getByTestId("task-read-retry"));
      await vi.advanceTimersByTimeAsync(0);
    });
    expect(attempts).toBe(4);
    rerender(<TaskDetailRoute taskId={TASK_ID} />);

    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });
    expect(storeRef.current?.getState().tasks.activeSessionId).toBe(SECONDARY_SESSION_ID);
    expect(storeRef.current?.getState().tasks.activeTaskId).toBe(TASK_ID);
    reads.dispose();
  });

  it("keeps successful foreground refreshes from repeating route enrichment", async () => {
    vi.useRealTimers();
    let attempts = 0;
    const identity = makeIdentity();
    const reads = createNavigationHarness(async () => {
      attempts++;
      return identity;
    });
    const state = {
      tasks: { activeTaskId: TASK_ID, activeSessionId: SECONDARY_SESSION_ID },
      taskSessions: { items: { [SECONDARY_SESSION_ID]: makeSession(SECONDARY_SESSION_ID) } },
    } as unknown as Partial<AppState>;
    const storeRef: { current: StoreApi<AppState> | null } = { current: null };
    const { rerender } = renderTaskRoute(<TaskDetailRoute taskId={TASK_ID} />, state, storeRef);
    await waitFor(() => expect(mocks.fetchTaskNavigationEnrichment).toHaveBeenCalledOnce());

    for (const expectedReads of [2, 3]) {
      await act(async () => {
        window.dispatchEvent(new Event("online"));
        await Promise.resolve();
        await Promise.resolve();
      });
      await waitFor(() => expect(attempts).toBe(expectedReads));
      rerender(<TaskDetailRoute taskId={TASK_ID} />);
      await act(async () => Promise.resolve());
      expect(mocks.fetchTaskNavigationEnrichment).toHaveBeenCalledOnce();
      await new Promise((resolve) => setTimeout(resolve, 251));
    }
    reads.dispose();
  });
});
