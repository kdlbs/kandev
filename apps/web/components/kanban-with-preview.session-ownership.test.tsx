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

const BETA_PRIMARY = "beta-primary";
const ALPHA_SESSION = "alpha-session";
const SESSION_LIST_ACTION = "task.session.list";
const SESSION_CHAT_TEST_ID = "session-chat";
const BETA_SESSION = "beta-session";

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
      steps: [{ id: "step", title: "Ready", color: "blue", position: 0 }],
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
    const secondary = "alpha-secondary";
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
  const secondary = "alpha-secondary";
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
    expect(screen.getByTestId(SESSION_CHAT_TEST_ID).getAttribute("data-session-id")).toBe(
      ALPHA_SESSION,
    ),
  );
  const secondaryTab = screen.getByTestId(`preview-session-tab-${secondary}`);
  fireEvent.mouseDown(secondaryTab, { button: 0, ctrlKey: false });
  fireEvent.mouseUp(secondaryTab, { button: 0 });
  fireEvent.click(secondaryTab);
  await waitFor(() =>
    expect(screen.getByTestId(SESSION_CHAT_TEST_ID).getAttribute("data-session-id")).toBe(
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
    expect(screen.getByTestId(SESSION_CHAT_TEST_ID).getAttribute("data-session-id")).toBe(
      secondary,
    ),
  );
  expect(getStore().getState().tasks.activeSessionId).toBe(secondary);
  expect(new URL(window.location.href).searchParams.get("sessionId")).toBe(secondary);
}, 15_000);
