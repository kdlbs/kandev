import { useLayoutEffect } from "react";
import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { ToastProvider } from "@/components/toast-provider";
import { CommandRegistryProvider } from "@/lib/commands/command-registry";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import type { AppState, HydrationState } from "@/lib/state/store";
import type { SessionId, TaskId, TaskSessionsResponse } from "@/lib/types/http";
import { listTaskSessions } from "@/lib/api/domains/session-api";
import type * as SessionApi from "@/lib/api/domains/session-api";
import { WebSocketClient } from "@/lib/ws/client";
import { setWebSocketClient } from "@/lib/ws/connection";
import { defaultSettingsState, defaultKanbanState } from "@/lib/state/slices";
import { KanbanWithPreview } from "./kanban-with-preview";
import { registerTaskSessionHandlers } from "@/lib/ws/handlers/agent-session";
import { registerTasksHandlers } from "@/lib/ws/handlers/tasks";
import { SESSION_STATE_CHANGED } from "@/lib/ws/handlers/task-primary-focus.test-helpers";
import { makeMessage, makeTask } from "@/lib/ws/handlers/tasks.test-helpers";

const BETA_PRIMARY = "beta-primary";
const ALPHA_SESSION = "alpha-session";
const SESSION_LIST_ACTION = "task.session.list";
const SESSION_CHAT_TEST_ID = "session-chat";
const BETA_SESSION = "beta-session";
const ALPHA_SECONDARY = "alpha-secondary";
const SESSION_ID_ATTRIBUTE = "data-session-id";

const transport = vi.hoisted(() => ({ request: vi.fn() }));
let client: WebSocketClient;
vi.mock("@/lib/api/domains/session-api", async (original) => ({
  ...(await original<typeof SessionApi>()),
  listTaskSessions: vi.fn(),
}));

function previewMembership(taskId: string, ids: string[]): TaskSessionsResponse {
  return {
    sessions: ids.map((id, index) => ({
      id: id as SessionId,
      task_id: taskId as TaskId,
      state: "WAITING_FOR_INPUT",
      started_at: "2026-11-20T00:00:00Z",
      updated_at: "2026-11-20T00:00:00Z",
      is_primary: index === 0,
    })),
    total: ids.length,
  };
}

function deferred() {
  let resolve!: (value: { sessions: Array<{ id: string }> }) => void;
  const promise = new Promise<{ sessions: Array<{ id: string }> }>((yes) => {
    resolve = yes;
  });
  return { promise, resolve };
}

function mountPreview(
  primarySessionId?: string,
  initialSessionId?: string,
  observations?: AppState["tasks"][],
) {
  if (primarySessionId) {
    const defaultRead = vi.mocked(listTaskSessions).getMockImplementation()!;
    vi.mocked(listTaskSessions).mockImplementation((taskId, options) =>
      taskId === "beta"
        ? Promise.resolve(previewMembership(taskId, [primarySessionId]))
        : defaultRead(taskId, options),
    );
  }
  let store!: ReturnType<typeof useAppStoreApi>;
  function Capture() {
    const appStore = useAppStoreApi();
    store = appStore;
    useLayoutEffect(() => {
      if (!observations) return;
      observations.push(appStore.getState().tasks);
      return appStore.subscribe((state, previous) => {
        if (state.tasks !== previous.tasks) observations.push(state.tasks);
      });
    }, [appStore, observations]);
    return null;
  }
  const initialState: HydrationState = {
    kanban: {
      workflowId: "workflow",
      steps: [
        { id: "step", title: "Ready", color: "blue", position: 0 },
        { id: "next-step", title: "Review", color: "green", position: 1 },
      ],
      tasks: ["alpha", "beta"].map((id) => ({
        id,
        title: id,
        description: "",
        state: "TODO",
        position: 0,
        workflowStepId: "step",
        workflowId: "workflow",
        ...(id === "alpha" ? { primarySessionId: ALPHA_SESSION } : {}),
        ...(id === "beta" && primarySessionId ? { primarySessionId } : {}),
      })),
    },
    userSettings: {
      ...defaultSettingsState.userSettings,
      enablePreviewOnClick: true,
      loaded: true,
    },
    workspaces: { items: [], activeId: "workspace", activeIdRevision: 0 },
    workflows: {
      items: [{ id: "workflow", name: "Workflow", workspaceId: "workspace" }],
      activeId: "workflow",
    },
  };
  initialState.kanbanMulti = {
    ...defaultKanbanState.kanbanMulti,
    snapshots: {
      workflow: { ...initialState.kanban!, workflowId: "workflow", workflowName: "Workflow" },
    },
  };
  render(
    <StateProvider initialState={initialState}>
      <TooltipProvider>
        <ToastProvider>
          <CommandRegistryProvider>
            <Capture />
            <KanbanWithPreview initialTaskId="alpha" initialSessionId={initialSessionId} />
          </CommandRegistryProvider>
        </ToastProvider>
      </TooltipProvider>
    </StateProvider>,
  );
  return () => store;
}

