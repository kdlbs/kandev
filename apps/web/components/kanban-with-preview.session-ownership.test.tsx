import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { ToastProvider } from "@/components/toast-provider";
import { CommandRegistryProvider } from "@/lib/commands/command-registry";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import type { HydrationState } from "@/lib/state/store";
import { defaultSettingsState, defaultKanbanState } from "@/lib/state/slices";
import { KanbanWithPreview } from "./kanban-with-preview";

const BETA_PRIMARY = "beta-primary";
const ALPHA_SESSION = "alpha-session";
const BETA_SESSION = "beta-session";

const transport = vi.hoisted(() => ({ request: vi.fn() }));
vi.mock("@/lib/ws/connection", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/lib/ws/connection")>()),
  getWebSocketClient: () => ({ request: transport.request }),
}));

function deferred() {
  let resolve!: (value: { sessions: Array<{ id: string }> }) => void;
  const promise = new Promise<{ sessions: Array<{ id: string }> }>((yes) => {
    resolve = yes;
  });
  return { promise, resolve };
}

function mountPreview(primarySessionId?: string) {
  let store!: ReturnType<typeof useAppStoreApi>;
  function Capture() {
    store = useAppStoreApi();
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
            <KanbanWithPreview initialTaskId="alpha" />
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
  localStorage.clear();
  window.history.replaceState({}, "", "/");
});
afterEach(() => {
  cleanup();
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
        if (method === "task.session.list") {
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
  );

  it("preserves current task primary metadata over the fetched fallback", async () => {
    const beta = deferred();
    transport.request.mockImplementation((method, args) => {
      if (method === "task.session.list") {
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
  });
});
