import { act, cleanup, render, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import type { StoreApi } from "zustand";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import { ToastProvider } from "@/components/toast-provider";
import type { AppState } from "@/lib/state/store";
import type { TaskNavigationIdentity } from "@/lib/state/task-navigation-reads";
import type { Task, TaskSession } from "@/lib/types/http";
import { sessionId, taskId, workflowId, workspaceId } from "@/lib/types/ids";
import { captureTaskSessionHydrationEpochs } from "@/lib/state/slices/session/hydration-epochs";
import { registerTaskSessionHandlers } from "@/lib/ws/handlers/agent-session";
import { listTaskSessions } from "@/lib/api/domains/session-api";
import type * as SessionApi from "@/lib/api/domains/session-api";
import { TaskDetailRoute } from "./task-detail-route";
import { commitWorkflowRecipient, taskRouteSelectionState } from "./task-detail-route.test-helpers";
import { buildTaskNavigationShellData } from "@/lib/ssr/session-page-state";

const mocks = vi.hoisted(() => ({ load: vi.fn() }));
vi.mock("@/lib/state/task-navigation-reads", async (original) => ({
  ...(await original<typeof import("@/lib/state/task-navigation-reads")>()),
  readTaskNavigationIdentity: mocks.load,
}));
vi.mock("@/lib/api/domains/session-api", async (original) => ({
  ...(await original<typeof SessionApi>()),
  listTaskSessions: vi.fn(),
}));
vi.mock("@/lib/ssr/session-page-state", async (original) => ({
  ...(await original<typeof import("@/lib/ssr/session-page-state")>()),
  fetchTaskNavigationEnrichment: vi.fn(
    async (identity: TaskNavigationIdentity, requested?: string) => ({
      task: identity.task,
      sessionId: requested ?? null,
      initialState: {},
      initialTerminals: [],
    }),
  ),
}));
vi.mock("@/app/tasks/[id]/kanban-task-shell", async () => {
  // The hoisted shell mock must load its real consumer after dependency mocks register.
  const { TaskPageContent } = await import("@/components/task/task-page-content");
  return {
    KanbanTaskShell: (props: { task: Task; taskId: string; sessionId: string | null }) => (
      <TaskPageContent {...props} />
    ),
  };
});
vi.mock("@/components/task/task-page-inner", () => ({ TaskPageInner: () => null }));

const TASK = "explicit-route-task";
const OLD = "removed";
const TARGET = "requested";
const SURVIVOR = "survivor";
const TIME = "2026-11-20T12:00:00Z";
function identity(empty: boolean): TaskNavigationIdentity {
  return {
    task: {
      id: taskId(TASK),
      title: "Explicit route admission",
      description: "",
      workspace_id: workspaceId("workspace-1"),
      workflow_id: workflowId("workflow-1"),
      workflow_step_id: "step-1",
      primary_session_id: sessionId(OLD),
      state: "COMPLETED",
      archived_at: TIME,
      priority: "medium",
      position: 0,
      repositories: [],
      created_at: TIME,
      updated_at: TIME,
    } satisfies Task,
    allSessionsResponse: {
      sessions: [
        {
          id: TARGET,
          task_id: TASK,
          state: "COMPLETED",
          started_at: TIME,
          updated_at: TIME,
          is_primary: false,
        },
        {
          id: SURVIVOR,
          task_id: TASK,
          state: "WAITING_FOR_INPUT",
          started_at: TIME,
          updated_at: TIME,
          is_primary: !empty,
        },
      ] as TaskSession[],
      total: 2,
    },
  };
}
let store: StoreApi<AppState>;
function CaptureStore() {
  store = useAppStoreApi();
  return null;
}
afterEach(() => {
  cleanup();
  vi.resetAllMocks();
});

const LATER_REMOVAL = "later-removal";
const CANCELLED_WORKFLOW = "cancelled-workflow";

async function openRequestedRoute(
  data: TaskNavigationIdentity,
  removed: TaskSession,
  empty: boolean,
) {
  const initial = Promise.withResolvers<TaskNavigationIdentity>();
  const requested = Promise.withResolvers<TaskNavigationIdentity>();
  let response = initial.promise;
  mocks.load.mockImplementation(() => response);
  vi.mocked(listTaskSessions).mockResolvedValue(data.allSessionsResponse);
  const page = render(
    <StateProvider
      initialState={taskRouteSelectionState(TASK, OLD, [
        removed,
        ...data.allSessionsResponse.sessions,
      ])}
    >
      <ToastProvider>
        <CaptureStore />
        <TaskDetailRoute taskId={TASK} />
      </ToastProvider>
    </StateProvider>,
  );
  await waitFor(() => expect(mocks.load).toHaveBeenCalled());
  await act(async () => {
    registerTaskSessionHandlers(store)["session.removed"]!({
      id: "remove-before-explicit-route",
      type: "notification",
      action: "session.removed",
      payload: { task_id: TASK, session_id: OLD },
    });
  });
  const retained = empty ? null : SURVIVOR;
  await waitFor(() => expect(store.getState().tasks.activeSessionId).toBe(retained));
  data.task.primary_session_id = empty ? undefined : data.allSessionsResponse.sessions[1].id;
  await act(async () => initial.resolve(data));
  response = requested.promise;
  const count = mocks.load.mock.calls.length;
  page.rerender(
    <StateProvider initialState={{}}>
      <ToastProvider>
        <CaptureStore />
        <TaskDetailRoute taskId={TASK} sessionId={TARGET} />
      </ToastProvider>
    </StateProvider>,
  );
  await waitFor(() => expect(mocks.load.mock.calls.length).toBeGreaterThan(count));

  return { requested, retained };
}

function applyEntryOwner(owner: string, pendingMove: boolean) {
  if (owner === "pin") act(() => store.getState().setActiveSession(TASK, SURVIVOR));
  if (owner === "workflow") act(() => commitWorkflowRecipient(store, TASK, SURVIVOR));
  if (pendingMove)
    act(() =>
      store.getState().beginWorkflowSessionFocus({
        taskId: TASK,
        workflowId: "workflow-1",
        destinationStepId: "review",
        presentationToken: 1,
        navigationRevision: store.getState().taskRemoval.navigationRevision,
      }),
    );
  if (owner === "background-workflow")
    act(() =>
      store.getState().beginWorkflowSessionFocus({
        taskId: "unrelated",
        workflowId: "workflow-1",
        destinationStepId: "review",
        presentationToken: 1,
        navigationRevision: store.getState().taskRemoval.navigationRevision,
      }),
    );
}

// @covers AC-TASKS-TASK-UNREAD-DOT-001.6, AC-TASKS-TASK-UNREAD-DOT-001.17
it.each([
  [false, "route"],
  [true, "route"],
  [false, "pin"],
  [false, "workflow"],
  [false, LATER_REMOVAL],
  [false, "unavailable"],
  [false, "background-workflow"],
  [false, CANCELLED_WORKFLOW],
  [true, CANCELLED_WORKFLOW],
  [false, "pending-pin"],
  [false, "pending-commit"],
  [false, "pending-membership"],
] as const)(
  "admits a newer explicit route after removal (empty=%s, owner=%s)",
  async (empty, owner) => {
    const pendingMove = owner === CANCELLED_WORKFLOW || owner.startsWith("pending-");
    const data = identity(empty);
    const later = {
      ...data.allSessionsResponse.sessions[1],
      id: "later",
      is_primary: false,
    } as TaskSession;
    if (owner === LATER_REMOVAL) {
      data.allSessionsResponse.sessions.push(later);
      data.allSessionsResponse.total++;
    }
    const removed = {
      ...data.allSessionsResponse.sessions[0],
      id: OLD,
      state: "FAILED",
    } as TaskSession;
    const { requested, retained } = await openRequestedRoute(data, removed, empty);
    applyEntryOwner(owner, pendingMove);
    if (owner === LATER_REMOVAL) {
      vi.mocked(listTaskSessions).mockResolvedValue({
        sessions: [data.allSessionsResponse.sessions[0], { ...later, is_primary: true }],
        total: 2,
      });
      await act(async () =>
        registerTaskSessionHandlers(store)["session.removed"]!({
          id: "remove-after-explicit-entry",
          type: "notification",
          action: "session.removed",
          payload: { task_id: TASK, session_id: SURVIVOR },
        }),
      );
      await waitFor(() => expect(store.getState().tasks.activeSessionId).toBe(later.id));
    }
    if (owner === "unavailable")
      data.allSessionsResponse.sessions = data.allSessionsResponse.sessions.filter(
        (row) => row.id !== TARGET,
      );
    await act(async () => requested.resolve(data));
    if (pendingMove) {
      expect(store.getState().tasks.activeSessionId).toBe(retained);
      if (owner === "pending-pin") act(() => store.getState().setActiveSession(TASK, SURVIVOR));
      if (owner === "pending-commit") act(() => commitWorkflowRecipient(store, TASK, SURVIVOR));
      if (owner === "pending-membership")
        act(() =>
          store.getState().setTaskSessionsForTask(
            TASK,
            store
              .getState()
              .taskSessionsByTask.itemsByTaskId[TASK].filter((row) => row.id !== TARGET),
            captureTaskSessionHydrationEpochs(store.getState(), TASK),
          ),
        );
      act(() => store.getState().cancelWorkflowSessionFocus({ taskId: TASK }));
      if (owner === CANCELLED_WORKFLOW)
        await waitFor(() => expect(store.getState().tasks.activeSessionId).toBe(TARGET));
    }
    if (owner === "unavailable") await screen.findByTestId("task-session-unavailable");
    let expected = SURVIVOR;
    if (owner === "route" || owner === "background-workflow" || owner === CANCELLED_WORKFLOW)
      expected = TARGET;
    if (owner === LATER_REMOVAL) expected = later.id;
    expect(store.getState().tasks.activeSessionId).toBe(expected);
    expect(store.getState().taskSessions.items[OLD]).toBeUndefined();
  },
);

// @covers AC-TASKS-TASK-UNREAD-DOT-001.6, AC-TASKS-TASK-UNREAD-DOT-001.12
it.each([true, false])("admits validated Go boot data with explicit route=%s", async (explicit) => {
  const data = identity(false);
  data.task.primary_session_id = sessionId(SURVIVOR);
  const boot = buildTaskNavigationShellData(data, explicit ? TARGET : undefined);
  vi.mocked(listTaskSessions).mockResolvedValue(data.allSessionsResponse);
  render(
    <StateProvider
      initialState={taskRouteSelectionState(TASK, SURVIVOR, data.allSessionsResponse.sessions)}
    >
      <ToastProvider>
        <CaptureStore />
        <TaskDetailRoute
          taskId={TASK}
          sessionId={explicit ? TARGET : undefined}
          initialData={boot}
        />
      </ToastProvider>
    </StateProvider>,
  );
  await waitFor(() =>
    expect(store.getState().tasks.activeSessionId).toBe(explicit ? TARGET : SURVIVOR),
  );
  expect(store.getState().tasks.pinnedSessionId).toBe(explicit ? TARGET : null);
});

it("does not replay a deferred route after its committed removal observation", async () => {
  const data = identity(false);
  const removed = {
    ...data.allSessionsResponse.sessions[0],
    id: OLD,
    state: "FAILED",
  } as TaskSession;
  const { requested, retained } = await openRequestedRoute(data, removed, false);
  applyEntryOwner("pending", true);
  await act(async () => requested.resolve(data));
  expect(store.getState().tasks.activeSessionId).toBe(retained);
  const selections: Array<string | null> = [];
  const stop = store.subscribe((state) => selections.push(state.tasks.activeSessionId));
  const proof = Promise.withResolvers<TaskNavigationIdentity["allSessionsResponse"]>();
  vi.mocked(listTaskSessions).mockReturnValue(proof.promise);
  await act(async () =>
    registerTaskSessionHandlers(store)["session.removed"]!({
      id: "remove-deferred-target",
      type: "notification",
      action: "session.removed",
      payload: { task_id: TASK, session_id: TARGET },
    }),
  );
  const intent = store.getState().workflowSessionFocus.intent!;
  act(() => {
    store.getState().bindWorkflowSessionFocus({
      requestId: intent.requestId,
      presentationToken: intent.presentationToken,
      entryIdentity: "entry:no-recipient",
    });
    store.getState().reconcileWorkflowSessionFocus(TASK, {
      metadata: data.task.metadata,
      entryIdentity: "entry:no-recipient",
      focusDisposition: "none",
    });
  });
  expect(store.getState().tasks.activeSessionId).toBe(retained);
  await act(async () =>
    proof.resolve({
      sessions: data.allSessionsResponse.sessions.filter((row) => row.id !== TARGET),
      total: 1,
    }),
  );
  await waitFor(() => expect(store.getState().taskSessions.items[TARGET]).toBeUndefined());
  expect(store.getState().tasks.activeSessionId).toBe(retained);
  expect(selections).not.toContain(TARGET);
  stop();
});