const viewportProperties = ["offsetHeight", "offsetWidth"] as const;
const originalViewportGetters = viewportProperties.map((property) =>
  Object.getOwnPropertyDescriptor(HTMLElement.prototype, property),
);

function installKanbanViewport() {
  viewportProperties.forEach((property, index) => {
    const original = originalViewportGetters[index];
    Object.defineProperty(HTMLElement.prototype, property, {
      configurable: true,
      get() {
        if (this.matches('[data-testid="kanban-column-scroll"]')) {
          return property === "offsetHeight" ? 600 : 320;
        }
        return original?.get?.call(this) ?? 0;
      },
    });
  });
}

function restoreViewportGetters() {
  viewportProperties.forEach((property, index) => {
    const original = originalViewportGetters[index];
    if (original) Object.defineProperty(HTMLElement.prototype, property, original);
    else delete (HTMLElement.prototype as unknown as Record<string, unknown>)[property];
  });
}

beforeEach(() => {
  installKanbanViewport();
  transport.request.mockReset();
  client = new WebSocketClient("ws://preview-unit.invalid");
  vi.spyOn(client, "request").mockImplementation(transport.request);
  setWebSocketClient(client);
  vi.mocked(listTaskSessions).mockReset();
  vi.mocked(listTaskSessions).mockImplementation(async (taskId) => {
    const payload: { sessions?: Array<{ id: string }> } = await transport.request(
      SESSION_LIST_ACTION,
      { task_id: taskId },
    );
    return previewMembership(
      taskId,
      (payload.sessions ?? []).map((row) => row.id),
    );
  });
  localStorage.clear();
  window.history.replaceState({}, "", "/");
});
afterEach(() => {
  cleanup();
  client.disconnect();
  setWebSocketClient(null);
  restoreViewportGetters();
  localStorage.clear();
  window.history.replaceState({}, "", "/");
});

// @covers AC-UI-TASK-NAVIGATION-RESPONSIVENESS-001.8, AC-UI-TASK-NAVIGATION-RESPONSIVENESS-001.9
// eslint-disable-next-line max-lines-per-function -- real preview integration and metadata precedence share the transport fixture.
describe("Kanban preview task session fallback ownership", () => {
  it.each([false, true])(
    "does not project Alpha into Beta URL or active store (close first: %s)",
    async (closeFirst) => {
      const beta = deferred();
      transport.request.mockImplementation((method, args) => {
        if (method === SESSION_LIST_ACTION) {
          return args.task_id === "alpha"
            ? Promise.resolve({ sessions: [{ id: ALPHA_SESSION }] })
            : beta.promise;
        }
        return Promise.resolve({});
      });
      const getStore = mountPreview();
      await waitFor(() => expect(getStore().getState().tasks.activeSessionId).toBe(ALPHA_SESSION));
      expect(new URL(window.location.href).searchParams.get("sessionId")).toBe(ALPHA_SESSION);
      try {
        if (closeFirst) {
          fireEvent.keyDown(window, { key: "Escape" });
          await waitFor(() =>
            expect(new URL(window.location.href).searchParams.get("taskId")).toBeNull(),
          );
        }
        fireEvent.click(await screen.findByTestId("task-card-beta"));
        await waitFor(() =>
          expect(new URL(window.location.href).searchParams.get("taskId")).toBe("beta"),
        );
        expect(getStore().getState().tasks.activeTaskId).toBe("beta");
        expect(getStore().getState().tasks.activeSessionId).toBeNull();
        expect(new URL(window.location.href).searchParams.get("sessionId")).toBeNull();
        await act(async () => beta.resolve({ sessions: [{ id: BETA_SESSION }] }));
        await waitFor(() => expect(getStore().getState().tasks.activeSessionId).toBe(BETA_SESSION));
        expect(new URL(window.location.href).searchParams.get("sessionId")).toBe(BETA_SESSION);
      } finally {
        await act(async () => beta.resolve({ sessions: [] }));
      }
    },
    15_000,
  );

  it("preserves current task primary metadata over the fetched fallback", async () => {
    const beta = deferred();
    transport.request.mockImplementation((method, args) => {
      if (method === SESSION_LIST_ACTION) {
        return args.task_id === "alpha"
          ? Promise.resolve({ sessions: [{ id: ALPHA_SESSION }] })
          : beta.promise;
      }
      return Promise.resolve({});
    });
    const getStore = mountPreview(BETA_PRIMARY);
    await waitFor(() => expect(getStore().getState().tasks.activeSessionId).toBe(ALPHA_SESSION));
    try {
      fireEvent.click(await screen.findByTestId("task-card-beta"));
      await waitFor(() => expect(getStore().getState().tasks.activeSessionId).toBe(BETA_PRIMARY));
      expect(new URL(window.location.href).searchParams.get("sessionId")).toBe(BETA_PRIMARY);
      await act(async () => beta.resolve({ sessions: [{ id: "beta-fallback" }] }));
      expect(getStore().getState().tasks.activeSessionId).toBe(BETA_PRIMARY);
      expect(new URL(window.location.href).searchParams.get("sessionId")).toBe(BETA_PRIMARY);
    } finally {
      await act(async () => beta.resolve({ sessions: [] }));
    }
  }, 15_000);
});

