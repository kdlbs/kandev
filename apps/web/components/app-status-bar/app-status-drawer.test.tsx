import { cleanup, render, screen } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { afterEach, describe, expect, it, vi } from "vitest";
import { StateProvider } from "@/components/state-provider";
import { defaultSettingsState } from "@/lib/state/slices/settings/settings-slice";
import { pluginRegistry } from "@/lib/plugins/registry";
import type { AppStatusBarSlotProps } from "@/lib/plugins/types";
import type { TaskLspLanguageSnapshot } from "@/lib/types/http-lsp";
import { AppStatusDrawer } from "./app-status-drawer";
import {
  APP_STATUS_CONNECTION_ID,
  APP_STATUS_LSP_ID,
  APP_STATUS_METRICS_ID,
} from "./app-status-bar-order";

const lspController = vi.hoisted(() => ({
  start: vi.fn(),
  stop: vi.fn(),
  restart: vi.fn(),
  setPolicy: vi.fn(),
}));

const kotlin: TaskLspLanguageSnapshot = {
  task_id: "task-1",
  language: "kotlin",
  policy: "inherit",
  detected: true,
  detection_state: "complete",
  detection_truncated: false,
  phase: "off",
  generation: 0,
  revision: 1,
  last_transition_at: "2026-08-05T12:00:00Z",
  last_action: "",
  last_initiator: "automatic",
  restart_required: false,
  created_at: "2026-08-05T12:00:00Z",
  updated_at: "2026-08-05T12:00:00Z",
  effective_policy: "inherit",
  activity: "idle",
  progress: [],
};

vi.mock("@kandev/ui/drawer", () => ({
  Drawer: ({ children }: { children: React.ReactNode }) => <>{children}</>,
  DrawerContent: ({ children, ...props }: React.HTMLAttributes<HTMLDivElement>) => (
    <div {...props}>{children}</div>
  ),
  DrawerHeader: ({ children, ...props }: React.HTMLAttributes<HTMLDivElement>) => (
    <div {...props}>{children}</div>
  ),
  DrawerTitle: ({ children }: { children: React.ReactNode }) => <h2>{children}</h2>,
}));

vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => ({ isFinePointer: false, isMobile: true }),
}));

vi.mock("@/hooks/domains/lsp/use-task-lsp", () => ({
  useTaskLsp: () => ({
    languages: [kotlin],
    pending: {},
    loaded: true,
    loading: false,
    error: null,
    capacity: { active: 0, queued: 0, limit: 4 },
    ...lspController,
  }),
}));

const LEFT_PLUGIN_ID = "drawer-left";
const RIGHT_PLUGIN_ID = "drawer-right";

function renderPhoneLspDrawer() {
  render(
    <StateProvider
      initialState={{
        userSettings: {
          ...defaultSettingsState.userSettings,
          appStatusBarEnabled: true,
          lspStatusLocation: "status_bar",
        },
      }}
    >
      <TooltipProvider>
        <AppStatusDrawer
          pathname="/tasks/task-1"
          activeWorkspaceId="workspace-1"
          activeTaskId="task-1"
          activeSessionId="session-1"
          open
          onOpenChange={() => {}}
          focusLspLanguage="kotlin"
        />
      </TooltipProvider>
    </StateProvider>,
  );
}

function renderConnectionOnlyDrawer() {
  pluginRegistry
    .forPlugin(LEFT_PLUGIN_ID)
    .registerComponent("app-status-bar-left", () => <span>Left plugin</span>);
  pluginRegistry
    .forPlugin(RIGHT_PLUGIN_ID)
    .registerComponent("app-status-bar-right", () => <span>Right plugin</span>);

  render(
    <StateProvider
      initialState={{
        userSettings: {
          ...defaultSettingsState.userSettings,
          systemMetricsDisplay: { showInTopbar: true, simplified: false },
        },
        connection: { status: "reconnecting", error: null, issueSeverity: "unstable" },
      }}
    >
      <TooltipProvider>
        <AppStatusDrawer
          pathname="/"
          activeWorkspaceId={null}
          activeTaskId={null}
          activeSessionId={null}
          open
          onOpenChange={() => {}}
          connectionOnly
        />
      </TooltipProvider>
    </StateProvider>,
  );
}