// @covers AC-TASKS-TASK-UNREAD-DOT-001.6
it.each([false, true])(
  "validates initial Preview membership (valid link: %s)",
  async (valid) => {
    const secondary = ALPHA_SECONDARY;
    const missing = "deleted-alpha";
    const membership = Promise.withResolvers<TaskSessionsResponse>();
    vi.mocked(listTaskSessions).mockReturnValue(membership.promise);
    transport.request.mockImplementation((method) =>
      Promise.resolve(method === SESSION_LIST_ACTION ? { sessions: [{ id: ALPHA_SESSION }] } : {}),
    );
    const observations: AppState["tasks"][] = [];
    const getStore = mountPreview(undefined, valid ? secondary : missing, observations);
    try {
      await act(async () =>
        membership.resolve(previewMembership("alpha", [ALPHA_SESSION, secondary])),
      );
      const expected = valid ? secondary : ALPHA_SESSION;
      await waitFor(() => expect(getStore().getState().tasks.activeSessionId).toBe(expected));
      expect(new URL(window.location.href).searchParams.get("sessionId")).toBe(expected);
      expect(observations.map((state) => state.pinnedSessionId)).not.toContain(missing);
    } finally {
      membership.resolve(previewMembership("alpha", []));
    }
  },
  15_000,
);

// @covers AC-UI-TASK-NAVIGATION-RESPONSIVENESS-001.8
it("restores the actual non-primary Preview transcript after task removal fails", async () => {
  const secondary = ALPHA_SECONDARY;
  vi.mocked(listTaskSessions).mockImplementation((taskId) =>
    Promise.resolve(
      previewMembership(taskId, taskId === "alpha" ? [ALPHA_SESSION, secondary] : []),
    ),
  );
  transport.request.mockImplementation((method) =>
    Promise.resolve(method === SESSION_LIST_ACTION ? { sessions: [{ id: ALPHA_SESSION }] } : {}),
  );
  const getStore = mountPreview();
  await waitFor(() =>
    expect(screen.getByTestId(SESSION_CHAT_TEST_ID).getAttribute(SESSION_ID_ATTRIBUTE)).toBe(
      ALPHA_SESSION,
    ),
  );
  const secondaryTab = screen.getByTestId(`preview-session-tab-${secondary}`);
  fireEvent.mouseDown(secondaryTab, { button: 0, ctrlKey: false });
  fireEvent.mouseUp(secondaryTab, { button: 0 });
  fireEvent.click(secondaryTab);
  await waitFor(() =>
    expect(screen.getByTestId(SESSION_CHAT_TEST_ID).getAttribute(SESSION_ID_ATTRIBUTE)).toBe(
      secondary,
    ),
  );
  let token: string | null = null;
  act(() => {
    token = getStore()
      .getState()
      .beginTaskRemoval({
        action: "archive",
        workspaceId: "workspace",
        taskIds: ["alpha"],
        requestIds: ["alpha"],
        departure: null,
      });
  });
  await waitFor(() => expect(screen.queryByTestId(SESSION_CHAT_TEST_ID)).toBeNull());
  act(() => {
    getStore().getState().releaseTaskRemoval(token!);
  });
  await waitFor(() =>
    expect(screen.getByTestId(SESSION_CHAT_TEST_ID).getAttribute(SESSION_ID_ATTRIBUTE)).toBe(
      secondary,
    ),
  );
  expect(getStore().getState().tasks.activeSessionId).toBe(secondary);
  expect(new URL(window.location.href).searchParams.get("sessionId")).toBe(secondary);
}, 15_000);

function observePreviewSession(store: ReturnType<typeof useAppStoreApi>, sessionId: string) {
  registerTaskSessionHandlers(store)[SESSION_STATE_CHANGED]!({
    id: `observed-${sessionId}`,
    type: "notification",
    action: SESSION_STATE_CHANGED,
    payload: {
      task_id: "alpha",
      session_id: sessionId,
      old_state: "STARTING",
      new_state: "WAITING_FOR_INPUT",
      updated_at: "2026-11-20T00:00:01Z",
    },
  });
}

// @covers AC-TASKS-TASK-UNREAD-DOT-001.6
it.each(["valid", "stale", "newer-pin"] as const)(
  "validates initial URL ownership despite passive session adoption (%s)",
  async (choice) => {
    const secondary = ALPHA_SECONDARY;
    const membership = Promise.withResolvers<TaskSessionsResponse>();
    const requested = Promise.withResolvers<void>();
    vi.mocked(listTaskSessions).mockImplementation(() => {
      requested.resolve();
      return membership.promise;
    });
    transport.request.mockResolvedValue({ sessions: [{ id: ALPHA_SESSION }] });
    const getStore = mountPreview(undefined, choice === "stale" ? "deleted-alpha" : secondary);
    try {
      await act(async () => {
        await requested.promise;
      });
      act(() => {
        observePreviewSession(getStore(), ALPHA_SESSION);
      });
      await waitFor(() => expect(getStore().getState().tasks.activeSessionId).toBe(ALPHA_SESSION));
      if (choice === "newer-pin")
        act(() => {
          getStore().getState().setActiveSession("alpha", ALPHA_SESSION);
        });
      await act(async () =>
        membership.resolve(previewMembership("alpha", [ALPHA_SESSION, secondary])),
      );
      const expected = choice === "valid" ? secondary : ALPHA_SESSION;
      await waitFor(() =>
        expect(screen.getByTestId(SESSION_CHAT_TEST_ID).getAttribute(SESSION_ID_ATTRIBUTE)).toBe(
          expected,
        ),
      );
      expect(getStore().getState().tasks.activeSessionId).toBe(expected);
      expect(new URL(window.location.href).searchParams.get("sessionId")).toBe(expected);
    } finally {
      membership.resolve(previewMembership("alpha", []));
    }
  },
  15_000,
);

// @covers AC-TASKS-TASK-UNREAD-DOT-001.6
it.each([false, true])(
  "retains committed workflow focus before Preview membership (initial URL: %s)",
  async (initialUrl) => {
    const destination = "alpha-workflow";
    const membership = Promise.withResolvers<TaskSessionsResponse>();
    const requested = Promise.withResolvers<void>();
    vi.mocked(listTaskSessions).mockImplementation(() => {
      requested.resolve();
      return membership.promise;
    });
    transport.request.mockResolvedValue({ sessions: [{ id: ALPHA_SESSION }] });
    const observations: AppState["tasks"][] = [];
    const getStore = mountPreview(
      undefined,
      initialUrl ? ALPHA_SECONDARY : undefined,
      observations,
    );
    try {
      await act(async () => {
        await requested.promise;
      });
      const navigation = getStore().getState().taskRemoval.navigationRevision;
      act(() => {
        const state = getStore().getState();
        const requestId = state.beginWorkflowSessionFocus({
          taskId: "alpha",
          workflowId: "workflow",
          destinationStepId: "next-step",
          presentationToken: 77,
          navigationRevision: navigation,
        });
        expect(requestId).not.toBeNull();
        state.bindWorkflowSessionFocus({
          requestId: requestId!,
          presentationToken: 77,
          entryIdentity: "entry:r12",
        });
        observePreviewSession(getStore(), destination);
        registerTasksHandlers(getStore())["task.updated"]!(
          makeMessage({
            ...makeTask("alpha", destination, "workflow"),
            workflow_step_id: "next-step",
            updated_at: "2026-11-20T00:00:02Z",
            metadata: {
              workflow_session_route: {
                operation_id: "move-r12",
                destination_step_id: "next-step",
                entry_identity: "entry:r12",
                destination_session_id: destination,
                phase: "committed",
              },
            },
          }),
        );
        expect(getStore().getState().workflowSessionFocus.request?.sessionId).toBe(destination);
      });
      expect(getStore().getState().tasks.activeSessionId).toBe(destination);
      expect(getStore().getState().taskRemoval.navigationRevision).toBe(navigation + 1);
      await act(async () =>
        membership.resolve(
          previewMembership("alpha", [ALPHA_SESSION, destination, ALPHA_SECONDARY]),
        ),
      );
      await waitFor(() =>
        expect(screen.getByTestId(SESSION_CHAT_TEST_ID).getAttribute(SESSION_ID_ATTRIBUTE)).toBe(
          destination,
        ),
      );
      const committed = observations.findIndex((state) => state.activeSessionId === destination);
      expect(committed).toBeGreaterThanOrEqual(0);
      expect(observations.slice(committed).map((state) => state.activeSessionId)).not.toContain(
        null,
      );
      expect(new URL(window.location.href).searchParams.get("sessionId")).toBe(destination);
    } finally {
      membership.resolve(previewMembership("alpha", []));
    }
  },
  15_000,
);