function resetDrawerTest() {
  cleanup();
  pluginRegistry.unregisterPlugin(LEFT_PLUGIN_ID);
  pluginRegistry.unregisterPlugin(RIGHT_PLUGIN_ID);
  lspController.start.mockReset();
  lspController.stop.mockReset();
  lspController.restart.mockReset();
  lspController.setPolicy.mockReset();
}

describe("AppStatusDrawer", () => {
  afterEach(resetDrawerTest);

  it("mirrors saved left then right order as non-draggable 44px rows", () => {
    pluginRegistry
      .forPlugin(LEFT_PLUGIN_ID)
      .registerComponent("app-status-bar-left", ({ slotProps }) => {
        const props = slotProps as AppStatusBarSlotProps;
        return <span data-testid={LEFT_PLUGIN_ID}>{props.presentation}</span>;
      });
    pluginRegistry
      .forPlugin(RIGHT_PLUGIN_ID)
      .registerComponent("app-status-bar-right", ({ slotProps }) => {
        const props = slotProps as AppStatusBarSlotProps;
        return <span data-testid={RIGHT_PLUGIN_ID}>{props.presentation}</span>;
      });
    const leftId = `plugin:${LEFT_PLUGIN_ID}:app-status-bar-left:0`;
    const rightId = `plugin:${RIGHT_PLUGIN_ID}:app-status-bar-right:0`;

    render(
      <StateProvider
        initialState={{
          userSettings: {
            ...defaultSettingsState.userSettings,
            systemMetricsDisplay: { showInTopbar: true, simplified: false },
            appStatusBarOrder: {
              leftItemIds: [rightId, APP_STATUS_METRICS_ID],
              rightItemIds: [APP_STATUS_CONNECTION_ID, leftId],
            },
          },
        }}
      >
        <TooltipProvider>
          <AppStatusDrawer
            pathname="/tasks/task-1"
            activeWorkspaceId="workspace-1"
            activeTaskId="task-1"
            activeSessionId="session-1"
            open
            onOpenChange={() => {}}
          />
        </TooltipProvider>
      </StateProvider>,
    );

    const rows = Array.from(
      screen
        .getByTestId("app-status-drawer")
        .querySelectorAll<HTMLElement>("[data-status-item-id]"),
    );
    expect(rows.map((row) => row.dataset.statusItemId)).toEqual([
      rightId,
      APP_STATUS_METRICS_ID,
      APP_STATUS_CONNECTION_ID,
      leftId,
      APP_STATUS_LSP_ID,
    ]);
    expect(rows.every((row) => row.className.includes("min-h-11"))).toBe(true);
    expect(screen.getByTestId(LEFT_PLUGIN_ID).textContent).toBe("mobile-drawer");
    expect(screen.getByTestId(RIGHT_PLUGIN_ID).textContent).toBe("mobile-drawer");
  });

  it("collapses a plugin row when its contribution renders nothing", () => {
    pluginRegistry.forPlugin(RIGHT_PLUGIN_ID).registerComponent("app-status-bar-right", () => null);

    render(
      <StateProvider>
        <TooltipProvider>
          <AppStatusDrawer
            pathname="/"
            activeWorkspaceId={null}
            activeTaskId={null}
            activeSessionId={null}
            open
            onOpenChange={() => {}}
          />
        </TooltipProvider>
      </StateProvider>,
    );

    const row = document.querySelector<HTMLElement>(
      `[data-status-item-id="plugin:${RIGHT_PLUGIN_ID}:app-status-bar-right:0"]`,
    );
    expect(row?.className).toContain("empty:hidden");
  });

  it("renders only connection state in connection-only mode", () => {
    renderConnectionOnlyDrawer();

    const rows = screen
      .getByTestId("app-status-drawer")
      .querySelectorAll<HTMLElement>("[data-status-item-id]");
    expect(Array.from(rows, (row) => row.dataset.statusItemId)).toEqual([APP_STATUS_CONNECTION_ID]);
  });

  it("shows task controls without an open file and does not start a server", () => {
    renderPhoneLspDrawer();

    expect(document.querySelector(`[data-status-item-id="${APP_STATUS_LSP_ID}"]`)).toBeTruthy();
    expect(screen.getByTestId("task-lsp-language-kotlin")).toBeTruthy();
    expect(lspController.start).not.toHaveBeenCalled();
    expect(screen.getByTestId("app-status-drawer-scroll-region").className).toContain(
      "overflow-y-auto",
    );
  });
});
